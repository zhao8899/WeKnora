package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	filesvc "github.com/Tencent/WeKnora/internal/application/service/file"
	"github.com/Tencent/WeKnora/internal/application/service/retriever"
	"github.com/Tencent/WeKnora/internal/infrastructure/chunker"
	"github.com/Tencent/WeKnora/internal/infrastructure/docparser"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/tracing"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel/attribute"
)

// processDocumentFromPassage handles asynchronous processing of text passages
func (s *knowledgeService) processDocumentFromPassage(ctx context.Context,
	kb *types.KnowledgeBase, knowledge *types.Knowledge, passage []string,
) {
	// Update status to processing
	knowledge.ParseStatus = "processing"
	knowledge.UpdatedAt = time.Now()
	if err := s.repo.UpdateKnowledge(ctx, knowledge); err != nil {
		return
	}

	// Convert passages to chunks
	chunks := make([]types.ParsedChunk, 0, len(passage))
	start, end := 0, 0
	for i, p := range passage {
		if p == "" {
			continue
		}
		end += len([]rune(p))
		chunks = append(chunks, types.ParsedChunk{
			Content: p,
			Seq:     i,
			Start:   start,
			End:     end,
		})
		start = end
	}
	// Process and store chunks
	var opts ProcessChunksOptions
	if kb.QuestionGenerationConfig != nil && kb.QuestionGenerationConfig.Enabled {
		opts.EnableQuestionGeneration = true
		opts.QuestionCount = kb.QuestionGenerationConfig.QuestionCount
		if opts.QuestionCount <= 0 {
			opts.QuestionCount = 3
		}
	}
	s.processChunks(ctx, kb, knowledge, chunks, opts)
}

// ProcessChunksOptions contains options for processing chunks
type ProcessChunksOptions struct {
	EnableQuestionGeneration bool
	QuestionCount            int
	EnableMultimodel         bool
	StoredImages             []docparser.StoredImage
	// ParentChunks holds parent chunk data when parent-child chunking is enabled.
	// When set, the chunks passed to processChunks are child chunks, and each
	// child's ParentIndex references an entry in this slice.
	ParentChunks []types.ParsedParentChunk
	// SourceMarkdown is the full document text used to derive a per-chunk
	// heading breadcrumb for contextual retrieval. When empty, only the
	// document title is prepended (legacy behavior).
	SourceMarkdown string
}

// buildParentChildConfigs derives parent and child SplitterConfig from ChunkingConfig.
// The base config (already validated with defaults) is used for separators.
func buildParentChildConfigs(cc types.ChunkingConfig, base chunker.SplitterConfig) (parent, child chunker.SplitterConfig) {
	parentSize := cc.ParentChunkSize
	if parentSize <= 0 {
		parentSize = 4096
	}
	childSize := cc.ChildChunkSize
	if childSize <= 0 {
		childSize = 384
	}
	parent = chunker.SplitterConfig{
		ChunkSize:    parentSize,
		ChunkOverlap: base.ChunkOverlap, // reuse configured overlap for parents
		Separators:   base.Separators,
	}
	child = chunker.SplitterConfig{
		ChunkSize:    childSize,
		ChunkOverlap: childSize / 5, // ~20% overlap for child chunks
		Separators:   base.Separators,
	}
	return
}

