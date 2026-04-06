package service

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"slices"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/application/service/retriever"
	"github.com/Tencent/WeKnora/internal/config"
	werrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/infrastructure/docparser"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
)

// Error definitions for knowledge service operations
var (
	// ErrInvalidFileType is returned when an unsupported file type is provided
	ErrInvalidFileType = errors.New("unsupported file type")
	// ErrInvalidURL is returned when an invalid URL is provided
	ErrInvalidURL = errors.New("invalid URL")
	// ErrChunkNotFound is returned when a requested chunk cannot be found
	ErrChunkNotFound = errors.New("chunk not found")
	// ErrDuplicateFile is returned when trying to add a file that already exists
	ErrDuplicateFile = errors.New("file already exists")
	// ErrDuplicateURL is returned when trying to add a URL that already exists
	ErrDuplicateURL = errors.New("URL already exists")
	// ErrImageNotParse is returned when trying to update image information without enabling multimodel
	ErrImageNotParse = errors.New("image not parse without enable multimodel")
)

// knowledgeService implements the knowledge service interface
// service 实现知识服务接口
type knowledgeService struct {
	config         *config.Config
	retrieveEngine interfaces.RetrieveEngineRegistry
	repo           interfaces.KnowledgeRepository
	kbService      interfaces.KnowledgeBaseService
	tenantRepo     interfaces.TenantRepository
	documentReader interfaces.DocumentReader
	chunkService   interfaces.ChunkService
	chunkRepo      interfaces.ChunkRepository
	tagRepo        interfaces.KnowledgeTagRepository
	tagService     interfaces.KnowledgeTagService
	fileSvc        interfaces.FileService
	modelService   interfaces.ModelService
	task           interfaces.TaskEnqueuer
	graphEngine    interfaces.RetrieveGraphRepository
	redisClient    *redis.Client
	kbShareService interfaces.KBShareService
	imageResolver  *docparser.ImageResolver
}

const (
	manualContentMaxLength = 200000
	manualFileExtension    = ".md"
	faqImportBatchSize     = 50 // 每批处理的FAQ条目数
)

// NewKnowledgeService creates a new knowledge service instance
func NewKnowledgeService(
	config *config.Config,
	repo interfaces.KnowledgeRepository,
	documentReader interfaces.DocumentReader,
	kbService interfaces.KnowledgeBaseService,
	tenantRepo interfaces.TenantRepository,
	chunkService interfaces.ChunkService,
	chunkRepo interfaces.ChunkRepository,
	tagRepo interfaces.KnowledgeTagRepository,
	tagService interfaces.KnowledgeTagService,
	fileSvc interfaces.FileService,
	modelService interfaces.ModelService,
	task interfaces.TaskEnqueuer,
	graphEngine interfaces.RetrieveGraphRepository,
	retrieveEngine interfaces.RetrieveEngineRegistry,
	redisClient *redis.Client,
	kbShareService interfaces.KBShareService,
	imageResolver *docparser.ImageResolver,
) (interfaces.KnowledgeService, error) {
	return &knowledgeService{
		config:         config,
		repo:           repo,
		kbService:      kbService,
		tenantRepo:     tenantRepo,
		documentReader: documentReader,
		chunkService:   chunkService,
		chunkRepo:      chunkRepo,
		tagRepo:        tagRepo,
		tagService:     tagService,
		fileSvc:        fileSvc,
		modelService:   modelService,
		task:           task,
		graphEngine:    graphEngine,
		retrieveEngine: retrieveEngine,
		redisClient:    redisClient,
		kbShareService: kbShareService,
		imageResolver:  imageResolver,
	}, nil
}

// getParserEngineOverridesFromContext returns parser engine overrides from tenant in context (e.g. MinerU endpoint, API key).
// Used when building document ReadRequest so UI-configured values take precedence over env.
func (s *knowledgeService) getParserEngineOverridesFromContext(ctx context.Context) map[string]string {
	if v := ctx.Value(types.TenantInfoContextKey); v != nil {
		if tenant, ok := v.(*types.Tenant); ok && tenant != nil {
			return tenant.ParserEngineConfig.ToOverridesMap()
		}
	}
	return nil
}

// GetRepository gets the knowledge repository
// Parameters:
//   - ctx: Context with authentication and request information
//
// Returns:
//   - interfaces.KnowledgeRepository: Knowledge repository
func (s *knowledgeService) GetRepository() interfaces.KnowledgeRepository {
	return s.repo
}

// isKnowledgeDeleting checks if a knowledge entry is being deleted.
// This is used to prevent async tasks from conflicting with deletion operations.
func (s *knowledgeService) isKnowledgeDeleting(ctx context.Context, tenantID uint64, knowledgeID string) bool {
	knowledge, err := s.repo.GetKnowledgeByID(ctx, tenantID, knowledgeID)
	if err != nil {
		// If we can't find the knowledge, assume it's deleted
		logger.Warnf(ctx, "Failed to check knowledge deletion status (assuming deleted): %v", err)
		return true
	}
	if knowledge == nil {
		return true
	}
	return knowledge.ParseStatus == types.ParseStatusDeleting
}

