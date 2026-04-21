package chatpipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type PluginEvidenceCapture struct {
	repo            interfaces.AnswerEvidenceRepository
	documentLogRepo interfaces.DocumentAccessLogRepository
}

func NewPluginEvidenceCapture(
	eventManager *EventManager,
	repo interfaces.AnswerEvidenceRepository,
	documentLogRepo interfaces.DocumentAccessLogRepository,
) *PluginEvidenceCapture {
	res := &PluginEvidenceCapture{repo: repo, documentLogRepo: documentLogRepo}
	eventManager.Register(res)
	return res
}

func (p *PluginEvidenceCapture) ActivationEvents() []types.EventType {
	return []types.EventType{types.EVIDENCE_CAPTURE}
}

func (p *PluginEvidenceCapture) OnEvent(
	ctx context.Context, eventType types.EventType, chatManage *types.ChatManage, next func() *PluginError,
) *PluginError {
	if chatManage.MessageID == "" || chatManage.SessionID == "" {
		return next()
	}

	evidences := buildAnswerEvidence(chatManage)
	bgCtx := context.WithoutCancel(ctx)
	go func() {
		if err := p.repo.ReplaceAnswerEvidence(
			bgCtx, chatManage.TenantID, chatManage.SessionID, chatManage.MessageID, evidences,
		); err != nil {
			pipelineWarn(bgCtx, "EvidenceCapture", "persist_failed", map[string]interface{}{
				"session_id":   chatManage.SessionID,
				"message_id":   chatManage.MessageID,
				"evidence_cnt": len(evidences),
				"error":        err.Error(),
			})
			return
		}
		pipelineInfo(bgCtx, "EvidenceCapture", "persisted", map[string]interface{}{
			"session_id":   chatManage.SessionID,
			"message_id":   chatManage.MessageID,
			"evidence_cnt": len(evidences),
		})
		accessLogs := buildDocumentAccessLogs(chatManage)
		if len(accessLogs) == 0 || p.documentLogRepo == nil {
			return
		}
		if err := p.documentLogRepo.BulkCreate(bgCtx, accessLogs); err != nil {
			pipelineWarn(bgCtx, "EvidenceCapture", "access_log_persist_failed", map[string]interface{}{
				"session_id": chatManage.SessionID,
				"message_id": chatManage.MessageID,
				"log_cnt":    len(accessLogs),
				"error":      err.Error(),
			})
			return
		}
		pipelineInfo(bgCtx, "EvidenceCapture", "access_logs_persisted", map[string]interface{}{
			"session_id": chatManage.SessionID,
			"message_id": chatManage.MessageID,
			"log_cnt":    len(accessLogs),
		})
	}()
	return next()
}

func buildAnswerEvidence(chatManage *types.ChatManage) []*types.AnswerEvidence {
	results := chatManage.MergeResult
	if len(results) == 0 {
		return nil
	}
	evidences := make([]*types.AnswerEvidence, 0, len(results))
	for i, result := range results {
		snapshot, _ := json.Marshal(map[string]interface{}{
			"knowledge_title":       result.KnowledgeTitle,
			"knowledge_filename":    result.KnowledgeFilename,
			"knowledge_source":      result.KnowledgeSource,
			"knowledge_channel":     result.KnowledgeChannel,
			"knowledge_description": result.KnowledgeDescription,
			"chunk_type":            result.ChunkType,
			"matched_content":       result.MatchedContent,
		})
		evidences = append(evidences, &types.AnswerEvidence{
			TenantID:              chatManage.TenantID,
			SessionID:             chatManage.SessionID,
			AnswerMessageID:       chatManage.MessageID,
			SourceKnowledgeID:     result.KnowledgeID,
			SourceKnowledgeBaseID: result.KnowledgeBaseID,
			SourceChunkID:         result.ID,
			SourceTitle:           chooseSourceTitle(result),
			SourceType:            inferSourceType(result),
			SourceChannel:         result.KnowledgeChannel,
			MatchType:             mapMatchType(result.MatchType),
			RetrievalScore:        parseScoreMetadata(result, "base_score", result.Score),
			RerankScore:           parseScoreMetadata(result, "final_score", result.Score),
			Position:              i + 1,
			SourceSnapshot:        types.JSON(snapshot),
		})
	}
	return evidences
}

func parseScoreMetadata(result *types.SearchResult, key string, fallback float64) float64 {
	if result == nil || result.Metadata == nil {
		return fallback
	}
	raw := result.Metadata[key]
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func buildDocumentAccessLogs(chatManage *types.ChatManage) []*types.DocumentAccessLog {
	if chatManage == nil || chatManage.SessionID == "" || chatManage.MessageID == "" {
		return nil
	}

	logs := make([]*types.DocumentAccessLog, 0)
	logs = append(logs, collectDocumentAccessLogs(
		chatManage.TenantID, chatManage.SessionID, chatManage.MessageID, types.DocumentAccessTypeRetrieved, chatManage.SearchResult,
	)...)
	logs = append(logs, collectDocumentAccessLogs(
		chatManage.TenantID, chatManage.SessionID, chatManage.MessageID, types.DocumentAccessTypeReranked, chatManage.RerankResult,
	)...)
	logs = append(logs, collectDocumentAccessLogs(
		chatManage.TenantID, chatManage.SessionID, chatManage.MessageID, types.DocumentAccessTypeCited, chatManage.MergeResult,
	)...)
	return logs
}

func collectDocumentAccessLogs(
	tenantID uint64, sessionID, messageID, accessType string, results []*types.SearchResult,
) []*types.DocumentAccessLog {
	if len(results) == 0 {
		return nil
	}

	logs := make([]*types.DocumentAccessLog, 0, len(results))
	seen := make(map[string]struct{}, len(results))
	for _, result := range results {
		if result == nil || result.KnowledgeID == "" {
			continue
		}
		key := fmt.Sprintf("%s:%s", accessType, result.KnowledgeID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		logs = append(logs, &types.DocumentAccessLog{
			TenantID:    tenantID,
			KnowledgeID: result.KnowledgeID,
			SessionID:   sessionID,
			MessageID:   messageID,
			AccessType:  accessType,
		})
	}
	return logs
}

func chooseSourceTitle(result *types.SearchResult) string {
	if result.KnowledgeTitle != "" {
		return result.KnowledgeTitle
	}
	if result.KnowledgeFilename != "" {
		return result.KnowledgeFilename
	}
	return result.KnowledgeID
}

func inferSourceType(result *types.SearchResult) string {
	if result.ChunkType == string(types.ChunkTypeFAQ) {
		return types.SourceTypeFAQ
	}
	if result.MatchType == types.MatchTypeWebSearch || result.KnowledgeSource == "url" {
		return types.SourceTypeWeb
	}
	return types.SourceTypeDocument
}

func mapMatchType(matchType types.MatchType) string {
	switch matchType {
	case types.MatchTypeEmbedding:
		return "vector"
	case types.MatchTypeKeywords:
		return "keyword"
	case types.MatchTypeNearByChunk:
		return "nearby"
	case types.MatchTypeHistory:
		return "history"
	case types.MatchTypeParentChunk:
		return "parent_chunk"
	case types.MatchTypeRelationChunk:
		return "relation_chunk"
	case types.MatchTypeGraph:
		return "graph"
	case types.MatchTypeWebSearch:
		return "web_search"
	case types.MatchTypeDirectLoad:
		return "direct_load"
	case types.MatchTypeDataAnalysis:
		return "data_analysis"
	default:
		return "unknown"
	}
}