// processChunks processes chunks and creates embeddings for knowledge content
func (s *knowledgeService) processChunks(ctx context.Context,
	kb *types.KnowledgeBase, knowledge *types.Knowledge, chunks []types.ParsedChunk,
	opts ...ProcessChunksOptions,
) {
	// Get options
	var options ProcessChunksOptions
	if len(opts) > 0 {
		options = opts[0]
	}

	ctx, span := tracing.ContextWithSpan(ctx, "knowledgeService.processChunks")
	defer span.End()
	span.SetAttributes(
		attribute.Int("tenant_id", int(knowledge.TenantID)),
		attribute.String("knowledge_base_id", knowledge.KnowledgeBaseID),
		attribute.String("knowledge_id", knowledge.ID),
		attribute.String("embedding_model_id", kb.EmbeddingModelID),
		attribute.Int("chunk_count", len(chunks)),
	)

	// Check if knowledge is being deleted before processing
	if s.isKnowledgeDeleting(ctx, knowledge.TenantID, knowledge.ID) {
		logger.Infof(ctx, "Knowledge is being deleted, aborting chunk processing: %s", knowledge.ID)
		span.AddEvent("aborted: knowledge is being deleted")
		return
	}

	// Get embedding model for vectorization
	embeddingModel, err := s.modelService.GetEmbeddingModel(ctx, kb.EmbeddingModelID)
	if err != nil {
		logger.GetLogger(ctx).WithField("error", err).Errorf("processChunks get embedding model failed")
		span.RecordError(err)
		return
	}

	// 幂等性处理：清理旧的chunks和索引数据，避免重复数据
	logger.Infof(ctx, "Cleaning up existing chunks and index data for knowledge: %s", knowledge.ID)

	// 删除旧的chunks
	if err := s.chunkService.DeleteChunksByKnowledgeID(ctx, knowledge.ID); err != nil {
		logger.Warnf(ctx, "Failed to delete existing chunks (may not exist): %v", err)
		// 不返回错误，继续处理（可能没有旧数据）
	}

	// 删除旧的索引数据
	tenantInfo := ctx.Value(types.TenantInfoContextKey).(*types.Tenant)
	retrieveEngine, err := retriever.NewCompositeRetrieveEngine(s.retrieveEngine, tenantInfo.GetEffectiveEngines())
	if err == nil {
		if err := retrieveEngine.DeleteByKnowledgeIDList(ctx, []string{knowledge.ID}, embeddingModel.GetDimensions(), knowledge.Type); err != nil {
			logger.Warnf(ctx, "Failed to delete existing index data (may not exist): %v", err)
			// 不返回错误，继续处理（可能没有旧数据）
		} else {
			logger.Infof(ctx, "Successfully deleted existing index data for knowledge: %s", knowledge.ID)
		}
	}

	// 删除知识图谱数据（如果存在）
	namespace := types.NameSpace{KnowledgeBase: knowledge.KnowledgeBaseID, Knowledge: knowledge.ID}
	if err := s.graphEngine.DelGraph(ctx, []types.NameSpace{namespace}); err != nil {
		logger.Warnf(ctx, "Failed to delete existing graph data (may not exist): %v", err)
		// 不返回错误，继续处理
	}

	logger.Infof(ctx, "Cleanup completed, starting to process new chunks")

	// ========== DocReader 解析结果日志 ==========
	logger.Infof(ctx, "[DocReader] ========== 解析结果概览 ==========")
	logger.Infof(ctx, "[DocReader] 知识ID: %s, 知识库ID: %s", knowledge.ID, knowledge.KnowledgeBaseID)
	logger.Infof(ctx, "[DocReader] 总Chunk数量: %d", len(chunks))

	// 统计图片信息
	totalImages := 0
	chunksWithImages := 0
	for _, chunkData := range chunks {
		if len(chunkData.Images) > 0 {
			chunksWithImages++
			totalImages += len(chunkData.Images)
		}
	}
	logger.Infof(ctx, "[DocReader] 包含图片的Chunk数: %d, 总图片数: %d", chunksWithImages, totalImages)

	// 打印每个Chunk的详细信息
	for idx, chunkData := range chunks {
		contentPreview := chunkData.Content
		if len(contentPreview) > 200 {
			contentPreview = contentPreview[:200] + "..."
		}
		logger.Infof(ctx, "[DocReader] Chunk #%d (seq=%d): 内容长度=%d, 图片数=%d, 范围=[%d-%d]",
			idx, chunkData.Seq, len(chunkData.Content), len(chunkData.Images), chunkData.Start, chunkData.End)
		logger.Debugf(ctx, "[DocReader] Chunk #%d 内容预览: %s", idx, contentPreview)

		// 打印图片详细信息
		for imgIdx, img := range chunkData.Images {
			logger.Infof(ctx, "[DocReader]   图片 #%d: URL=%s", imgIdx, img.URL)
			logger.Infof(ctx, "[DocReader]   图片 #%d: OriginalURL=%s", imgIdx, img.OriginalURL)
			if img.Caption != "" {
				captionPreview := img.Caption
				if len(captionPreview) > 100 {
					captionPreview = captionPreview[:100] + "..."
				}
				logger.Infof(ctx, "[DocReader]   图片 #%d: Caption=%s", imgIdx, captionPreview)
			}
			if img.OCRText != "" {
				ocrPreview := img.OCRText
				if len(ocrPreview) > 100 {
					ocrPreview = ocrPreview[:100] + "..."
				}
				logger.Infof(ctx, "[DocReader]   图片 #%d: OCRText=%s", imgIdx, ocrPreview)
			}
			logger.Infof(ctx, "[DocReader]   图片 #%d: 位置=[%d-%d]", imgIdx, img.Start, img.End)
		}
	}
	logger.Infof(ctx, "[DocReader] ========== 解析结果概览结束 ==========")

	// Create chunk objects from proto chunks
	maxSeq := 0

	// 统计图片相关的子Chunk数量，用于扩展insertChunks的容量
	imageChunkCount := 0
	for _, chunkData := range chunks {
		if len(chunkData.Images) > 0 {
			// 为每个图片的OCR和Caption分别创建一个Chunk
			imageChunkCount += len(chunkData.Images) * 2
		}
		if int(chunkData.Seq) > maxSeq {
			maxSeq = int(chunkData.Seq)
		}
	}

	// === Parent-Child Chunking: create parent chunks first ===
	hasParentChild := len(options.ParentChunks) > 0
	var parentDBChunks []*types.Chunk // indexed by ParsedParentChunk position
	if hasParentChild {
		parentDBChunks = make([]*types.Chunk, len(options.ParentChunks))
		for i, pc := range options.ParentChunks {
			parentDBChunks[i] = &types.Chunk{
				ID:              uuid.New().String(),
				TenantID:        knowledge.TenantID,
				KnowledgeID:     knowledge.ID,
				KnowledgeBaseID: knowledge.KnowledgeBaseID,
				Content:         pc.Content,
				ChunkIndex:      pc.Seq,
				IsEnabled:       true,
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
				StartAt:         pc.Start,
				EndAt:           pc.End,
				ChunkType:       types.ChunkTypeParentText,
			}
		}
		// Set prev/next links for parent chunks
		for i := range parentDBChunks {
			if i > 0 {
				parentDBChunks[i-1].NextChunkID = parentDBChunks[i].ID
				parentDBChunks[i].PreChunkID = parentDBChunks[i-1].ID
			}
		}
		logger.Infof(ctx, "Created %d parent chunks for parent-child strategy", len(parentDBChunks))
	}

	// 重新分配容量，考虑图片相关的Chunk + parent chunks
	parentCount := len(options.ParentChunks)
	insertChunks := make([]*types.Chunk, 0, len(chunks)+imageChunkCount+parentCount)
	// Add parent chunks first (they go into DB but NOT into the vector index)
	if hasParentChild {
		insertChunks = append(insertChunks, parentDBChunks...)
	}

	for idx, chunkData := range chunks {
		if strings.TrimSpace(chunkData.Content) == "" {
			continue
		}

		// 创建主文本Chunk
		textChunk := &types.Chunk{
			ID:              uuid.New().String(),
			TenantID:        knowledge.TenantID,
			KnowledgeID:     knowledge.ID,
			KnowledgeBaseID: knowledge.KnowledgeBaseID,
			Content:         chunkData.Content,
			ChunkIndex:      int(chunkData.Seq),
			IsEnabled:       true,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
			StartAt:         int(chunkData.Start),
			EndAt:           int(chunkData.End),
			ChunkType:       types.ChunkTypeText,
		}

		// Wire up ParentChunkID for child chunks
		if hasParentChild && chunkData.ParentIndex >= 0 && chunkData.ParentIndex < len(parentDBChunks) {
			textChunk.ParentChunkID = parentDBChunks[chunkData.ParentIndex].ID
		}

		chunks[idx].ChunkID = textChunk.ID
		insertChunks = append(insertChunks, textChunk)
	}

	// Sort chunks by index for proper ordering
	sort.Slice(insertChunks, func(i, j int) bool {
		return insertChunks[i].ChunkIndex < insertChunks[j].ChunkIndex
	})

	// 仅为文本类型的Chunk设置前后关系（child chunks only, parents already linked above）
	textChunks := make([]*types.Chunk, 0, len(chunks))
	for _, chunk := range insertChunks {
		if chunk.ChunkType == types.ChunkTypeText && chunk.ParentChunkID != "" {
			// This is a child chunk in parent-child mode
			textChunks = append(textChunks, chunk)
		} else if chunk.ChunkType == types.ChunkTypeText && !hasParentChild {
			// Normal flat chunk (no parent-child mode)
			textChunks = append(textChunks, chunk)
		}
	}

	// 设置文本Chunk之间的前后关系 (skip if parent-child, children don't need prev/next links)
	if !hasParentChild {
		for i, chunk := range textChunks {
			if i > 0 {
				textChunks[i-1].NextChunkID = chunk.ID
			}
			if i < len(textChunks)-1 {
				textChunks[i+1].PreChunkID = chunk.ID
			}
		}
	}

	// Create index information — only for child/flat chunks, NOT parent chunks.
	// Parent chunks are stored for context retrieval but do not need vector embeddings.
	//
	// Contextual Retrieval prefix: prepend both the document title and the
	// hierarchical heading breadcrumb (e.g. "# Q3 Report > ## Financials")
	// active at the chunk's byte offset. This disambiguates chunk content
	// for the embedding model at zero extra LLM cost.
	indexInfoList := make([]*types.IndexInfo, 0, len(textChunks))
	titlePrefix := ""
	if t := strings.TrimSpace(knowledge.Title); t != "" {
		titlePrefix = t + "\n"
	}
	var headingIndex *chunker.HeadingIndex
	if options.SourceMarkdown != "" {
		headingIndex = chunker.NewHeadingIndex(options.SourceMarkdown)
	}
	for _, chunk := range textChunks {
		prefix := titlePrefix
		if headingIndex != nil {
			if breadcrumb := headingIndex.PathAt(chunk.StartAt); breadcrumb != "" {
				prefix += breadcrumb + "\n"
			}
		}
		indexContent := prefix + chunk.Content
		indexInfoList = append(indexInfoList, &types.IndexInfo{
			Content:         indexContent,
			SourceID:        chunk.ID,
			SourceType:      types.ChunkSourceType,
			ChunkID:         chunk.ID,
			KnowledgeID:     knowledge.ID,
			KnowledgeBaseID: knowledge.KnowledgeBaseID,
			IsEnabled:       true,
		})
	}

	// Initialize retrieval engine

	// Calculate storage size required for embeddings
	span.AddEvent("estimate storage size")
	totalStorageSize := retrieveEngine.EstimateStorageSize(ctx, embeddingModel, indexInfoList)
	if tenantInfo.StorageQuota > 0 {
		// Re-fetch tenant storage information
		tenantInfo, err = s.tenantRepo.GetTenantByID(ctx, tenantInfo.ID)
		if err != nil {
			knowledge.ParseStatus = types.ParseStatusFailed
			knowledge.ErrorMessage = err.Error()
			knowledge.UpdatedAt = time.Now()
			s.repo.UpdateKnowledge(ctx, knowledge)
			span.RecordError(err)
			return
		}
		// Check if there's enough storage quota available
		if tenantInfo.StorageUsed+totalStorageSize > tenantInfo.StorageQuota {
			knowledge.ParseStatus = types.ParseStatusFailed
			knowledge.ErrorMessage = "存储空间不足"
			knowledge.UpdatedAt = time.Now()
			s.repo.UpdateKnowledge(ctx, knowledge)
			span.RecordError(errors.New("storage quota exceeded"))
			return
		}
	}

	// Check again if knowledge is being deleted before writing to database
	if s.isKnowledgeDeleting(ctx, knowledge.TenantID, knowledge.ID) {
		logger.Infof(ctx, "Knowledge is being deleted, aborting before saving chunks: %s", knowledge.ID)
		span.AddEvent("aborted: knowledge is being deleted before saving")
		return
	}

	// Save chunks to database
	span.AddEvent("create chunks")
	if err := s.chunkService.CreateChunks(ctx, insertChunks); err != nil {
		knowledge.ParseStatus = types.ParseStatusFailed
		knowledge.ErrorMessage = err.Error()
		knowledge.UpdatedAt = time.Now()
		s.repo.UpdateKnowledge(ctx, knowledge)
		span.RecordError(err)
		return
	}

	// Check again before batch indexing (this is a heavy operation)
	if s.isKnowledgeDeleting(ctx, knowledge.TenantID, knowledge.ID) {
		logger.Infof(ctx, "Knowledge is being deleted, cleaning up and aborting before indexing: %s", knowledge.ID)
		// Clean up the chunks we just created
		if err := s.chunkService.DeleteChunksByKnowledgeID(ctx, knowledge.ID); err != nil {
			logger.Warnf(ctx, "Failed to cleanup chunks after deletion detected: %v", err)
		}
		span.AddEvent("aborted: knowledge is being deleted before indexing")
		return
	}

	span.AddEvent("batch index")
	err = retrieveEngine.BatchIndex(ctx, embeddingModel, indexInfoList)
	if err != nil {
		knowledge.ParseStatus = types.ParseStatusFailed
		knowledge.ErrorMessage = err.Error()
		knowledge.UpdatedAt = time.Now()
		s.repo.UpdateKnowledge(ctx, knowledge)

		// delete failed chunks
		if err := s.chunkService.DeleteChunksByKnowledgeID(ctx, knowledge.ID); err != nil {
			logger.Errorf(ctx, "Delete chunks failed: %v", err)
		}

		// delete index
		if err := retrieveEngine.DeleteByKnowledgeIDList(
			ctx, []string{knowledge.ID}, embeddingModel.GetDimensions(), kb.Type,
		); err != nil {
			logger.Errorf(ctx, "Delete index failed: %v", err)
		}
		span.RecordError(err)
		return
	}
	logger.GetLogger(ctx).Infof("processChunks batch index successfully, with %d index", len(indexInfoList))

	logger.Infof(ctx, "processChunks create relationship rag task")
	if kb.ExtractConfig != nil && kb.ExtractConfig.Enabled {
		for _, chunk := range textChunks {
			err := NewChunkExtractTask(ctx, s.task, chunk.TenantID, chunk.ID, kb.SummaryModelID)
			if err != nil {
				logger.GetLogger(ctx).WithField("error", err).Errorf("processChunks create chunk extract task failed")
				span.RecordError(err)
			}
		}
	}

	// Final check before marking as completed - if deleted during processing, don't update status
	if s.isKnowledgeDeleting(ctx, knowledge.TenantID, knowledge.ID) {
		logger.Infof(ctx, "Knowledge was deleted during processing, skipping completion update: %s", knowledge.ID)
		// Clean up the data we just created since the knowledge is being deleted
		if err := s.chunkService.DeleteChunksByKnowledgeID(ctx, knowledge.ID); err != nil {
			logger.Warnf(ctx, "Failed to cleanup chunks after deletion detected: %v", err)
		}
		if err := retrieveEngine.DeleteByKnowledgeIDList(ctx, []string{knowledge.ID}, embeddingModel.GetDimensions(), kb.Type); err != nil {
			logger.Warnf(ctx, "Failed to cleanup index after deletion detected: %v", err)
		}
		span.AddEvent("aborted: knowledge was deleted during processing")
		return
	}

	// Skip summary/question generation for image-type knowledge — the text chunk
	// is just a markdown image reference, so LLM summary would be useless.
	// The multimodal task will provide a caption as the description instead.
	isImage := IsImageType(knowledge.FileType)
	pendingMultimodal := isImage && options.EnableMultimodel && len(options.StoredImages) > 0

	// For image files with pending multimodal processing, keep "processing" status
	// so the frontend waits until the description is ready before showing "completed".
	if pendingMultimodal {
		knowledge.ParseStatus = types.ParseStatusProcessing
	} else {
		knowledge.ParseStatus = types.ParseStatusCompleted
	}
	knowledge.EnableStatus = "enabled"
	knowledge.StorageSize = totalStorageSize
	now := time.Now()
	knowledge.ProcessedAt = &now
	knowledge.UpdatedAt = now

	// Set summary status based on whether summary generation will be triggered
	if len(textChunks) > 0 && !isImage {
		knowledge.SummaryStatus = types.SummaryStatusPending
	} else {
		knowledge.SummaryStatus = types.SummaryStatusNone
	}

	if err := s.repo.UpdateKnowledge(ctx, knowledge); err != nil {
		logger.GetLogger(ctx).WithField("error", err).Errorf("processChunks update knowledge failed")
	}

	// Enqueue question generation task if enabled (async, non-blocking)
	if options.EnableQuestionGeneration && len(textChunks) > 0 && !isImage {
		questionCount := options.QuestionCount
		if questionCount <= 0 {
			questionCount = 3
		}
		if questionCount > 10 {
			questionCount = 10
		}
		s.enqueueQuestionGenerationTask(ctx, knowledge.KnowledgeBaseID, knowledge.ID, questionCount)
	}

	// Enqueue summary generation task (async, non-blocking)
	if len(textChunks) > 0 && !isImage {
		s.enqueueSummaryGenerationTask(ctx, knowledge.KnowledgeBaseID, knowledge.ID)
	}

	// Enqueue multimodal tasks for images (async, non-blocking)
	if options.EnableMultimodel && len(options.StoredImages) > 0 {
		s.enqueueImageMultimodalTasks(ctx, knowledge, kb, options.StoredImages, chunks)
	}

	// Update tenant's storage usage
	tenantInfo.StorageUsed += totalStorageSize
	if err := s.tenantRepo.AdjustStorageUsed(ctx, tenantInfo.ID, totalStorageSize); err != nil {
		logger.GetLogger(ctx).WithField("error", err).Errorf("processChunks update tenant storage used failed")
	}
	logger.GetLogger(ctx).Infof("processChunks successfully")
}