// checkStorageEngineConfigured verifies that the knowledge base has a storage engine configured
// (either at the KB level or via the tenant default). Returns an error if no storage engine is found.
func checkStorageEngineConfigured(ctx context.Context, kb *types.KnowledgeBase) error {
	provider := kb.GetStorageProvider()
	if provider == "" {
		tenant, _ := ctx.Value(types.TenantInfoContextKey).(*types.Tenant)
		if tenant != nil && tenant.StorageEngineConfig != nil {
			provider = strings.ToLower(strings.TrimSpace(tenant.StorageEngineConfig.DefaultProvider))
		}
	}
	if provider == "" {
		return werrors.NewBadRequestError("请先为知识库选择存储引擎，再上传内容。请前往知识库设置页面进行配置。")
	}
	return nil
}

func defaultChannel(ch string) string {
	if ch == "" {
		return types.ChannelWeb
	}
	return ch
}

// GetKnowledgeByID retrieves a knowledge entry by its ID
func (s *knowledgeService) GetKnowledgeByID(ctx context.Context, id string) (*types.Knowledge, error) {
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)

	knowledge, err := s.repo.GetKnowledgeByID(ctx, tenantID, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_id": id,
			"tenant_id":    tenantID,
		})
		return nil, err
	}

	logger.Infof(ctx, "Knowledge retrieved successfully, ID: %s, type: %s", knowledge.ID, knowledge.Type)
	return knowledge, nil
}

// GetKnowledgeByIDOnly retrieves knowledge by ID without tenant filter (for permission resolution).
func (s *knowledgeService) GetKnowledgeByIDOnly(ctx context.Context, id string) (*types.Knowledge, error) {
	return s.repo.GetKnowledgeByIDOnly(ctx, id)
}

// ListKnowledgeByKnowledgeBaseID returns all knowledge entries in a knowledge base
func (s *knowledgeService) ListKnowledgeByKnowledgeBaseID(ctx context.Context,
	kbID string,
) ([]*types.Knowledge, error) {
	return s.repo.ListKnowledgeByKnowledgeBaseID(ctx, ctx.Value(types.TenantIDContextKey).(uint64), kbID)
}

// ListPagedKnowledgeByKnowledgeBaseID returns paginated knowledge entries in a knowledge base
func (s *knowledgeService) ListPagedKnowledgeByKnowledgeBaseID(ctx context.Context,
	kbID string, page *types.Pagination, tagID string, keyword string, fileType string,
) (*types.PageResult, error) {
	knowledges, total, err := s.repo.ListPagedKnowledgeByKnowledgeBaseID(ctx,
		ctx.Value(types.TenantIDContextKey).(uint64), kbID, page, tagID, keyword, fileType)
	if err != nil {
		return nil, err
	}

	return types.NewPageResult(total, page, knowledges), nil
}

// collectImageURLs extracts unique provider:// image URLs from image_info JSON strings.
func collectImageURLs(ctx context.Context, imageInfos []string) []string {
	seen := make(map[string]struct{})
	var urls []string
	for _, info := range imageInfos {
		if info == "" {
			continue
		}
		var images []*types.ImageInfo
		if err := json.Unmarshal([]byte(info), &images); err != nil {
			logger.Warnf(ctx, "Failed to parse image_info JSON: %v", err)
			continue
		}
		for _, img := range images {
			if img.URL != "" {
				if _, exists := seen[img.URL]; !exists {
					seen[img.URL] = struct{}{}
					urls = append(urls, img.URL)
				}
			}
		}
	}
	return urls
}

// deleteExtractedImages deletes all extracted image files from storage.
// Standalone function — callable from both knowledgeService and knowledgeBaseService.
// Errors are logged but do not fail the overall deletion.
func deleteExtractedImages(ctx context.Context, fileSvc interfaces.FileService, imageURLs []string) {
	if len(imageURLs) == 0 {
		return
	}
	logger.Infof(ctx, "Deleting %d extracted images", len(imageURLs))
	for _, url := range imageURLs {
		if err := fileSvc.DeleteFile(ctx, url); err != nil {
			logger.Errorf(ctx, "Failed to delete extracted image %s: %v", url, err)
		}
	}
}

