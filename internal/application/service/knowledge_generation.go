package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/application/service/retriever"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/tracing"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// defaultMaxInputChars is the default maximum characters used as input for summary generation.
const defaultMaxInputChars = 16384

// getSummary generates a summary for knowledge content using an AI model
func (s *knowledgeService) getSummary(ctx context.Context,
	summaryModel chat.Chat, knowledge *types.Knowledge, chunks []*types.Chunk,
) (string, error) {
	// Get knowledge info from the first chunk
	if len(chunks) == 0 {
		return "", fmt.Errorf("no chunks provided for summary generation")
	}

	// Determine max input chars from config
	maxInputChars := defaultMaxInputChars
	if s.config.Conversation.Summary != nil && s.config.Conversation.Summary.MaxInputChars > 0 {
		maxInputChars = s.config.Conversation.Summary.MaxInputChars
	}

	// concat chunk contents
	chunkContents := ""
	allImageInfos := make([]*types.ImageInfo, 0)

	// then, sort chunks by StartAt
	sortedChunks := make([]*types.Chunk, len(chunks))
	copy(sortedChunks, chunks)
	sort.Slice(sortedChunks, func(i, j int) bool {
		return sortedChunks[i].StartAt < sortedChunks[j].StartAt
	})

	// concat ALL chunk contents (no early truncation) and collect image infos
	for _, chunk := range sortedChunks {
		// Ensure we don't slice beyond the current content length
		runes := []rune(chunkContents)
		if chunk.StartAt <= len(runes) {
			chunkContents = string(runes[:chunk.StartAt]) + chunk.Content
		} else {
			// If StartAt is beyond current content, just append
			chunkContents = chunkContents + chunk.Content
		}
		if chunk.ImageInfo != "" {
			var images []*types.ImageInfo
			if err := json.Unmarshal([]byte(chunk.ImageInfo), &images); err == nil {
				allImageInfos = append(allImageInfos, images...)
			}
		}
	}
	// remove markdown image syntax
	re := regexp.MustCompile(`!\[[^\]]*\]\([^)]+\)`)
	chunkContents = re.ReplaceAllString(chunkContents, "")
	// collect all image infos
	if len(allImageInfos) > 0 {
		// add image infos to chunk contents
		var imageAnnotations string
		for _, img := range allImageInfos {
			if img.Caption != "" {
				imageAnnotations += fmt.Sprintf("\n[Image Description: %s]", img.Caption)
			}
			if img.OCRText != "" {
				imageAnnotations += fmt.Sprintf("\n[Image OCR Text: %s]", img.OCRText)
			}
		}

		// concat chunk contents and image annotations
		chunkContents = chunkContents + imageAnnotations
	}

	// Apply length limit: sample long content to fit within maxInputChars
	chunkContents = sampleLongContent(chunkContents, maxInputChars)

	logger.GetLogger(ctx).Infof("getSummary: content length=%d chars (max=%d) for knowledge %s",
		len([]rune(chunkContents)), maxInputChars, knowledge.ID)

	// Prepare content with metadata for summary generation
	contentWithMetadata := chunkContents

	// Add knowledge metadata if available
	if knowledge != nil {
		metadataIntro := fmt.Sprintf("Document Type: %s\nFile Name: %s\n", knowledge.FileType, knowledge.FileName)

		// Add additional metadata if available
		if knowledge.Type != "" {
			metadataIntro += fmt.Sprintf("Knowledge Type: %s\n", knowledge.Type)
		}

		// Prepend metadata to content
		contentWithMetadata = metadataIntro + "\nContent:\n" + contentWithMetadata
	}

	// Determine max output tokens from config
	maxTokens := 2048
	if s.config.Conversation.Summary != nil && s.config.Conversation.Summary.MaxCompletionTokens > 0 {
		maxTokens = s.config.Conversation.Summary.MaxCompletionTokens
	}

	// Generate summary using AI model
	summaryPrompt := types.RenderPromptPlaceholders(s.config.Conversation.GenerateSummaryPrompt, types.PlaceholderValues{
		"language": types.LanguageNameFromContext(ctx),
	})
	thinking := false
	summary, err := summaryModel.Chat(ctx, []chat.Message{
		{
			Role:    "system",
			Content: summaryPrompt,
		},
		{
			Role:    "user",
			Content: contentWithMetadata,
		},
	}, &chat.ChatOptions{
		Temperature: 0.3,
		MaxTokens:   maxTokens,
		Thinking:    &thinking,
	})
	if err != nil {
		logger.GetLogger(ctx).WithField("error", err).Errorf("GetSummary failed")
		return "", err
	}
	logger.GetLogger(ctx).WithField("summary", summary.Content).Infof("GetSummary success")
	return summary.Content, nil
}

