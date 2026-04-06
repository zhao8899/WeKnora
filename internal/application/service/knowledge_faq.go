package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/application/service/retriever"
	werrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/embedding"
	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

// ListFAQEntries lists FAQ entries under a FAQ knowledge base.
func (s *knowledgeService) ListFAQEntries(ctx context.Context,
	kbID string, page *types.Pagination, tagSeqID int64, keyword string, searchField string, sortOrder string,
) (*types.PageResult, error) {
	if page == nil {
		page = &types.Pagination{}
	}
	keyword = strings.TrimSpace(keyword)
	kb, err := s.validateFAQKnowledgeBase(ctx, kbID)
	if err != nil {
		return nil, err
	}

	// Check if this is a shared knowledge base access
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	effectiveTenantID := tenantID

	// If the kb belongs to a different tenant, check for shared access
	if kb.TenantID != tenantID {
		// Get user ID from context
		userIDVal := ctx.Value(types.UserIDContextKey)
		if userIDVal == nil {
			return nil, werrors.NewForbiddenError("无权访问该知识库")
		}
		userID := userIDVal.(string)

		// Check if user has at least viewer permission through organization sharing
		hasPermission, err := s.kbShareService.HasKBPermission(ctx, kbID, userID, types.OrgRoleViewer)
		if err != nil || !hasPermission {
			return nil, werrors.NewForbiddenError("无权访问该知识库")
		}

		// Use the source tenant ID for data access
		sourceTenantID, err := s.kbShareService.GetKBSourceTenant(ctx, kbID)
		if err != nil {
			return nil, werrors.NewForbiddenError("无权访问该知识库")
		}
		effectiveTenantID = sourceTenantID
	}

	faqKnowledge, err := s.findFAQKnowledge(ctx, effectiveTenantID, kb.ID)
	if err != nil {
		return nil, err
	}
	if faqKnowledge == nil {
		return types.NewPageResult(0, page, []*types.FAQEntry{}), nil
	}

	// Convert tagSeqID to tagID (UUID)
	var tagID string
	if tagSeqID > 0 {
		tag, err := s.tagRepo.GetBySeqID(ctx, effectiveTenantID, tagSeqID)
		if err != nil {
			return nil, werrors.NewNotFoundError("标签不存在")
		}
		tagID = tag.ID
	}

	chunkType := []types.ChunkType{types.ChunkTypeFAQ}
	chunks, total, err := s.chunkRepo.ListPagedChunksByKnowledgeID(
		ctx, effectiveTenantID, faqKnowledge.ID, page, chunkType, tagID, keyword, searchField, sortOrder, types.KnowledgeTypeFAQ,
	)
	if err != nil {
		return nil, err
	}

	// Build tag ID to name and seq_id mapping for all unique tag IDs (batch query)
	tagNameMap := make(map[string]string)
	tagSeqIDMap := make(map[string]int64)
	tagIDs := make([]string, 0)
	tagIDSet := make(map[string]struct{})
	for _, chunk := range chunks {
		if chunk.TagID != "" {
			if _, exists := tagIDSet[chunk.TagID]; !exists {
				tagIDSet[chunk.TagID] = struct{}{}
				tagIDs = append(tagIDs, chunk.TagID)
			}
		}
	}
	if len(tagIDs) > 0 {
		tags, err := s.tagRepo.GetByIDs(ctx, effectiveTenantID, tagIDs)
		if err == nil {
			for _, tag := range tags {
				tagNameMap[tag.ID] = tag.Name
				tagSeqIDMap[tag.ID] = tag.SeqID
			}
		}
	}

	kb.EnsureDefaults()
	entries := make([]*types.FAQEntry, 0, len(chunks))
	for _, chunk := range chunks {
		entry, err := s.chunkToFAQEntry(chunk, kb, tagSeqIDMap)
		if err != nil {
			return nil, err
		}
		// Set tag name from mapping
		if chunk.TagID != "" {
			entry.TagName = tagNameMap[chunk.TagID]
		}
		entries = append(entries, entry)
	}
	return types.NewPageResult(total, page, entries), nil
}

// UpsertFAQEntries imports or appends FAQ entries asynchronously.
// Returns task ID (UUID) for tracking import progress.
func (s *knowledgeService) UpsertFAQEntries(ctx context.Context,
	kbID string, payload *types.FAQBatchUpsertPayload,
) (string, error) {
	if payload == nil || len(payload.Entries) == 0 {
		return "", werrors.NewBadRequestError("FAQ 条目不能为空")
	}
	if payload.Mode == "" {
		payload.Mode = types.FAQBatchModeAppend
	}
	if payload.Mode != types.FAQBatchModeAppend && payload.Mode != types.FAQBatchModeReplace {
		return "", werrors.NewBadRequestError("模式仅支持 append 或 replace")
	}

	// 验证知识库是否存在且有效
	kb, err := s.validateFAQKnowledgeBase(ctx, kbID)
	if err != nil {
		return "", err
	}

	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)

	// 使用传入的TaskID，如果没传则生成增强的TaskID
	taskID := payload.TaskID
	if taskID == "" {
		taskID = secutils.GenerateTaskID("faq_import", tenantID, kbID)
	}

	var knowledgeID string

	// 检查是否有正在进行的导入任务（通过Redis）
	runningTaskID, err := s.getRunningFAQImportTaskID(ctx, kbID)
	if err != nil {
		logger.Errorf(ctx, "Failed to check running import task: %v", err)
		// 检查失败不影响导入，继续执行
	} else if runningTaskID != "" {
		logger.Warnf(ctx, "Import task already running for KB %s: %s", kbID, runningTaskID)
		return "", werrors.NewBadRequestError(fmt.Sprintf("该知识库已有导入任务正在进行中（任务ID: %s），请等待完成后再试", runningTaskID))
	}

	// 确保 FAQ knowledge 存在
	faqKnowledge, err := s.ensureFAQKnowledge(ctx, tenantID, kb)
	if err != nil {
		return "", fmt.Errorf("failed to ensure FAQ knowledge: %w", err)
	}
	knowledgeID = faqKnowledge.ID

	// 记录任务入队时间
	enqueuedAt := time.Now().Unix()

	// 设置 KB 的运行中任务信息
	if err := s.setRunningFAQImportInfo(ctx, kbID, &runningFAQImportInfo{
		TaskID:     taskID,
		EnqueuedAt: enqueuedAt,
	}); err != nil {
		logger.Errorf(ctx, "Failed to set running FAQ import task info: %v", err)
		// 不影响任务执行，继续
	}

	// 初始化导入任务状态到Redis
	progress := &types.FAQImportProgress{
		TaskID:        taskID,
		KBID:          kbID,
		KnowledgeID:   knowledgeID,
		Status:        types.FAQImportStatusPending,
		Progress:      0,
		Total:         len(payload.Entries),
		Processed:     0,
		SuccessCount:  0,
		FailedCount:   0,
		FailedEntries: make([]types.FAQFailedEntry, 0),
		Message:       "任务已创建，等待处理",
		CreatedAt:     time.Now().Unix(),
		UpdatedAt:     time.Now().Unix(),
		DryRun:        payload.DryRun,
	}
	if err := s.saveFAQImportProgress(ctx, progress); err != nil {
		logger.Errorf(ctx, "Failed to initialize FAQ import task status: %v", err)
		return "", fmt.Errorf("failed to initialize task: %w", err)
	}

	logger.Infof(ctx, "FAQ import task initialized: %s, kb_id: %s, total entries: %d, dry_run: %v",
		taskID, kbID, len(payload.Entries), payload.DryRun)

	// Enqueue FAQ import task to Asynq
	logger.Info(ctx, "Enqueuing FAQ import task to Asynq")

	// 构建任务 payload
	taskPayload := types.FAQImportPayload{
		TenantID:    tenantID,
		TaskID:      taskID,
		KBID:        kbID,
		KnowledgeID: knowledgeID,
		Mode:        payload.Mode,
		DryRun:      payload.DryRun,
		EnqueuedAt:  enqueuedAt,
	}

	// 阈值：超过 200 条或序列化后超过 50KB 时使用对象存储
	const (
		entryCountThreshold  = 200
		payloadSizeThreshold = 50 * 1024 // 50KB
	)

	entryCount := len(payload.Entries)
	if entryCount > entryCountThreshold {
		// 数据量较大，上传到对象存储
		entriesData, err := json.Marshal(payload.Entries)
		if err != nil {
			logger.Errorf(ctx, "Failed to marshal FAQ entries: %v", err)
			return "", fmt.Errorf("failed to marshal entries: %w", err)
		}

		logger.Infof(ctx, "FAQ entries size: %d bytes, uploading to object storage", len(entriesData))

		// 上传到私有桶（主桶），任务处理完成后清理
		fileName := fmt.Sprintf("faq_import_entries_%s_%d.json", taskID, enqueuedAt)
		entriesURL, err := s.fileSvc.SaveBytes(ctx, entriesData, tenantID, fileName, false)
		if err != nil {
			logger.Errorf(ctx, "Failed to upload FAQ entries to object storage: %v", err)
			return "", fmt.Errorf("failed to upload entries: %w", err)
		}

		logger.Infof(ctx, "FAQ entries uploaded to: %s", entriesURL)
		taskPayload.EntriesURL = entriesURL
		taskPayload.EntryCount = entryCount
	} else {
		// 数据量较小，直接存储在 payload 中
		taskPayload.Entries = payload.Entries
	}

	payloadBytes, err := json.Marshal(taskPayload)
	if err != nil {
		logger.Errorf(ctx, "Failed to marshal FAQ import task payload: %v", err)
		return "", fmt.Errorf("failed to marshal task payload: %w", err)
	}

	// 再次检查 payload 大小
	if len(payloadBytes) > payloadSizeThreshold && taskPayload.EntriesURL == "" {
		// payload 太大但还没上传，现在上传
		entriesData, _ := json.Marshal(payload.Entries)
		fileName := fmt.Sprintf("faq_import_entries_%s_%d.json", taskID, enqueuedAt)
		entriesURL, err := s.fileSvc.SaveBytes(ctx, entriesData, tenantID, fileName, false)
		if err != nil {
			logger.Errorf(ctx, "Failed to upload FAQ entries to object storage: %v", err)
			return "", fmt.Errorf("failed to upload entries: %w", err)
		}

		logger.Infof(ctx, "FAQ entries uploaded to (size exceeded): %s", entriesURL)
		taskPayload.Entries = nil
		taskPayload.EntriesURL = entriesURL
		taskPayload.EntryCount = entryCount

		payloadBytes, _ = json.Marshal(taskPayload)
	}

	logger.Infof(ctx, "FAQ import task payload size: %d bytes", len(payloadBytes))

	maxRetry := 5
	if payload.DryRun {
		maxRetry = 3 // dry run 重试次数少一些
	}

	// 使用 taskID:enqueuedAt 作为 asynq 的唯一任务标识
	// 这样同一个用户 TaskID 的不同次提交不会冲突
	asynqTaskID := fmt.Sprintf("%s:%d", taskID, enqueuedAt)

	task := asynq.NewTask(
		types.TypeFAQImport,
		payloadBytes,
		asynq.TaskID(asynqTaskID),
		asynq.Queue("default"),
		asynq.MaxRetry(maxRetry),
	)
	info, err := s.task.Enqueue(task)
	if err != nil {
		logger.Errorf(ctx, "Failed to enqueue FAQ import task: %v", err)
		return "", fmt.Errorf("failed to enqueue task: %w", err)
	}
	logger.Infof(ctx, "Enqueued FAQ import task: id=%s queue=%s task_id=%s dry_run=%v", info.ID, info.Queue, taskID, payload.DryRun)

	return taskID, nil
}

// generateFailedEntriesCSV 生成失败条目的 CSV 文件并上传
func (s *knowledgeService) generateFailedEntriesCSV(ctx context.Context,
	tenantID uint64, taskID string, failedEntries []types.FAQFailedEntry,
) (string, error) {
	// 生成 CSV 内容
	var buf strings.Builder

	// 写入 BOM 以支持 Excel 正确识别 UTF-8
	buf.WriteString("\xEF\xBB\xBF")

	// 写入表头
	buf.WriteString("错误原因,分类(必填),问题(必填),相似问题(选填-多个用##分隔),反例问题(选填-多个用##分隔),机器人回答(必填-多个用##分隔),是否全部回复(选填-默认FALSE),是否停用(选填-默认FALSE)\n")

	// 写入数据行
	for _, entry := range failedEntries {
		// CSV 转义：如果内容包含逗号、引号或换行，需要用引号包裹并转义内部引号
		reason := csvEscape(entry.Reason)
		tagName := csvEscape(entry.TagName)
		standardQ := csvEscape(entry.StandardQuestion)
		similarQs := ""
		if len(entry.SimilarQuestions) > 0 {
			similarQs = csvEscape(strings.Join(entry.SimilarQuestions, "##"))
		}
		negativeQs := ""
		if len(entry.NegativeQuestions) > 0 {
			negativeQs = csvEscape(strings.Join(entry.NegativeQuestions, "##"))
		}
		answers := ""
		if len(entry.Answers) > 0 {
			answers = csvEscape(strings.Join(entry.Answers, "##"))
		}
		answerAll := "false"
		if entry.AnswerAll {
			answerAll = "true"
		}
		isDisabled := "false"
		if entry.IsDisabled {
			isDisabled = "true"
		}

		buf.WriteString(fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s,%s\n",
			reason, tagName, standardQ, similarQs, negativeQs, answers, answerAll, isDisabled))
	}

	// 上传 CSV 文件到临时存储（会自动过期）
	fileName := fmt.Sprintf("faq_dryrun_failed_%s.csv", taskID)
	filePath, err := s.fileSvc.SaveBytes(ctx, []byte(buf.String()), tenantID, fileName, true)
	if err != nil {
		return "", fmt.Errorf("failed to save CSV file: %w", err)
	}

	// 获取下载 URL
	fileURL, err := s.fileSvc.GetFileURL(ctx, filePath)
	if err != nil {
		return "", fmt.Errorf("failed to get file URL: %w", err)
	}

	logger.Infof(ctx, "Generated failed entries CSV: %s, entries: %d", fileURL, len(failedEntries))
	return fileURL, nil
}

// csvEscape 转义 CSV 字段
func csvEscape(s string) string {
	if strings.ContainsAny(s, ",\"\n\r") {
		// 将内部引号替换为两个引号，并用引号包裹整个字段
		return "\"" + strings.ReplaceAll(s, "\"", "\"\"") + "\""
	}
	return s
}

// saveFAQImportResultToDatabase 保存FAQ导入结果统计到数据库
func (s *knowledgeService) saveFAQImportResultToDatabase(ctx context.Context,
	payload *types.FAQImportPayload, progress *types.FAQImportProgress, originalTotalEntries int,
) error {
	// 获取FAQ知识库实例
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	knowledge, err := s.repo.GetKnowledgeByID(ctx, tenantID, payload.KnowledgeID)
	if err != nil {
		return fmt.Errorf("failed to get FAQ knowledge: %w", err)
	}

	// 计算跳过的条目数（总数 - 成功 - 失败）
	skippedCount := originalTotalEntries - progress.SuccessCount - progress.FailedCount
	if skippedCount < 0 {
		skippedCount = 0
	}

	// 创建导入结果统计
	importResult := &types.FAQImportResult{
		TotalEntries:   originalTotalEntries,
		SuccessCount:   progress.SuccessCount,
		FailedCount:    progress.FailedCount,
		SkippedCount:   skippedCount,
		ImportMode:     payload.Mode,
		ImportedAt:     time.Now(),
		TaskID:         payload.TaskID,
		ProcessingTime: time.Now().Unix() - progress.CreatedAt, // 处理耗时（秒）
		DisplayStatus:  "open",                                 // 新导入的结果默认显示
	}

	// 如果有失败条目且提供了下载URL，设置失败URL
	if progress.FailedCount > 0 && progress.FailedEntriesURL != "" {
		importResult.FailedEntriesURL = progress.FailedEntriesURL
	}

	// 设置导入结果到Knowledge的metadata中
	if err := knowledge.SetLastFAQImportResult(importResult); err != nil {
		return fmt.Errorf("failed to set FAQ import result: %w", err)
	}

	// 更新数据库
	if err := s.repo.UpdateKnowledge(ctx, knowledge); err != nil {
		return fmt.Errorf("failed to update knowledge with import result: %w", err)
	}

	logger.Infof(ctx, "Saved FAQ import result to database: knowledge_id=%s, task_id=%s, total=%d, success=%d, failed=%d, skipped=%d",
		payload.KnowledgeID, payload.TaskID, originalTotalEntries, progress.SuccessCount, progress.FailedCount, skippedCount)

	return nil
}

// buildFAQFailedEntry 构建 FAQFailedEntry
func buildFAQFailedEntry(idx int, reason string, entry *types.FAQEntryPayload) types.FAQFailedEntry {
	answerAll := false
	if entry.AnswerStrategy != nil && *entry.AnswerStrategy == types.AnswerStrategyAll {
		answerAll = true
	}
	isDisabled := false
	if entry.IsEnabled != nil && !*entry.IsEnabled {
		isDisabled = true
	}
	return types.FAQFailedEntry{
		Index:             idx,
		Reason:            reason,
		TagName:           entry.TagName,
		StandardQuestion:  strings.TrimSpace(entry.StandardQuestion),
		SimilarQuestions:  entry.SimilarQuestions,
		NegativeQuestions: entry.NegativeQuestions,
		Answers:           entry.Answers,
		AnswerAll:         answerAll,
		IsDisabled:        isDisabled,
	}
}

// executeFAQDryRunValidation 执行 FAQ dry run 验证，返回通过验证的条目索引
func (s *knowledgeService) executeFAQDryRunValidation(ctx context.Context,
	payload *types.FAQImportPayload, progress *types.FAQImportProgress,
) []int {
	entries := payload.Entries

	// 用于记录已通过基本验证和重复检查的条目索引，后续进行安全检查
	validEntryIndices := make([]int, 0, len(entries))

	// 根据模式选择不同的验证逻辑
	if payload.Mode == types.FAQBatchModeAppend {
		validEntryIndices = s.validateEntriesForAppendModeWithProgress(ctx, payload.TenantID, payload.KBID, entries, progress)
	} else {
		validEntryIndices = s.validateEntriesForReplaceModeWithProgress(ctx, entries, progress)
	}

	return validEntryIndices
}