// DeleteKnowledge deletes a knowledge entry and all related resources
func (s *knowledgeService) DeleteKnowledge(ctx context.Context, id string) error {
	// Get the knowledge entry
	knowledge, err := s.repo.GetKnowledgeByID(ctx, ctx.Value(types.TenantIDContextKey).(uint64), id)
	if err != nil {
		return err
	}

	// Mark as deleting first to prevent async task conflicts
	// This ensures that any running async tasks will detect the deletion and abort
	originalStatus := knowledge.ParseStatus
	knowledge.ParseStatus = types.ParseStatusDeleting
	knowledge.UpdatedAt = time.Now()
	if err := s.repo.UpdateKnowledge(ctx, knowledge); err != nil {
		logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge failed to mark as deleting")
		// Continue with deletion even if marking fails
	} else {
		logger.Infof(ctx, "Marked knowledge %s as deleting (previous status: %s)", id, originalStatus)
	}

	// Resolve file service for this KB before spawning goroutines
	kb, _ := s.kbService.GetKnowledgeBaseByID(ctx, knowledge.KnowledgeBaseID)
	kbFileSvc := s.resolveFileService(ctx, kb)

	// Collect image URLs before chunks are deleted (ImageInfo references are lost after deletion)
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	chunkImageInfos, err := s.chunkService.GetRepository().ListImageInfoByKnowledgeIDs(ctx, tenantID, []string{id})
	if err != nil {
		logger.Errorf(ctx, "Failed to collect image URLs for cleanup: %v", err)
	}
	var imageInfoStrs []string
	for _, ci := range chunkImageInfos {
		imageInfoStrs = append(imageInfoStrs, ci.ImageInfo)
	}
	imageURLs := collectImageURLs(ctx, imageInfoStrs)

	wg := errgroup.Group{}
	// Delete knowledge embeddings from vector store
	wg.Go(func() error {
		tenantInfo := ctx.Value(types.TenantInfoContextKey).(*types.Tenant)
		retrieveEngine, err := retriever.NewCompositeRetrieveEngine(
			s.retrieveEngine,
			tenantInfo.GetEffectiveEngines(),
		)
		if err != nil {
			logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge delete knowledge embedding failed")
			return err
		}
		embeddingModel, err := s.modelService.GetEmbeddingModel(ctx, knowledge.EmbeddingModelID)
		if err != nil {
			logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge delete knowledge embedding failed")
			return err
		}
		if err := retrieveEngine.DeleteByKnowledgeIDList(ctx, []string{knowledge.ID}, embeddingModel.GetDimensions(), knowledge.Type); err != nil {
			logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge delete knowledge embedding failed")
			return err
		}
		return nil
	})

	// Delete all chunks associated with this knowledge
	wg.Go(func() error {
		if err := s.chunkService.DeleteChunksByKnowledgeID(ctx, knowledge.ID); err != nil {
			logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge delete chunks failed")
			return err
		}
		return nil
	})

	// Delete the physical file and extracted images if they exist
	wg.Go(func() error {
		if knowledge.FilePath != "" {
			if err := kbFileSvc.DeleteFile(ctx, knowledge.FilePath); err != nil {
				logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge delete file failed")
			}
		}
		deleteExtractedImages(ctx, kbFileSvc, imageURLs)
		tenantInfo := ctx.Value(types.TenantInfoContextKey).(*types.Tenant)
		tenantInfo.StorageUsed -= knowledge.StorageSize
		if err := s.tenantRepo.AdjustStorageUsed(ctx, tenantInfo.ID, -knowledge.StorageSize); err != nil {
			logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge update tenant storage used failed")
		}
		return nil
	})

	// Delete the knowledge graph
	wg.Go(func() error {
		namespace := types.NameSpace{KnowledgeBase: knowledge.KnowledgeBaseID, Knowledge: knowledge.ID}
		if err := s.graphEngine.DelGraph(ctx, []types.NameSpace{namespace}); err != nil {
			logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge delete knowledge graph failed")
			return err
		}
		return nil
	})

	if err = wg.Wait(); err != nil {
		return err
	}
	// Delete the knowledge entry itself from the database
	return s.repo.DeleteKnowledge(ctx, ctx.Value(types.TenantIDContextKey).(uint64), id)
}