func ensureManualFileName(title string) string {
	if title == "" {
		return fmt.Sprintf("manual-%s%s", time.Now().Format("20060102-150405"), manualFileExtension)
	}
	trimmed := strings.TrimSpace(title)
	if strings.HasSuffix(strings.ToLower(trimmed), manualFileExtension) {
		return trimmed
	}
	return trimmed + manualFileExtension
}

// sanitizeManualDownloadFilename converts a knowledge title into a safe .md
// download filename. Characters that are illegal or dangerous in HTTP header
// values and file-system paths are removed or replaced; a blank result falls
// back to "untitled".
func sanitizeManualDownloadFilename(title string) string {
	safeName := strings.NewReplacer(
		"\n", "", "\r", "", "\t", "", "/", "-", "\\", "-", "\"", "'",
	).Replace(title)
	if strings.TrimSpace(safeName) == "" {
		safeName = "untitled"
	}
	if !strings.HasSuffix(strings.ToLower(safeName), manualFileExtension) {
		safeName += manualFileExtension
	}
	return safeName
}

func (s *knowledgeService) triggerManualProcessing(ctx context.Context,
	kb *types.KnowledgeBase, knowledge *types.Knowledge, content string, doSync bool,
) {
	clean := strings.TrimSpace(content)
	if clean == "" {
		return
	}

	// Resolve embedded data:base64 images and remote http(s) images → storage, replace URLs.
	// Runs before chunking so chunks contain stable provider:// URLs.
	var resolvedImages []docparser.StoredImage
	if s.imageResolver != nil {
		fileSvc := s.resolveFileService(ctx, kb)
		afterDataURI, fromDataURI, _ := s.imageResolver.ResolveDataURIImages(ctx, clean, fileSvc, knowledge.TenantID)
		if len(fromDataURI) > 0 {
			logger.Infof(ctx, "Resolved %d data-URI images for manual knowledge %s", len(fromDataURI), knowledge.ID)
			clean = afterDataURI
			resolvedImages = append(resolvedImages, fromDataURI...)
		}
		updatedContent, storedImages, resolveErr := s.imageResolver.ResolveRemoteImages(ctx, clean, fileSvc, knowledge.TenantID)
		if resolveErr != nil {
			logger.Warnf(ctx, "Remote image resolution partially failed: %v", resolveErr)
		}
		if len(storedImages) > 0 {
			logger.Infof(ctx, "Resolved %d remote images for manual knowledge %s", len(storedImages), knowledge.ID)
			clean = updatedContent
			resolvedImages = append(resolvedImages, storedImages...)
		}
	}

	// Manual content is markdown - chunk directly with Go chunker
	chunkCfg := chunker.SplitterConfig{
		ChunkSize:    kb.ChunkingConfig.ChunkSize,
		ChunkOverlap: kb.ChunkingConfig.ChunkOverlap,
		Separators:   kb.ChunkingConfig.Separators,
	}
	if chunkCfg.ChunkSize <= 0 {
		chunkCfg.ChunkSize = 512
	}
	if chunkCfg.ChunkOverlap <= 0 {
		chunkCfg.ChunkOverlap = 50
	}
	if len(chunkCfg.Separators) == 0 {
		chunkCfg.Separators = []string{"\n\n", "\n", "。"}
	}

	var parsed []types.ParsedChunk
	opts := ProcessChunksOptions{
		// When the KB has VLM enabled and we resolved remote images, pass them
		// through so processChunks will enqueue image:multimodal tasks (OCR + caption).
		EnableMultimodel: kb.IsMultimodalEnabled() && len(resolvedImages) > 0,
		StoredImages:     resolvedImages,
		SourceMarkdown:   clean,
	}
	if kb.QuestionGenerationConfig != nil && kb.QuestionGenerationConfig.Enabled {
		opts.EnableQuestionGeneration = true
		opts.QuestionCount = kb.QuestionGenerationConfig.QuestionCount
		if opts.QuestionCount <= 0 {
			opts.QuestionCount = 3
		}
	}

	if kb.ChunkingConfig.EnableParentChild {
		parentCfg, childCfg := buildParentChildConfigs(kb.ChunkingConfig, chunkCfg)
		pcResult := chunker.SplitTextParentChild(clean, parentCfg, childCfg)
		parsed = make([]types.ParsedChunk, len(pcResult.Children))
		for i, c := range pcResult.Children {
			parsed[i] = types.ParsedChunk{
				Content:     c.Content,
				Seq:         c.Seq,
				Start:       c.Start,
				End:         c.End,
				ParentIndex: c.ParentIndex,
			}
		}
		parentChunks := make([]types.ParsedParentChunk, len(pcResult.Parents))
		for i, p := range pcResult.Parents {
			parentChunks[i] = types.ParsedParentChunk{Content: p.Content, Seq: p.Seq, Start: p.Start, End: p.End}
		}
		opts.ParentChunks = parentChunks
	} else {
		splitChunks := chunker.SplitText(clean, chunkCfg)
		parsed = make([]types.ParsedChunk, len(splitChunks))
		for i, c := range splitChunks {
			parsed[i] = types.ParsedChunk{
				Content: c.Content,
				Seq:     c.Seq,
				Start:   c.Start,
				End:     c.End,
			}
		}
	}

	if doSync {
		s.processChunks(ctx, kb, knowledge, parsed, opts)
		return
	}

	newCtx := logger.CloneContext(ctx)
	go s.processChunks(newCtx, kb, knowledge, parsed, opts)
}