// validateEntriesForAppendModeWithProgress 验证 Append 模式下的条目（带进度更新）
// 注意：验证阶段不更新 Processed，只有实际导入时才更新
func (s *knowledgeService) validateEntriesForAppendModeWithProgress(ctx context.Context,
	tenantID uint64, kbID string, entries []types.FAQEntryPayload, progress *types.FAQImportProgress,
) []int {
	validIndices := make([]int, 0, len(entries))

	// 查询知识库中已有的所有FAQ chunks的metadata
	existingChunks, err := s.chunkRepo.ListAllFAQChunksWithMetadataByKnowledgeBaseID(ctx, tenantID, kbID)
	if err != nil {
		logger.Warnf(ctx, "Failed to list existing FAQ chunks for dry run: %v", err)
		// 无法获取已有数据时，仅做批次内验证
	}

	// 构建已存在的标准问和相似问集合
	existingQuestions := make(map[string]bool)
	for _, chunk := range existingChunks {
		meta, err := chunk.FAQMetadata()
		if err != nil || meta == nil {
			continue
		}
		if meta.StandardQuestion != "" {
			existingQuestions[meta.StandardQuestion] = true
		}
		for _, q := range meta.SimilarQuestions {
			if q != "" {
				existingQuestions[q] = true
			}
		}
	}

	// 构建当前批次的标准问和相似问集合（用于批次内去重）
	batchQuestions := make(map[string]int) // value 为首次出现的索引

	for i, entry := range entries {
		// 验证条目基本格式
		if err := validateFAQEntryPayloadBasic(&entry); err != nil {
			progress.FailedCount++
			progress.FailedEntries = append(progress.FailedEntries, buildFAQFailedEntry(i, err.Error(), &entry))
			continue
		}

		standardQ := strings.TrimSpace(entry.StandardQuestion)

		// 检查标准问是否与已有知识库重复
		if existingQuestions[standardQ] {
			progress.FailedCount++
			progress.FailedEntries = append(progress.FailedEntries, buildFAQFailedEntry(i, "标准问与知识库中已有问题重复", &entry))
			continue
		}

		// 检查标准问是否与同批次重复
		if firstIdx, exists := batchQuestions[standardQ]; exists {
			progress.FailedCount++
			progress.FailedEntries = append(progress.FailedEntries, buildFAQFailedEntry(i, fmt.Sprintf("标准问与批次内第 %d 条重复", firstIdx+1), &entry))
			continue
		}

		// 检查相似问是否有重复
		hasDuplicate := false
		for _, q := range entry.SimilarQuestions {
			q = strings.TrimSpace(q)
			if q == "" {
				continue
			}
			if existingQuestions[q] {
				progress.FailedCount++
				progress.FailedEntries = append(progress.FailedEntries, buildFAQFailedEntry(i, fmt.Sprintf("相似问 \"%s\" 与知识库中已有问题重复", q), &entry))
				hasDuplicate = true
				break
			}
			if firstIdx, exists := batchQuestions[q]; exists {
				progress.FailedCount++
				progress.FailedEntries = append(progress.FailedEntries, buildFAQFailedEntry(i, fmt.Sprintf("相似问 \"%s\" 与批次内第 %d 条重复", q, firstIdx+1), &entry))
				hasDuplicate = true
				break
			}
		}
		if hasDuplicate {
			continue
		}

		// 将当前条目的标准问和相似问加入批次集合
		batchQuestions[standardQ] = i
		for _, q := range entry.SimilarQuestions {
			q = strings.TrimSpace(q)
			if q != "" {
				batchQuestions[q] = i
			}
		}

		// 记录通过验证的条目索引
		validIndices = append(validIndices, i)

		// 定期更新进度消息（验证阶段不更新 Processed）
		if (i+1)%100 == 0 {
			progress.Message = fmt.Sprintf("正在验证条目 %d/%d...", i+1, len(entries))
			progress.UpdatedAt = time.Now().Unix()
			if err := s.saveFAQImportProgress(ctx, progress); err != nil {
				logger.Warnf(ctx, "Failed to update FAQ dry run progress: %v", err)
			}
		}
	}

	return validIndices
}

// validateEntriesForReplaceModeWithProgress 验证 Replace 模式下的条目（带进度更新）
// 注意：验证阶段不更新 Processed，只有实际导入时才更新
func (s *knowledgeService) validateEntriesForReplaceModeWithProgress(ctx context.Context,
	entries []types.FAQEntryPayload, progress *types.FAQImportProgress,
) []int {
	validIndices := make([]int, 0, len(entries))

	// Replace 模式下只检查批次内重复
	batchQuestions := make(map[string]int) // value 为首次出现的索引

	for i, entry := range entries {
		// 验证条目基本格式
		if err := validateFAQEntryPayloadBasic(&entry); err != nil {
			progress.FailedCount++
			progress.FailedEntries = append(progress.FailedEntries, buildFAQFailedEntry(i, err.Error(), &entry))
			continue
		}

		standardQ := strings.TrimSpace(entry.StandardQuestion)

		// 检查标准问是否与同批次重复
		if firstIdx, exists := batchQuestions[standardQ]; exists {
			progress.FailedCount++
			progress.FailedEntries = append(progress.FailedEntries, buildFAQFailedEntry(i, fmt.Sprintf("标准问与批次内第 %d 条重复", firstIdx+1), &entry))
			continue
		}

		// 检查相似问是否有重复
		hasDuplicate := false
		for _, q := range entry.SimilarQuestions {
			q = strings.TrimSpace(q)
			if q == "" {
				continue
			}
			if firstIdx, exists := batchQuestions[q]; exists {
				progress.FailedCount++
				progress.FailedEntries = append(progress.FailedEntries, buildFAQFailedEntry(i, fmt.Sprintf("相似问 \"%s\" 与批次内第 %d 条重复", q, firstIdx+1), &entry))
				hasDuplicate = true
				break
			}
		}
		if hasDuplicate {
			continue
		}

		// 将当前条目的标准问和相似问加入批次集合
		batchQuestions[standardQ] = i
		for _, q := range entry.SimilarQuestions {
			q = strings.TrimSpace(q)
			if q != "" {
				batchQuestions[q] = i
			}
		}

		// 记录通过验证的条目索引
		validIndices = append(validIndices, i)

		// 定期更新进度消息（验证阶段不更新 Processed）
		if (i+1)%100 == 0 {
			progress.Message = fmt.Sprintf("正在验证条目 %d/%d...", i+1, len(entries))
			progress.UpdatedAt = time.Now().Unix()
			if err := s.saveFAQImportProgress(ctx, progress); err != nil {
				logger.Warnf(ctx, "Failed to update FAQ dry run progress: %v", err)
			}
		}
	}

	return validIndices
}

// validateFAQEntryPayloadBasic 验证 FAQ 条目的基本格式
func validateFAQEntryPayloadBasic(entry *types.FAQEntryPayload) error {
	if entry == nil {
		return fmt.Errorf("条目不能为空")
	}
	standardQ := strings.TrimSpace(entry.StandardQuestion)
	if standardQ == "" {
		return fmt.Errorf("标准问不能为空")
	}
	if len(entry.Answers) == 0 {
		return fmt.Errorf("答案不能为空")
	}
	hasValidAnswer := false
	for _, a := range entry.Answers {
		if strings.TrimSpace(a) != "" {
			hasValidAnswer = true
			break
		}
	}
	if !hasValidAnswer {
		return fmt.Errorf("答案不能全为空")
	}
	return nil
}

// calculateAppendOperations 计算Append模式下需要处理的条目，跳过已存在且内容相同的条目
// 同时过滤掉标准问或相似问与同批次或已有知识库中重复的条目
func (s *knowledgeService) calculateAppendOperations(ctx context.Context,
	tenantID uint64, kbID string, entries []types.FAQEntryPayload,
) ([]types.FAQEntryPayload, int, error) {
	if len(entries) == 0 {
		return []types.FAQEntryPayload{}, 0, nil
	}

	// 1. 查询知识库中已有的所有FAQ chunks的metadata
	existingChunks, err := s.chunkRepo.ListAllFAQChunksWithMetadataByKnowledgeBaseID(ctx, tenantID, kbID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list existing FAQ chunks: %w", err)
	}

	// 2. 构建已存在的标准问和相似问集合
	existingQuestions := make(map[string]bool)
	for _, chunk := range existingChunks {
		meta, err := chunk.FAQMetadata()
		if err != nil || meta == nil {
			continue
		}
		// 添加标准问
		if meta.StandardQuestion != "" {
			existingQuestions[meta.StandardQuestion] = true
		}
		// 添加相似问
		for _, q := range meta.SimilarQuestions {
			if q != "" {
				existingQuestions[q] = true
			}
		}
	}

	// 3. 构建当前批次的标准问和相似问集合（用于批次内去重）
	batchQuestions := make(map[string]bool)
	entriesToProcess := make([]types.FAQEntryPayload, 0, len(entries))
	skippedCount := 0

	for _, entry := range entries {
		meta, err := sanitizeFAQEntryPayload(&entry)
		if err != nil {
			// 跳过无效条目
			skippedCount++
			logger.Warnf(ctx, "Skipping invalid FAQ entry: %v", err)
			continue
		}

		// 检查标准问是否重复（与已有或同批次）
		if existingQuestions[meta.StandardQuestion] || batchQuestions[meta.StandardQuestion] {
			skippedCount++
			logger.Infof(ctx, "Skipping FAQ entry with duplicate standard question: %s", meta.StandardQuestion)
			continue
		}

		// 检查相似问是否有重复（与已有或同批次）
		hasDuplicateSimilar := false
		for _, q := range meta.SimilarQuestions {
			if existingQuestions[q] || batchQuestions[q] {
				hasDuplicateSimilar = true
				logger.Infof(ctx, "Skipping FAQ entry with duplicate similar question: %s (standard: %s)", q, meta.StandardQuestion)
				break
			}
		}
		if hasDuplicateSimilar {
			skippedCount++
			continue
		}

		// 将当前条目的标准问和相似问加入批次集合
		batchQuestions[meta.StandardQuestion] = true
		for _, q := range meta.SimilarQuestions {
			batchQuestions[q] = true
		}

		entriesToProcess = append(entriesToProcess, entry)
	}

	return entriesToProcess, skippedCount, nil
}

// calculateReplaceOperations 计算Replace模式下需要删除、创建、更新的条目
// 同时过滤掉同批次内标准问或相似问重复的条目
func (s *knowledgeService) calculateReplaceOperations(ctx context.Context,
	tenantID uint64, knowledgeID string, newEntries []types.FAQEntryPayload,
) ([]types.FAQEntryPayload, []*types.Chunk, int, error) {
	// 获取 kbID 用于解析 tag
	var kbID string
	if len(newEntries) > 0 {
		// 从 knowledgeID 获取 kbID
		knowledge, err := s.repo.GetKnowledgeByID(ctx, tenantID, knowledgeID)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("failed to get knowledge: %w", err)
		}
		if knowledge != nil {
			kbID = knowledge.KnowledgeBaseID
		}
	}

	// 计算所有新条目的 content hash，并同时构建 hash 到 entry 的映射
	type entryWithHash struct {
		entry types.FAQEntryPayload
		hash  string
		meta  *types.FAQChunkMetadata
	}
	entriesWithHash := make([]entryWithHash, 0, len(newEntries))
	newHashSet := make(map[string]bool)
	// 用于批次内标准问和相似问去重
	batchQuestions := make(map[string]bool)
	batchSkippedCount := 0

	for _, entry := range newEntries {
		meta, err := sanitizeFAQEntryPayload(&entry)
		if err != nil {
			batchSkippedCount++
			logger.Warnf(ctx, "Skipping invalid FAQ entry in replace mode: %v", err)
			continue
		}

		// 检查标准问是否在同批次中重复
		if batchQuestions[meta.StandardQuestion] {
			batchSkippedCount++
			logger.Infof(ctx, "Skipping FAQ entry with duplicate standard question in batch: %s", meta.StandardQuestion)
			continue
		}

		// 检查相似问是否在同批次中重复
		hasDuplicateSimilar := false
		for _, q := range meta.SimilarQuestions {
			if batchQuestions[q] {
				hasDuplicateSimilar = true
				logger.Infof(ctx, "Skipping FAQ entry with duplicate similar question in batch: %s (standard: %s)", q, meta.StandardQuestion)
				break
			}
		}
		if hasDuplicateSimilar {
			batchSkippedCount++
			continue
		}

		// 将当前条目的标准问和相似问加入批次集合
		batchQuestions[meta.StandardQuestion] = true
		for _, q := range meta.SimilarQuestions {
			batchQuestions[q] = true
		}

		hash := types.CalculateFAQContentHash(meta)
		if hash != "" {
			entriesWithHash = append(entriesWithHash, entryWithHash{entry: entry, hash: hash, meta: meta})
			newHashSet[hash] = true
		}
	}

	// 查询所有已存在的chunks
	allExistingChunks, err := s.chunkRepo.ListAllFAQChunksByKnowledgeID(ctx, tenantID, knowledgeID)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("failed to list existing chunks: %w", err)
	}

	// 在内存中过滤出匹配新条目hash的chunks，并构建map
	existingHashMap := make(map[string]*types.Chunk)
	for _, chunk := range allExistingChunks {
		if chunk.ContentHash != "" && newHashSet[chunk.ContentHash] {
			existingHashMap[chunk.ContentHash] = chunk
		}
	}

	// 计算需要删除的chunks（数据库中有但新批次中没有的，或hash不匹配的）
	chunksToDelete := make([]*types.Chunk, 0)
	for _, chunk := range allExistingChunks {
		if chunk.ContentHash == "" {
			// 如果没有hash，需要删除（可能是旧数据）
			chunksToDelete = append(chunksToDelete, chunk)
		} else if !newHashSet[chunk.ContentHash] {
			// hash不在新条目中，需要删除
			chunksToDelete = append(chunksToDelete, chunk)
		}
	}

	// 计算需要创建的条目（利用已经计算好的hash，避免重复计算）
	entriesToProcess := make([]types.FAQEntryPayload, 0, len(entriesWithHash))
	skippedCount := batchSkippedCount

	for _, ewh := range entriesWithHash {
		existingChunk := existingHashMap[ewh.hash]
		if existingChunk != nil {
			// hash 匹配，检查 tag 是否变化
			newTagID, err := s.resolveTagID(ctx, kbID, &ewh.entry)
			if err != nil {
				logger.Warnf(ctx, "Failed to resolve tag for entry, treating as new: %v", err)
				entriesToProcess = append(entriesToProcess, ewh.entry)
				continue
			}

			if existingChunk.TagID != newTagID {
				// tag 变化了，需要删除旧的并创建新的
				logger.Infof(ctx, "FAQ entry tag changed from %s to %s, will update", existingChunk.TagID, newTagID)
				chunksToDelete = append(chunksToDelete, existingChunk)
				entriesToProcess = append(entriesToProcess, ewh.entry)
			} else {
				// hash 和 tag 都相同，跳过
				skippedCount++
			}
			continue
		}

		// hash不匹配或不存在，需要创建
		entriesToProcess = append(entriesToProcess, ewh.entry)
	}

	return entriesToProcess, chunksToDelete, skippedCount, nil
}