// DeleteKnowledgeList deletes a knowledge entry and all related resources
func (s *knowledgeService) DeleteKnowledgeList(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	// 1. Get the knowledge entry
	tenantInfo := ctx.Value(types.TenantInfoContextKey).(*types.Tenant)
	knowledgeList, err := s.repo.GetKnowledgeBatch(ctx, tenantInfo.ID, ids)
	if err != nil {
		return err
	}

	// Mark all as deleting first to prevent async task conflicts
	for _, knowledge := range knowledgeList {
		knowledge.ParseStatus = types.ParseStatusDeleting
		knowledge.UpdatedAt = time.Now()
		if err := s.repo.UpdateKnowledge(ctx, knowledge); err != nil {
			logger.GetLogger(ctx).WithField("error", err).WithField("knowledge_id", knowledge.ID).
				Errorf("DeleteKnowledgeList failed to mark as deleting")
			// Continue with deletion even if marking fails
		}
	}
	logger.Infof(ctx, "Marked %d knowledge entries as deleting", len(knowledgeList))

	// Pre-resolve file services per KB so goroutines don't need DB access
	kbFileServices := make(map[string]interfaces.FileService)
	for _, knowledge := range knowledgeList {
		if _, ok := kbFileServices[knowledge.KnowledgeBaseID]; !ok {
			kb, _ := s.kbService.GetKnowledgeBaseByID(ctx, knowledge.KnowledgeBaseID)
			kbFileServices[knowledge.KnowledgeBaseID] = s.resolveFileService(ctx, kb)
		}
	}

	// Collect image URLs before chunks are deleted
	chunkImageInfos, err := s.chunkService.GetRepository().ListImageInfoByKnowledgeIDs(ctx, tenantInfo.ID, ids)
	if err != nil {
		logger.Errorf(ctx, "Failed to collect image URLs for batch cleanup: %v", err)
	}
	knowledgeToKB := make(map[string]string)
	for _, k := range knowledgeList {
		knowledgeToKB[k.ID] = k.KnowledgeBaseID
	}
	kbImageInfos := make(map[string][]string) // kbID → []imageInfo JSON
	for _, ci := range chunkImageInfos {
		kbID := knowledgeToKB[ci.KnowledgeID]
		kbImageInfos[kbID] = append(kbImageInfos[kbID], ci.ImageInfo)
	}
	kbImageURLs := make(map[string][]string) // kbID → []imageURL (deduplicated)
	for kbID, infos := range kbImageInfos {
		kbImageURLs[kbID] = collectImageURLs(ctx, infos)
	}

	wg := errgroup.Group{}
	// 2. Delete knowledge embeddings from vector store
	wg.Go(func() error {
		tenantInfo := ctx.Value(types.TenantInfoContextKey).(*types.Tenant)
		retrieveEngine, err := retriever.NewCompositeRetrieveEngine(
			s.retrieveEngine,
			tenantInfo.GetEffectiveEngines(),
		)
		if err != nil {
			logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge delete knowledge embedding failed")
			return err
		}
		// Group by EmbeddingModelID and Type
		type groupKey struct {
			EmbeddingModelID string
			Type             string
		}
		group := map[groupKey][]string{}
		for _, knowledge := range knowledgeList {
			key := groupKey{EmbeddingModelID: knowledge.EmbeddingModelID, Type: knowledge.Type}
			group[key] = append(group[key], knowledge.ID)
		}
		for key, knowledgeIDs := range group {
			embeddingModel, err := s.modelService.GetEmbeddingModel(ctx, key.EmbeddingModelID)
			if err != nil {
				logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge get embedding model failed")
				return err
			}
			if err := retrieveEngine.DeleteByKnowledgeIDList(ctx, knowledgeIDs, embeddingModel.GetDimensions(), key.Type); err != nil {
				logger.GetLogger(ctx).
					WithField("error", err).
					Errorf("DeleteKnowledge delete knowledge embedding failed")
				return err
			}
		}
		return nil
	})

	// 3. Delete all chunks associated with this knowledge
	wg.Go(func() error {
		if err := s.chunkService.DeleteByKnowledgeList(ctx, ids); err != nil {
			logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge delete chunks failed")
			return err
		}
		return nil
	})

	// 4. Delete the physical file and extracted images if they exist
	wg.Go(func() error {
		storageAdjust := int64(0)
		for _, knowledge := range knowledgeList {
			if knowledge.FilePath != "" {
				fSvc := kbFileServices[knowledge.KnowledgeBaseID]
				if err := fSvc.DeleteFile(ctx, knowledge.FilePath); err != nil {
					logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge delete file failed")
				}
			}
			storageAdjust -= knowledge.StorageSize
		}
		// Delete extracted images per KB
		for kbID, urls := range kbImageURLs {
			fSvc := kbFileServices[kbID]
			if fSvc == nil {
				logger.Warnf(ctx, "No file service for KB %s, skipping %d image deletions", kbID, len(urls))
				continue
			}
			deleteExtractedImages(ctx, fSvc, urls)
		}
		tenantInfo.StorageUsed += storageAdjust
		if err := s.tenantRepo.AdjustStorageUsed(ctx, tenantInfo.ID, storageAdjust); err != nil {
			logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge update tenant storage used failed")
		}
		return nil
	})

	// Delete the knowledge graph
	wg.Go(func() error {
		namespaces := []types.NameSpace{}
		for _, knowledge := range knowledgeList {
			namespaces = append(
				namespaces,
				types.NameSpace{KnowledgeBase: knowledge.KnowledgeBaseID, Knowledge: knowledge.ID},
			)
		}
		if err := s.graphEngine.DelGraph(ctx, namespaces); err != nil {
			logger.GetLogger(ctx).WithField("error", err).Errorf("DeleteKnowledge delete knowledge graph failed")
			return err
		}
		return nil
	})

	if err = wg.Wait(); err != nil {
		return err
	}
	// 5. Delete the knowledge entry itself from the database
	return s.repo.DeleteKnowledgeList(ctx, tenantInfo.ID, ids)
}

// GetKnowledgeFile retrieves the physical file associated with a knowledge entry
func (s *knowledgeService) GetKnowledgeFile(ctx context.Context, id string) (io.ReadCloser, string, error) {
	// Get knowledge record
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	knowledge, err := s.repo.GetKnowledgeByID(ctx, tenantID, id)
	if err != nil {
		return nil, "", err
	}

	// Manual knowledge stores content in Metadata — stream it directly as a .md file.
	if knowledge.IsManual() {
		meta, err := knowledge.ManualMetadata()
		if err != nil {
			return nil, "", err
		}
		// ManualMetadata returns (nil, nil) when Metadata column is empty; treat as empty content.
		content := ""
		if meta != nil {
			content = meta.Content
		}
		filename := sanitizeManualDownloadFilename(knowledge.Title)
		return io.NopCloser(strings.NewReader(content)), filename, nil
	}

	// Resolve KB-level file service with FilePath fallback protection
	kb, _ := s.kbService.GetKnowledgeBaseByID(ctx, knowledge.KnowledgeBaseID)
	file, err := s.resolveFileServiceForPath(ctx, kb, knowledge.FilePath).GetFile(ctx, knowledge.FilePath)
	if err != nil {
		return nil, "", err
	}

	return file, knowledge.FileName, nil
}