// sampleLongContent returns content that fits within maxChars.
// For short content (≤ maxChars), it is returned as-is.
// For long content, it samples: head (60%), tail (20%), and evenly-spaced middle (20%),
// joined by "[...content omitted...]" markers so the LLM knows content was skipped.
func sampleLongContent(content string, maxChars int) string {
	runes := []rune(content)
	if len(runes) <= maxChars {
		return content
	}

	const omitMarker = "\n\n[...content omitted...]\n\n"
	omitRunes := len([]rune(omitMarker))

	// Reserve space for two omit markers (head→middle, middle→tail)
	usable := maxChars - 2*omitRunes
	if usable < 100 {
		// Fallback: just truncate
		return string(runes[:maxChars])
	}

	headLen := usable * 60 / 100
	tailLen := usable * 20 / 100
	midLen := usable - headLen - tailLen

	head := string(runes[:headLen])
	tail := string(runes[len(runes)-tailLen:])

	// Sample middle portion: take a contiguous block from the center of the document
	midStart := len(runes)/2 - midLen/2
	if midStart < headLen {
		midStart = headLen
	}
	midEnd := midStart + midLen
	if midEnd > len(runes)-tailLen {
		midEnd = len(runes) - tailLen
		midStart = midEnd - midLen
		if midStart < headLen {
			midStart = headLen
		}
	}
	middle := string(runes[midStart:midEnd])

	return head + omitMarker + middle + omitMarker + tail
}

// enqueueQuestionGenerationTask enqueues an async task for question generation
func (s *knowledgeService) enqueueQuestionGenerationTask(ctx context.Context,
	kbID, knowledgeID string, questionCount int,
) {
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	lang, _ := types.LanguageFromContext(ctx)
	payload := types.QuestionGenerationPayload{
		TenantID:        tenantID,
		KnowledgeBaseID: kbID,
		KnowledgeID:     knowledgeID,
		QuestionCount:   questionCount,
		Language:        lang,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		logger.Errorf(ctx, "Failed to marshal question generation payload: %v", err)
		return
	}

	task := asynq.NewTask(types.TypeQuestionGeneration, payloadBytes, asynq.Queue("low"), asynq.MaxRetry(3))
	info, err := s.task.Enqueue(task)
	if err != nil {
		logger.Errorf(ctx, "Failed to enqueue question generation task: %v", err)
		return
	}
	logger.Infof(ctx, "Enqueued question generation task: %s for knowledge: %s", info.ID, knowledgeID)
}

// enqueueSummaryGenerationTask enqueues an async task for summary generation
func (s *knowledgeService) enqueueSummaryGenerationTask(ctx context.Context,
	kbID, knowledgeID string,
) {
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	lang, _ := types.LanguageFromContext(ctx)
	payload := types.SummaryGenerationPayload{
		TenantID:        tenantID,
		KnowledgeBaseID: kbID,
		KnowledgeID:     knowledgeID,
		Language:        lang,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		logger.Errorf(ctx, "Failed to marshal summary generation payload: %v", err)
		return
	}

	task := asynq.NewTask(types.TypeSummaryGeneration, payloadBytes, asynq.Queue("low"), asynq.MaxRetry(3))
	info, err := s.task.Enqueue(task)
	if err != nil {
		logger.Errorf(ctx, "Failed to enqueue summary generation task: %v", err)
		return
	}
	logger.Infof(ctx, "Enqueued summary generation task: %s for knowledge: %s", info.ID, knowledgeID)
}