func (s *knowledgeService) cleanupKnowledgeResources(ctx context.Context, knowledge *types.Knowledge) error {
	logger.GetLogger(ctx).Infof("Cleaning knowledge resources before manual update, knowledge ID: %s", knowledge.ID)

	var cleanupErr error

	if knowledge.ParseStatus == types.ManualKnowledgeStatusDraft && knowledge.StorageSize == 0 {
		// Draft without indexed data, skip cleanup.
		return nil
	}

	tenantInfo := ctx.Value(types.TenantInfoContextKey).(*types.Tenant)
	if knowledge.EmbeddingModelID != "" {
		retrieveEngine, err := retriever.NewCompositeRetrieveEngine(
			s.retrieveEngine,
			tenantInfo.GetEffectiveEngines(),
		)
		if err != nil {
			logger.GetLogger(ctx).WithField("error", err).Error("Failed to init retrieve engine during cleanup")
			cleanupErr = errors.Join(cleanupErr, err)
		} else {
			embeddingModel, modelErr := s.modelService.GetEmbeddingModel(ctx, knowledge.EmbeddingModelID)
			if modelErr != nil {
				logger.GetLogger(ctx).WithField("error", modelErr).Error("Failed to get embedding model during cleanup")
				cleanupErr = errors.Join(cleanupErr, modelErr)
			} else {
				if err := retrieveEngine.DeleteByKnowledgeIDList(ctx, []string{knowledge.ID}, embeddingModel.GetDimensions(), knowledge.Type); err != nil {
					logger.GetLogger(ctx).WithField("error", err).Error("Failed to delete manual knowledge index")
					cleanupErr = errors.Join(cleanupErr, err)
				}
			}
		}
	}

	// Collect image URLs before chunks are deleted
	kb, _ := s.kbService.GetKnowledgeBaseByID(ctx, knowledge.KnowledgeBaseID)
	fileSvc := s.resolveFileService(ctx, kb)
	chunkImageInfos, imgErr := s.chunkService.GetRepository().ListImageInfoByKnowledgeIDs(ctx, tenantInfo.ID, []string{knowledge.ID})
	if imgErr != nil {
		logger.GetLogger(ctx).WithField("error", imgErr).Error("Failed to collect image URLs for cleanup")
		cleanupErr = errors.Join(cleanupErr, imgErr)
	}
	var imageInfoStrs []string
	for _, ci := range chunkImageInfos {
		imageInfoStrs = append(imageInfoStrs, ci.ImageInfo)
	}
	imageURLs := collectImageURLs(ctx, imageInfoStrs)

	if err := s.chunkService.DeleteChunksByKnowledgeID(ctx, knowledge.ID); err != nil {
		logger.GetLogger(ctx).WithField("error", err).Error("Failed to delete manual knowledge chunks")
		cleanupErr = errors.Join(cleanupErr, err)
	}

	// Delete extracted images after chunks are deleted
	deleteExtractedImages(ctx, fileSvc, imageURLs)

	namespace := types.NameSpace{KnowledgeBase: knowledge.KnowledgeBaseID, Knowledge: knowledge.ID}
	if err := s.graphEngine.DelGraph(ctx, []types.NameSpace{namespace}); err != nil {
		logger.GetLogger(ctx).WithField("error", err).Error("Failed to delete manual knowledge graph data")
		cleanupErr = errors.Join(cleanupErr, err)
	}

	if knowledge.StorageSize > 0 {
		tenantInfo.StorageUsed -= knowledge.StorageSize
		if tenantInfo.StorageUsed < 0 {
			tenantInfo.StorageUsed = 0
		}
		if err := s.tenantRepo.AdjustStorageUsed(ctx, tenantInfo.ID, -knowledge.StorageSize); err != nil {
			logger.GetLogger(ctx).WithField("error", err).Error("Failed to adjust storage usage during manual cleanup")
			cleanupErr = errors.Join(cleanupErr, err)
		}
		knowledge.StorageSize = 0
	}

	return cleanupErr
}

func (s *knowledgeService) getVLMConfig(ctx context.Context, kb *types.KnowledgeBase) (*types.DocParserVLMConfig, error) {
	if kb == nil {
		return nil, nil
	}
	// 兼容老版本：直接使用 ModelName 和 BaseURL
	if kb.VLMConfig.ModelName != "" && kb.VLMConfig.BaseURL != "" {
		return &types.DocParserVLMConfig{
			ModelName:     kb.VLMConfig.ModelName,
			BaseURL:       kb.VLMConfig.BaseURL,
			APIKey:        kb.VLMConfig.APIKey,
			InterfaceType: kb.VLMConfig.InterfaceType,
		}, nil
	}

	// 新版本：未启用或无模型ID时返回nil
	if !kb.VLMConfig.Enabled || kb.VLMConfig.ModelID == "" {
		return nil, nil
	}

	model, err := s.modelService.GetModelByID(ctx, kb.VLMConfig.ModelID)
	if err != nil {
		return nil, err
	}

	interfaceType := model.Parameters.InterfaceType
	if interfaceType == "" {
		interfaceType = "openai"
	}

	return &types.DocParserVLMConfig{
		ModelName:     model.Name,
		BaseURL:       model.Parameters.BaseURL,
		APIKey:        model.Parameters.APIKey,
		InterfaceType: interfaceType,
	}, nil
}

func (s *knowledgeService) buildStorageConfig(ctx context.Context, kb *types.KnowledgeBase) *types.DocParserStorageConfig {
	provider := kb.GetStorageProvider()
	if provider == "" {
		provider = "local"
	}

	// Backward compatibility: if legacy cos_config has full params for the chosen provider, use them.
	sc := &kb.StorageConfig
	hasKBFull := false
	switch provider {
	case "cos":
		hasKBFull = sc.SecretID != "" && sc.BucketName != ""
	case "minio":
		hasKBFull = sc.BucketName != ""
	case "local":
		hasKBFull = false
	}

	if hasKBFull {
		logger.Infof(ctx, "[storage] buildStorageConfig use legacy kb config: kb=%s provider=%s bucket=%s path_prefix=%s",
			kb.ID, provider, sc.BucketName, sc.PathPrefix)
		return &types.DocParserStorageConfig{
			Provider:        strings.ToUpper(provider),
			Region:          sc.Region,
			BucketName:      sc.BucketName,
			AccessKeyID:     sc.SecretID,
			SecretAccessKey: sc.SecretKey,
			AppID:           sc.AppID,
			PathPrefix:      sc.PathPrefix,
		}
	}

	// Merge from tenant's StorageEngineConfig.
	var out types.DocParserStorageConfig
	out.Provider = strings.ToUpper(provider)

	tenant, _ := ctx.Value(types.TenantInfoContextKey).(*types.Tenant)
	if tenant != nil && tenant.StorageEngineConfig != nil {
		sec := tenant.StorageEngineConfig
		if sec.DefaultProvider != "" && provider == "" {
			provider = strings.ToLower(strings.TrimSpace(sec.DefaultProvider))
			out.Provider = strings.ToUpper(provider)
		}
		switch provider {
		case "local":
			if sec.Local != nil {
				out.PathPrefix = sec.Local.PathPrefix
			}
		case "minio":
			if sec.MinIO != nil {
				out.BucketName = sec.MinIO.BucketName
				out.PathPrefix = sec.MinIO.PathPrefix
				if sec.MinIO.Mode == "remote" {
					out.Endpoint = sec.MinIO.Endpoint
					out.AccessKeyID = sec.MinIO.AccessKeyID
					out.SecretAccessKey = sec.MinIO.SecretAccessKey
				} else {
					out.Endpoint = os.Getenv("MINIO_ENDPOINT")
					out.AccessKeyID = os.Getenv("MINIO_ACCESS_KEY_ID")
					out.SecretAccessKey = os.Getenv("MINIO_SECRET_ACCESS_KEY")
				}
			}
		case "cos":
			if sec.COS != nil {
				out.Region = sec.COS.Region
				out.BucketName = sec.COS.BucketName
				out.AccessKeyID = sec.COS.SecretID
				out.SecretAccessKey = sec.COS.SecretKey
				out.AppID = sec.COS.AppID
				out.PathPrefix = sec.COS.PathPrefix
			}
		}
	}

	logger.Infof(ctx, "[storage] buildStorageConfig use merged tenant/global config: kb=%s provider=%s bucket=%s path_prefix=%s endpoint=%s",
		kb.ID, strings.ToLower(out.Provider), out.BucketName, out.PathPrefix, out.Endpoint)
	return &out
}