func (s *knowledgeService) UpdateKnowledge(ctx context.Context, knowledge *types.Knowledge) error {
	record, err := s.repo.GetKnowledgeByID(ctx, ctx.Value(types.TenantIDContextKey).(uint64), knowledge.ID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get knowledge record: %v", err)
		return err
	}
	// if need other fields update, please add here
	if knowledge.Title != "" {
		record.Title = knowledge.Title
	}
	if knowledge.Description != "" {
		record.Description = knowledge.Description
	}

	// Update knowledge record in the repository
	if err := s.repo.UpdateKnowledge(ctx, record); err != nil {
		logger.Errorf(ctx, "Failed to update knowledge: %v", err)
		return err
	}
	logger.Infof(ctx, "Knowledge updated successfully, ID: %s", knowledge.ID)
	return nil
}

// UpdateManualKnowledge updates manual Markdown knowledge content.
// For publish status, the heavy operations (cleanup old indexes, re-chunking,
// re-embedding) are offloaded to an Asynq task so the HTTP response returns quickly.
func (s *knowledgeService) UpdateManualKnowledge(ctx context.Context,
	knowledgeID string, payload *types.ManualKnowledgePayload,
) (*types.Knowledge, error) {
	logger.Info(ctx, "Start updating manual knowledge entry")
	if payload == nil {
		return nil, werrors.NewBadRequestError("请求内容不能为空")
	}

	cleanContent := secutils.CleanMarkdown(payload.Content)
	if strings.TrimSpace(cleanContent) == "" {
		return nil, werrors.NewValidationError("内容不能为空")
	}
	if len([]rune(cleanContent)) > manualContentMaxLength {
		return nil, werrors.NewValidationError(fmt.Sprintf("内容长度超出限制（最多%d个字符）", manualContentMaxLength))
	}

	safeTitle, ok := secutils.ValidateInput(payload.Title)
	if !ok {
		return nil, werrors.NewValidationError("标题包含非法字符或超出长度限制")
	}

	status := strings.ToLower(strings.TrimSpace(payload.Status))
	if status == "" {
		status = types.ManualKnowledgeStatusDraft
	}
	if status != types.ManualKnowledgeStatusDraft && status != types.ManualKnowledgeStatusPublish {
		return nil, werrors.NewValidationError("状态仅支持 draft 或 publish")
	}

	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	existing, err := s.repo.GetKnowledgeByID(ctx, tenantID, knowledgeID)
	if err != nil {
		logger.Errorf(ctx, "Failed to load knowledge: %v", err)
		return nil, err
	}
	if !existing.IsManual() {
		return nil, werrors.NewBadRequestError("仅支持手工知识的在线编辑")
	}

	kb, err := s.kbService.GetKnowledgeBaseByID(ctx, existing.KnowledgeBaseID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get knowledge base for manual update: %v", err)
		return nil, err
	}

	var version int
	if meta, err := existing.ManualMetadata(); err == nil && meta != nil {
		version = meta.Version + 1
	} else {
		version = 1
	}

	meta := types.NewManualKnowledgeMetadata(cleanContent, status, version)
	if err := existing.SetManualMetadata(meta); err != nil {
		logger.Errorf(ctx, "Failed to set manual metadata during update: %v", err)
		return nil, err
	}

	if safeTitle != "" {
		existing.Title = safeTitle
	} else if existing.Title == "" {
		existing.Title = fmt.Sprintf("手工知识-%s", time.Now().Format("20060102-150405"))
	}
	existing.FileName = ensureManualFileName(existing.Title)
	existing.FileType = types.KnowledgeTypeManual
	existing.Type = types.KnowledgeTypeManual
	existing.Source = types.KnowledgeTypeManual
	existing.EnableStatus = "disabled"
	existing.UpdatedAt = time.Now()
	existing.EmbeddingModelID = kb.EmbeddingModelID

	if status == types.ManualKnowledgeStatusDraft {
		existing.ParseStatus = types.ManualKnowledgeStatusDraft
		existing.Description = ""
		existing.ProcessedAt = nil

		if err := s.repo.UpdateKnowledge(ctx, existing); err != nil {
			logger.Errorf(ctx, "Failed to persist manual draft: %v", err)
			return nil, err
		}
		return existing, nil
	}

	// Publish: persist pending status and enqueue async task for cleanup + re-indexing
	existing.ParseStatus = "pending"
	existing.Description = ""
	existing.ProcessedAt = nil

	if err := s.repo.UpdateKnowledge(ctx, existing); err != nil {
		logger.Errorf(ctx, "Failed to persist manual knowledge before indexing: %v", err)
		return nil, err
	}

	logger.Infof(ctx, "Manual knowledge updated, enqueuing async processing task, ID: %s", existing.ID)
	if err := s.enqueueManualProcessing(ctx, existing, cleanContent, true); err != nil {
		logger.Errorf(ctx, "Failed to enqueue manual processing task: %v", err)
		// Non-fatal: mark as failed so user can retry
		existing.ParseStatus = "failed"
		existing.ErrorMessage = "Failed to enqueue processing task"
		s.repo.UpdateKnowledge(ctx, existing)
		return nil, werrors.NewInternalServerError("Failed to submit processing task")
	}
	return existing, nil
}