// ProcessSummaryGeneration handles async summary generation task
func (s *knowledgeService) ProcessSummaryGeneration(ctx context.Context, t *asynq.Task) error {
	var payload types.SummaryGenerationPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		logger.Errorf(ctx, "Failed to unmarshal summary generation payload: %v", err)
		return nil // Don't retry on unmarshal error
	}

	logger.Infof(ctx, "Processing summary generation for knowledge: %s", payload.KnowledgeID)

	// Set tenant and language context
	ctx = context.WithValue(ctx, types.TenantIDContextKey, payload.TenantID)
	if payload.Language != "" {
		ctx = context.WithValue(ctx, types.LanguageContextKey, payload.Language)
	}

	// Get knowledge base
	kb, err := s.kbService.GetKnowledgeBaseByID(ctx, payload.KnowledgeBaseID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get knowledge base: %v", err)
		return nil
	}

	if kb.SummaryModelID == "" {
		logger.Warn(ctx, "Knowledge base summary model ID is empty, skipping summary generation")
		return nil
	}

	// Get knowledge
	knowledge, err := s.repo.GetKnowledgeByID(ctx, payload.TenantID, payload.KnowledgeID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get knowledge: %v", err)
		return nil
	}

	// Update summary status to processing
	knowledge.SummaryStatus = types.SummaryStatusProcessing
	knowledge.UpdatedAt = time.Now()
	if err := s.repo.UpdateKnowledge(ctx, knowledge); err != nil {
		logger.Warnf(ctx, "Failed to update summary status to processing: %v", err)
	}

	// Helper function to mark summary as failed
	markSummaryFailed := func() {
		knowledge.SummaryStatus = types.SummaryStatusFailed
		knowledge.UpdatedAt = time.Now()
		if err := s.repo.UpdateKnowledge(ctx, knowledge); err != nil {
			logger.Warnf(ctx, "Failed to update summary status to failed: %v", err)
		}
	}

	// Get text chunks for this knowledge
	chunks, err := s.chunkService.ListChunksByKnowledgeID(ctx, payload.KnowledgeID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get chunks: %v", err)
		markSummaryFailed()
		return nil
	}

	// Filter text chunks only
	textChunks := make([]*types.Chunk, 0)
	for _, chunk := range chunks {
		if chunk.ChunkType == types.ChunkTypeText {
			textChunks = append(textChunks, chunk)
		}
	}

	if len(textChunks) == 0 {
		logger.Infof(ctx, "No text chunks found for knowledge: %s", payload.KnowledgeID)
		// Mark as completed since there's nothing to summarize
		knowledge.SummaryStatus = types.SummaryStatusCompleted
		knowledge.UpdatedAt = time.Now()
		s.repo.UpdateKnowledge(ctx, knowledge)
		return nil
	}

	// Sort chunks by ChunkIndex for proper ordering
	sort.Slice(textChunks, func(i, j int) bool {
		return textChunks[i].ChunkIndex < textChunks[j].ChunkIndex
	})

	// Initialize chat model for summary
	chatModel, err := s.modelService.GetChatModel(ctx, kb.SummaryModelID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get chat model: %v", err)
		markSummaryFailed()
		return fmt.Errorf("failed to get chat model: %w", err)
	}

	// Generate summary
	summary, err := s.getSummary(ctx, chatModel, knowledge, textChunks)
	if err != nil {
		logger.Errorf(ctx, "Failed to generate summary for knowledge %s: %v", payload.KnowledgeID, err)
		// Use first chunk content as fallback
		if len(textChunks) > 0 {
			summary = textChunks[0].Content
			if len(summary) > 500 {
				// Use rune-based truncation to avoid cutting UTF-8 multi-byte characters
				runes := []rune(summary)
				if len(runes) > 500 {
					summary = string(runes[:500])
				}
			}
		}
	}

	// Update knowledge description
	knowledge.Description = summary
	knowledge.SummaryStatus = types.SummaryStatusCompleted
	knowledge.UpdatedAt = time.Now()
	if err := s.repo.UpdateKnowledge(ctx, knowledge); err != nil {
		logger.Errorf(ctx, "Failed to update knowledge description: %v", err)
		return fmt.Errorf("failed to update knowledge: %w", err)
	}

	// Create summary chunk and index it
	if strings.TrimSpace(summary) != "" {
		// Get max chunk index
		maxChunkIndex := 0
		for _, chunk := range chunks {
			if chunk.ChunkIndex > maxChunkIndex {
				maxChunkIndex = chunk.ChunkIndex
			}
		}

		summaryChunk := &types.Chunk{
			ID:              uuid.New().String(),
			TenantID:        knowledge.TenantID,
			KnowledgeID:     knowledge.ID,
			KnowledgeBaseID: knowledge.KnowledgeBaseID,
			Content:         fmt.Sprintf("# Document\n%s\n\n# Summary\n%s", knowledge.FileName, summary),
			ChunkIndex:      maxChunkIndex + 1,
			IsEnabled:       true,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
			StartAt:         0,
			EndAt:           0,
			ChunkType:       types.ChunkTypeSummary,
			ParentChunkID:   textChunks[0].ID,
		}

		// Save summary chunk
		if err := s.chunkService.CreateChunks(ctx, []*types.Chunk{summaryChunk}); err != nil {
			logger.Errorf(ctx, "Failed to create summary chunk: %v", err)
			return fmt.Errorf("failed to create summary chunk: %w", err)
		}

		// Index summary chunk
		tenantInfo, err := s.tenantRepo.GetTenantByID(ctx, payload.TenantID)
		if err != nil {
			logger.Errorf(ctx, "Failed to get tenant info: %v", err)
			return fmt.Errorf("failed to get tenant info: %w", err)
		}
		ctx = context.WithValue(ctx, types.TenantInfoContextKey, tenantInfo)

		retrieveEngine, err := retriever.NewCompositeRetrieveEngine(s.retrieveEngine, tenantInfo.GetEffectiveEngines())
		if err != nil {
			logger.Errorf(ctx, "Failed to init retrieve engine: %v", err)
			return fmt.Errorf("failed to init retrieve engine: %w", err)
		}

		embeddingModel, err := s.modelService.GetEmbeddingModel(ctx, kb.EmbeddingModelID)
		if err != nil {
			logger.Errorf(ctx, "Failed to get embedding model: %v", err)
			return fmt.Errorf("failed to get embedding model: %w", err)
		}

		indexInfo := []*types.IndexInfo{{
			Content:         summaryChunk.Content,
			SourceID:        summaryChunk.ID,
			SourceType:      types.ChunkSourceType,
			ChunkID:         summaryChunk.ID,
			KnowledgeID:     knowledge.ID,
			KnowledgeBaseID: knowledge.KnowledgeBaseID,
			IsEnabled:       true,
		}}

		if err := retrieveEngine.BatchIndex(ctx, embeddingModel, indexInfo); err != nil {
			logger.Errorf(ctx, "Failed to index summary chunk: %v", err)
			return fmt.Errorf("failed to index summary chunk: %w", err)
		}

		logger.Infof(ctx, "Successfully created and indexed summary chunk for knowledge: %s", payload.KnowledgeID)
	}

	logger.Infof(ctx, "Successfully generated summary for knowledge: %s", payload.KnowledgeID)
	return nil
}