// resolveFileService returns the FileService for the given knowledge base,
// based on the KB's StorageProviderConfig (or legacy StorageConfig.Provider) and the tenant's StorageEngineConfig.
// Falls back to the global fileSvc when no tenant-level storage config is found.
func (s *knowledgeService) resolveFileService(ctx context.Context, kb *types.KnowledgeBase) interfaces.FileService {
	if kb == nil {
		logger.Infof(ctx, "[storage] resolveFileService fallback default: kb=nil")
		return s.fileSvc
	}

	provider := kb.GetStorageProvider()

	tenant, _ := ctx.Value(types.TenantInfoContextKey).(*types.Tenant)
	if provider == "" && tenant != nil && tenant.StorageEngineConfig != nil {
		provider = strings.ToLower(strings.TrimSpace(tenant.StorageEngineConfig.DefaultProvider))
	}

	if provider == "" || tenant == nil || tenant.StorageEngineConfig == nil {
		logger.Infof(ctx, "[storage] resolveFileService fallback default: kb=%s provider=%q tenant_cfg=%v",
			kb.ID, provider, tenant != nil && tenant.StorageEngineConfig != nil)
		return s.fileSvc
	}

	sec := tenant.StorageEngineConfig
	baseDir := strings.TrimSpace(os.Getenv("LOCAL_STORAGE_BASE_DIR"))
	svc, resolvedProvider, err := filesvc.NewFileServiceFromStorageConfig(provider, sec, baseDir)
	if err != nil {
		logger.Errorf(ctx, "Failed to create %s file service from tenant config: %v, falling back to default", provider, err)
		return s.fileSvc
	}
	logger.Infof(ctx, "[storage] resolveFileService selected: kb=%s provider=%s", kb.ID, resolvedProvider)
	return svc
}

// resolveFileServiceForPath is like resolveFileService but adds a safety check:
// if the resolved provider doesn't match what the filePath implies, fall back to
// the provider inferred from the file path. This protects historical data when
// tenant/KB config changes but files were stored under the old provider.
func (s *knowledgeService) resolveFileServiceForPath(ctx context.Context, kb *types.KnowledgeBase, filePath string) interfaces.FileService {
	svc := s.resolveFileService(ctx, kb)
	if filePath == "" {
		return svc
	}

	inferred := types.InferStorageFromFilePath(filePath)
	if inferred == "" {
		return svc
	}

	configured := kb.GetStorageProvider()
	if configured == "" {
		tenant, _ := ctx.Value(types.TenantInfoContextKey).(*types.Tenant)
		if tenant != nil && tenant.StorageEngineConfig != nil {
			configured = strings.ToLower(strings.TrimSpace(tenant.StorageEngineConfig.DefaultProvider))
		}
	}
	if configured == "" {
		configured = strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_TYPE")))
	}

	if configured != "" && configured != inferred {
		logger.Warnf(ctx, "[storage] FilePath format mismatch: configured=%s inferred=%s filePath=%s, using global fallback",
			configured, inferred, filePath)
		return s.fileSvc
	}
	return svc
}

func IsImageType(fileType string) bool {
	switch fileType {
	case "jpg", "jpeg", "png", "gif", "webp", "bmp", "svg", "tiff":
		return true
	default:
		return false
	}
}

// IsAudioType checks if a file type is an audio format
func IsAudioType(fileType string) bool {
	switch strings.ToLower(fileType) {
	case "mp3", "wav", "m4a", "flac", "ogg":
		return true
	default:
		return false
	}
}

// downloadFileFromURL downloads a remote file to a temp file and returns its binary content.
// payloadFileName and payloadFileType are in/out pointers: if they point to an empty string,
// the function resolves the value from Content-Disposition / URL path and writes it back.
// It does NOT perform SSRF validation — callers are responsible for that.
func downloadFileFromURL(ctx context.Context, fileURL string, payloadFileName, payloadFileType *string) ([]byte, error) {
	httpClient := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for file URL: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download file from URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote server returned status %d", resp.StatusCode)
	}

	// Reject oversized files early via Content-Length
	if contentLength := resp.ContentLength; contentLength > maxFileURLSize {
		return nil, fmt.Errorf("file size %d bytes exceeds limit of %d bytes (10MB)", contentLength, maxFileURLSize)
	}

	// Resolve fileName: payload > Content-Disposition > URL path
	if *payloadFileName == "" {
		if cd := resp.Header.Get("Content-Disposition"); cd != "" {
			*payloadFileName = extractFileNameFromContentDisposition(cd)
		}
	}
	if *payloadFileName == "" {
		*payloadFileName = extractFileNameFromURL(fileURL)
	}
	if *payloadFileType == "" && *payloadFileName != "" {
		*payloadFileType = getFileType(*payloadFileName)
	}

	// Stream response body into a temp file, capped at maxFileURLSize
	tmpFile, err := os.CreateTemp("", "weknora-fileurl-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	limiter := &io.LimitedReader{R: resp.Body, N: maxFileURLSize + 1}
	written, err := io.Copy(tmpFile, limiter)
	tmpFile.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to write temp file: %w", err)
	}
	if written > maxFileURLSize {
		return nil, fmt.Errorf("file size exceeds limit of 10MB")
	}

	contentBytes, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read temp file: %w", err)
	}

	return contentBytes, nil
}

// ProcessManualUpdate handles Asynq manual knowledge update tasks.
// It performs cleanup of old indexes/chunks (when NeedCleanup is true) and re-indexes the content.
func (s *knowledgeService) ProcessManualUpdate(ctx context.Context, t *asynq.Task) error {
	var payload types.ManualProcessPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		logger.Errorf(ctx, "failed to unmarshal manual process task payload: %v", err)
		return nil
	}

	ctx = logger.WithRequestID(ctx, payload.RequestId)
	ctx = logger.WithField(ctx, "manual_process", payload.KnowledgeID)
	ctx = context.WithValue(ctx, types.TenantIDContextKey, payload.TenantID)

	tenantInfo, err := s.tenantRepo.GetTenantByID(ctx, payload.TenantID)
	if err != nil {
		logger.Errorf(ctx, "ProcessManualUpdate: failed to get tenant: %v", err)
		return nil
	}
	ctx = context.WithValue(ctx, types.TenantInfoContextKey, tenantInfo)

	knowledge, err := s.repo.GetKnowledgeByID(ctx, payload.TenantID, payload.KnowledgeID)
	if err != nil {
		logger.Errorf(ctx, "ProcessManualUpdate: failed to get knowledge: %v", err)
		return nil
	}
	if knowledge == nil {
		logger.Warnf(ctx, "ProcessManualUpdate: knowledge not found: %s", payload.KnowledgeID)
		return nil
	}

	// Skip if already completed or being deleted
	if knowledge.ParseStatus == types.ParseStatusCompleted {
		logger.Infof(ctx, "ProcessManualUpdate: already completed, skipping: %s", payload.KnowledgeID)
		return nil
	}
	if knowledge.ParseStatus == types.ParseStatusDeleting {
		logger.Infof(ctx, "ProcessManualUpdate: being deleted, skipping: %s", payload.KnowledgeID)
		return nil
	}

	kb, err := s.kbService.GetKnowledgeBaseByID(ctx, payload.KnowledgeBaseID)
	if err != nil {
		logger.Errorf(ctx, "ProcessManualUpdate: failed to get knowledge base: %v", err)
		knowledge.ParseStatus = "failed"
		knowledge.ErrorMessage = fmt.Sprintf("failed to get knowledge base: %v", err)
		knowledge.UpdatedAt = time.Now()
		s.repo.UpdateKnowledge(ctx, knowledge)
		return nil
	}

	// Update status to processing
	knowledge.ParseStatus = "processing"
	knowledge.UpdatedAt = time.Now()
	if err := s.repo.UpdateKnowledge(ctx, knowledge); err != nil {
		logger.Errorf(ctx, "ProcessManualUpdate: failed to update status to processing: %v", err)
		return nil
	}

	// Cleanup old resources (indexes, chunks, graph) for update operations
	if payload.NeedCleanup {
		if err := s.cleanupKnowledgeResources(ctx, knowledge); err != nil {
			logger.ErrorWithFields(ctx, err, map[string]interface{}{
				"knowledge_id": payload.KnowledgeID,
			})
			knowledge.ParseStatus = "failed"
			knowledge.ErrorMessage = fmt.Sprintf("failed to cleanup old resources: %v", err)
			knowledge.UpdatedAt = time.Now()
			s.repo.UpdateKnowledge(ctx, knowledge)
			return nil
		}
	}

	// Run manual processing (image resolution + chunking + embedding) synchronously within the worker
	s.triggerManualProcessing(ctx, kb, knowledge, payload.Content, true)
	return nil
}