// enqueueManualProcessing enqueues a manual:process Asynq task for async cleanup + re-indexing.
func (s *knowledgeService) enqueueManualProcessing(ctx context.Context,
	knowledge *types.Knowledge, content string, needCleanup bool,
) error {
	requestID, _ := types.RequestIDFromContext(ctx)
	payload := types.ManualProcessPayload{
		RequestId:       requestID,
		TenantID:        knowledge.TenantID,
		KnowledgeID:     knowledge.ID,
		KnowledgeBaseID: knowledge.KnowledgeBaseID,
		Content:         content,
		NeedCleanup:     needCleanup,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal manual process payload: %w", err)
	}

	task := asynq.NewTask(types.TypeManualProcess, payloadBytes, asynq.Queue("default"), asynq.MaxRetry(3))
	info, err := s.task.Enqueue(task)
	if err != nil {
		return fmt.Errorf("failed to enqueue manual process task: %w", err)
	}
	logger.Infof(ctx, "Enqueued manual process task: knowledge_id=%s, asynq_id=%s", knowledge.ID, info.ID)
	return nil
}

// ReparseKnowledge deletes existing document content and re-parses the knowledge asynchronously.
// This method reuses the logic from UpdateManualKnowledge for resource cleanup and async parsing.
func (s *knowledgeService) ReparseKnowledge(ctx context.Context, knowledgeID string) (*types.Knowledge, error) {
	logger.Info(ctx, "Start re-parsing knowledge")

	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	existing, err := s.repo.GetKnowledgeByID(ctx, tenantID, knowledgeID)
	if err != nil {
		logger.Errorf(ctx, "Failed to load knowledge: %v", err)
		return nil, err
	}

	// Get knowledge base configuration
	kb, err := s.kbService.GetKnowledgeBaseByID(ctx, existing.KnowledgeBaseID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get knowledge base for reparse: %v", err)
		return nil, err
	}

	// For manual knowledge, use async manual processing (cleanup + re-indexing in worker)
	if existing.IsManual() {
		meta, metaErr := existing.ManualMetadata()
		if metaErr != nil || meta == nil {
			logger.Errorf(ctx, "Failed to get manual metadata for reparse: %v", metaErr)
			return nil, werrors.NewBadRequestError("无法获取手工知识内容")
		}

		existing.ParseStatus = "pending"
		existing.EnableStatus = "disabled"
		existing.Description = ""
		existing.ProcessedAt = nil
		existing.EmbeddingModelID = kb.EmbeddingModelID

		if err := s.repo.UpdateKnowledge(ctx, existing); err != nil {
			logger.Errorf(ctx, "Failed to update knowledge status before reparse: %v", err)
			return nil, err
		}

		if err := s.enqueueManualProcessing(ctx, existing, meta.Content, true); err != nil {
			logger.Errorf(ctx, "Failed to enqueue manual reparse task: %v", err)
			existing.ParseStatus = "failed"
			existing.ErrorMessage = "Failed to enqueue processing task"
			s.repo.UpdateKnowledge(ctx, existing)
		}
		return existing, nil
	}

	// For non-manual knowledge, cleanup synchronously then enqueue document processing
	logger.Infof(ctx, "Cleaning up existing resources for knowledge: %s", knowledgeID)
	if err := s.cleanupKnowledgeResources(ctx, existing); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_id": knowledgeID,
		})
		return nil, err
	}

	// Step 2: Update knowledge status and metadata
	existing.ParseStatus = "pending"
	existing.EnableStatus = "disabled"
	existing.Description = ""
	existing.ProcessedAt = nil
	existing.EmbeddingModelID = kb.EmbeddingModelID

	if err := s.repo.UpdateKnowledge(ctx, existing); err != nil {
		logger.Errorf(ctx, "Failed to update knowledge status before reparse: %v", err)
		return nil, err
	}

	// Step 3: Trigger async re-parsing based on knowledge type
	logger.Infof(ctx, "Knowledge status updated, scheduling async reparse, ID: %s, Type: %s", existing.ID, existing.Type)

	// For file-based knowledge, enqueue document processing task
	if existing.FilePath != "" {
		tenantID := ctx.Value(types.TenantIDContextKey).(uint64)

		// Determine multimodal setting
		enableMultimodel := kb.IsMultimodalEnabled()

		// Check question generation config
		enableQuestionGeneration := false
		questionCount := 3 // default
		if kb.QuestionGenerationConfig != nil && kb.QuestionGenerationConfig.Enabled {
			enableQuestionGeneration = true
			if kb.QuestionGenerationConfig.QuestionCount > 0 {
				questionCount = kb.QuestionGenerationConfig.QuestionCount
			}
		}

		lang, _ := types.LanguageFromContext(ctx)
		taskPayload := types.DocumentProcessPayload{
			TenantID:                 tenantID,
			KnowledgeID:              existing.ID,
			KnowledgeBaseID:          existing.KnowledgeBaseID,
			FilePath:                 existing.FilePath,
			FileName:                 existing.FileName,
			FileType:                 getFileType(existing.FileName),
			EnableMultimodel:         enableMultimodel,
			EnableQuestionGeneration: enableQuestionGeneration,
			QuestionCount:            questionCount,
			Language:                 lang,
		}

		payloadBytes, err := json.Marshal(taskPayload)
		if err != nil {
			logger.Errorf(ctx, "Failed to marshal reparse task payload: %v", err)
			return existing, nil
		}

		task := asynq.NewTask(types.TypeDocumentProcess, payloadBytes, asynq.Queue("default"), asynq.MaxRetry(3))
		info, err := s.task.Enqueue(task)
		if err != nil {
			logger.Errorf(ctx, "Failed to enqueue reparse task: %v", err)
			return existing, nil
		}
		logger.Infof(ctx, "Enqueued reparse task: id=%s queue=%s knowledge_id=%s", info.ID, info.Queue, existing.ID)

		// For data tables (csv, xlsx, xls), also enqueue summary task
		if slices.Contains([]string{"csv", "xlsx", "xls"}, getFileType(existing.FileName)) {
			NewDataTableSummaryTask(ctx, s.task, tenantID, existing.ID, kb.SummaryModelID, kb.EmbeddingModelID)
		}

		return existing, nil
	}

	// For file-URL-based knowledge, enqueue document processing task with FileURL field
	if existing.Type == "file_url" && existing.Source != "" {
		tenantID := ctx.Value(types.TenantIDContextKey).(uint64)

		enableMultimodel := kb.IsMultimodalEnabled()

		// Check question generation config
		enableQuestionGeneration := false
		questionCount := 3
		if kb.QuestionGenerationConfig != nil && kb.QuestionGenerationConfig.Enabled {
			enableQuestionGeneration = true
			if kb.QuestionGenerationConfig.QuestionCount > 0 {
				questionCount = kb.QuestionGenerationConfig.QuestionCount
			}
		}

		lang, _ := types.LanguageFromContext(ctx)
		taskPayload := types.DocumentProcessPayload{
			TenantID:                 tenantID,
			KnowledgeID:              existing.ID,
			KnowledgeBaseID:          existing.KnowledgeBaseID,
			FileURL:                  existing.Source,
			FileName:                 existing.FileName,
			FileType:                 existing.FileType,
			EnableMultimodel:         enableMultimodel,
			EnableQuestionGeneration: enableQuestionGeneration,
			QuestionCount:            questionCount,
			Language:                 lang,
		}

		payloadBytes, err := json.Marshal(taskPayload)
		if err != nil {
			logger.Errorf(ctx, "Failed to marshal file URL reparse task payload: %v", err)
			return existing, nil
		}

		task := asynq.NewTask(types.TypeDocumentProcess, payloadBytes, asynq.Queue("default"))
		info, err := s.task.Enqueue(task)
		if err != nil {
			logger.Errorf(ctx, "Failed to enqueue file URL reparse task: %v", err)
			return existing, nil
		}
		logger.Infof(ctx, "Enqueued file URL reparse task: id=%s queue=%s knowledge_id=%s", info.ID, info.Queue, existing.ID)

		return existing, nil
	}

	// For URL-based knowledge, enqueue URL processing task
	if existing.Type == "url" && existing.Source != "" {
		tenantID := ctx.Value(types.TenantIDContextKey).(uint64)

		enableMultimodel := kb.IsMultimodalEnabled()

		// Check question generation config
		enableQuestionGeneration := false
		questionCount := 3
		if kb.QuestionGenerationConfig != nil && kb.QuestionGenerationConfig.Enabled {
			enableQuestionGeneration = true
			if kb.QuestionGenerationConfig.QuestionCount > 0 {
				questionCount = kb.QuestionGenerationConfig.QuestionCount
			}
		}

		lang, _ := types.LanguageFromContext(ctx)
		taskPayload := types.DocumentProcessPayload{
			TenantID:                 tenantID,
			KnowledgeID:              existing.ID,
			KnowledgeBaseID:          existing.KnowledgeBaseID,
			URL:                      existing.Source,
			EnableMultimodel:         enableMultimodel,
			EnableQuestionGeneration: enableQuestionGeneration,
			QuestionCount:            questionCount,
			Language:                 lang,
		}

		payloadBytes, err := json.Marshal(taskPayload)
		if err != nil {
			logger.Errorf(ctx, "Failed to marshal URL reparse task payload: %v", err)
			return existing, nil
		}

		task := asynq.NewTask(types.TypeDocumentProcess, payloadBytes, asynq.Queue("default"), asynq.MaxRetry(3))
		info, err := s.task.Enqueue(task)
		if err != nil {
			logger.Errorf(ctx, "Failed to enqueue URL reparse task: %v", err)
			return existing, nil
		}
		logger.Infof(ctx, "Enqueued URL reparse task: id=%s queue=%s knowledge_id=%s", info.ID, info.Queue, existing.ID)

		return existing, nil
	}

	logger.Warnf(ctx, "Knowledge %s has no parseable content (no file, URL, or manual content)", knowledgeID)
	return existing, nil
}