// ProcessQuestionGeneration handles async question generation task
func (s *knowledgeService) ProcessQuestionGeneration(ctx context.Context, t *asynq.Task) error {
	ctx, span := tracing.ContextWithSpan(ctx, "knowledgeService.ProcessQuestionGeneration")
	defer span.End()

	var payload types.QuestionGenerationPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		logger.Errorf(ctx, "Failed to unmarshal question generation payload: %v", err)
		return nil // Don't retry on unmarshal error
	}

	logger.Infof(ctx, "Processing question generation for knowledge: %s", payload.KnowledgeID)

	// Set tenant context
	ctx = context.WithValue(ctx, types.TenantIDContextKey, payload.TenantID)
	if payload.Language != "" {
		ctx = context.WithValue(ctx, types.LanguageContextKey, payload.Language)
	}

	if strings.TrimSpace(s.config.Conversation.GenerateQuestionsPrompt) == "" {
		logger.Errorf(ctx, "GenerateQuestionsPrompt is empty: configure conversation.generate_questions_prompt_id")
		return fmt.Errorf("generate questions prompt not configured")
	}

	// Get knowledge base
	kb, err := s.kbService.GetKnowledgeBaseByID(ctx, payload.KnowledgeBaseID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get knowledge base: %v", err)
		return nil
	}

	// Get knowledge
	knowledge, err := s.repo.GetKnowledgeByID(ctx, payload.TenantID, payload.KnowledgeID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get knowledge: %v", err)
		return nil
	}

	// Get text chunks for this knowledge
	chunks, err := s.chunkService.ListChunksByKnowledgeID(ctx, payload.KnowledgeID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get chunks: %v", err)
		return nil
	}

	// Filter text chunks only
	textChunks := make([]*types.Chunk, 0)
	for _, chunk := range chunks {
		if chunk.ChunkType == types.ChunkTypeText {
			textChunks = append(textChunks, chunk)
		}
	}

	if len(textChunks) == 0 {
		logger.Infof(ctx, "No text chunks found for knowledge: %s", payload.KnowledgeID)
		return nil
	}

	// Sort chunks by StartAt for context building
	sort.Slice(textChunks, func(i, j int) bool {
		return textChunks[i].StartAt < textChunks[j].StartAt
	})

	// Initialize chat model
	chatModel, err := s.modelService.GetChatModel(ctx, kb.SummaryModelID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get chat model: %v", err)
		return fmt.Errorf("failed to get chat model: %w", err)
	}

	// Initialize embedding model and retrieval engine
	embeddingModel, err := s.modelService.GetEmbeddingModel(ctx, kb.EmbeddingModelID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get embedding model: %v", err)
		return fmt.Errorf("failed to get embedding model: %w", err)
	}

	tenantInfo, err := s.tenantRepo.GetTenantByID(ctx, payload.TenantID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get tenant info: %v", err)
		return fmt.Errorf("failed to get tenant info: %w", err)
	}
	ctx = context.WithValue(ctx, types.TenantInfoContextKey, tenantInfo)

	retrieveEngine, err := retriever.NewCompositeRetrieveEngine(s.retrieveEngine, tenantInfo.GetEffectiveEngines())
	if err != nil {
		logger.Errorf(ctx, "Failed to init retrieve engine: %v", err)
		return fmt.Errorf("failed to init retrieve engine: %w", err)
	}

	questionCount := payload.QuestionCount
	if questionCount <= 0 {
		questionCount = 3
	}
	if questionCount > 10 {
		questionCount = 10
	}

	// Generate questions for each chunk with context
	var indexInfoList []*types.IndexInfo
	for i, chunk := range textChunks {
		// Build context from adjacent chunks
		var prevContent, nextContent string
		if i > 0 {
			prevContent = textChunks[i-1].Content
			// Limit context size
			if len(prevContent) > 500 {
				prevContent = prevContent[len(prevContent)-500:]
			}
		}
		if i < len(textChunks)-1 {
			nextContent = textChunks[i+1].Content
			// Limit context size
			if len(nextContent) > 500 {
				nextContent = nextContent[:500]
			}
		}

		questions, err := s.generateQuestionsWithContext(ctx, chatModel, chunk.Content, prevContent, nextContent, knowledge.Title, questionCount)
		if err != nil {
			logger.Warnf(ctx, "Failed to generate questions for chunk %s: %v", chunk.ID, err)
			continue
		}

		if len(questions) == 0 {
			continue
		}

		// Update chunk metadata with unique IDs for each question
		generatedQuestions := make([]types.GeneratedQuestion, len(questions))
		for j, question := range questions {
			questionID := fmt.Sprintf("q%d", time.Now().UnixNano()+int64(j))
			generatedQuestions[j] = types.GeneratedQuestion{
				ID:       questionID,
				Question: question,
			}
		}
		meta := &types.DocumentChunkMetadata{
			GeneratedQuestions: generatedQuestions,
		}
		if err := chunk.SetDocumentMetadata(meta); err != nil {
			logger.Warnf(ctx, "Failed to set document metadata for chunk %s: %v", chunk.ID, err)
			continue
		}

		// Update chunk in database
		if err := s.chunkService.UpdateChunk(ctx, chunk); err != nil {
			logger.Warnf(ctx, "Failed to update chunk %s: %v", chunk.ID, err)
			continue
		}

		// Create index entries for generated questions
		for _, gq := range generatedQuestions {
			sourceID := fmt.Sprintf("%s-%s", chunk.ID, gq.ID)
			indexInfoList = append(indexInfoList, &types.IndexInfo{
				Content:         gq.Question,
				SourceID:        sourceID,
				SourceType:      types.ChunkSourceType,
				ChunkID:         chunk.ID,
				KnowledgeID:     knowledge.ID,
				KnowledgeBaseID: knowledge.KnowledgeBaseID,
				IsEnabled:       true,
			})
		}
		logger.Debugf(ctx, "Generated %d questions for chunk %s", len(questions), chunk.ID)
	}

	// Index generated questions
	if len(indexInfoList) > 0 {
		if err := retrieveEngine.BatchIndex(ctx, embeddingModel, indexInfoList); err != nil {
			logger.Errorf(ctx, "Failed to index generated questions: %v", err)
			return fmt.Errorf("failed to index questions: %w", err)
		}
		logger.Infof(ctx, "Successfully indexed %d generated questions for knowledge: %s", len(indexInfoList), payload.KnowledgeID)
	}

	return nil
}