// executeFAQImport 执行实际的FAQ导入逻辑
func (s *knowledgeService) executeFAQImport(ctx context.Context, taskID string, kbID string,
	payload *types.FAQBatchUpsertPayload, tenantID uint64, processedCount int,
	progress *types.FAQImportProgress,
) (err error) {
	// 保存知识库和embedding模型信息，用于清理索引
	var kb *types.KnowledgeBase
	var embeddingModel embedding.Embedder
	totalEntries := len(payload.Entries) + processedCount

	// Recovery机制：如果发生任何错误或panic，回滚所有已创建的chunks和索引数据
	defer func() {
		// 捕获panic
		if r := recover(); r != nil {
			buf := make([]byte, 8192)
			n := runtime.Stack(buf, false)
			stack := string(buf[:n])
			logger.Errorf(ctx, "FAQ import task %s panicked: %v\n%s", taskID, r, stack)
			err = fmt.Errorf("panic during FAQ import: %v", r)
		}
	}()

	kb, err = s.validateFAQKnowledgeBase(ctx, kbID)
	if err != nil {
		return err
	}

	kb.EnsureDefaults()

	// 获取embedding模型，用于后续清理索引
	embeddingModel, err = s.modelService.GetEmbeddingModel(ctx, kb.EmbeddingModelID)
	if err != nil {
		return fmt.Errorf("failed to get embedding model: %w", err)
	}
	faqKnowledge, err := s.ensureFAQKnowledge(ctx, tenantID, kb)
	if err != nil {
		return err
	}

	// 获取索引模式
	indexMode := types.FAQIndexModeQuestionOnly
	if kb.FAQConfig != nil && kb.FAQConfig.IndexMode != "" {
		indexMode = kb.FAQConfig.IndexMode
	}

	// 增量更新逻辑：计算需要处理的条目
	var entriesToProcess []types.FAQEntryPayload
	var chunksToDelete []*types.Chunk
	var skippedCount int

	if payload.Mode == types.FAQBatchModeReplace {
		// Replace模式：计算需要删除、创建、更新的条目
		entriesToProcess, chunksToDelete, skippedCount, err = s.calculateReplaceOperations(
			ctx,
			tenantID,
			faqKnowledge.ID,
			payload.Entries,
		)
		if err != nil {
			return fmt.Errorf("failed to calculate replace operations: %w", err)
		}

		// 删除需要删除的chunks（包括需要更新的旧chunks）
		if len(chunksToDelete) > 0 {
			chunkIDsToDelete := make([]string, 0, len(chunksToDelete))
			for _, chunk := range chunksToDelete {
				chunkIDsToDelete = append(chunkIDsToDelete, chunk.ID)
			}
			if err := s.chunkRepo.DeleteChunks(ctx, tenantID, chunkIDsToDelete); err != nil {
				return fmt.Errorf("failed to delete chunks: %w", err)
			}
			// 删除索引
			if err := s.deleteFAQChunkVectors(ctx, kb, faqKnowledge, chunksToDelete); err != nil {
				return fmt.Errorf("failed to delete chunk vectors: %w", err)
			}
			logger.Infof(ctx, "FAQ import task %s: deleted %d chunks (including updates)", taskID, len(chunksToDelete))
		}
	} else {
		// Append模式：查询已存在的条目，跳过未变化的
		entriesToProcess, skippedCount, err = s.calculateAppendOperations(ctx, tenantID, kb.ID, payload.Entries)
		if err != nil {
			return fmt.Errorf("failed to calculate append operations: %w", err)
		}
	}

	logger.Infof(
		ctx,
		"FAQ import task %s: total entries: %d, to process: %d, skipped: %d",
		taskID,
		len(payload.Entries),
		len(entriesToProcess),
		skippedCount,
	)

	// 如果没有需要处理的条目，直接返回
	if len(entriesToProcess) == 0 {
		logger.Infof(ctx, "FAQ import task %s: no entries to process, all skipped", taskID)
		return nil
	}

	// 分批处理需要创建的条目
	remainingEntries := len(entriesToProcess)
	totalStartTime := time.Now()
	actualProcessed := skippedCount + processedCount

	logger.Infof(
		ctx,
		"FAQ import task %s: starting batch processing, remaining entries: %d, total entries: %d, batch size: %d",
		taskID,
		remainingEntries,
		totalEntries,
		faqImportBatchSize,
	)

	for i := 0; i < remainingEntries; i += faqImportBatchSize {
		batchStartTime := time.Now()
		end := i + faqImportBatchSize
		if end > remainingEntries {
			end = remainingEntries
		}

		batch := entriesToProcess[i:end]
		logger.Infof(ctx, "FAQ import task %s: processing batch %d-%d (%d entries)", taskID, i+1, end, len(batch))

		// 构建chunks
		buildStartTime := time.Now()
		chunks := make([]*types.Chunk, 0, len(batch))
		chunkIds := make([]string, 0, len(batch))
		for idx, entry := range batch {
			meta, err := sanitizeFAQEntryPayload(&entry)
			if err != nil {
				logger.ErrorWithFields(ctx, err, map[string]interface{}{
					"entry":   entry,
					"task_id": taskID,
				})
				return fmt.Errorf("failed to sanitize entry at index %d: %w", i+idx, err)
			}

			// 解析 TagID
			tagID, err := s.resolveTagID(ctx, kbID, &entry)
			if err != nil {
				logger.ErrorWithFields(ctx, err, map[string]interface{}{
					"entry":   entry,
					"task_id": taskID,
				})
				return fmt.Errorf("failed to resolve tag for entry at index %d: %w", i+idx, err)
			}

			isEnabled := true
			if entry.IsEnabled != nil {
				isEnabled = *entry.IsEnabled
			}
			// ChunkIndex计算：startChunkIndex + (i+idx) + initialProcessed
			chunk := &types.Chunk{
				ID:              uuid.New().String(),
				TenantID:        tenantID,
				KnowledgeID:     faqKnowledge.ID,
				KnowledgeBaseID: kb.ID,
				Content:         buildFAQChunkContent(meta, indexMode),
				// ChunkIndex:      0,
				IsEnabled: isEnabled,
				ChunkType: types.ChunkTypeFAQ,
				TagID:     tagID,                        // 使用解析后的 TagID
				Status:    int(types.ChunkStatusStored), // store but not indexed
			}
			// 如果指定了 ID（用于数据迁移），设置 SeqID
			if entry.ID != nil && *entry.ID > 0 {
				chunk.SeqID = *entry.ID
			}
			if err := chunk.SetFAQMetadata(meta); err != nil {
				return fmt.Errorf("failed to set FAQ metadata: %w", err)
			}
			chunks = append(chunks, chunk)
			chunkIds = append(chunkIds, chunk.ID)
		}
		buildDuration := time.Since(buildStartTime)
		logger.Debugf(ctx, "FAQ import task %s: batch %d-%d built %d chunks in %v, chunk IDs: %v",
			taskID, i+1, end, len(chunks), buildDuration, chunkIds)
		// 创建chunks
		createStartTime := time.Now()
		if err := s.chunkService.CreateChunks(ctx, chunks); err != nil {
			return fmt.Errorf("failed to create chunks: %w", err)
		}
		createDuration := time.Since(createStartTime)
		logger.Infof(
			ctx,
			"FAQ import task %s: batch %d-%d created %d chunks in %v",
			taskID,
			i+1,
			end,
			len(chunks),
			createDuration,
		)

		// 索引chunks
		indexStartTime := time.Now()
		// 注意：如果索引失败，defer中的recovery机制会自动回滚已创建的chunks和索引数据
		if err := s.indexFAQChunks(ctx, kb, faqKnowledge, chunks, embeddingModel, true, false); err != nil {
			return fmt.Errorf("failed to index chunks: %w", err)
		}
		indexDuration := time.Since(indexStartTime)
		logger.Infof(
			ctx,
			"FAQ import task %s: batch %d-%d indexed %d chunks in %v",
			taskID,
			i+1,
			end,
			len(chunks),
			indexDuration,
		)

		// 更新chunks的Status为已索引
		chunksToUpdate := make([]*types.Chunk, 0, len(chunks))
		for _, chunk := range chunks {
			chunk.Status = int(types.ChunkStatusIndexed) // indexed
			chunksToUpdate = append(chunksToUpdate, chunk)
		}
		if err := s.chunkService.UpdateChunks(ctx, chunksToUpdate); err != nil {
			return fmt.Errorf("failed to update chunks status: %w", err)
		}

		// 收集成功条目信息
		for idx, chunk := range chunks {
			entryIdx := i + idx + processedCount // 原始条目索引
			meta, _ := chunk.FAQMetadata()
			standardQ := ""
			if meta != nil {
				standardQ = meta.StandardQuestion
			}
			// 获取 tag info
			var tagID int64
			tagName := ""
			if chunk.TagID != "" {
				if tag, err := s.tagRepo.GetByID(ctx, tenantID, chunk.TagID); err == nil && tag != nil {
					tagID = tag.SeqID
					tagName = tag.Name
				}
			}
			progress.SuccessEntries = append(progress.SuccessEntries, types.FAQSuccessEntry{
				Index:            entryIdx,
				SeqID:            chunk.SeqID,
				TagID:            tagID,
				TagName:          tagName,
				StandardQuestion: standardQ,
			})
		}

		actualProcessed += len(batch)
		// 更新任务进度
		progress := int(float64(actualProcessed) / float64(totalEntries) * 100)
		if err := s.updateFAQImportProgressStatus(ctx, taskID, types.FAQImportStatusProcessing, progress, totalEntries, actualProcessed, fmt.Sprintf("正在处理第 %d/%d 条", actualProcessed, totalEntries), ""); err != nil {
			logger.Errorf(ctx, "Failed to update task progress: %v", err)
		}

		batchDuration := time.Since(batchStartTime)
		logger.Infof(
			ctx,
			"FAQ import task %s: batch %d-%d completed in %v (build: %v, create: %v, index: %v), total progress: %d/%d (%d%%)",
			taskID,
			i+1,
			end,
			batchDuration,
			buildDuration,
			createDuration,
			indexDuration,
			actualProcessed,
			totalEntries,
			progress,
		)
	}

	totalDuration := time.Since(totalStartTime)
	logger.Infof(
		ctx,
		"FAQ import task %s: all batches completed, processed: %d entries (skipped: %d) in %v, avg: %v per entry",
		taskID,
		actualProcessed,
		skippedCount,
		totalDuration,
		totalDuration/time.Duration(actualProcessed),
	)

	return nil
}

// CreateFAQEntry creates a single FAQ entry synchronously.
func (s *knowledgeService) CreateFAQEntry(ctx context.Context,
	kbID string, payload *types.FAQEntryPayload,
) (*types.FAQEntry, error) {
	if payload == nil {
		return nil, werrors.NewBadRequestError("请求体不能为空")
	}

	kb, err := s.validateFAQKnowledgeBase(ctx, kbID)
	if err != nil {
		return nil, err
	}
	kb.EnsureDefaults()

	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)

	// 验证并清理输入
	meta, err := sanitizeFAQEntryPayload(payload)
	if err != nil {
		return nil, err
	}

	// 解析 TagID
	tagID, err := s.resolveTagID(ctx, kbID, payload)
	if err != nil {
		return nil, err
	}

	// 检查标准问和相似问是否与其他条目重复
	if err := s.checkFAQQuestionDuplicate(ctx, tenantID, kb.ID, "", meta); err != nil {
		return nil, err
	}

	// 确保FAQ Knowledge存在
	faqKnowledge, err := s.ensureFAQKnowledge(ctx, tenantID, kb)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure FAQ knowledge: %w", err)
	}

	// 获取索引模式
	indexMode := types.FAQIndexModeQuestionOnly
	if kb.FAQConfig != nil && kb.FAQConfig.IndexMode != "" {
		indexMode = kb.FAQConfig.IndexMode
	}

	// 获取embedding模型
	embeddingModel, err := s.modelService.GetEmbeddingModel(ctx, kb.EmbeddingModelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get embedding model: %w", err)
	}

	// 创建chunk
	isEnabled := true
	if payload.IsEnabled != nil {
		isEnabled = *payload.IsEnabled
	}
	// 默认可推荐
	flags := types.ChunkFlagRecommended
	if payload.IsRecommended != nil && !*payload.IsRecommended {
		flags = 0
	}

	chunk := &types.Chunk{
		ID:              uuid.New().String(),
		TenantID:        tenantID,
		KnowledgeID:     faqKnowledge.ID,
		KnowledgeBaseID: kb.ID,
		Content:         buildFAQChunkContent(meta, indexMode),
		IsEnabled:       isEnabled,
		Flags:           flags,
		ChunkType:       types.ChunkTypeFAQ,
		TagID:           tagID, // 使用解析后的 TagID
		Status:          int(types.ChunkStatusStored),
	}
	// 如果指定了 ID（用于数据迁移），设置 SeqID
	if payload.ID != nil && *payload.ID > 0 {
		chunk.SeqID = *payload.ID
	}

	if err := chunk.SetFAQMetadata(meta); err != nil {
		return nil, fmt.Errorf("failed to set FAQ metadata: %w", err)
	}

	// 保存chunk
	if err := s.chunkService.CreateChunks(ctx, []*types.Chunk{chunk}); err != nil {
		return nil, fmt.Errorf("failed to create chunk: %w", err)
	}

	// 索引chunk
	if err := s.indexFAQChunks(ctx, kb, faqKnowledge, []*types.Chunk{chunk}, embeddingModel, true, false); err != nil {
		// 如果索引失败，删除已创建的chunk
		_ = s.chunkService.DeleteChunk(ctx, chunk.ID)
		return nil, fmt.Errorf("failed to index chunk: %w", err)
	}

	// 更新chunk状态为已索引
	chunk.Status = int(types.ChunkStatusIndexed)
	if err := s.chunkService.UpdateChunk(ctx, chunk); err != nil {
		return nil, fmt.Errorf("failed to update chunk status: %w", err)
	}

	// Build tag seq_id map for conversion
	tagSeqIDMap := make(map[string]int64)
	if chunk.TagID != "" {
		tag, tagErr := s.tagRepo.GetByID(ctx, tenantID, chunk.TagID)
		if tagErr == nil && tag != nil {
			tagSeqIDMap[tag.ID] = tag.SeqID
		}
	}

	// 转换为FAQEntry返回
	entry, err := s.chunkToFAQEntry(chunk, kb, tagSeqIDMap)
	if err != nil {
		return nil, err
	}

	// 查询TagName
	if chunk.TagID != "" {
		tag, tagErr := s.tagRepo.GetByID(ctx, tenantID, chunk.TagID)
		if tagErr == nil && tag != nil {
			entry.TagName = tag.Name
		}
	}

	return entry, nil
}

// GetFAQEntry retrieves a single FAQ entry by seq_id.
func (s *knowledgeService) GetFAQEntry(ctx context.Context,
	kbID string, entrySeqID int64,
) (*types.FAQEntry, error) {
	if entrySeqID <= 0 {
		return nil, werrors.NewBadRequestError("条目ID不能为空")
	}

	kb, err := s.validateFAQKnowledgeBase(ctx, kbID)
	if err != nil {
		return nil, err
	}
	kb.EnsureDefaults()

	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)

	// 获取chunk by seq_id
	chunk, err := s.chunkRepo.GetChunkBySeqID(ctx, tenantID, entrySeqID)
	if err != nil {
		return nil, werrors.NewNotFoundError("FAQ条目不存在")
	}

	// 验证chunk属于当前知识库
	if chunk.KnowledgeBaseID != kb.ID || chunk.TenantID != tenantID {
		return nil, werrors.NewNotFoundError("FAQ条目不存在")
	}

	// 验证是FAQ类型
	if chunk.ChunkType != types.ChunkTypeFAQ {
		return nil, werrors.NewNotFoundError("FAQ条目不存在")
	}

	// Build tag seq_id map for conversion
	tagSeqIDMap := make(map[string]int64)
	if chunk.TagID != "" {
		tag, tagErr := s.tagRepo.GetByID(ctx, tenantID, chunk.TagID)
		if tagErr == nil && tag != nil {
			tagSeqIDMap[tag.ID] = tag.SeqID
		}
	}

	// 转换为FAQEntry返回
	entry, err := s.chunkToFAQEntry(chunk, kb, tagSeqIDMap)
	if err != nil {
		return nil, err
	}

	// 查询TagName
	if chunk.TagID != "" {
		tag, tagErr := s.tagRepo.GetByID(ctx, tenantID, chunk.TagID)
		if tagErr == nil && tag != nil {
			entry.TagName = tag.Name
		}
	}

	return entry, nil
}

// UpdateFAQEntry updates a single FAQ entry.
func (s *knowledgeService) UpdateFAQEntry(ctx context.Context,
	kbID string, entrySeqID int64, payload *types.FAQEntryPayload,
) (*types.FAQEntry, error) {
	if payload == nil {
		return nil, werrors.NewBadRequestError("请求体不能为空")
	}
	kb, err := s.validateFAQKnowledgeBase(ctx, kbID)
	if err != nil {
		return nil, err
	}
	kb.EnsureDefaults()
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)

	chunk, err := s.chunkRepo.GetChunkBySeqID(ctx, tenantID, entrySeqID)
	if err != nil {
		return nil, werrors.NewNotFoundError("FAQ条目不存在")
	}
	if chunk.KnowledgeBaseID != kb.ID {
		return nil, werrors.NewForbiddenError("无权操作该 FAQ 条目")
	}
	if chunk.ChunkType != types.ChunkTypeFAQ {
		return nil, werrors.NewBadRequestError("仅支持更新 FAQ 条目")
	}
	meta, err := sanitizeFAQEntryPayload(payload)
	if err != nil {
		return nil, err
	}

	// 检查标准问和相似问是否与其他条目重复
	if err := s.checkFAQQuestionDuplicate(ctx, tenantID, kb.ID, chunk.ID, meta); err != nil {
		return nil, err
	}

	// 获取旧的相似问列表，用于增量更新
	var oldSimilarQuestions []string
	var oldStandardQuestion string
	var oldAnswers []string
	questionIndexMode := types.FAQQuestionIndexModeCombined
	if kb.FAQConfig != nil && kb.FAQConfig.QuestionIndexMode != "" {
		questionIndexMode = kb.FAQConfig.QuestionIndexMode
	}
	if existing, err := chunk.FAQMetadata(); err == nil && existing != nil {
		meta.Version = existing.Version + 1
		// 保存旧的内容用于增量比较
		if questionIndexMode == types.FAQQuestionIndexModeSeparate {
			oldSimilarQuestions = existing.SimilarQuestions
			oldStandardQuestion = existing.StandardQuestion
			oldAnswers = existing.Answers
		}
	}
	if err := chunk.SetFAQMetadata(meta); err != nil {
		return nil, err
	}
	// 获取索引模式
	indexMode := types.FAQIndexModeQuestionOnly
	if kb.FAQConfig != nil && kb.FAQConfig.IndexMode != "" {
		indexMode = kb.FAQConfig.IndexMode
	}
	chunk.Content = buildFAQChunkContent(meta, indexMode)

	// Convert tag seq_id to UUID
	if payload.TagID > 0 {
		tag, tagErr := s.tagRepo.GetBySeqID(ctx, tenantID, payload.TagID)
		if tagErr != nil {
			return nil, werrors.NewNotFoundError("标签不存在")
		}
		chunk.TagID = tag.ID
	} else {
		chunk.TagID = ""
	}

	if payload.IsEnabled != nil {
		chunk.IsEnabled = *payload.IsEnabled
	}
	// 处理推荐状态
	if payload.IsRecommended != nil {
		if *payload.IsRecommended {
			chunk.Flags = chunk.Flags.SetFlag(types.ChunkFlagRecommended)
		} else {
			chunk.Flags = chunk.Flags.ClearFlag(types.ChunkFlagRecommended)
		}
	}
	chunk.UpdatedAt = time.Now()
	if err := s.chunkService.UpdateChunk(ctx, chunk); err != nil {
		return nil, err
	}

	// Note: We don't need to call BatchUpdateChunkEnabledStatus here because
	// indexFAQChunks will delete old vectors and re-insert with the latest chunk data
	// (including the updated is_enabled status). Calling both would cause version conflicts.

	faqKnowledge, err := s.repo.GetKnowledgeByID(ctx, tenantID, chunk.KnowledgeID)
	if err != nil {
		return nil, err
	}

	embeddingModel, err := s.modelService.GetEmbeddingModel(ctx, kb.EmbeddingModelID)
	if err != nil {
		return nil, err
	}

	// 增量索引优化：只对变化的内容进行索引操作
	if questionIndexMode == types.FAQQuestionIndexModeSeparate && len(oldSimilarQuestions) > 0 {
		// 分别索引模式下的增量更新
		if err := s.incrementalIndexFAQEntry(ctx, kb, faqKnowledge, chunk, embeddingModel,
			oldStandardQuestion, oldSimilarQuestions, oldAnswers, meta); err != nil {
			return nil, err
		}
	} else {
		// Combined 模式或首次创建，使用全量索引
		// 增量删除：只删除被移除的相似问索引
		oldSimilarQuestionCount := len(oldSimilarQuestions)
		newSimilarQuestionCount := len(meta.SimilarQuestions)
		if questionIndexMode == types.FAQQuestionIndexModeSeparate && oldSimilarQuestionCount > newSimilarQuestionCount {
			tenantInfo := ctx.Value(types.TenantInfoContextKey).(*types.Tenant)
			retrieveEngine, engineErr := retriever.NewCompositeRetrieveEngine(s.retrieveEngine, tenantInfo.GetEffectiveEngines())
			if engineErr == nil {
				sourceIDsToDelete := make([]string, 0, oldSimilarQuestionCount-newSimilarQuestionCount)
				for i := newSimilarQuestionCount; i < oldSimilarQuestionCount; i++ {
					sourceIDsToDelete = append(sourceIDsToDelete, fmt.Sprintf("%s-%d", chunk.ID, i))
				}
				if len(sourceIDsToDelete) > 0 {
					logger.Debugf(ctx, "UpdateFAQEntry: incremental delete %d obsolete source IDs", len(sourceIDsToDelete))
					if delErr := retrieveEngine.DeleteBySourceIDList(ctx, sourceIDsToDelete, embeddingModel.GetDimensions(), types.KnowledgeTypeFAQ); delErr != nil {
						logger.Warnf(ctx, "UpdateFAQEntry: failed to delete obsolete source IDs: %v", delErr)
					}
				}
			}
		}

		// 使用 needDelete=false，因为 EFPutDocument 会自动覆盖相同 SourceID 的文档
		if err := s.indexFAQChunks(ctx, kb, faqKnowledge, []*types.Chunk{chunk}, embeddingModel, false, false); err != nil {
			return nil, err
		}
	}

	// Build tag seq_id map for conversion
	tagSeqIDMap := make(map[string]int64)
	if chunk.TagID != "" {
		tag, tagErr := s.tagRepo.GetByID(ctx, tenantID, chunk.TagID)
		if tagErr == nil && tag != nil {
			tagSeqIDMap[tag.ID] = tag.SeqID
		}
	}

	// 转换为FAQEntry返回
	entry, err := s.chunkToFAQEntry(chunk, kb, tagSeqIDMap)
	if err != nil {
		return nil, err
	}

	// 查询TagName
	if chunk.TagID != "" {
		tag, tagErr := s.tagRepo.GetByID(ctx, tenantID, chunk.TagID)
		if tagErr == nil && tag != nil {
			entry.TagName = tag.Name
		}
	}

	return entry, nil
}