// ProcessDocument handles Asynq document processing tasks
func (s *knowledgeService) ProcessDocument(ctx context.Context, t *asynq.Task) error {
	var payload types.DocumentProcessPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		logger.Errorf(ctx, "failed to unmarshal document process task payload: %v", err)
		return nil
	}

	ctx = logger.WithRequestID(ctx, payload.RequestId)
	ctx = logger.WithField(ctx, "document_process", payload.KnowledgeID)
	ctx = context.WithValue(ctx, types.TenantIDContextKey, payload.TenantID)
	if payload.Language != "" {
		ctx = context.WithValue(ctx, types.LanguageContextKey, payload.Language)
	}

	// 获取任务重试信息，用于判断是否是最后一次重试
	retryCount, _ := asynq.GetRetryCount(ctx)
	maxRetry, _ := asynq.GetMaxRetry(ctx)
	isLastRetry := retryCount >= maxRetry

	tenantInfo, err := s.tenantRepo.GetTenantByID(ctx, payload.TenantID)
	if err != nil {
		logger.Errorf(ctx, "failed to get tenant: %v", err)
		return nil
	}
	ctx = context.WithValue(ctx, types.TenantInfoContextKey, tenantInfo)

	logger.Infof(ctx, "Processing document task: knowledge_id=%s, file_path=%s, retry=%d/%d",
		payload.KnowledgeID, payload.FilePath, retryCount, maxRetry)

	// 幂等性检查：获取knowledge记录
	knowledge, err := s.repo.GetKnowledgeByID(ctx, payload.TenantID, payload.KnowledgeID)
	if err != nil {
		logger.Errorf(ctx, "failed to get knowledge: %v", err)
		return nil
	}

	if knowledge == nil {
		return nil
	}

	// 检查是否正在删除 - 如果是则直接退出，避免与删除操作冲突
	if knowledge.ParseStatus == types.ParseStatusDeleting {
		logger.Infof(ctx, "Knowledge is being deleted, aborting processing: %s", payload.KnowledgeID)
		return nil
	}

	// 检查任务状态 - 幂等性处理
	if knowledge.ParseStatus == types.ParseStatusCompleted {
		logger.Infof(ctx, "Document already completed, skipping: %s", payload.KnowledgeID)
		return nil // 幂等：已完成的任务直接返回
	}

	if knowledge.ParseStatus == types.ParseStatusFailed {
		// 检查是否可恢复（例如：超时、临时错误等）
		// 对于不可恢复的错误，直接返回
		logger.Warnf(
			ctx,
			"Document processing previously failed: %s, error: %s",
			payload.KnowledgeID,
			knowledge.ErrorMessage,
		)
		// 这里可以根据错误类型判断是否可恢复，暂时允许重试
	}

	// 检查是否有部分处理（有chunks但状态不是completed）
	if knowledge.ParseStatus != "completed" && knowledge.ParseStatus != "pending" &&
		knowledge.ParseStatus != "processing" {
		// 状态异常，记录日志但继续处理
		logger.Warnf(ctx, "Unexpected parse status: %s for knowledge: %s", knowledge.ParseStatus, payload.KnowledgeID)
	}

	// 获取知识库信息
	kb, err := s.kbService.GetKnowledgeBaseByID(ctx, payload.KnowledgeBaseID)
	if err != nil {
		logger.Errorf(ctx, "failed to get knowledge base: %v", err)
		knowledge.ParseStatus = "failed"
		knowledge.ErrorMessage = fmt.Sprintf("failed to get knowledge base: %v", err)
		knowledge.UpdatedAt = time.Now()
		s.repo.UpdateKnowledge(ctx, knowledge)
		return nil
	}

	knowledge.ParseStatus = "processing"
	knowledge.UpdatedAt = time.Now()
	if err := s.repo.UpdateKnowledge(ctx, knowledge); err != nil {
		logger.Errorf(ctx, "failed to update knowledge status to processing: %v", err)
		return nil
	}

	// 检查多模态配置（仅对文件导入）
	if payload.FilePath != "" && !payload.EnableMultimodel && IsImageType(payload.FileType) {
		logger.GetLogger(ctx).WithField("knowledge_id", knowledge.ID).
			WithField("error", ErrImageNotParse).Errorf("processDocument image without enable multimodel")
		knowledge.ParseStatus = "failed"
		knowledge.ErrorMessage = ErrImageNotParse.Error()
		knowledge.UpdatedAt = time.Now()
		s.repo.UpdateKnowledge(ctx, knowledge)
		return nil
	}

	// 检查音频ASR配置（仅对文件导入）
	if payload.FilePath != "" && IsAudioType(payload.FileType) && !kb.ASRConfig.IsASREnabled() {
		logger.GetLogger(ctx).WithField("knowledge_id", knowledge.ID).
			Errorf("processDocument audio without ASR model configured")
		knowledge.ParseStatus = "failed"
		knowledge.ErrorMessage = "上传音频文件需要设置ASR语音识别模型"
		knowledge.UpdatedAt = time.Now()
		s.repo.UpdateKnowledge(ctx, knowledge)
		return nil
	}

	// New pipeline: convert -> store images -> chunk -> vectorize -> multimodal tasks
	var convertResult *types.ReadResult
	var chunks []types.ParsedChunk

	if payload.FileURL != "" {
		// file_url import: SSRF re-check (防 DNS 重绑定), download, persist, then delegate to convert()
		if err := secutils.ValidateURLForSSRF(payload.FileURL); err != nil {
			logger.Errorf(ctx, "File URL rejected for SSRF protection in ProcessDocument: %s, err: %v", payload.FileURL, err)
			knowledge.ParseStatus = "failed"
			knowledge.ErrorMessage = "File URL is not allowed for security reasons"
			knowledge.UpdatedAt = time.Now()
			s.repo.UpdateKnowledge(ctx, knowledge)
			return nil
		}

		resolvedFileName := payload.FileName
		resolvedFileType := payload.FileType
		contentBytes, err := downloadFileFromURL(ctx, payload.FileURL, &resolvedFileName, &resolvedFileType)
		if err != nil {
			logger.Errorf(ctx, "Failed to download file from URL: %s, error: %v", payload.FileURL, err)
			if isLastRetry {
				knowledge.ParseStatus = "failed"
				knowledge.ErrorMessage = err.Error()
				knowledge.UpdatedAt = time.Now()
				s.repo.UpdateKnowledge(ctx, knowledge)
			}
			return fmt.Errorf("failed to download file from URL: %w", err)
		}

		if resolvedFileType != "" && !allowedFileURLExtensions[strings.ToLower(resolvedFileType)] {
			logger.Errorf(ctx, "Unsupported file type resolved from file URL: %s", resolvedFileType)
			knowledge.ParseStatus = "failed"
			knowledge.ErrorMessage = fmt.Sprintf("unsupported file type: %s", resolvedFileType)
			knowledge.UpdatedAt = time.Now()
			s.repo.UpdateKnowledge(ctx, knowledge)
			return nil
		}

		if resolvedFileName != "" && knowledge.FileName == "" {
			knowledge.FileName = resolvedFileName
		}
		if resolvedFileType != "" && knowledge.FileType == "" {
			knowledge.FileType = resolvedFileType
			s.repo.UpdateKnowledge(ctx, knowledge)
		}

		fileSvc := s.resolveFileService(ctx, kb)
		filePath, err := fileSvc.SaveBytes(ctx, contentBytes, payload.TenantID, resolvedFileName, true)
		if err != nil {
			if isLastRetry {
				knowledge.ParseStatus = "failed"
				knowledge.ErrorMessage = err.Error()
				knowledge.UpdatedAt = time.Now()
				s.repo.UpdateKnowledge(ctx, knowledge)
			}
			return fmt.Errorf("failed to save downloaded file: %w", err)
		}

		payload.FilePath = filePath
		payload.FileName = resolvedFileName
		payload.FileType = resolvedFileType
		convertResult, err = s.convert(ctx, payload, kb, knowledge, isLastRetry)
		if err != nil {
			return err
		}
		if convertResult == nil {
			return nil
		}
	} else if payload.URL != "" {
		// URL import
		convertResult, err = s.convert(ctx, payload, kb, knowledge, isLastRetry)
		if err != nil {
			return err
		}
		if convertResult == nil {
			return nil
		}
		// Update knowledge title from extracted page title if not already set
		if knowledge.Title == "" || knowledge.Title == payload.URL {
			if extractedTitle := convertResult.Metadata["title"]; extractedTitle != "" {
				knowledge.Title = extractedTitle
				knowledge.UpdatedAt = time.Now()
				if err := s.repo.UpdateKnowledge(ctx, knowledge); err != nil {
					logger.Warnf(ctx, "Failed to update knowledge title from extracted page title: %v", err)
				} else {
					logger.Infof(ctx, "Updated knowledge title to extracted page title: %s", extractedTitle)
				}
			}
		}
	} else if len(payload.Passages) > 0 {
		// Text passage import - direct chunking, no conversion needed
		passageChunks := make([]types.ParsedChunk, 0, len(payload.Passages))
		start, end := 0, 0
		for i, p := range payload.Passages {
			if p == "" {
				continue
			}
			end += len([]rune(p))
			passageChunks = append(passageChunks, types.ParsedChunk{
				Content: p,
				Seq:     i,
				Start:   start,
				End:     end,
			})
			start = end
		}
		passageOpts := ProcessChunksOptions{
			EnableQuestionGeneration: payload.EnableQuestionGeneration,
			QuestionCount:            payload.QuestionCount,
		}
		s.processChunks(ctx, kb, knowledge, passageChunks, passageOpts)
		return nil
	} else {
		// File import
		convertResult, err = s.convert(ctx, payload, kb, knowledge, isLastRetry)
		if err != nil {
			return err
		}
		if convertResult == nil {
			return nil
		}
		// Update knowledge title from extracted document title if the current
		// title is just the file name (default set during upload).
		if extractedTitle := convertResult.Metadata["title"]; extractedTitle != "" {
			baseName := knowledge.FileName
			if idx := strings.LastIndex(baseName, "."); idx > 0 {
				baseName = baseName[:idx]
			}
			if knowledge.Title == "" || knowledge.Title == knowledge.FileName || knowledge.Title == baseName {
				knowledge.Title = extractedTitle
				knowledge.UpdatedAt = time.Now()
				if err := s.repo.UpdateKnowledge(ctx, knowledge); err != nil {
					logger.Warnf(ctx, "Failed to update knowledge title from document: %v", err)
				} else {
					logger.Infof(ctx, "Updated knowledge title to extracted document title: %s", extractedTitle)
				}
			}
		}
	}

	// Step 1.5: ASR transcription for audio files
	if convertResult != nil && convertResult.IsAudio && len(convertResult.AudioData) > 0 {
		if !kb.ASRConfig.IsASREnabled() {
			logger.Error(ctx, "Audio file detected but ASR is not configured")
			knowledge.ParseStatus = "failed"
			knowledge.ErrorMessage = "ASR model is not configured for audio transcription"
			knowledge.UpdatedAt = time.Now()
			s.repo.UpdateKnowledge(ctx, knowledge)
			return nil
		}

		logger.Infof(ctx, "[ASR] Starting audio transcription for knowledge %s, audio size=%d bytes",
			knowledge.ID, len(convertResult.AudioData))

		asrModel, err := s.modelService.GetASRModel(ctx, kb.ASRConfig.ModelID)
		if err != nil {
			logger.Errorf(ctx, "[ASR] Failed to get ASR model: %v", err)
			knowledge.ParseStatus = "failed"
			knowledge.ErrorMessage = fmt.Sprintf("failed to get ASR model: %v", err)
			knowledge.UpdatedAt = time.Now()
			s.repo.UpdateKnowledge(ctx, knowledge)
			return nil
		}

		transcribedText, err := asrModel.Transcribe(ctx, convertResult.AudioData, knowledge.FileName)
		if err != nil {
			logger.Errorf(ctx, "[ASR] Transcription failed: %v", err)
			if isLastRetry {
				knowledge.ParseStatus = "failed"
				knowledge.ErrorMessage = fmt.Sprintf("audio transcription failed: %v", err)
				knowledge.UpdatedAt = time.Now()
				s.repo.UpdateKnowledge(ctx, knowledge)
			}
			return fmt.Errorf("audio transcription failed: %w", err)
		}

		if transcribedText == "" {
			logger.Warn(ctx, "[ASR] Transcription returned empty text")
			transcribedText = "[No speech detected in audio file]"
		}

		logger.Infof(ctx, "[ASR] Transcription completed, text length=%d", len(transcribedText))
		// Replace the audio placeholder with the transcribed text
		convertResult.MarkdownContent = transcribedText
		convertResult.IsAudio = false
		convertResult.AudioData = nil
	}

	// Step 2: Store images and update markdown references
	var storedImages []docparser.StoredImage

	if s.imageResolver != nil && convertResult != nil {
		fileSvc := s.resolveFileService(ctx, kb)
		tenantID, _ := ctx.Value(types.TenantIDContextKey).(uint64)
		updatedMarkdown, images, resolveErr := s.imageResolver.ResolveAndStore(ctx, convertResult, fileSvc, tenantID)
		if resolveErr != nil {
			logger.Warnf(ctx, "Image resolution partially failed: %v", resolveErr)
		}
		if updatedMarkdown != "" {
			convertResult.MarkdownContent = updatedMarkdown
		}
		storedImages = images

		// Resolve remote http(s) images (e.g. markdown external URLs) → download + upload to storage.
		// ResolveAndStore handles inline bytes and base64; ResolveRemoteImages handles http/https URLs.
		updatedContent, remoteImages, remoteErr := s.imageResolver.ResolveRemoteImages(ctx, convertResult.MarkdownContent, fileSvc, tenantID)
		if remoteErr != nil {
			logger.Warnf(ctx, "Remote image resolution partially failed: %v", remoteErr)
		}
		if len(remoteImages) > 0 {
			logger.Infof(ctx, "Resolved %d remote images for knowledge %s", len(remoteImages), knowledge.ID)
			convertResult.MarkdownContent = updatedContent
			storedImages = append(storedImages, remoteImages...)
		}

		logger.Infof(ctx, "Resolved %d total images for knowledge %s", len(storedImages), knowledge.ID)
	}

	// Step 3: Split into chunks using Go chunker
	chunkCfg := chunker.SplitterConfig{
		ChunkSize:    kb.ChunkingConfig.ChunkSize,
		ChunkOverlap: kb.ChunkingConfig.ChunkOverlap,
		Separators:   kb.ChunkingConfig.Separators,
	}
	if chunkCfg.ChunkSize <= 0 {
		chunkCfg.ChunkSize = 512
	}
	if chunkCfg.ChunkOverlap <= 0 {
		chunkCfg.ChunkOverlap = 50
	}
	if len(chunkCfg.Separators) == 0 {
		chunkCfg.Separators = []string{"\n\n", "\n", "。"}
	}

	processOpts := ProcessChunksOptions{
		EnableQuestionGeneration: payload.EnableQuestionGeneration,
		QuestionCount:            payload.QuestionCount,
		EnableMultimodel:         payload.EnableMultimodel,
		StoredImages:             storedImages,
		SourceMarkdown:           convertResult.MarkdownContent,
	}

	if kb.ChunkingConfig.EnableParentChild {
		parentCfg, childCfg := buildParentChildConfigs(kb.ChunkingConfig, chunkCfg)
		pcResult := chunker.SplitTextParentChild(convertResult.MarkdownContent, parentCfg, childCfg)
		chunks = make([]types.ParsedChunk, len(pcResult.Children))
		for i, c := range pcResult.Children {
			chunks[i] = types.ParsedChunk{
				Content:     c.Content,
				Seq:         c.Seq,
				Start:       c.Start,
				End:         c.End,
				ParentIndex: c.ParentIndex,
			}
		}
		parentChunks := make([]types.ParsedParentChunk, len(pcResult.Parents))
		for i, p := range pcResult.Parents {
			parentChunks[i] = types.ParsedParentChunk{Content: p.Content, Seq: p.Seq, Start: p.Start, End: p.End}
		}
		processOpts.ParentChunks = parentChunks
		logger.Infof(ctx, "Split document into %d parent + %d child chunks for knowledge %s",
			len(pcResult.Parents), len(pcResult.Children), knowledge.ID)
	} else {
		splitChunks := chunker.SplitText(convertResult.MarkdownContent, chunkCfg)
		chunks = make([]types.ParsedChunk, len(splitChunks))
		for i, c := range splitChunks {
			chunks[i] = types.ParsedChunk{
				Content: c.Content,
				Seq:     c.Seq,
				Start:   c.Start,
				End:     c.End,
			}
		}
		logger.Infof(ctx, "Split document into %d chunks for knowledge %s", len(chunks), knowledge.ID)
	}

	// Step 4: Process chunks (vectorize + index + enqueue async tasks)
	s.processChunks(ctx, kb, knowledge, chunks, processOpts)

	return nil
}