// generateQuestionsWithContext generates questions for a chunk with surrounding context
func (s *knowledgeService) generateQuestionsWithContext(ctx context.Context,
	chatModel chat.Chat, content, prevContent, nextContent, docName string, questionCount int,
) ([]string, error) {
	if content == "" || questionCount <= 0 {
		return nil, nil
	}

	prompt := strings.TrimSpace(s.config.Conversation.GenerateQuestionsPrompt)
	if prompt == "" {
		return nil, fmt.Errorf("generate questions prompt not configured")
	}

	// Build context section
	var contextSection string
	if prevContent != "" || nextContent != "" {
		contextSection = "<surrounding_context>\n"
		if prevContent != "" {
			contextSection += fmt.Sprintf("[Preceding Content]\n%s\n\n", prevContent)
		}
		if nextContent != "" {
			contextSection += fmt.Sprintf("[Following Content]\n%s\n\n", nextContent)
		}
		contextSection += "</surrounding_context>\n\n"
	}

	langName := types.LanguageNameFromContext(ctx)
	prompt = types.RenderPromptPlaceholders(prompt, types.PlaceholderValues{
		"question_count": fmt.Sprintf("%d", questionCount),
		"content":        content,
		"context":        contextSection,
		"doc_name":       docName,
		"language":       langName,
	})

	thinking := false
	response, err := chatModel.Chat(ctx, []chat.Message{
		{
			Role:    "user",
			Content: prompt,
		},
	}, &chat.ChatOptions{
		Temperature: 0.7,
		MaxTokens:   512,
		Thinking:    &thinking,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate questions: %w", err)
	}

	// Parse response
	lines := strings.Split(response.Content, "\n")
	questions := make([]string, 0, questionCount)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimLeft(line, "0123456789.-*) ")
		line = strings.TrimSpace(line)
		if line != "" && len(line) > 5 {
			questions = append(questions, line)
			if len(questions) >= questionCount {
				break
			}
		}
	}

	return questions, nil
}