// AddSimilarQuestions adds similar questions to a FAQ entry.
// This will append the new questions to the existing similar questions list.
func (s *knowledgeService) AddSimilarQuestions(ctx context.Context,
	kbID string, entrySeqID int64, questions []string,
) (*types.FAQEntry, error) {
	if len(questions) == 0 {
		return nil, werrors.NewBadRequestError("相似问列表不能为空")
	}

	kb, err := s.validateFAQKnowledgeBase(ctx, kbID)
	if err != nil {
		return nil, err
	}
	kb.EnsureDefaults()
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)

	// Get existing FAQ entry
	chunk, err := s.chunkRepo.GetChunkBySeqID(ctx, tenantID, entrySeqID)
	if err != nil {
		return nil, werrors.NewNotFoundError("FAQ条目不存在")
	}
	if chunk.KnowledgeBaseID != kb.ID {
		return nil, werrors.NewForbiddenError("无权操作该 FAQ 条目")
	}
	if chunk.ChunkType != types.ChunkTypeFAQ {
		return nil, werrors.NewBadRequestError("仅支持更新 FAQ 条目")
	}

	// Get existing metadata
	meta, err := chunk.FAQMetadata()
	if err != nil || meta == nil {
		return nil, werrors.NewBadRequestError("获取 FAQ 元数据失败")
	}

	// Deduplicate and sanitize new questions
	existingSet := make(map[string]struct{})
	for _, q := range meta.SimilarQuestions {
		existingSet[q] = struct{}{}
	}
	// Also add standard question to prevent duplicates
	existingSet[meta.StandardQuestion] = struct{}{}

	newQuestions := make([]string, 0, len(questions))
	for _, q := range questions {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		if _, exists := existingSet[q]; exists {
			continue
		}
		existingSet[q] = struct{}{}
		newQuestions = append(newQuestions, q)
	}

	if len(newQuestions) == 0 {
		// No new questions to add, return current entry
		tagSeqIDMap := make(map[string]int64)
		if chunk.TagID != "" {
			tag, tagErr := s.tagRepo.GetByID(ctx, tenantID, chunk.TagID)
			if tagErr == nil && tag != nil {
				tagSeqIDMap[tag.ID] = tag.SeqID
			}
		}
		return s.chunkToFAQEntry(chunk, kb, tagSeqIDMap)
	}

	// Check for duplicates with other entries
	tempMeta := &types.FAQChunkMetadata{
		StandardQuestion: meta.StandardQuestion,
		SimilarQuestions: append(meta.SimilarQuestions, newQuestions...),
	}
	if err := s.checkFAQQuestionDuplicate(ctx, tenantID, kb.ID, chunk.ID, tempMeta); err != nil {
		return nil, err
	}

	// Update metadata
	oldSimilarQuestions := meta.SimilarQuestions
	meta.SimilarQuestions = append(meta.SimilarQuestions, newQuestions...)
	meta.Version++

	if err := chunk.SetFAQMetadata(meta); err != nil {
		return nil, err
	}

	// Update chunk content
	indexMode := types.FAQIndexModeQuestionOnly
	if kb.FAQConfig != nil && kb.FAQConfig.IndexMode != "" {
		indexMode = kb.FAQConfig.IndexMode
	}
	chunk.Content = buildFAQChunkContent(meta, indexMode)
	chunk.UpdatedAt = time.Now()

	if err := s.chunkService.UpdateChunk(ctx, chunk); err != nil {
		return nil, err
	}

	// Index new similar questions
	faqKnowledge, err := s.repo.GetKnowledgeByID(ctx, tenantID, chunk.KnowledgeID)
	if err != nil {
		return nil, err
	}

	embeddingModel, err := s.modelService.GetEmbeddingModel(ctx, kb.EmbeddingModelID)
	if err != nil {
		return nil, err
	}

	questionIndexMode := types.FAQQuestionIndexModeCombined
	if kb.FAQConfig != nil && kb.FAQConfig.QuestionIndexMode != "" {
		questionIndexMode = kb.FAQConfig.QuestionIndexMode
	}

	if questionIndexMode == types.FAQQuestionIndexModeSeparate {
		// Only index the new similar questions
		if err := s.incrementalIndexFAQEntry(ctx, kb, faqKnowledge, chunk, embeddingModel,
			meta.StandardQuestion, oldSimilarQuestions, meta.Answers, meta); err != nil {
			return nil, err
		}
	} else {
		// Combined mode, re-index the whole entry
		if err := s.indexFAQChunks(ctx, kb, faqKnowledge, []*types.Chunk{chunk}, embeddingModel, false, false); err != nil {
			return nil, err
		}
	}

	// Build response
	tagSeqIDMap := make(map[string]int64)
	if chunk.TagID != "" {
		tag, tagErr := s.tagRepo.GetByID(ctx, tenantID, chunk.TagID)
		if tagErr == nil && tag != nil {
			tagSeqIDMap[tag.ID] = tag.SeqID
		}
	}

	entry, err := s.chunkToFAQEntry(chunk, kb, tagSeqIDMap)
	if err != nil {
		return nil, err
	}

	if chunk.TagID != "" {
		tag, tagErr := s.tagRepo.GetByID(ctx, tenantID, chunk.TagID)
		if tagErr == nil && tag != nil {
			entry.TagName = tag.Name
		}
	}

	return entry, nil
}

// UpdateFAQEntryStatus updates enable status for a FAQ entry.
func (s *knowledgeService) UpdateFAQEntryStatus(ctx context.Context,
	kbID string, entryID string, isEnabled bool,
) error {
	kb, err := s.validateFAQKnowledgeBase(ctx, kbID)
	if err != nil {
		return err
	}
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	chunk, err := s.chunkRepo.GetChunkByID(ctx, tenantID, entryID)
	if err != nil {
		return err
	}
	if chunk.KnowledgeBaseID != kb.ID || chunk.ChunkType != types.ChunkTypeFAQ {
		return werrors.NewBadRequestError("仅支持更新 FAQ 条目")
	}
	if chunk.IsEnabled == isEnabled {
		return nil
	}
	chunk.IsEnabled = isEnabled
	chunk.UpdatedAt = time.Now()
	if err := s.chunkService.UpdateChunk(ctx, chunk); err != nil {
		return err
	}

	// Sync update to retriever engines
	chunkStatusMap := map[string]bool{chunk.ID: isEnabled}
	tenantInfo := ctx.Value(types.TenantInfoContextKey).(*types.Tenant)
	retrieveEngine, err := retriever.NewCompositeRetrieveEngine(s.retrieveEngine, tenantInfo.GetEffectiveEngines())
	if err != nil {
		return err
	}
	if err := retrieveEngine.BatchUpdateChunkEnabledStatus(ctx, chunkStatusMap); err != nil {
		return err
	}

	return nil
}

// UpdateFAQEntryFieldsBatch updates multiple fields for FAQ entries in batch.
// This is the unified API for batch updating FAQ entry fields.
// Supports two modes:
// 1. By entry seq_id: use ByID field
// 2. By Tag seq_id: use ByTag field to apply the same update to all entries under a tag
func (s *knowledgeService) UpdateFAQEntryFieldsBatch(ctx context.Context,
	kbID string, req *types.FAQEntryFieldsBatchUpdate,
) error {
	if req == nil || (len(req.ByID) == 0 && len(req.ByTag) == 0) {
		return nil
	}
	kb, err := s.validateFAQKnowledgeBase(ctx, kbID)
	if err != nil {
		return err
	}
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)

	enabledUpdates := make(map[string]bool)
	tagUpdates := make(map[string]string)

	// Convert exclude seq_ids to UUIDs
	excludeUUIDs := make([]string, 0, len(req.ExcludeIDs))
	if len(req.ExcludeIDs) > 0 {
		excludeChunks, err := s.chunkRepo.ListChunksBySeqID(ctx, tenantID, req.ExcludeIDs)
		if err == nil {
			for _, c := range excludeChunks {
				excludeUUIDs = append(excludeUUIDs, c.ID)
			}
		}
	}

	// Handle ByTag updates first (by tag seq_id)
	if len(req.ByTag) > 0 {
		for tagSeqID, update := range req.ByTag {
			// Convert tag seq_id to UUID
			tag, err := s.tagRepo.GetBySeqID(ctx, tenantID, tagSeqID)
			if err != nil {
				return werrors.NewNotFoundError(fmt.Sprintf("标签 %d 不存在", tagSeqID))
			}

			var setFlags, clearFlags types.ChunkFlags

			// Handle IsRecommended
			if update.IsRecommended != nil {
				if *update.IsRecommended {
					setFlags = types.ChunkFlagRecommended
				} else {
					clearFlags = types.ChunkFlagRecommended
				}
			}

			// Convert new tag seq_id to UUID if provided
			var newTagUUID *string
			if update.TagID != nil {
				if *update.TagID > 0 {
					newTag, err := s.tagRepo.GetBySeqID(ctx, tenantID, *update.TagID)
					if err != nil {
						return werrors.NewNotFoundError(fmt.Sprintf("标签 %d 不存在", *update.TagID))
					}
					newTagUUID = &newTag.ID
				} else {
					emptyStr := ""
					newTagUUID = &emptyStr
				}
			}

			// Update all chunks with this tag
			affectedIDs, err := s.chunkRepo.UpdateChunkFieldsByTagID(
				ctx, tenantID, kb.ID, tag.ID,
				update.IsEnabled, setFlags, clearFlags, newTagUUID, excludeUUIDs,
			)
			if err != nil {
				return err
			}

			// Collect affected IDs for retriever sync
			if len(affectedIDs) > 0 {
				if update.IsEnabled != nil {
					for _, id := range affectedIDs {
						enabledUpdates[id] = *update.IsEnabled
					}
				}
				if newTagUUID != nil {
					for _, id := range affectedIDs {
						tagUpdates[id] = *newTagUUID
					}
				}
			}
		}
	}

	// Handle ByID updates (by entry seq_id)
	if len(req.ByID) > 0 {
		entrySeqIDs := make([]int64, 0, len(req.ByID))
		for entrySeqID := range req.ByID {
			entrySeqIDs = append(entrySeqIDs, entrySeqID)
		}
		chunks, err := s.chunkRepo.ListChunksBySeqID(ctx, tenantID, entrySeqIDs)
		if err != nil {
			return err
		}

		// Build chunk seq_id to chunk map
		chunkBySeqID := make(map[int64]*types.Chunk)
		for _, chunk := range chunks {
			chunkBySeqID[chunk.SeqID] = chunk
		}

		setFlags := make(map[string]types.ChunkFlags)
		clearFlags := make(map[string]types.ChunkFlags)
		chunksToUpdate := make([]*types.Chunk, 0)

		for entrySeqID, update := range req.ByID {
			chunk, exists := chunkBySeqID[entrySeqID]
			if !exists {
				continue
			}
			if chunk.KnowledgeBaseID != kb.ID || chunk.ChunkType != types.ChunkTypeFAQ {
				continue
			}

			needUpdate := false

			// Handle IsEnabled
			if update.IsEnabled != nil && chunk.IsEnabled != *update.IsEnabled {
				chunk.IsEnabled = *update.IsEnabled
				enabledUpdates[chunk.ID] = *update.IsEnabled
				needUpdate = true
			}

			// Handle IsRecommended (via Flags)
			if update.IsRecommended != nil {
				currentRecommended := chunk.Flags.HasFlag(types.ChunkFlagRecommended)
				if currentRecommended != *update.IsRecommended {
					if *update.IsRecommended {
						setFlags[chunk.ID] = types.ChunkFlagRecommended
					} else {
						clearFlags[chunk.ID] = types.ChunkFlagRecommended
					}
				}
			}

			// Handle TagID (convert seq_id to UUID)
			if update.TagID != nil {
				var newTagID string
				if *update.TagID > 0 {
					newTag, err := s.tagRepo.GetBySeqID(ctx, tenantID, *update.TagID)
					if err != nil {
						return werrors.NewNotFoundError(fmt.Sprintf("标签 %d 不存在", *update.TagID))
					}
					newTagID = newTag.ID
				}
				if chunk.TagID != newTagID {
					chunk.TagID = newTagID
					tagUpdates[chunk.ID] = newTagID
					needUpdate = true
				}
			}

			if needUpdate {
				chunk.UpdatedAt = time.Now()
				chunksToUpdate = append(chunksToUpdate, chunk)
			}
		}

		// Batch update chunks (for IsEnabled and TagID)
		if len(chunksToUpdate) > 0 {
			if err := s.chunkRepo.UpdateChunks(ctx, chunksToUpdate); err != nil {
				return err
			}
		}

		// Batch update flags (for IsRecommended)
		if len(setFlags) > 0 || len(clearFlags) > 0 {
			if err := s.chunkRepo.UpdateChunkFlagsBatch(ctx, tenantID, kb.ID, setFlags, clearFlags); err != nil {
				return err
			}
		}
	}

	// Sync to retriever engines
	if len(enabledUpdates) > 0 || len(tagUpdates) > 0 {
		tenantInfo := ctx.Value(types.TenantInfoContextKey).(*types.Tenant)
		retrieveEngine, err := retriever.NewCompositeRetrieveEngine(
			s.retrieveEngine,
			tenantInfo.GetEffectiveEngines(),
		)
		if err != nil {
			return err
		}
		if len(enabledUpdates) > 0 {
			if err := retrieveEngine.BatchUpdateChunkEnabledStatus(ctx, enabledUpdates); err != nil {
				return err
			}
		}
		if len(tagUpdates) > 0 {
			if err := retrieveEngine.BatchUpdateChunkTagID(ctx, tagUpdates); err != nil {
				return err
			}
		}
	}

	return nil
}

// UpdateKnowledgeTag updates the tag assigned to a knowledge document.
func (s *knowledgeService) UpdateKnowledgeTag(ctx context.Context, knowledgeID string, tagID *string) error {
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	knowledge, err := s.repo.GetKnowledgeByID(ctx, tenantID, knowledgeID)
	if err != nil {
		return err
	}

	var resolvedTagID string
	if tagID != nil && *tagID != "" {
		tag, err := s.tagRepo.GetByID(ctx, tenantID, *tagID)
		if err != nil {
			return err
		}
		if tag.KnowledgeBaseID != knowledge.KnowledgeBaseID {
			return werrors.NewBadRequestError("标签不属于当前知识库")
		}
		resolvedTagID = tag.ID
	}

	knowledge.TagID = resolvedTagID
	return s.repo.UpdateKnowledge(ctx, knowledge)
}

// UpdateKnowledgeTagBatch updates tags for document knowledge items in batch.
// authorizedKBID restricts all updates to knowledge items belonging to this KB;
// pass empty string to skip the check (caller must ensure authorization by other means).
func (s *knowledgeService) UpdateKnowledgeTagBatch(ctx context.Context, authorizedKBID string, updates map[string]*string) error {
	if len(updates) == 0 {
		return nil
	}
	tenantIDVal := ctx.Value(types.TenantIDContextKey)
	if tenantIDVal == nil {
		return werrors.NewUnauthorizedError("tenant ID not found in context")
	}
	tenantID, ok := tenantIDVal.(uint64)
	if !ok {
		return werrors.NewUnauthorizedError("invalid tenant ID in context")
	}

	// Get all knowledge items in batch
	knowledgeIDs := make([]string, 0, len(updates))
	for knowledgeID := range updates {
		knowledgeIDs = append(knowledgeIDs, knowledgeID)
	}
	knowledgeList, err := s.repo.GetKnowledgeBatch(ctx, tenantID, knowledgeIDs)
	if err != nil {
		return err
	}

	// Validate all requested IDs were found and belong to the authorized KB
	if authorizedKBID != "" {
		if len(knowledgeList) != len(updates) {
			return werrors.NewForbiddenError("some knowledge IDs are not accessible in the authorized scope")
		}
		for _, k := range knowledgeList {
			if k.KnowledgeBaseID != authorizedKBID {
				return werrors.NewForbiddenError(
					fmt.Sprintf("knowledge %s does not belong to authorized knowledge base", k.ID))
			}
		}
	}

	// Build tag ID map for validation
	tagIDSet := make(map[string]bool)
	for _, tagID := range updates {
		if tagID != nil && *tagID != "" {
			tagIDSet[*tagID] = true
		}
	}

	// Validate all tags in batch
	tagMap := make(map[string]*types.KnowledgeTag)
	if len(tagIDSet) > 0 {
		tagIDs := make([]string, 0, len(tagIDSet))
		for tagID := range tagIDSet {
			tagIDs = append(tagIDs, tagID)
		}
		for _, tagID := range tagIDs {
			tag, err := s.tagRepo.GetByID(ctx, tenantID, tagID)
			if err != nil {
				return err
			}
			tagMap[tagID] = tag
		}
	}

	// Update knowledge items
	knowledgeToUpdate := make([]*types.Knowledge, 0)
	for _, knowledge := range knowledgeList {
		tagID, exists := updates[knowledge.ID]
		if !exists {
			continue
		}

		var resolvedTagID string
		if tagID != nil && *tagID != "" {
			tag, ok := tagMap[*tagID]
			if !ok {
				return werrors.NewBadRequestError(fmt.Sprintf("标签 %s 不存在", *tagID))
			}
			if tag.KnowledgeBaseID != knowledge.KnowledgeBaseID {
				return werrors.NewBadRequestError(fmt.Sprintf("标签 %s 不属于知识库 %s", *tagID, knowledge.KnowledgeBaseID))
			}
			resolvedTagID = tag.ID
		}

		knowledge.TagID = resolvedTagID
		knowledgeToUpdate = append(knowledgeToUpdate, knowledge)
	}

	if len(knowledgeToUpdate) > 0 {
		return s.repo.UpdateKnowledgeBatch(ctx, knowledgeToUpdate)
	}

	return nil
}