// isValidFileType checks if a file type is supported

func isValidFileType(filename string) bool {
	switch strings.ToLower(getFileType(filename)) {
	case "pdf", "txt", "docx", "doc", "md", "markdown", "png", "jpg", "jpeg", "gif", "csv", "xlsx", "xls", "pptx", "ppt", "json",
		"mp3", "wav", "m4a", "flac", "ogg":
		return true
	default:
		return false
	}
}

// getFileType extracts the file extension from a filename
func getFileType(filename string) string {
	ext := strings.Split(filename, ".")
	if len(ext) < 2 {
		return "unknown"
	}
	return ext[len(ext)-1]
}

// isValidURL verifies if a URL is valid
// isValidURL 检查URL是否有效
func isValidURL(url string) bool {
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return true
	}
	return false
}

// GetKnowledgeBatch retrieves multiple knowledge entries by their IDs
func (s *knowledgeService) GetKnowledgeBatch(ctx context.Context,
	tenantID uint64, ids []string,
) ([]*types.Knowledge, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return s.repo.GetKnowledgeBatch(ctx, tenantID, ids)
}

// GetKnowledgeBatchWithSharedAccess retrieves knowledge by IDs, including items from shared KBs the user has access to.
// Used when building search targets so that @mentioned files from shared KBs are included.
func (s *knowledgeService) GetKnowledgeBatchWithSharedAccess(ctx context.Context,
	tenantID uint64, ids []string,
) ([]*types.Knowledge, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	ownList, err := s.repo.GetKnowledgeBatch(ctx, tenantID, ids)
	if err != nil {
		return nil, err
	}
	foundSet := make(map[string]bool)
	for _, k := range ownList {
		if k != nil {
			foundSet[k.ID] = true
		}
	}
	userIDVal := ctx.Value(types.UserIDContextKey)
	if userIDVal == nil {
		return ownList, nil
	}
	userID, ok := userIDVal.(string)
	if !ok || userID == "" {
		return ownList, nil
	}
	for _, id := range ids {
		if foundSet[id] {
			continue
		}
		k, err := s.repo.GetKnowledgeByIDOnly(ctx, id)
		if err != nil || k == nil || k.KnowledgeBaseID == "" {
			continue
		}
		hasPermission, err := s.kbShareService.HasKBPermission(ctx, k.KnowledgeBaseID, userID, types.OrgRoleViewer)
		if err != nil || !hasPermission {
			continue
		}
		foundSet[k.ID] = true
		ownList = append(ownList, k)
	}
	return ownList, nil
}