// convert handles both file and URL reading using a unified ReadRequest.
func (s *knowledgeService) convert(
	ctx context.Context,
	payload types.DocumentProcessPayload,
	kb *types.KnowledgeBase,
	knowledge *types.Knowledge,
	isLastRetry bool,
) (*types.ReadResult, error) {
	isURL := payload.URL != ""
	fileType := payload.FileType
	overrides := s.getParserEngineOverridesFromContext(ctx)

	if isURL {
		if err := secutils.ValidateURLForSSRF(payload.URL); err != nil {
			logger.Errorf(ctx, "URL rejected for SSRF protection: %s, err: %v", payload.URL, err)
			knowledge.ParseStatus = "failed"
			knowledge.ErrorMessage = "URL is not allowed for security reasons"
			knowledge.UpdatedAt = time.Now()
			s.repo.UpdateKnowledge(ctx, knowledge)
			return nil, nil
		}
	}

	parserEngine := kb.ChunkingConfig.ResolveParserEngine(fileType)
	if isURL {
		parserEngine = kb.ChunkingConfig.ResolveParserEngine("url")
	}

	logger.Infof(ctx, "[convert] kb=%s fileType=%s isURL=%v engine=%q rules=%+v",
		kb.ID, fileType, isURL, parserEngine, kb.ChunkingConfig.ParserEngineRules)

	var reader interfaces.DocReader = s.resolveDocReader(parserEngine, fileType, isURL, overrides)
	if reader == nil {
		knowledge.ParseStatus = "failed"
		knowledge.ErrorMessage = "Document parsing service is not configured. Please use text/paragraph import or set DOCREADER_ADDR."
		knowledge.UpdatedAt = time.Now()
		s.repo.UpdateKnowledge(ctx, knowledge)
		return nil, nil
	}

	req := &types.ReadRequest{
		URL:                   payload.URL,
		Title:                 knowledge.Title,
		ParserEngine:          parserEngine,
		RequestID:             payload.RequestId,
		ParserEngineOverrides: overrides,
	}

	if !isURL {
		fileReader, err := s.resolveFileServiceForPath(ctx, kb, payload.FilePath).GetFile(ctx, payload.FilePath)
		if err != nil {
			return s.failKnowledge(ctx, knowledge, isLastRetry, "failed to get file: %v", err)
		}
		defer fileReader.Close()
		contentBytes, err := io.ReadAll(fileReader)
		if err != nil {
			return s.failKnowledge(ctx, knowledge, isLastRetry, "failed to read file: %v", err)
		}
		req.FileContent = contentBytes
		req.FileName = payload.FileName
		req.FileType = fileType
	}

	result, err := reader.Read(ctx, req)
	if err != nil {
		return s.failKnowledge(ctx, knowledge, isLastRetry, "document read failed: %v", err)
	}
	if result.Error != "" {
		knowledge.ParseStatus = "failed"
		knowledge.ErrorMessage = result.Error
		knowledge.UpdatedAt = time.Now()
		s.repo.UpdateKnowledge(ctx, knowledge)
		return nil, nil
	}
	return result, nil
}