// UpdateFAQEntryTag updates the tag assigned to an FAQ entry.
func (s *knowledgeService) UpdateFAQEntryTag(ctx context.Context, kbID string, entryID string, tagID *string) error {
	kb, err := s.validateFAQKnowledgeBase(ctx, kbID)
	if err != nil {
		return err
	}
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	chunk, err := s.chunkRepo.GetChunkByID(ctx, tenantID, entryID)
	if err != nil {
		return err
	}
	if chunk.KnowledgeBaseID != kb.ID || chunk.ChunkType != types.ChunkTypeFAQ {
		return werrors.NewBadRequestError("仅支持更新 FAQ 条目标签")
	}

	var resolvedTagID string
	if tagID != nil && *tagID != "" {
		tag, err := s.tagRepo.GetByID(ctx, tenantID, *tagID)
		if err != nil {
			return err
		}
		if tag.KnowledgeBaseID != kb.ID {
			return werrors.NewBadRequestError("标签不属于当前知识库")
		}
		resolvedTagID = tag.ID
	}

	// Check if tag actually changed
	if chunk.TagID == resolvedTagID {
		return nil
	}

	chunk.TagID = resolvedTagID
	chunk.UpdatedAt = time.Now()
	if err := s.chunkRepo.UpdateChunk(ctx, chunk); err != nil {
		return err
	}

	// Sync tag update to retriever engines
	tenantInfo := ctx.Value(types.TenantInfoContextKey).(*types.Tenant)
	retrieveEngine, err := retriever.NewCompositeRetrieveEngine(
		s.retrieveEngine,
		tenantInfo.GetEffectiveEngines(),
	)
	if err != nil {
		return err
	}
	return retrieveEngine.BatchUpdateChunkTagID(ctx, map[string]string{chunk.ID: resolvedTagID})
}

// UpdateFAQEntryTagBatch updates tags for FAQ entries in batch.
// Key: entry seq_id, Value: tag seq_id (nil to remove tag)
func (s *knowledgeService) UpdateFAQEntryTagBatch(ctx context.Context, kbID string, updates map[int64]*int64) error {
	if len(updates) == 0 {
		return nil
	}
	kb, err := s.validateFAQKnowledgeBase(ctx, kbID)
	if err != nil {
		return err
	}
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)

	// Get all chunks in batch by seq_id
	entrySeqIDs := make([]int64, 0, len(updates))
	for entrySeqID := range updates {
		entrySeqIDs = append(entrySeqIDs, entrySeqID)
	}
	chunks, err := s.chunkRepo.ListChunksBySeqID(ctx, tenantID, entrySeqIDs)
	if err != nil {
		return err
	}

	// Build chunk seq_id to chunk map
	chunkBySeqID := make(map[int64]*types.Chunk)
	for _, chunk := range chunks {
		chunkBySeqID[chunk.SeqID] = chunk
	}

	// Build tag seq_id set for validation
	tagSeqIDSet := make(map[int64]bool)
	for _, tagSeqID := range updates {
		if tagSeqID != nil && *tagSeqID > 0 {
			tagSeqIDSet[*tagSeqID] = true
		}
	}

	// Validate all tags in batch by seq_id
	tagMap := make(map[int64]*types.KnowledgeTag)
	if len(tagSeqIDSet) > 0 {
		tagSeqIDs := make([]int64, 0, len(tagSeqIDSet))
		for tagSeqID := range tagSeqIDSet {
			tagSeqIDs = append(tagSeqIDs, tagSeqID)
		}
		tags, err := s.tagRepo.GetBySeqIDs(ctx, tenantID, tagSeqIDs)
		if err != nil {
			return err
		}
		for _, tag := range tags {
			if tag.KnowledgeBaseID != kb.ID {
				return werrors.NewBadRequestError(fmt.Sprintf("标签 %d 不属于当前知识库", tag.SeqID))
			}
			tagMap[tag.SeqID] = tag
		}
	}

	// Update chunks
	chunksToUpdate := make([]*types.Chunk, 0)
	for entrySeqID, tagSeqID := range updates {
		chunk, exists := chunkBySeqID[entrySeqID]
		if !exists {
			continue
		}
		if chunk.KnowledgeBaseID != kb.ID || chunk.ChunkType != types.ChunkTypeFAQ {
			continue
		}

		var resolvedTagID string
		if tagSeqID != nil && *tagSeqID > 0 {
			tag, ok := tagMap[*tagSeqID]
			if !ok {
				return werrors.NewBadRequestError(fmt.Sprintf("标签 %d 不存在", *tagSeqID))
			}
			resolvedTagID = tag.ID
		}

		chunk.TagID = resolvedTagID
		chunk.UpdatedAt = time.Now()
		chunksToUpdate = append(chunksToUpdate, chunk)
	}

	if len(chunksToUpdate) > 0 {
		if err := s.chunkRepo.UpdateChunks(ctx, chunksToUpdate); err != nil {
			return err
		}

		// Sync tag updates to retriever engines
		tagUpdates := make(map[string]string)
		for _, chunk := range chunksToUpdate {
			tagUpdates[chunk.ID] = chunk.TagID
		}
		tenantInfo := ctx.Value(types.TenantInfoContextKey).(*types.Tenant)
		retrieveEngine, err := retriever.NewCompositeRetrieveEngine(
			s.retrieveEngine,
			tenantInfo.GetEffectiveEngines(),
		)
		if err != nil {
			return err
		}
		if err := retrieveEngine.BatchUpdateChunkTagID(ctx, tagUpdates); err != nil {
			return err
		}
	}

	return nil
}

// SearchFAQEntries searches FAQ entries using hybrid search.
func (s *knowledgeService) SearchFAQEntries(ctx context.Context,
	kbID string, req *types.FAQSearchRequest,
) ([]*types.FAQEntry, error) {
	// Validate FAQ knowledge base
	kb, err := s.validateFAQKnowledgeBase(ctx, kbID)
	if err != nil {
		return nil, err
	}

	// Set default values
	if req.VectorThreshold <= 0 {
		req.VectorThreshold = 0.7
	}
	if req.MatchCount <= 0 {
		req.MatchCount = 10
	}
	if req.MatchCount > 50 {
		req.MatchCount = 50
	}

	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)

	// Convert tag seq_ids to UUIDs
	var firstPriorityTagUUIDs, secondPriorityTagUUIDs []string
	firstPrioritySeqIDSet := make(map[int64]struct{})
	secondPrioritySeqIDSet := make(map[int64]struct{})

	if len(req.FirstPriorityTagIDs) > 0 {
		tags, err := s.tagRepo.GetBySeqIDs(ctx, tenantID, req.FirstPriorityTagIDs)
		if err == nil {
			firstPriorityTagUUIDs = make([]string, 0, len(tags))
			for _, tag := range tags {
				firstPriorityTagUUIDs = append(firstPriorityTagUUIDs, tag.ID)
				firstPrioritySeqIDSet[tag.SeqID] = struct{}{}
			}
		}
	}
	if len(req.SecondPriorityTagIDs) > 0 {
		tags, err := s.tagRepo.GetBySeqIDs(ctx, tenantID, req.SecondPriorityTagIDs)
		if err == nil {
			secondPriorityTagUUIDs = make([]string, 0, len(tags))
			for _, tag := range tags {
				secondPriorityTagUUIDs = append(secondPriorityTagUUIDs, tag.ID)
				secondPrioritySeqIDSet[tag.SeqID] = struct{}{}
			}
		}
	}

	// Build priority tag sets for sorting (using UUID)
	hasFirstPriority := len(firstPriorityTagUUIDs) > 0
	hasSecondPriority := len(secondPriorityTagUUIDs) > 0
	hasPriorityFilter := hasFirstPriority || hasSecondPriority

	firstPrioritySet := make(map[string]struct{}, len(firstPriorityTagUUIDs))
	for _, tagID := range firstPriorityTagUUIDs {
		firstPrioritySet[tagID] = struct{}{}
	}
	secondPrioritySet := make(map[string]struct{}, len(secondPriorityTagUUIDs))
	for _, tagID := range secondPriorityTagUUIDs {
		secondPrioritySet[tagID] = struct{}{}
	}

	// Perform separate searches for each priority level to ensure FirstPriority results
	// are not crowded out by higher-scoring SecondPriority results in TopK truncation
	var searchResults []*types.SearchResult

	if hasPriorityFilter {
		// Use goroutines to search both priority levels concurrently
		var (
			firstResults  []*types.SearchResult
			secondResults []*types.SearchResult
			firstErr      error
			secondErr     error
			wg            sync.WaitGroup
		)

		if hasFirstPriority {
			wg.Add(1)
			go func() {
				defer wg.Done()
				firstParams := types.SearchParams{
					QueryText:            secutils.SanitizeForLog(req.QueryText),
					VectorThreshold:      req.VectorThreshold,
					MatchCount:           req.MatchCount,
					DisableKeywordsMatch: true,
					TagIDs:               firstPriorityTagUUIDs,
					OnlyRecommended:      req.OnlyRecommended,
				}
				firstResults, firstErr = s.kbService.HybridSearch(ctx, kbID, firstParams)
			}()
		}

		if hasSecondPriority {
			wg.Add(1)
			go func() {
				defer wg.Done()
				secondParams := types.SearchParams{
					QueryText:            secutils.SanitizeForLog(req.QueryText),
					VectorThreshold:      req.VectorThreshold,
					MatchCount:           req.MatchCount,
					DisableKeywordsMatch: true,
					TagIDs:               secondPriorityTagUUIDs,
					OnlyRecommended:      req.OnlyRecommended,
				}
				secondResults, secondErr = s.kbService.HybridSearch(ctx, kbID, secondParams)
			}()
		}

		wg.Wait()

		// Check errors
		if firstErr != nil {
			return nil, firstErr
		}
		if secondErr != nil {
			return nil, secondErr
		}

		// Merge results: FirstPriority first, then SecondPriority (deduplicated)
		seenChunkIDs := make(map[string]struct{})
		for _, result := range firstResults {
			if _, exists := seenChunkIDs[result.ID]; !exists {
				seenChunkIDs[result.ID] = struct{}{}
				searchResults = append(searchResults, result)
			}
		}
		for _, result := range secondResults {
			if _, exists := seenChunkIDs[result.ID]; !exists {
				seenChunkIDs[result.ID] = struct{}{}
				searchResults = append(searchResults, result)
			}
		}
	} else {
		// No priority filter, search all
		searchParams := types.SearchParams{
			QueryText:            secutils.SanitizeForLog(req.QueryText),
			VectorThreshold:      req.VectorThreshold,
			MatchCount:           req.MatchCount,
			DisableKeywordsMatch: true,
		}
		var err error
		searchResults, err = s.kbService.HybridSearch(ctx, kbID, searchParams)
		if err != nil {
			return nil, err
		}
	}

	if len(searchResults) == 0 {
		return []*types.FAQEntry{}, nil
	}

	// Extract chunk IDs and build score/match type/matched content maps
	chunkIDs := make([]string, 0, len(searchResults))
	chunkScores := make(map[string]float64)
	chunkMatchTypes := make(map[string]types.MatchType)
	chunkMatchedContents := make(map[string]string)
	for _, result := range searchResults {
		// SearchResult.ID is the chunk ID
		chunkID := result.ID
		chunkIDs = append(chunkIDs, chunkID)
		chunkScores[chunkID] = result.Score
		chunkMatchTypes[chunkID] = result.MatchType
		chunkMatchedContents[chunkID] = result.MatchedContent
	}

	// Batch fetch chunks
	chunks, err := s.chunkRepo.ListChunksByID(ctx, tenantID, chunkIDs)
	if err != nil {
		return nil, err
	}

	// Build tag UUID to seq_id map for conversion
	tagSeqIDMap := make(map[string]int64)
	tagIDs := make([]string, 0)
	tagIDSet := make(map[string]struct{})
	for _, chunk := range chunks {
		if chunk.TagID != "" {
			if _, exists := tagIDSet[chunk.TagID]; !exists {
				tagIDSet[chunk.TagID] = struct{}{}
				tagIDs = append(tagIDs, chunk.TagID)
			}
		}
	}
	if len(tagIDs) > 0 {
		tags, err := s.tagRepo.GetByIDs(ctx, tenantID, tagIDs)
		if err == nil {
			for _, tag := range tags {
				tagSeqIDMap[tag.ID] = tag.SeqID
			}
		}
	}

	// Filter FAQ chunks and convert to FAQEntry
	kb.EnsureDefaults()
	entries := make([]*types.FAQEntry, 0, len(chunks))
	for _, chunk := range chunks {
		// Only process FAQ type chunks
		if chunk.ChunkType != types.ChunkTypeFAQ {
			continue
		}
		if !chunk.IsEnabled {
			continue
		}

		entry, err := s.chunkToFAQEntry(chunk, kb, tagSeqIDMap)
		if err != nil {
			logger.Warnf(ctx, "Failed to convert chunk to FAQ entry: %v", err)
			continue
		}

		// Preserve score and match type from search results
		// Note: Negative question filtering is now handled in HybridSearch
		if score, ok := chunkScores[chunk.ID]; ok {
			entry.Score = score
		}
		if matchType, ok := chunkMatchTypes[chunk.ID]; ok {
			entry.MatchType = matchType
		}

		// Set MatchedQuestion from search result's matched content
		if matchedContent, ok := chunkMatchedContents[chunk.ID]; ok && matchedContent != "" {
			entry.MatchedQuestion = matchedContent
		}

		entries = append(entries, entry)
	}

	// Sort entries with two-level priority tag support
	if hasPriorityFilter {
		// getPriorityLevel returns: 0 = first priority, 1 = second priority, 2 = no priority
		// Use chunk.TagID (UUID) for comparison
		getPriorityLevel := func(chunk *types.Chunk) int {
			if _, ok := firstPrioritySet[chunk.TagID]; ok {
				return 0
			}
			if _, ok := secondPrioritySet[chunk.TagID]; ok {
				return 1
			}
			return 2
		}

		// Build chunk map for priority lookup
		chunkMap := make(map[int64]*types.Chunk)
		for _, chunk := range chunks {
			chunkMap[chunk.SeqID] = chunk
		}

		slices.SortFunc(entries, func(a, b *types.FAQEntry) int {
			aChunk := chunkMap[a.ID]
			bChunk := chunkMap[b.ID]
			var aPriority, bPriority int
			if aChunk != nil {
				aPriority = getPriorityLevel(aChunk)
			} else {
				aPriority = 2
			}
			if bChunk != nil {
				bPriority = getPriorityLevel(bChunk)
			} else {
				bPriority = 2
			}

			// Compare by priority level first
			if aPriority != bPriority {
				return aPriority - bPriority // Lower level = higher priority
			}

			// Same priority level, sort by score descending
			if b.Score > a.Score {
				return 1
			} else if b.Score < a.Score {
				return -1
			}
			return 0
		})
	} else {
		// No priority tags, sort by score only
		slices.SortFunc(entries, func(a, b *types.FAQEntry) int {
			if b.Score > a.Score {
				return 1
			} else if b.Score < a.Score {
				return -1
			}
			return 0
		})
	}

	// Limit results to requested match count
	if len(entries) > req.MatchCount {
		entries = entries[:req.MatchCount]
	}

	// 批量查询TagName并补充到结果中
	if len(entries) > 0 {
		// 收集所有需要查询的TagID (seq_id)
		tagSeqIDs := make([]int64, 0)
		tagSeqIDSet := make(map[int64]struct{})
		for _, entry := range entries {
			if entry.TagID != 0 {
				if _, exists := tagSeqIDSet[entry.TagID]; !exists {
					tagSeqIDs = append(tagSeqIDs, entry.TagID)
					tagSeqIDSet[entry.TagID] = struct{}{}
				}
			}
		}

		// 批量查询标签
		if len(tagSeqIDs) > 0 {
			tags, err := s.tagRepo.GetBySeqIDs(ctx, tenantID, tagSeqIDs)
			if err != nil {
				logger.Warnf(ctx, "Failed to batch query tags: %v", err)
			} else {
				// 构建TagSeqID到TagName的映射
				tagNameMap := make(map[int64]string)
				for _, tag := range tags {
					tagNameMap[tag.SeqID] = tag.Name
				}

				// 补充TagName
				for _, entry := range entries {
					if entry.TagID != 0 {
						if tagName, exists := tagNameMap[entry.TagID]; exists {
							entry.TagName = tagName
						}
					}
				}
			}
		}
	}

	return entries, nil
}

// DeleteFAQEntries deletes FAQ entries in batch by seq_id.
func (s *knowledgeService) DeleteFAQEntries(ctx context.Context,
	kbID string, entrySeqIDs []int64,
) error {
	if len(entrySeqIDs) == 0 {
		return werrors.NewBadRequestError("请选择需要删除的 FAQ 条目")
	}
	kb, err := s.validateFAQKnowledgeBase(ctx, kbID)
	if err != nil {
		return err
	}

	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	var faqKnowledge *types.Knowledge
	chunksToRemove := make([]*types.Chunk, 0, len(entrySeqIDs))
	for _, seqID := range entrySeqIDs {
		if seqID <= 0 {
			continue
		}
		chunk, err := s.chunkRepo.GetChunkBySeqID(ctx, tenantID, seqID)
		if err != nil {
			return werrors.NewNotFoundError("FAQ条目不存在")
		}
		if chunk.KnowledgeBaseID != kb.ID || chunk.ChunkType != types.ChunkTypeFAQ {
			return werrors.NewBadRequestError("包含无效的 FAQ 条目")
		}
		if err := s.chunkService.DeleteChunk(ctx, chunk.ID); err != nil {
			return err
		}
		if faqKnowledge == nil {
			faqKnowledge, err = s.repo.GetKnowledgeByID(ctx, tenantID, chunk.KnowledgeID)
			if err != nil {
				return err
			}
		}
		chunksToRemove = append(chunksToRemove, chunk)
	}
	if len(chunksToRemove) > 0 && faqKnowledge != nil {
		if err := s.deleteFAQChunkVectors(ctx, kb, faqKnowledge, chunksToRemove); err != nil {
			return err
		}
	}
	return nil
}