// calculateFileHash calculates MD5 hash of a file
func calculateFileHash(file *multipart.FileHeader) (string, error) {
	f, err := file.Open()
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	// Reset file pointer for subsequent operations
	if _, err := f.Seek(0, 0); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func calculateStr(strList ...string) string {
	h := md5.New()
	input := strings.Join(strList, "")
	h.Write([]byte(input))
	return hex.EncodeToString(h.Sum(nil))
}

// SearchKnowledge searches knowledge items by keyword across the tenant and shared knowledge bases.
// fileTypes: optional list of file extensions to filter by (e.g., ["csv", "xlsx"])
func (s *knowledgeService) SearchKnowledge(ctx context.Context, keyword string, offset, limit int, fileTypes []string) ([]*types.Knowledge, bool, error) {
	tenantID, ok := ctx.Value(types.TenantIDContextKey).(uint64)
	if !ok {
		return nil, false, werrors.NewUnauthorizedError("Tenant ID not found in context")
	}

	scopes := make([]types.KnowledgeSearchScope, 0)

	// Own tenant: document-type knowledge bases
	ownKBs, err := s.kbService.ListKnowledgeBases(ctx)
	if err == nil {
		for _, kb := range ownKBs {
			if kb != nil && kb.Type == types.KnowledgeBaseTypeDocument {
				scopes = append(scopes, types.KnowledgeSearchScope{TenantID: tenantID, KBID: kb.ID})
			}
		}
	}

	// Shared knowledge bases (document type only)
	if userIDVal := ctx.Value(types.UserIDContextKey); userIDVal != nil {
		if userID, ok := userIDVal.(string); ok && userID != "" {
			sharedList, err := s.kbShareService.ListSharedKnowledgeBases(ctx, userID, tenantID)
			if err == nil {
				for _, info := range sharedList {
					if info != nil && info.KnowledgeBase != nil && info.KnowledgeBase.Type == types.KnowledgeBaseTypeDocument {
						scopes = append(scopes, types.KnowledgeSearchScope{
							TenantID: info.SourceTenantID,
							KBID:     info.KnowledgeBase.ID,
						})
					}
				}
			}
		}
	}

	if len(scopes) == 0 {
		return nil, false, nil
	}
	return s.repo.SearchKnowledgeInScopes(ctx, scopes, keyword, offset, limit, fileTypes)
}

// SearchKnowledgeForScopes searches knowledge within the given scopes (e.g. for shared agent context).
func (s *knowledgeService) SearchKnowledgeForScopes(ctx context.Context, scopes []types.KnowledgeSearchScope, keyword string, offset, limit int, fileTypes []string) ([]*types.Knowledge, bool, error) {
	if len(scopes) == 0 {
		return nil, false, nil
	}
	return s.repo.SearchKnowledgeInScopes(ctx, scopes, keyword, offset, limit, fileTypes)
}

// ProcessKnowledgeListDelete handles Asynq knowledge list delete tasks