// resolveDocReader returns the appropriate DocReader for the given engine.
// Returns nil when the required service is unavailable.
func (s *knowledgeService) resolveDocReader(engine, fileType string, isURL bool, overrides map[string]string) interfaces.DocReader {
	switch engine {
	case docparser.SimpleEngineName:
		return &docparser.SimpleFormatReader{}
	case "mineru":
		return docparser.NewMinerUReader(overrides)
	case "mineru_cloud":
		return docparser.NewMinerUCloudReader(overrides)
	case "builtin":
		// 明确指定使用 builtin 引擎（docreader），不使用 simple format 兜底
		return s.documentReader
	default:
		// 未指定引擎时的兜底逻辑：simple format 使用 Go 原生处理，其他使用 docreader
		if !isURL && docparser.IsSimpleFormat(fileType) {
			return &docparser.SimpleFormatReader{}
		}
		return s.documentReader
	}
}

// failKnowledge marks knowledge as failed (only on last retry) and returns an error.
func (s *knowledgeService) failKnowledge(
	ctx context.Context,
	knowledge *types.Knowledge,
	isLastRetry bool,
	format string,
	args ...interface{},
) (*types.ReadResult, error) {
	errMsg := fmt.Sprintf(format, args...)
	if isLastRetry {
		knowledge.ParseStatus = "failed"
		knowledge.ErrorMessage = errMsg
		knowledge.UpdatedAt = time.Now()
		s.repo.UpdateKnowledge(ctx, knowledge)
	}
	return nil, fmt.Errorf(format, args...)
}

// enqueueImageMultimodalTasks enqueues asynq tasks for multimodal image processing.
func (s *knowledgeService) enqueueImageMultimodalTasks(
	ctx context.Context,
	knowledge *types.Knowledge,
	kb *types.KnowledgeBase,
	images []docparser.StoredImage,
	chunks []types.ParsedChunk,
) {
	if s.task == nil || len(images) == 0 {
		return
	}

	for _, img := range images {
		// Match image to the ParsedChunk whose content contains the image URL.
		// ChunkID was populated by processChunks with the real DB UUID.
		chunkID := ""
		for _, c := range chunks {
			if strings.Contains(c.Content, img.ServingURL) {
				chunkID = c.ChunkID
				break
			}
		}
		if chunkID == "" && len(chunks) > 0 {
			chunkID = chunks[0].ChunkID
		}

		lang, _ := types.LanguageFromContext(ctx)
		payload := types.ImageMultimodalPayload{
			TenantID:        knowledge.TenantID,
			KnowledgeID:     knowledge.ID,
			KnowledgeBaseID: kb.ID,
			ChunkID:         chunkID,
			ImageURL:        img.ServingURL,
			EnableOCR:       true,
			EnableCaption:   true,
			Language:        lang,
		}

		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			logger.Warnf(ctx, "Failed to marshal image multimodal payload: %v", err)
			continue
		}

		task := asynq.NewTask(types.TypeImageMultimodal, payloadBytes)
		if _, err := s.task.Enqueue(task); err != nil {
			logger.Warnf(ctx, "Failed to enqueue image multimodal task for %s: %v", img.ServingURL, err)
		} else {
			logger.Infof(ctx, "Enqueued image:multimodal task for %s", img.ServingURL)
		}
	}
}

func (s *knowledgeService) ProcessKnowledgeListDelete(ctx context.Context, t *asynq.Task) error {
	var payload types.KnowledgeListDeletePayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		logger.Errorf(ctx, "Failed to unmarshal knowledge list delete payload: %v", err)
		return err
	}

	logger.Infof(ctx, "Processing knowledge list delete task for %d knowledge items", len(payload.KnowledgeIDs))

	// Get tenant info
	tenant, err := s.tenantRepo.GetTenantByID(ctx, payload.TenantID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get tenant %d: %v", payload.TenantID, err)
		return err
	}

	// Set context values
	ctx = context.WithValue(ctx, types.TenantIDContextKey, payload.TenantID)
	ctx = context.WithValue(ctx, types.TenantInfoContextKey, tenant)

	// Delete knowledge list
	if err := s.DeleteKnowledgeList(ctx, payload.KnowledgeIDs); err != nil {
		logger.Errorf(ctx, "Failed to delete knowledge list: %v", err)
		return err
	}

	logger.Infof(ctx, "Successfully deleted %d knowledge items", len(payload.KnowledgeIDs))
	return nil
}