// ExportFAQEntries exports all FAQ entries for a knowledge base as CSV data.
// The CSV format matches the import example format with 8 columns:
// 分类(必填), 问题(必填), 相似问题(选填-多个用##分隔), 反例问题(选填-多个用##分隔),
// 机器人回答(必填-多个用##分隔), 是否全部回复(选填-默认FALSE), 是否停用(选填-默认FALSE),
// 是否禁止被推荐(选填-默认False 可被推荐)
func (s *knowledgeService) ExportFAQEntries(ctx context.Context, kbID string) ([]byte, error) {
	kb, err := s.validateFAQKnowledgeBase(ctx, kbID)
	if err != nil {
		return nil, err
	}

	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	faqKnowledge, err := s.findFAQKnowledge(ctx, tenantID, kb.ID)
	if err != nil {
		return nil, err
	}
	if faqKnowledge == nil {
		// Return empty CSV with headers only
		return s.buildFAQCSV(nil, nil), nil
	}

	// Get all FAQ chunks
	chunks, err := s.chunkRepo.ListAllFAQChunksForExport(ctx, tenantID, faqKnowledge.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list FAQ chunks: %w", err)
	}

	// Build tag map for tag_id -> tag_name conversion
	tagMap, err := s.buildTagMap(ctx, tenantID, kbID)
	if err != nil {
		return nil, fmt.Errorf("failed to build tag map: %w", err)
	}

	return s.buildFAQCSV(chunks, tagMap), nil
}

// buildTagMap builds a map from tag_id to tag_name for the given knowledge base.
func (s *knowledgeService) buildTagMap(ctx context.Context, tenantID uint64, kbID string) (map[string]string, error) {
	const pageSize = 1000
	tagMap := make(map[string]string)

	for pageNum := 1; ; pageNum++ {
		page := &types.Pagination{Page: pageNum, PageSize: pageSize}
		tags, _, err := s.tagRepo.ListByKB(ctx, tenantID, kbID, page, "")
		if err != nil {
			return nil, err
		}
		for _, tag := range tags {
			if tag != nil {
				tagMap[tag.ID] = tag.Name
			}
		}
		if len(tags) < pageSize {
			break
		}
	}
	return tagMap, nil
}

// buildFAQCSV builds CSV content from FAQ chunks.
func (s *knowledgeService) buildFAQCSV(chunks []*types.Chunk, tagMap map[string]string) []byte {
	var buf strings.Builder

	// Write CSV header (matching import example format)
	headers := []string{
		"分类(必填)",
		"问题(必填)",
		"相似问题(选填-多个用##分隔)",
		"反例问题(选填-多个用##分隔)",
		"机器人回答(必填-多个用##分隔)",
		"是否全部回复(选填-默认FALSE)",
		"是否停用(选填-默认FALSE)",
		"是否禁止被推荐(选填-默认False 可被推荐)",
	}
	buf.WriteString(strings.Join(headers, ","))
	buf.WriteString("\n")

	// Write data rows
	for _, chunk := range chunks {
		meta, err := chunk.FAQMetadata()
		if err != nil || meta == nil {
			continue
		}

		// Get tag name
		tagName := ""
		if chunk.TagID != "" && tagMap != nil {
			if name, ok := tagMap[chunk.TagID]; ok {
				tagName = name
			}
		}

		// Build row
		row := []string{
			escapeCSVField(tagName),
			escapeCSVField(meta.StandardQuestion),
			escapeCSVField(strings.Join(meta.SimilarQuestions, "##")),
			escapeCSVField(strings.Join(meta.NegativeQuestions, "##")),
			escapeCSVField(strings.Join(meta.Answers, "##")),
			boolToCSV(meta.AnswerStrategy == types.AnswerStrategyAll),
			boolToCSV(!chunk.IsEnabled),                                 // 是否停用：取反
			boolToCSV(!chunk.Flags.HasFlag(types.ChunkFlagRecommended)), // 是否禁止被推荐：取反
		}
		buf.WriteString(strings.Join(row, ","))
		buf.WriteString("\n")
	}

	return []byte(buf.String())
}

// escapeCSVField escapes a field for CSV format.
func escapeCSVField(field string) string {
	// If field contains comma, newline, or quote, wrap in quotes and escape internal quotes
	if strings.ContainsAny(field, ",\"\n\r") {
		return "\"" + strings.ReplaceAll(field, "\"", "\"\"") + "\""
	}
	return field
}

// boolToCSV converts a boolean to CSV TRUE/FALSE string.
func boolToCSV(b bool) string {
	if b {
		return "TRUE"
	}
	return "FALSE"
}

func (s *knowledgeService) validateFAQKnowledgeBase(ctx context.Context, kbID string) (*types.KnowledgeBase, error) {
	if kbID == "" {
		return nil, werrors.NewBadRequestError("知识库 ID 不能为空")
	}
	kb, err := s.kbService.GetKnowledgeBaseByID(ctx, kbID)
	if err != nil {
		return nil, err
	}
	kb.EnsureDefaults()
	if kb.Type != types.KnowledgeBaseTypeFAQ {
		return nil, werrors.NewBadRequestError("仅 FAQ 知识库支持该操作")
	}
	return kb, nil
}

func (s *knowledgeService) findFAQKnowledge(
	ctx context.Context,
	tenantID uint64,
	kbID string,
) (*types.Knowledge, error) {
	knowledges, err := s.repo.ListKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID)
	if err != nil {
		return nil, err
	}
	for _, knowledge := range knowledges {
		if knowledge.Type == types.KnowledgeTypeFAQ {
			return knowledge, nil
		}
	}
	return nil, nil
}

func (s *knowledgeService) ensureFAQKnowledge(
	ctx context.Context,
	tenantID uint64,
	kb *types.KnowledgeBase,
) (*types.Knowledge, error) {
	existing, err := s.findFAQKnowledge(ctx, tenantID, kb.ID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	knowledge := &types.Knowledge{
		TenantID:         tenantID,
		KnowledgeBaseID:  kb.ID,
		Type:             types.KnowledgeTypeFAQ,
		Channel:          types.ChannelWeb,
		Title:            fmt.Sprintf("%s - FAQ", kb.Name),
		Description:      "FAQ 条目容器",
		Source:           types.KnowledgeTypeFAQ,
		ParseStatus:      "completed",
		EnableStatus:     "enabled",
		EmbeddingModelID: kb.EmbeddingModelID,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := s.repo.CreateKnowledge(ctx, knowledge); err != nil {
		return nil, err
	}
	return knowledge, nil
}

// updateFAQImportProgressStatus updates the FAQ import progress in Redis
func (s *knowledgeService) updateFAQImportProgressStatus(
	ctx context.Context,
	taskID string,
	status types.FAQImportTaskStatus,
	progress, total, processed int,
	message, errorMsg string,
) error {
	// Get existing progress from Redis
	existingProgress, err := s.GetFAQImportProgress(ctx, taskID)
	if err != nil {
		// If not found, create a new progress entry
		existingProgress = &types.FAQImportProgress{
			TaskID:    taskID,
			CreatedAt: time.Now().Unix(),
		}
	}

	// Update progress fields
	existingProgress.Status = status
	existingProgress.Progress = progress
	existingProgress.Total = total
	existingProgress.Processed = processed
	if message != "" {
		existingProgress.Message = message
	}
	existingProgress.Error = errorMsg
	if status == types.FAQImportStatusCompleted {
		existingProgress.Error = ""
	}

	// 任务完成或失败时，清除 running key
	if status == types.FAQImportStatusCompleted || status == types.FAQImportStatusFailed {
		if existingProgress.KBID != "" {
			if clearErr := s.clearRunningFAQImportTaskID(ctx, existingProgress.KBID); clearErr != nil {
				logger.Errorf(ctx, "Failed to clear running FAQ import task ID: %v", clearErr)
			}
		}
	}

	return s.saveFAQImportProgress(ctx, existingProgress)
}

// cleanupFAQEntriesFileOnFinalFailure 在任务最终失败时清理对象存储中的 entries 文件
// 只有当 retryCount >= maxRetry 时才执行清理，否则重试时还需要使用这个文件
func (s *knowledgeService) cleanupFAQEntriesFileOnFinalFailure(ctx context.Context, entriesURL string, retryCount, maxRetry int) {
	if entriesURL == "" || retryCount < maxRetry {
		return
	}
	if err := s.fileSvc.DeleteFile(ctx, entriesURL); err != nil {
		logger.Warnf(ctx, "Failed to delete FAQ entries file from object storage on final failure: %v", err)
	} else {
		logger.Infof(ctx, "Deleted FAQ entries file from object storage on final failure: %s", entriesURL)
	}
}

// runningFAQImportInfo stores the task ID and enqueued timestamp for uniquely identifying a task instance
type runningFAQImportInfo struct {
	TaskID     string `json:"task_id"`
	EnqueuedAt int64  `json:"enqueued_at"`
}

// getRunningFAQImportInfo checks if there's a running FAQ import task for the given KB
// Returns the task info if found, nil otherwise
func (s *knowledgeService) getRunningFAQImportInfo(ctx context.Context, kbID string) (*runningFAQImportInfo, error) {
	key := getFAQImportRunningKey(kbID)
	data, err := s.redisClient.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get running FAQ import task: %w", err)
	}

	// Try to parse as JSON first (new format)
	var info runningFAQImportInfo
	if err := json.Unmarshal([]byte(data), &info); err != nil {
		// Fallback: old format was just taskID string
		return &runningFAQImportInfo{TaskID: data, EnqueuedAt: 0}, nil
	}
	return &info, nil
}

// getRunningFAQImportTaskID checks if there's a running FAQ import task for the given KB
// Returns the task ID if found, empty string otherwise (for backward compatibility)
func (s *knowledgeService) getRunningFAQImportTaskID(ctx context.Context, kbID string) (string, error) {
	info, err := s.getRunningFAQImportInfo(ctx, kbID)
	if err != nil {
		return "", err
	}
	if info == nil {
		return "", nil
	}
	return info.TaskID, nil
}

// setRunningFAQImportInfo sets the running task info for a KB
func (s *knowledgeService) setRunningFAQImportInfo(ctx context.Context, kbID string, info *runningFAQImportInfo) error {
	key := getFAQImportRunningKey(kbID)
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to marshal running info: %w", err)
	}
	return s.redisClient.Set(ctx, key, data, faqImportProgressTTL).Err()
}

// clearRunningFAQImportTaskID clears the running task ID for a KB
func (s *knowledgeService) clearRunningFAQImportTaskID(ctx context.Context, kbID string) error {
	key := getFAQImportRunningKey(kbID)
	return s.redisClient.Del(ctx, key).Err()
}

func (s *knowledgeService) chunkToFAQEntry(chunk *types.Chunk, kb *types.KnowledgeBase, tagSeqIDMap map[string]int64) (*types.FAQEntry, error) {
	meta, err := chunk.FAQMetadata()
	if err != nil {
		return nil, err
	}
	if meta == nil {
		meta = &types.FAQChunkMetadata{StandardQuestion: chunk.Content}
	}
	// 默认使用 all 策略
	answerStrategy := meta.AnswerStrategy
	if answerStrategy == "" {
		answerStrategy = types.AnswerStrategyAll
	}

	// Get tag seq_id from map
	var tagSeqID int64
	if chunk.TagID != "" && tagSeqIDMap != nil {
		tagSeqID = tagSeqIDMap[chunk.TagID]
	}

	entry := &types.FAQEntry{
		ID:                chunk.SeqID,
		ChunkID:           chunk.ID,
		KnowledgeID:       chunk.KnowledgeID,
		KnowledgeBaseID:   chunk.KnowledgeBaseID,
		TagID:             tagSeqID,
		IsEnabled:         chunk.IsEnabled,
		IsRecommended:     chunk.Flags.HasFlag(types.ChunkFlagRecommended),
		StandardQuestion:  meta.StandardQuestion,
		SimilarQuestions:  meta.SimilarQuestions,
		NegativeQuestions: meta.NegativeQuestions,
		Answers:           meta.Answers,
		AnswerStrategy:    answerStrategy,
		IndexMode:         kb.FAQConfig.IndexMode,
		UpdatedAt:         chunk.UpdatedAt,
		CreatedAt:         chunk.CreatedAt,
		ChunkType:         chunk.ChunkType,
	}
	return entry, nil
}

func buildFAQChunkContent(meta *types.FAQChunkMetadata, mode types.FAQIndexMode) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Q: %s\n", meta.StandardQuestion))
	if len(meta.SimilarQuestions) > 0 {
		builder.WriteString("Similar Questions:\n")
		for _, q := range meta.SimilarQuestions {
			builder.WriteString(fmt.Sprintf("- %s\n", q))
		}
	}
	// 负例不应该包含在 Content 中，因为它们不应该被索引
	// 答案根据索引模式决定是否包含
	if mode == types.FAQIndexModeQuestionAnswer && len(meta.Answers) > 0 {
		builder.WriteString("Answers:\n")
		for _, ans := range meta.Answers {
			builder.WriteString(fmt.Sprintf("- %s\n", ans))
		}
	}
	return builder.String()
}

// checkFAQQuestionDuplicate 检查标准问和相似问是否与知识库中其他条目重复
// excludeChunkID 用于排除当前正在编辑的条目（更新时使用）
// 按照批量导入时的检查方式：先构建已存在问题集合，再统一检查
func (s *knowledgeService) checkFAQQuestionDuplicate(
	ctx context.Context,
	tenantID uint64,
	kbID string,
	excludeChunkID string,
	meta *types.FAQChunkMetadata,
) error {
	// 1. 首先检查当前条目自身的相似问是否与标准问重复
	for _, q := range meta.SimilarQuestions {
		if q == meta.StandardQuestion {
			return werrors.NewBadRequestError(fmt.Sprintf("相似问「%s」不能与标准问相同", q))
		}
	}

	// 2. 检查当前条目自身的相似问之间是否有重复
	seen := make(map[string]struct{})
	for _, q := range meta.SimilarQuestions {
		if _, exists := seen[q]; exists {
			return werrors.NewBadRequestError(fmt.Sprintf("相似问「%s」重复", q))
		}
		seen[q] = struct{}{}
	}

	// 3. 检查反例问题是否与标准问或相似问重复（反例不能和正例相同）
	positiveQuestions := make(map[string]struct{})
	positiveQuestions[meta.StandardQuestion] = struct{}{}
	for _, q := range meta.SimilarQuestions {
		positiveQuestions[q] = struct{}{}
	}
	negativeQuestionsSeen := make(map[string]struct{})
	for _, q := range meta.NegativeQuestions {
		if q == "" {
			continue
		}
		// 检查反例是否与标准问重复
		if q == meta.StandardQuestion {
			return werrors.NewBadRequestError(fmt.Sprintf("反例问题「%s」不能与标准问相同", q))
		}
		// 检查反例是否与相似问重复
		if _, exists := positiveQuestions[q]; exists {
			return werrors.NewBadRequestError(fmt.Sprintf("反例问题「%s」不能与相似问相同", q))
		}
		// 检查反例之间是否重复
		if _, exists := negativeQuestionsSeen[q]; exists {
			return werrors.NewBadRequestError(fmt.Sprintf("反例问题「%s」重复", q))
		}
		negativeQuestionsSeen[q] = struct{}{}
	}

	// 4. 将标准问和所有相似问合并，用一条 DB 查询检查是否与其他条目冲突（替代全量扫描）
	allQuestions := make([]string, 0, 1+len(meta.SimilarQuestions))
	allQuestions = append(allQuestions, meta.StandardQuestion)
	allQuestions = append(allQuestions, meta.SimilarQuestions...)

	dupChunk, err := s.chunkRepo.FindFAQChunkWithDuplicateQuestion(ctx, tenantID, kbID, excludeChunkID, allQuestions)
	if err != nil {
		return fmt.Errorf("failed to check FAQ question duplicate: %w", err)
	}
	if dupChunk == nil {
		return nil
	}

	existingMeta, err := dupChunk.FAQMetadata()
	if err != nil || existingMeta == nil {
		return werrors.NewBadRequestError("标准问或相似问与已有条目重复")
	}

	// 5–7. 与原先全量扫描一致的报错语义：先检查标准问，再逐条检查相似问
	existingSimilarSet := make(map[string]struct{}, len(existingMeta.SimilarQuestions))
	for _, q := range existingMeta.SimilarQuestions {
		if q != "" {
			existingSimilarSet[q] = struct{}{}
		}
	}

	if meta.StandardQuestion != "" {
		if meta.StandardQuestion == existingMeta.StandardQuestion {
			return werrors.NewBadRequestError(fmt.Sprintf("标准问「%s」已存在", meta.StandardQuestion))
		}
		if _, ok := existingSimilarSet[meta.StandardQuestion]; ok {
			return werrors.NewBadRequestError(fmt.Sprintf("标准问「%s」已存在", meta.StandardQuestion))
		}
	}

	for _, q := range meta.SimilarQuestions {
		if q == "" {
			continue
		}
		if q == existingMeta.StandardQuestion {
			return werrors.NewBadRequestError(fmt.Sprintf("相似问「%s」已存在", q))
		}
		if _, ok := existingSimilarSet[q]; ok {
			return werrors.NewBadRequestError(fmt.Sprintf("相似问「%s」已存在", q))
		}
	}

	return werrors.NewBadRequestError("标准问或相似问与已有条目重复")
}

// resolveTagID resolves tag ID (UUID) from payload, prioritizing tag_id (seq_id) over tag_name
// If no tag is specified, creates or finds the "未分类" tag
// Returns the internal UUID of the tag
func (s *knowledgeService) resolveTagID(ctx context.Context, kbID string, payload *types.FAQEntryPayload) (string, error) {
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)

	// 如果提供了 tag_id (seq_id)，优先使用 tag_id
	if payload.TagID != 0 {
		tag, err := s.tagRepo.GetBySeqID(ctx, tenantID, payload.TagID)
		if err != nil {
			return "", fmt.Errorf("failed to find tag by seq_id %d: %w", payload.TagID, err)
		}
		return tag.ID, nil
	}

	// 如果提供了 tag_name，查找或创建标签
	if payload.TagName != "" {
		tag, err := s.tagService.FindOrCreateTagByName(ctx, kbID, payload.TagName)
		if err != nil {
			return "", fmt.Errorf("failed to resolve tag by name '%s': %w", payload.TagName, err)
		}
		return tag.ID, nil
	}

	// 都没有提供，使用"未分类"标签
	tag, err := s.tagService.FindOrCreateTagByName(ctx, kbID, types.UntaggedTagName)
	if err != nil {
		return "", fmt.Errorf("failed to get or create default untagged tag: %w", err)
	}
	return tag.ID, nil
}

func sanitizeFAQEntryPayload(payload *types.FAQEntryPayload) (*types.FAQChunkMetadata, error) {
	// 处理 AnswerStrategy，默认为 all
	answerStrategy := types.AnswerStrategyAll
	if payload.AnswerStrategy != nil && *payload.AnswerStrategy != "" {
		switch *payload.AnswerStrategy {
		case types.AnswerStrategyAll, types.AnswerStrategyRandom:
			answerStrategy = *payload.AnswerStrategy
		default:
			return nil, werrors.NewBadRequestError("answer_strategy 必须是 'all' 或 'random'")
		}
	}
	meta := &types.FAQChunkMetadata{
		StandardQuestion:  strings.TrimSpace(payload.StandardQuestion),
		SimilarQuestions:  payload.SimilarQuestions,
		NegativeQuestions: payload.NegativeQuestions,
		Answers:           payload.Answers,
		AnswerStrategy:    answerStrategy,
		Version:           1,
		Source:            "faq",
	}
	meta.Normalize()
	if meta.StandardQuestion == "" {
		return nil, werrors.NewBadRequestError("标准问不能为空")
	}
	if len(meta.Answers) == 0 {
		return nil, werrors.NewBadRequestError("至少提供一个答案")
	}
	return meta, nil
}

func buildFAQIndexContent(meta *types.FAQChunkMetadata, mode types.FAQIndexMode) string {
	var builder strings.Builder
	builder.WriteString(meta.StandardQuestion)
	for _, q := range meta.SimilarQuestions {
		builder.WriteString("\n")
		builder.WriteString(q)
	}
	if mode == types.FAQIndexModeQuestionAnswer {
		for _, ans := range meta.Answers {
			builder.WriteString("\n")
			builder.WriteString(ans)
		}
	}
	return builder.String()
}

// buildFAQIndexInfoList 构建FAQ索引信息列表，支持分别索引模式
func (s *knowledgeService) buildFAQIndexInfoList(
	ctx context.Context,
	kb *types.KnowledgeBase,
	chunk *types.Chunk,
) ([]*types.IndexInfo, error) {
	indexMode := types.FAQIndexModeQuestionAnswer
	questionIndexMode := types.FAQQuestionIndexModeCombined
	if kb.FAQConfig != nil {
		if kb.FAQConfig.IndexMode != "" {
			indexMode = kb.FAQConfig.IndexMode
		}
		if kb.FAQConfig.QuestionIndexMode != "" {
			questionIndexMode = kb.FAQConfig.QuestionIndexMode
		}
	}

	meta, err := chunk.FAQMetadata()
	if err != nil {
		return nil, err
	}
	if meta == nil {
		meta = &types.FAQChunkMetadata{StandardQuestion: chunk.Content}
	}

	// 如果是一起索引模式，使用原有逻辑
	if questionIndexMode == types.FAQQuestionIndexModeCombined {
		content := buildFAQIndexContent(meta, indexMode)
		return []*types.IndexInfo{
			{
				Content:         content,
				SourceID:        chunk.ID,
				SourceType:      types.ChunkSourceType,
				ChunkID:         chunk.ID,
				KnowledgeID:     chunk.KnowledgeID,
				KnowledgeBaseID: chunk.KnowledgeBaseID,
				KnowledgeType:   types.KnowledgeTypeFAQ,
				TagID:           chunk.TagID,
				IsEnabled:       chunk.IsEnabled,
				IsRecommended:   chunk.Flags.HasFlag(types.ChunkFlagRecommended),
			},
		}, nil
	}

	// 分别索引模式：为每个问题创建独立的索引项
	indexInfoList := make([]*types.IndexInfo, 0)

	// 标准问索引项
	standardContent := meta.StandardQuestion
	if indexMode == types.FAQIndexModeQuestionAnswer && len(meta.Answers) > 0 {
		var builder strings.Builder
		builder.WriteString(meta.StandardQuestion)
		for _, ans := range meta.Answers {
			builder.WriteString("\n")
			builder.WriteString(ans)
		}
		standardContent = builder.String()
	}
	indexInfoList = append(indexInfoList, &types.IndexInfo{
		Content:         standardContent,
		SourceID:        chunk.ID,
		SourceType:      types.ChunkSourceType,
		ChunkID:         chunk.ID,
		KnowledgeID:     chunk.KnowledgeID,
		KnowledgeBaseID: chunk.KnowledgeBaseID,
		KnowledgeType:   types.KnowledgeTypeFAQ,
		TagID:           chunk.TagID,
		IsEnabled:       chunk.IsEnabled,
		IsRecommended:   chunk.Flags.HasFlag(types.ChunkFlagRecommended),
	})

	// 每个相似问创建一个索引项
	for i, similarQ := range meta.SimilarQuestions {
		similarContent := similarQ
		if indexMode == types.FAQIndexModeQuestionAnswer && len(meta.Answers) > 0 {
			var builder strings.Builder
			builder.WriteString(similarQ)
			for _, ans := range meta.Answers {
				builder.WriteString("\n")
				builder.WriteString(ans)
			}
			similarContent = builder.String()
		}
		sourceID := fmt.Sprintf("%s-%d", chunk.ID, i)
		indexInfoList = append(indexInfoList, &types.IndexInfo{
			Content:         similarContent,
			SourceID:        sourceID,
			SourceType:      types.ChunkSourceType,
			ChunkID:         chunk.ID,
			KnowledgeID:     chunk.KnowledgeID,
			KnowledgeBaseID: chunk.KnowledgeBaseID,
			KnowledgeType:   types.KnowledgeTypeFAQ,
			TagID:           chunk.TagID,
			IsEnabled:       chunk.IsEnabled,
			IsRecommended:   chunk.Flags.HasFlag(types.ChunkFlagRecommended),
		})
	}

	return indexInfoList, nil
}

// incrementalIndexFAQEntry 增量更新FAQ条目的索引
// 只对内容变化的部分进行embedding计算和索引更新，跳过未变化的部分
func (s *knowledgeService) incrementalIndexFAQEntry(
	ctx context.Context,
	kb *types.KnowledgeBase,
	knowledge *types.Knowledge,
	chunk *types.Chunk,
	embeddingModel embedding.Embedder,
	oldStandardQuestion string,
	oldSimilarQuestions []string,
	oldAnswers []string,
	newMeta *types.FAQChunkMetadata,
) error {
	indexStartTime := time.Now()

	tenantInfo := ctx.Value(types.TenantInfoContextKey).(*types.Tenant)
	retrieveEngine, err := retriever.NewCompositeRetrieveEngine(s.retrieveEngine, tenantInfo.GetEffectiveEngines())
	if err != nil {
		return err
	}

	indexMode := types.FAQIndexModeQuestionAnswer
	if kb.FAQConfig != nil && kb.FAQConfig.IndexMode != "" {
		indexMode = kb.FAQConfig.IndexMode
	}

	// 构建旧的内容（用于比较）
	buildOldContent := func(question string) string {
		if indexMode == types.FAQIndexModeQuestionAnswer && len(oldAnswers) > 0 {
			var builder strings.Builder
			builder.WriteString(question)
			for _, ans := range oldAnswers {
				builder.WriteString("\n")
				builder.WriteString(ans)
			}
			return builder.String()
		}
		return question
	}

	// 构建新的内容
	buildNewContent := func(question string) string {
		if indexMode == types.FAQIndexModeQuestionAnswer && len(newMeta.Answers) > 0 {
			var builder strings.Builder
			builder.WriteString(question)
			for _, ans := range newMeta.Answers {
				builder.WriteString("\n")
				builder.WriteString(ans)
			}
			return builder.String()
		}
		return question
	}

	// 检查答案是否变化
	answersChanged := !slices.Equal(oldAnswers, newMeta.Answers)

	// 收集需要更新的索引项
	var indexInfoToUpdate []*types.IndexInfo

	// 1. 检查标准问是否需要更新
	oldStdContent := buildOldContent(oldStandardQuestion)
	newStdContent := buildNewContent(newMeta.StandardQuestion)
	if oldStdContent != newStdContent {
		indexInfoToUpdate = append(indexInfoToUpdate, &types.IndexInfo{
			Content:         newStdContent,
			SourceID:        chunk.ID,
			SourceType:      types.ChunkSourceType,
			ChunkID:         chunk.ID,
			KnowledgeID:     chunk.KnowledgeID,
			KnowledgeBaseID: chunk.KnowledgeBaseID,
			KnowledgeType:   types.KnowledgeTypeFAQ,
			TagID:           chunk.TagID,
			IsEnabled:       chunk.IsEnabled,
			IsRecommended:   chunk.Flags.HasFlag(types.ChunkFlagRecommended),
		})
	}

	// 2. 检查每个相似问是否需要更新
	oldCount := len(oldSimilarQuestions)
	newCount := len(newMeta.SimilarQuestions)

	for i, newQ := range newMeta.SimilarQuestions {
		needUpdate := false
		if i >= oldCount {
			// 新增的相似问
			needUpdate = true
		} else {
			// 已存在的相似问，检查内容是否变化
			oldQ := oldSimilarQuestions[i]
			if oldQ != newQ || answersChanged {
				needUpdate = true
			}
		}

		if needUpdate {
			sourceID := fmt.Sprintf("%s-%d", chunk.ID, i)
			indexInfoToUpdate = append(indexInfoToUpdate, &types.IndexInfo{
				Content:         buildNewContent(newQ),
				SourceID:        sourceID,
				SourceType:      types.ChunkSourceType,
				ChunkID:         chunk.ID,
				KnowledgeID:     chunk.KnowledgeID,
				KnowledgeBaseID: chunk.KnowledgeBaseID,
				KnowledgeType:   types.KnowledgeTypeFAQ,
				TagID:           chunk.TagID,
				IsEnabled:       chunk.IsEnabled,
				IsRecommended:   chunk.Flags.HasFlag(types.ChunkFlagRecommended),
			})
		}
	}

	// 3. 删除多余的旧相似问索引
	if oldCount > newCount {
		sourceIDsToDelete := make([]string, 0, oldCount-newCount)
		for i := newCount; i < oldCount; i++ {
			sourceIDsToDelete = append(sourceIDsToDelete, fmt.Sprintf("%s-%d", chunk.ID, i))
		}
		logger.Debugf(ctx, "incrementalIndexFAQEntry: deleting %d obsolete source IDs", len(sourceIDsToDelete))
		if delErr := retrieveEngine.DeleteBySourceIDList(ctx, sourceIDsToDelete, embeddingModel.GetDimensions(), types.KnowledgeTypeFAQ); delErr != nil {
			logger.Warnf(ctx, "incrementalIndexFAQEntry: failed to delete obsolete source IDs: %v", delErr)
		}
	}

	// 4. 批量索引需要更新的内容
	if len(indexInfoToUpdate) > 0 {
		logger.Debugf(ctx, "incrementalIndexFAQEntry: updating %d index entries (skipped %d unchanged)",
			len(indexInfoToUpdate), 1+newCount-len(indexInfoToUpdate))
		if err := retrieveEngine.BatchIndex(ctx, embeddingModel, indexInfoToUpdate); err != nil {
			return err
		}
	} else {
		logger.Debugf(ctx, "incrementalIndexFAQEntry: all %d entries unchanged, skipping index update", 1+newCount)
	}

	// 5. 更新 knowledge 记录
	now := time.Now()
	knowledge.UpdatedAt = now
	knowledge.ProcessedAt = &now
	if err := s.repo.UpdateKnowledge(ctx, knowledge); err != nil {
		return err
	}

	totalDuration := time.Since(indexStartTime)
	logger.Debugf(ctx, "incrementalIndexFAQEntry: completed in %v, updated %d/%d entries",
		totalDuration, len(indexInfoToUpdate), 1+newCount)

	return nil
}

func (s *knowledgeService) indexFAQChunks(ctx context.Context,
	kb *types.KnowledgeBase, knowledge *types.Knowledge,
	chunks []*types.Chunk, embeddingModel embedding.Embedder,
	adjustStorage bool, needDelete bool,
) error {
	if len(chunks) == 0 {
		return nil
	}
	indexStartTime := time.Now()
	logger.Debugf(ctx, "indexFAQChunks: starting to index %d chunks", len(chunks))

	tenantInfo := ctx.Value(types.TenantInfoContextKey).(*types.Tenant)
	retrieveEngine, err := retriever.NewCompositeRetrieveEngine(s.retrieveEngine, tenantInfo.GetEffectiveEngines())
	if err != nil {
		return err
	}

	// 构建索引信息
	buildIndexInfoStartTime := time.Now()
	indexInfo := make([]*types.IndexInfo, 0)
	chunkIDs := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		infoList, err := s.buildFAQIndexInfoList(ctx, kb, chunk)
		if err != nil {
			return err
		}
		indexInfo = append(indexInfo, infoList...)
		chunkIDs = append(chunkIDs, chunk.ID)
	}
	buildIndexInfoDuration := time.Since(buildIndexInfoStartTime)
	logger.Debugf(
		ctx,
		"indexFAQChunks: built %d index info entries for %d chunks in %v",
		len(indexInfo),
		len(chunks),
		buildIndexInfoDuration,
	)

	var size int64
	if adjustStorage {
		estimateStartTime := time.Now()
		size = retrieveEngine.EstimateStorageSize(ctx, embeddingModel, indexInfo)
		estimateDuration := time.Since(estimateStartTime)
		logger.Debugf(ctx, "indexFAQChunks: estimated storage size %d bytes in %v", size, estimateDuration)
		if tenantInfo.StorageQuota > 0 && tenantInfo.StorageUsed+size > tenantInfo.StorageQuota {
			return types.NewStorageQuotaExceededError()
		}
	}

	// 删除旧向量
	var deleteDuration time.Duration
	if needDelete {
		deleteStartTime := time.Now()
		if err := retrieveEngine.DeleteByChunkIDList(ctx, chunkIDs, embeddingModel.GetDimensions(), types.KnowledgeTypeFAQ); err != nil {
			logger.Warnf(ctx, "Delete FAQ vectors failed: %v", err)
		}
		deleteDuration = time.Since(deleteStartTime)
		if deleteDuration > 100*time.Millisecond {
			logger.Debugf(ctx, "indexFAQChunks: deleted old vectors for %d chunks in %v", len(chunkIDs), deleteDuration)
		}
	}

	// 批量索引（这里可能是性能瓶颈）
	batchIndexStartTime := time.Now()
	if err := retrieveEngine.BatchIndex(ctx, embeddingModel, indexInfo); err != nil {
		return err
	}
	batchIndexDuration := time.Since(batchIndexStartTime)
	logger.Debugf(ctx, "indexFAQChunks: batch indexed %d index info entries in %v (avg: %v per entry)",
		len(indexInfo), batchIndexDuration, batchIndexDuration/time.Duration(len(indexInfo)))

	if adjustStorage && size > 0 {
		adjustStartTime := time.Now()
		if err := s.tenantRepo.AdjustStorageUsed(ctx, tenantInfo.ID, size); err == nil {
			tenantInfo.StorageUsed += size
		}
		knowledge.StorageSize += size
		adjustDuration := time.Since(adjustStartTime)
		if adjustDuration > 50*time.Millisecond {
			logger.Debugf(ctx, "indexFAQChunks: adjusted storage in %v", adjustDuration)
		}
	}

	updateStartTime := time.Now()
	now := time.Now()
	knowledge.UpdatedAt = now
	knowledge.ProcessedAt = &now
	err = s.repo.UpdateKnowledge(ctx, knowledge)
	updateDuration := time.Since(updateStartTime)
	if updateDuration > 50*time.Millisecond {
		logger.Debugf(ctx, "indexFAQChunks: updated knowledge in %v", updateDuration)
	}

	totalDuration := time.Since(indexStartTime)
	logger.Debugf(
		ctx,
		"indexFAQChunks: completed indexing %d chunks in %v (build: %v, delete: %v, batchIndex: %v, update: %v)",
		len(chunks),
		totalDuration,
		buildIndexInfoDuration,
		deleteDuration,
		batchIndexDuration,
		updateDuration,
	)

	return err
}

func (s *knowledgeService) deleteFAQChunkVectors(ctx context.Context,
	kb *types.KnowledgeBase, knowledge *types.Knowledge, chunks []*types.Chunk,
) error {
	if len(chunks) == 0 {
		return nil
	}
	embeddingModel, err := s.modelService.GetEmbeddingModel(ctx, kb.EmbeddingModelID)
	if err != nil {
		return err
	}
	tenantInfo := ctx.Value(types.TenantInfoContextKey).(*types.Tenant)
	retrieveEngine, err := retriever.NewCompositeRetrieveEngine(s.retrieveEngine, tenantInfo.GetEffectiveEngines())
	if err != nil {
		return err
	}

	indexInfo := make([]*types.IndexInfo, 0)
	chunkIDs := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		infoList, err := s.buildFAQIndexInfoList(ctx, kb, chunk)
		if err != nil {
			return err
		}
		indexInfo = append(indexInfo, infoList...)
		chunkIDs = append(chunkIDs, chunk.ID)
	}

	size := retrieveEngine.EstimateStorageSize(ctx, embeddingModel, indexInfo)
	if err := retrieveEngine.DeleteByChunkIDList(ctx, chunkIDs, embeddingModel.GetDimensions(), types.KnowledgeTypeFAQ); err != nil {
		return err
	}
	if size > 0 {
		if err := s.tenantRepo.AdjustStorageUsed(ctx, tenantInfo.ID, -size); err == nil {
			tenantInfo.StorageUsed -= size
			if tenantInfo.StorageUsed < 0 {
				tenantInfo.StorageUsed = 0
			}
		}
		if knowledge.StorageSize >= size {
			knowledge.StorageSize -= size
		} else {
			knowledge.StorageSize = 0
		}
	}
	knowledge.UpdatedAt = time.Now()
	return s.repo.UpdateKnowledge(ctx, knowledge)
}

// ProcessFAQImport handles Asynq FAQ import tasks (including dry run mode)
func (s *knowledgeService) ProcessFAQImport(ctx context.Context, t *asynq.Task) error {
	var payload types.FAQImportPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		logger.Errorf(ctx, "failed to unmarshal FAQ import task payload: %v", err)
		return fmt.Errorf("failed to unmarshal task payload: %w", err)
	}

	ctx = logger.WithRequestID(ctx, uuid.New().String())
	ctx = logger.WithField(ctx, "faq_import", payload.TaskID)
	ctx = context.WithValue(ctx, types.TenantIDContextKey, payload.TenantID)

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

	// 如果 entries 存储在对象存储中，先下载
	if payload.EntriesURL != "" && len(payload.Entries) == 0 {
		logger.Infof(ctx, "Downloading FAQ entries from object storage: %s", payload.EntriesURL)
		reader, err := s.fileSvc.GetFile(ctx, payload.EntriesURL)
		if err != nil {
			logger.Errorf(ctx, "Failed to download FAQ entries from object storage: %v", err)
			return fmt.Errorf("failed to download entries: %w", err)
		}
		defer reader.Close()

		entriesData, err := io.ReadAll(reader)
		if err != nil {
			logger.Errorf(ctx, "Failed to read FAQ entries data: %v", err)
			return fmt.Errorf("failed to read entries data: %w", err)
		}

		var entries []types.FAQEntryPayload
		if err := json.Unmarshal(entriesData, &entries); err != nil {
			logger.Errorf(ctx, "Failed to unmarshal FAQ entries: %v", err)
			return fmt.Errorf("failed to unmarshal entries: %w", err)
		}

		payload.Entries = entries
		logger.Infof(ctx, "Downloaded %d FAQ entries from object storage", len(entries))
	}

	logger.Infof(ctx, "Processing FAQ import task: task_id=%s, kb_id=%s, total_entries=%d, dry_run=%v, retry=%d/%d",
		payload.TaskID, payload.KBID, len(payload.Entries), payload.DryRun, retryCount, maxRetry)

	// 保存原始总数量
	originalTotalEntries := len(payload.Entries)

	// 初始化进度
	// 检查是否已有验证结果（用于重试时跳过验证）
	// 注意：必须在保存新 progress 之前查询，否则会被覆盖
	existingProgress, _ := s.GetFAQImportProgress(ctx, payload.TaskID)

	progress := &types.FAQImportProgress{
		TaskID:         payload.TaskID,
		KBID:           payload.KBID,
		KnowledgeID:    payload.KnowledgeID,
		Status:         types.FAQImportStatusProcessing,
		Progress:       0,
		Total:          originalTotalEntries,
		Processed:      0,
		SuccessCount:   0,
		FailedCount:    0,
		FailedEntries:  make([]types.FAQFailedEntry, 0),
		SuccessEntries: make([]types.FAQSuccessEntry, 0),
		Message:        "正在验证条目...",
		CreatedAt:      time.Now().Unix(),
		UpdatedAt:      time.Now().Unix(),
		DryRun:         payload.DryRun,
	}
	if err := s.saveFAQImportProgress(ctx, progress); err != nil {
		logger.Warnf(ctx, "Failed to save initial FAQ import progress: %v", err)
	}

	var validEntryIndices []int
	if existingProgress != nil && len(existingProgress.ValidEntryIndices) > 0 {
		// 重试时直接使用之前的验证结果
		validEntryIndices = existingProgress.ValidEntryIndices
		progress.FailedCount = existingProgress.FailedCount
		progress.FailedEntries = existingProgress.FailedEntries
		logger.Infof(ctx, "Reusing previous validation result: valid=%d, failed=%d",
			len(validEntryIndices), progress.FailedCount)
	} else {
		// 第一步：执行验证（无论是 dry run 还是 import 模式都需要验证）
		validEntryIndices = s.executeFAQDryRunValidation(ctx, &payload, progress)
		// 保存验证通过的索引，用于重试时跳过验证
		progress.ValidEntryIndices = validEntryIndices
		if err := s.saveFAQImportProgress(ctx, progress); err != nil {
			logger.Warnf(ctx, "Failed to save validation result: %v", err)
		}
		logger.Infof(ctx, "FAQ validation completed: total=%d, valid=%d, failed=%d",
			originalTotalEntries, len(validEntryIndices), progress.FailedCount)
	}

	// Dry run 模式：验证完成后直接返回结果
	if payload.DryRun {
		return s.finalizeFAQValidation(ctx, &payload, progress, originalTotalEntries)
	}

	// Import 模式：检查是否有有效条目需要导入
	if len(validEntryIndices) == 0 {
		// 没有有效条目，直接完成
		return s.finalizeFAQValidation(ctx, &payload, progress, originalTotalEntries)
	}

	// 提取有效的条目
	validEntries := make([]types.FAQEntryPayload, 0, len(validEntryIndices))
	for _, idx := range validEntryIndices {
		validEntries = append(validEntries, payload.Entries[idx])
	}

	// 更新进度消息
	progress.Message = fmt.Sprintf("验证完成，开始导入 %d 条有效数据...", len(validEntries))
	progress.UpdatedAt = time.Now().Unix()
	if err := s.saveFAQImportProgress(ctx, progress); err != nil {
		logger.Warnf(ctx, "Failed to update FAQ import progress: %v", err)
	}

	// 幂等性检查：获取knowledge记录（FAQ任务使用knowledge ID作为taskID）
	knowledge, err := s.repo.GetKnowledgeByID(ctx, payload.TenantID, payload.KnowledgeID)
	if err != nil {
		logger.Errorf(ctx, "failed to get FAQ knowledge: %v", err)
		return nil
	}

	if knowledge == nil {
		return nil
	}

	kb, err := s.kbService.GetKnowledgeBaseByID(ctx, payload.KBID)
	if err != nil {
		logger.Errorf(ctx, "Failed to get knowledge base: %v", err)
		// 如果是最后一次重试，更新状态为失败
		if isLastRetry {
			if updateErr := s.updateFAQImportProgressStatus(ctx, payload.TaskID, types.FAQImportStatusFailed, 0, originalTotalEntries, 0, "获取知识库失败", err.Error()); updateErr != nil {
				logger.Errorf(ctx, "Failed to update task status to failed: %v", updateErr)
			}
		}
		s.cleanupFAQEntriesFileOnFinalFailure(ctx, payload.EntriesURL, retryCount, maxRetry)
		return fmt.Errorf("failed to get knowledge base: %w", err)
	}

	// 检查任务状态 - 幂等性处理（复用之前获取的 existingProgress）
	var processedCount int
	if existingProgress != nil {
		if existingProgress.Status == types.FAQImportStatusCompleted {
			logger.Infof(ctx, "FAQ import already completed, skipping: %s", payload.TaskID)
			return nil // 幂等：已完成的任务直接返回
		}
		// 获取已处理的数量（注意：这是相对于 validEntries 的索引）
		processedCount = existingProgress.Processed - progress.FailedCount // 已处理数 - 验证失败数 = 已导入的有效条目数
		if processedCount < 0 {
			processedCount = 0
		}
		logger.Infof(ctx, "Resuming FAQ import from progress: %d/%d (valid entries)", processedCount, len(validEntries))
	}

	// 幂等性处理：清理可能已部分处理的chunks和索引数据
	chunksDeleted, err := s.chunkRepo.DeleteUnindexedChunks(ctx, payload.TenantID, payload.KnowledgeID)
	if err != nil {
		logger.Errorf(ctx, "Failed to delete unindexed chunks: %v", err)
		// 如果是最后一次重试，更新状态为失败
		if isLastRetry {
			if updateErr := s.updateFAQImportProgressStatus(ctx, payload.TaskID, types.FAQImportStatusFailed, 0, originalTotalEntries, 0, "清理未索引数据失败", err.Error()); updateErr != nil {
				logger.Errorf(ctx, "Failed to update task status to failed: %v", updateErr)
			}
		}
		s.cleanupFAQEntriesFileOnFinalFailure(ctx, payload.EntriesURL, retryCount, maxRetry)
		return fmt.Errorf("failed to delete unindexed chunks: %w", err)
	}
	if len(chunksDeleted) > 0 {
		logger.Infof(ctx, "Deleted unindexed chunks: %d", len(chunksDeleted))

		// 删除索引数据
		embeddingModel, err := s.modelService.GetEmbeddingModel(ctx, kb.EmbeddingModelID)
		if err == nil {
			retrieveEngine, err := retriever.NewCompositeRetrieveEngine(
				s.retrieveEngine,
				tenantInfo.GetEffectiveEngines(),
			)
			if err == nil {
				chunkIDs := make([]string, 0, len(chunksDeleted))
				for _, chunk := range chunksDeleted {
					chunkIDs = append(chunkIDs, chunk.ID)
				}
				if err := retrieveEngine.DeleteByChunkIDList(ctx, chunkIDs, embeddingModel.GetDimensions(), types.KnowledgeTypeFAQ); err != nil {
					logger.Warnf(ctx, "Failed to delete index data for chunks (may not exist): %v", err)
				} else {
					logger.Infof(ctx, "Successfully deleted index data for %d chunks", len(chunksDeleted))
				}
			}
		}
	}

	// 如果已经处理了一部分有效条目，从该位置继续
	entriesToImport := validEntries
	importMode := payload.Mode
	if processedCount > 0 && processedCount < len(validEntries) {
		entriesToImport = validEntries[processedCount:]
		// 重试场景下，如果之前已经处理了一部分数据，需要切换到 Append 模式
		// 因为 Replace 模式的删除操作在第一次运行时已经执行过了
		// 如果继续使用 Replace 模式，calculateReplaceOperations 会将之前成功导入的数据标记为删除
		// 导致数据丢失
		if payload.Mode == types.FAQBatchModeReplace {
			importMode = types.FAQBatchModeAppend
			logger.Infof(ctx, "Switching to Append mode for retry, original mode was Replace")
		}
		logger.Infof(ctx, "Continuing FAQ import from entry %d, remaining: %d entries", processedCount, len(entriesToImport))
	}

	// 构建FAQBatchUpsertPayload（使用验证通过的有效条目）
	faqPayload := &types.FAQBatchUpsertPayload{
		Entries: entriesToImport,
		Mode:    importMode,
	}

	// 执行FAQ导入（传入已处理的偏移量，用于进度计算）
	if err := s.executeFAQImport(ctx, payload.TaskID, payload.KBID, faqPayload, payload.TenantID, progress.FailedCount+processedCount, progress); err != nil {
		logger.Errorf(ctx, "FAQ import task failed: %s, error: %v", payload.TaskID, err)
		// 如果是最后一次重试，更新状态为失败
		if isLastRetry {
			if updateErr := s.updateFAQImportProgressStatus(ctx, payload.TaskID, types.FAQImportStatusFailed, 0, originalTotalEntries, len(validEntries), "导入失败", err.Error()); updateErr != nil {
				logger.Errorf(ctx, "Failed to update task status to failed: %v", updateErr)
			}
		}
		s.cleanupFAQEntriesFileOnFinalFailure(ctx, payload.EntriesURL, retryCount, maxRetry)
		return fmt.Errorf("FAQ import failed: %w", err)
	}

	// 任务成功完成
	logger.Infof(ctx, "FAQ import task completed: %s, imported: %d, failed: %d",
		payload.TaskID, len(validEntries), progress.FailedCount)

	// 最终完成处理（生成失败条目 CSV 等）
	return s.finalizeFAQValidation(ctx, &payload, progress, originalTotalEntries)
}

// finalizeFAQValidation 完成 FAQ 验证/导入任务，生成失败条目 CSV（如果有）
func (s *knowledgeService) finalizeFAQValidation(ctx context.Context, payload *types.FAQImportPayload,
	progress *types.FAQImportProgress, originalTotalEntries int,
) error {
	// 清理对象存储中的 entries 文件（如果有）
	if payload.EntriesURL != "" {
		if err := s.fileSvc.DeleteFile(ctx, payload.EntriesURL); err != nil {
			logger.Warnf(ctx, "Failed to delete FAQ entries file from object storage: %v", err)
		} else {
			logger.Infof(ctx, "Deleted FAQ entries file from object storage: %s", payload.EntriesURL)
		}
	}
	progress.UpdatedAt = time.Now().Unix()

	// 如果有失败条目，生成 CSV 文件
	if len(progress.FailedEntries) > 0 {
		csvURL, err := s.generateFailedEntriesCSV(ctx, payload.TenantID, payload.TaskID, progress.FailedEntries)
		if err != nil {
			logger.Warnf(ctx, "Failed to generate failed entries CSV: %v", err)
		} else {
			progress.FailedEntriesURL = csvURL
			progress.FailedEntries = nil // 清空内联数据，使用 URL
			progress.Message += " (失败记录已导出为CSV)"
		}
	}

	// 如果不是 dry run 模式，保存导入结果统计到数据库
	if !payload.DryRun {
		if err := s.saveFAQImportResultToDatabase(ctx, payload, progress, originalTotalEntries); err != nil {
			logger.Warnf(ctx, "Failed to save FAQ import result to database: %v", err)
		}

		// 只有 replace 模式才清理未使用的 Tag
		// append 模式不应删除用户预先创建的空标签
		if payload.Mode == types.FAQBatchModeReplace {
			deletedTags, err := s.tagRepo.DeleteUnusedTags(ctx, payload.TenantID, payload.KBID)
			if err != nil {
				logger.Warnf(ctx, "FAQ import task %s: failed to cleanup unused tags: %v", payload.TaskID, err)
			} else if deletedTags > 0 {
				logger.Infof(ctx, "FAQ import task %s: cleaned up %d unused tags after replace import", payload.TaskID, deletedTags)
			}
		}
	}

	// 使用 updateFAQImportProgressStatus 来确保正确清理 running key
	// 但是需要先保存其他字段，因为 updateFAQImportProgressStatus 不会保存所有字段
	if err := s.saveFAQImportProgress(ctx, progress); err != nil {
		logger.Warnf(ctx, "Failed to save final FAQ import progress: %v", err)
	}

	// 然后调用状态更新来清理 running key
	if err := s.updateFAQImportProgressStatus(ctx, payload.TaskID, types.FAQImportStatusCompleted,
		100, originalTotalEntries, originalTotalEntries, progress.Message, ""); err != nil {
		logger.Warnf(ctx, "Failed to update final FAQ import status: %v", err)
	}

	logger.Infof(ctx, "FAQ task completed: %s, dry_run=%v, success: %d, failed: %d",
		payload.TaskID, payload.DryRun, progress.SuccessCount, progress.FailedCount)

	return nil
}

const (
	kbCloneProgressKeyPrefix = "kb_clone_progress:"
	kbCloneProgressTTL       = 24 * time.Hour
)

// getKBCloneProgressKey returns the Redis key for storing KB clone progress
func getKBCloneProgressKey(taskID string) string {
	return kbCloneProgressKeyPrefix + taskID
}

const (
	faqImportProgressKeyPrefix = "faq_import_progress:"
	faqImportRunningKeyPrefix  = "faq_import_running:"
	faqImportProgressTTL       = 3 * time.Hour
)

// getFAQImportProgressKey returns the Redis key for storing FAQ import progress
func getFAQImportProgressKey(taskID string) string {
	return faqImportProgressKeyPrefix + taskID
}

// getFAQImportRunningKey returns the Redis key for storing running task ID by KB ID
func getFAQImportRunningKey(kbID string) string {
	return faqImportRunningKeyPrefix + kbID
}

// saveFAQImportProgress saves the FAQ import progress to Redis
func (s *knowledgeService) saveFAQImportProgress(ctx context.Context, progress *types.FAQImportProgress) error {
	key := getFAQImportProgressKey(progress.TaskID)
	progress.UpdatedAt = time.Now().Unix()
	data, err := json.Marshal(progress)
	if err != nil {
		return fmt.Errorf("failed to marshal FAQ import progress: %w", err)
	}
	return s.redisClient.Set(ctx, key, data, faqImportProgressTTL).Err()
}

// GetFAQImportProgress retrieves the progress of an FAQ import task
func (s *knowledgeService) GetFAQImportProgress(ctx context.Context, taskID string) (*types.FAQImportProgress, error) {
	key := getFAQImportProgressKey(taskID)
	data, err := s.redisClient.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, werrors.NewNotFoundError("FAQ import task not found")
		}
		return nil, fmt.Errorf("failed to get FAQ import progress from Redis: %w", err)
	}

	var progress types.FAQImportProgress
	if err := json.Unmarshal(data, &progress); err != nil {
		return nil, fmt.Errorf("failed to unmarshal FAQ import progress: %w", err)
	}

	// If task is completed, enrich with persisted result fields from database
	if progress.Status == types.FAQImportStatusCompleted && progress.KnowledgeID != "" {
		tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
		knowledge, err := s.repo.GetKnowledgeByID(ctx, tenantID, progress.KnowledgeID)
		if err == nil && knowledge != nil {
			if result, err := knowledge.GetLastFAQImportResult(); err == nil && result != nil {
				progress.SkippedCount = result.SkippedCount
				progress.ImportMode = result.ImportMode
				progress.ImportedAt = result.ImportedAt
				progress.DisplayStatus = result.DisplayStatus
				progress.ProcessingTime = result.ProcessingTime
			}
		}
	}

	return &progress, nil
}

// UpdateLastFAQImportResultDisplayStatus updates the display status of FAQ import result
func (s *knowledgeService) UpdateLastFAQImportResultDisplayStatus(ctx context.Context, kbID string, displayStatus string) error {
	// 验证displayStatus参数
	if displayStatus != "open" && displayStatus != "close" {
		return werrors.NewBadRequestError("invalid display status, must be 'open' or 'close'")
	}

	// 获取当前租户ID
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)

	// 查找FAQ类型的knowledge
	knowledgeList, err := s.repo.ListKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID)
	if err != nil {
		return fmt.Errorf("failed to list knowledge: %w", err)
	}

	// 查找FAQ类型的knowledge
	var faqKnowledge *types.Knowledge
	for _, k := range knowledgeList {
		if k.Type == types.KnowledgeTypeFAQ {
			faqKnowledge = k
			break
		}
	}

	if faqKnowledge == nil {
		return werrors.NewNotFoundError("FAQ knowledge not found in this knowledge base")
	}

	// 解析当前的导入结果
	result, err := faqKnowledge.GetLastFAQImportResult()
	if err != nil {
		return fmt.Errorf("failed to parse FAQ import result: %w", err)
	}

	if result == nil {
		return werrors.NewNotFoundError("no FAQ import result found")
	}

	// 更新显示状态
	result.DisplayStatus = displayStatus

	// 保存更新后的结果
	if err := faqKnowledge.SetLastFAQImportResult(result); err != nil {
		return fmt.Errorf("failed to set FAQ import result: %w", err)
	}

	// 更新数据库
	if err := s.repo.UpdateKnowledge(ctx, faqKnowledge); err != nil {
		return fmt.Errorf("failed to update knowledge: %w", err)
	}

	return nil
}
