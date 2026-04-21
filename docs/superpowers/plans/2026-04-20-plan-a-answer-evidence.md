# Plan A: Answer Evidence Layer

> **For agentic workers:** Use superpowers:subagent-driven-development or superpowers:executing-plans to implement task-by-task.

**Goal:** 在现有召回链路末端写入答案证据，不改检索逻辑，暴露 /confidence 接口，添加来源级反馈 source_feedback，使置信度和来源运营数据可查询。

**Architecture:** 新增 PluginEvidenceCapture 注册 EVIDENCE_CAPTURE 事件（INTO_CHAT_MESSAGE 之后），异步将 MergeResult 得分写入 answer_evidence 表。新增 /confidence 和 /feedback 端点，每日任务更新 knowledge.source_weight。

**Tech Stack:** Go, Gin, GORM, PostgreSQL, Vue 3, TypeScript, TDesign Vue Next

---

## 文件清单

新建：
- migrations/versioned/000032_answer_evidence.up.sql
- migrations/versioned/000032_answer_evidence.down.sql
- migrations/versioned/000033_source_feedback_weight.up.sql
- migrations/versioned/000033_source_feedback_weight.down.sql
- internal/models/answer_evidence.go
- internal/models/source_feedback.go
- internal/application/repository/answer_evidence.go
- internal/application/service/chat_pipeline/evidence_capture.go
- internal/application/service/confidence_service.go
- internal/application/service/source_weight_updater.go
- internal/handler/confidence.go
- frontend/src/api/confidence/index.ts
- frontend/src/components/ConfidencePanel.vue

修改：
- internal/types/chat_manage.go
- internal/container/container.go
- internal/router/router.go
- frontend/src/views/chat/index.vue

---

## Task 1: DB Migration — answer_evidence

- [ ] 写入 migrations/versioned/000032_answer_evidence.up.sql：

```sql
DO $$ BEGIN RAISE NOTICE '[Migration 000032] Creating answer_evidence table'; END $$;
CREATE TABLE IF NOT EXISTS answer_evidence (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id      UUID        NOT NULL,
    message_id      TEXT        NOT NULL,
    knowledge_id    UUID        REFERENCES knowledge(id) ON DELETE SET NULL,
    chunk_id        TEXT,
    vector_score    FLOAT,
    keyword_hit     BOOLEAN     NOT NULL DEFAULT false,
    rerank_score    FLOAT,
    match_type      VARCHAR(20),
    source_url      TEXT,
    source_channel  VARCHAR(50),
    is_cited        BOOLEAN     NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_answer_evidence_message_id   ON answer_evidence(message_id);
CREATE INDEX IF NOT EXISTS idx_answer_evidence_knowledge_id ON answer_evidence(knowledge_id);
CREATE INDEX IF NOT EXISTS idx_answer_evidence_created_at   ON answer_evidence(created_at);
DO $$ BEGIN RAISE NOTICE '[Migration 000032] done'; END $$;
```

- [ ] 写入 migrations/versioned/000032_answer_evidence.down.sql：

```sql
DROP TABLE IF EXISTS answer_evidence;
```

- [ ] 执行并验证：

```bash
make migrate-up
psql $DATABASE_URL -c "\d answer_evidence"
```

- [ ] Commit：`git add migrations/versioned/000032_answer_evidence.* && git commit -m "feat(migration): add answer_evidence table"`

---

## Task 2: DB Migration — source_feedback + source_weight

- [ ] 写入 migrations/versioned/000033_source_feedback_weight.up.sql：

```sql
DO $$ BEGIN RAISE NOTICE '[Migration 000033] Creating source_feedback and weight columns'; END $$;
CREATE TABLE IF NOT EXISTS source_feedback (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id      UUID        NOT NULL,
    message_id      TEXT        NOT NULL,
    evidence_id     UUID        REFERENCES answer_evidence(id) ON DELETE CASCADE,
    knowledge_id    UUID        REFERENCES knowledge(id) ON DELETE SET NULL,
    chunk_id        TEXT,
    feedback_type   VARCHAR(30) NOT NULL,
    notes           TEXT,
    user_id         UUID,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_source_feedback_knowledge_id ON source_feedback(knowledge_id);
CREATE INDEX IF NOT EXISTS idx_source_feedback_message_id   ON source_feedback(message_id);
CREATE INDEX IF NOT EXISTS idx_source_feedback_type         ON source_feedback(feedback_type);
ALTER TABLE knowledge ADD COLUMN IF NOT EXISTS source_weight  FLOAT   NOT NULL DEFAULT 1.0;
ALTER TABLE knowledge ADD COLUMN IF NOT EXISTS freshness_flag BOOLEAN NOT NULL DEFAULT false;
DO $$ BEGIN RAISE NOTICE '[Migration 000033] done'; END $$;
```

- [ ] 写入 migrations/versioned/000033_source_feedback_weight.down.sql：

```sql
DROP TABLE IF EXISTS source_feedback;
ALTER TABLE knowledge DROP COLUMN IF EXISTS source_weight;
ALTER TABLE knowledge DROP COLUMN IF EXISTS freshness_flag;
```

- [ ] 执行并验证：

```bash
make migrate-up
psql $DATABASE_URL -c "\d source_feedback"
psql $DATABASE_URL -c "\d knowledge" | grep -E "source_weight|freshness_flag"
```

- [ ] Commit：`git add migrations/versioned/000033_source_feedback_weight.* && git commit -m "feat(migration): add source_feedback table and knowledge weight columns"`

---

## Task 3: GORM Models

- [ ] 写入 internal/models/answer_evidence.go：

```go
package models

import (
	"time"
	"github.com/google/uuid"
)

type AnswerEvidence struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SessionID     uuid.UUID  `gorm:"type:uuid;not null"                            json:"session_id"`
	MessageID     string     `gorm:"not null"                                      json:"message_id"`
	KnowledgeID   *uuid.UUID `gorm:"type:uuid"                                     json:"knowledge_id,omitempty"`
	ChunkID       string     `gorm:"column:chunk_id"                               json:"chunk_id,omitempty"`
	VectorScore   float64    `gorm:"column:vector_score"                           json:"vector_score"`
	KeywordHit    bool       `gorm:"column:keyword_hit;default:false"              json:"keyword_hit"`
	RerankScore   float64    `gorm:"column:rerank_score"                           json:"rerank_score"`
	MatchType     string     `gorm:"column:match_type"                             json:"match_type"`
	SourceURL     string     `gorm:"column:source_url"                             json:"source_url,omitempty"`
	SourceChannel string     `gorm:"column:source_channel"                         json:"source_channel,omitempty"`
	IsCited       bool       `gorm:"column:is_cited;default:false"                 json:"is_cited"`
	CreatedAt     time.Time  `gorm:"autoCreateTime"                                json:"created_at"`
}

func (AnswerEvidence) TableName() string { return "answer_evidence" }
```

- [ ] 写入 internal/models/source_feedback.go：

```go
package models

import (
	"time"
	"github.com/google/uuid"
)

type FeedbackType string

const (
	FeedbackAccurate   FeedbackType = "accurate"
	FeedbackPartial    FeedbackType = "partial"
	FeedbackWrong      FeedbackType = "wrong"
	FeedbackExpired    FeedbackType = "expired"
	FeedbackUnclear    FeedbackType = "unclear"
	FeedbackCorrection FeedbackType = "correction"
)

type SourceFeedback struct {
	ID           uuid.UUID    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SessionID    uuid.UUID    `gorm:"type:uuid;not null"                            json:"session_id"`
	MessageID    string       `gorm:"not null"                                      json:"message_id"`
	EvidenceID   *uuid.UUID   `gorm:"type:uuid"                                     json:"evidence_id,omitempty"`
	KnowledgeID  *uuid.UUID   `gorm:"type:uuid"                                     json:"knowledge_id,omitempty"`
	ChunkID      string       `gorm:"column:chunk_id"                               json:"chunk_id,omitempty"`
	FeedbackType FeedbackType `gorm:"not null"                                      json:"feedback_type"`
	Notes        string       `gorm:"column:notes"                                  json:"notes,omitempty"`
	UserID       *uuid.UUID   `gorm:"type:uuid"                                     json:"user_id,omitempty"`
	CreatedAt    time.Time    `gorm:"autoCreateTime"                                json:"created_at"`
}

func (SourceFeedback) TableName() string { return "source_feedback" }
```

- [ ] 验证编译：`go build ./internal/models/...`

- [ ] Commit：`git add internal/models/answer_evidence.go internal/models/source_feedback.go && git commit -m "feat(models): add AnswerEvidence and SourceFeedback"`

---

## Task 4: Repository

- [ ] 写入 internal/application/repository/answer_evidence.go：

```go
package repository

import (
	"context"
	"github.com/Tencent/WeKnora/internal/models"
	"gorm.io/gorm"
)

type AnswerEvidenceRepository struct{ db *gorm.DB }

func NewAnswerEvidenceRepository(db *gorm.DB) *AnswerEvidenceRepository {
	return &AnswerEvidenceRepository{db: db}
}

func (r *AnswerEvidenceRepository) BulkCreate(ctx context.Context, records []models.AnswerEvidence) error {
	if len(records) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&records).Error
}

func (r *AnswerEvidenceRepository) FindByMessageID(ctx context.Context, messageID string) ([]models.AnswerEvidence, error) {
	var records []models.AnswerEvidence
	err := r.db.WithContext(ctx).
		Where("message_id = ?", messageID).
		Order("rerank_score DESC").
		Find(&records).Error
	return records, err
}

func (r *AnswerEvidenceRepository) CreateFeedback(ctx context.Context, fb models.SourceFeedback) error {
	return r.db.WithContext(ctx).Create(&fb).Error
}
```

- [ ] 验证：`go build ./internal/application/repository/...`

- [ ] Commit：`git add internal/application/repository/answer_evidence.go && git commit -m "feat(repository): add AnswerEvidenceRepository"`

---

## Task 5: 新增 EVIDENCE_CAPTURE 事件

修改 internal/types/chat_manage.go：

- [ ] 在 FILTER_TOP_K 常量行之后添加：

```go
EVIDENCE_CAPTURE EventType = "evidence_capture"
```

- [ ] 在 "rag_stream" pipeline 的 INTO_CHAT_MESSAGE 之后加入 EVIDENCE_CAPTURE：

```go
INTO_CHAT_MESSAGE,
EVIDENCE_CAPTURE,
CHAT_COMPLETION_STREAM,
```

- [ ] 验证：`go build ./internal/types/...`

- [ ] Commit：`git add internal/types/chat_manage.go && git commit -m "feat(pipeline): add EVIDENCE_CAPTURE event"`

---

## Task 6: EvidenceCapture Plugin

- [ ] 先检查 MatchType 常量实际值：

```bash
grep -n "MatchType\|keyword\|hybrid" internal/types/search.go | head -15
```

- [ ] 写入 internal/application/service/chat_pipeline/evidence_capture.go：

```go
package chatpipeline

import (
	"context"
	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/models"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
)

type PluginEvidenceCapture struct {
	repo *repository.AnswerEvidenceRepository
}

func NewPluginEvidenceCapture(em *EventManager, repo *repository.AnswerEvidenceRepository) *PluginEvidenceCapture {
	p := &PluginEvidenceCapture{repo: repo}
	em.Register(p)
	return p
}

func (p *PluginEvidenceCapture) ActivationEvents() []types.EventType {
	return []types.EventType{types.EVIDENCE_CAPTURE}
}

func (p *PluginEvidenceCapture) OnEvent(ctx context.Context, _ types.EventType, cm *types.ChatManage, next func() *PluginError) *PluginError {
	if cm.MessageID == "" || len(cm.MergeResult) == 0 {
		return next()
	}
	sid, err := uuid.Parse(cm.SessionID)
	if err != nil {
		return next()
	}
	records := make([]models.AnswerEvidence, 0, len(cm.MergeResult))
	for _, r := range cm.MergeResult {
		ev := models.AnswerEvidence{
			SessionID:     sid,
			MessageID:     cm.MessageID,
			ChunkID:       r.ID,
			VectorScore:   r.Score,
			KeywordHit:    string(r.MatchType) == "keyword" || string(r.MatchType) == "hybrid",
			RerankScore:   r.Score,
			MatchType:     string(r.MatchType),
			SourceURL:     r.KnowledgeSource,
			SourceChannel: r.KnowledgeChannel,
			IsCited:       true,
		}
		if r.KnowledgeID != "" {
			if kid, err := uuid.Parse(r.KnowledgeID); err == nil {
				ev.KnowledgeID = &kid
			}
		}
		records = append(records, ev)
	}
	bgCtx := context.WithoutCancel(ctx)
	go func() {
		if err := p.repo.BulkCreate(bgCtx, records); err != nil {
			pipelineWarn(bgCtx, "EvidenceCapture", "write_error", map[string]interface{}{
				"message_id": cm.MessageID, "error": err.Error(),
			})
		}
	}()
	return next()
}
```

注意：根据 Task 6 Step 1 查到的实际 MatchType 常量值，调整 KeywordHit 判断中的字符串字面量。

- [ ] 验证：`go build ./internal/application/service/chat_pipeline/...`

- [ ] Commit：`git add internal/application/service/chat_pipeline/evidence_capture.go && git commit -m "feat(pipeline): add PluginEvidenceCapture"`

---

## Task 7: Confidence Service

- [ ] 写入 internal/application/service/confidence_service.go：

```go
package service

import (
	"context"
	"math"
	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/models"
)

type ConfidenceDetail struct {
	MessageID   string         `json:"message_id"`
	Score       float64        `json:"score"`
	Level       string         `json:"level"`
	SourceCount int            `json:"source_count"`
	Evidence    []EvidenceItem `json:"evidence"`
}

type EvidenceItem struct {
	EvidenceID    string  `json:"evidence_id"`
	KnowledgeID   string  `json:"knowledge_id,omitempty"`
	VectorScore   float64 `json:"vector_score"`
	KeywordHit    bool    `json:"keyword_hit"`
	RerankScore   float64 `json:"rerank_score"`
	SourceChannel string  `json:"source_channel,omitempty"`
	SourceURL     string  `json:"source_url,omitempty"`
	IsCited       bool    `json:"is_cited"`
}

type ConfidenceService struct {
	repo *repository.AnswerEvidenceRepository
}

func NewConfidenceService(repo *repository.AnswerEvidenceRepository) *ConfidenceService {
	return &ConfidenceService{repo: repo}
}

func (s *ConfidenceService) GetConfidence(ctx context.Context, messageID string) (*ConfidenceDetail, error) {
	records, err := s.repo.FindByMessageID(ctx, messageID)
	if err != nil {
		return nil, err
	}
	var cited []models.AnswerEvidence
	for _, r := range records {
		if r.IsCited {
			cited = append(cited, r)
		}
	}
	score := computeConfidenceScore(cited)
	return &ConfidenceDetail{
		MessageID:   messageID,
		Score:       score,
		Level:       confidenceLevel(score),
		SourceCount: len(cited),
		Evidence:    evidenceItems(records),
	}, nil
}

func computeConfidenceScore(cited []models.AnswerEvidence) float64 {
	if len(cited) == 0 {
		return 0
	}
	var totalVec, totalKw float64
	for _, r := range cited {
		totalVec += r.VectorScore
		if r.KeywordHit {
			totalKw++
		}
	}
	n := float64(len(cited))
	score := 0.40*(totalVec/n) + 0.25*(totalKw/n) + 0.20 + 0.15
	if len(cited) >= 2 {
		score = math.Min(score+0.10, 1.0)
	}
	return math.Round(score*1000) / 1000
}

func confidenceLevel(score float64) string {
	switch {
	case score >= 0.85:
		return "high"
	case score >= 0.60:
		return "medium"
	case score >= 0.40:
		return "low"
	default:
		return "insufficient"
	}
}

func evidenceItems(records []models.AnswerEvidence) []EvidenceItem {
	items := make([]EvidenceItem, len(records))
	for i, r := range records {
		items[i] = EvidenceItem{
			EvidenceID:    r.ID.String(),
			VectorScore:   r.VectorScore,
			KeywordHit:    r.KeywordHit,
			RerankScore:   r.RerankScore,
			SourceChannel: r.SourceChannel,
			SourceURL:     r.SourceURL,
			IsCited:       r.IsCited,
		}
		if r.KnowledgeID != nil {
			items[i].KnowledgeID = r.KnowledgeID.String()
		}
	}
	return items
}
```

- [ ] 验证：`go build ./internal/application/service/...`

- [ ] Commit：`git add internal/application/service/confidence_service.go && git commit -m "feat(service): add ConfidenceService"`

---

## Task 8: HTTP Handler + Routes

- [ ] 写入 internal/handler/confidence.go：

```go
package handler

import (
	"net/http"
	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ConfidenceHandler struct {
	svc  *service.ConfidenceService
	repo *repository.AnswerEvidenceRepository
}

func NewConfidenceHandler(svc *service.ConfidenceService, repo *repository.AnswerEvidenceRepository) *ConfidenceHandler {
	return &ConfidenceHandler{svc: svc, repo: repo}
}

func (h *ConfidenceHandler) GetConfidence(c *gin.Context) {
	messageID := c.Param("messageID")
	if messageID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "messageID required"})
		return
	}
	detail, err := h.svc.GetConfidence(c.Request.Context(), messageID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, detail)
}

type feedbackRequest struct {
	EvidenceID   string `json:"evidence_id"`
	KnowledgeID  string `json:"knowledge_id"`
	FeedbackType string `json:"feedback_type" binding:"required"`
	Notes        string `json:"notes"`
}

func (h *ConfidenceHandler) SubmitFeedback(c *gin.Context) {
	messageID := c.Param("messageID")
	var req feedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	valid := map[string]bool{"accurate": true, "partial": true, "wrong": true, "expired": true, "unclear": true, "correction": true}
	if !valid[req.FeedbackType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid feedback_type"})
		return
	}
	fb := models.SourceFeedback{MessageID: messageID, FeedbackType: models.FeedbackType(req.FeedbackType), Notes: req.Notes}
	if req.EvidenceID != "" {
		if eid, err := uuid.Parse(req.EvidenceID); err == nil {
			fb.EvidenceID = &eid
		}
	}
	if req.KnowledgeID != "" {
		if kid, err := uuid.Parse(req.KnowledgeID); err == nil {
			fb.KnowledgeID = &kid
		}
	}
	if err := h.repo.CreateFeedback(c.Request.Context(), fb); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "ok"})
}
```

- [ ] 在 internal/router/router.go 的 RouterParams 中添加：

```go
ConfidenceHandler *handler.ConfidenceHandler
```

在认证路由组注册（跟随现有 message 路由风格）：

```go
answerGroup := api.Group("/chat/answer")
{
    answerGroup.GET("/:messageID/confidence", p.ConfidenceHandler.GetConfidence)
    answerGroup.POST("/:messageID/feedback",  p.ConfidenceHandler.SubmitFeedback)
}
```

- [ ] 在 internal/container/container.go 中添加：

```go
container.Provide(repository.NewAnswerEvidenceRepository),
container.Provide(service.NewConfidenceService),
container.Provide(handler.NewConfidenceHandler),
container.Provide(chatpipeline.NewPluginEvidenceCapture),
```

- [ ] 全量编译：`go build ./...`

- [ ] Smoke test：

```bash
make dev-app
# 发一次 RAG 问答，取 assistant message ID，然后：
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/chat/answer/$MSG_ID/confidence
# 期望：{"message_id":"...","score":0.xx,"level":"...","source_count":N,"evidence":[...]}

curl -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"feedback_type":"accurate"}' \
  http://localhost:8080/api/chat/answer/$MSG_ID/feedback
# 期望：{"status":"ok"}
```

- [ ] Commit：`git add internal/handler/confidence.go internal/router/router.go internal/container/container.go && git commit -m "feat(handler): add confidence and feedback endpoints"`

---

## Task 9: source_weight Daily Updater

- [ ] 写入 internal/application/service/source_weight_updater.go：

```go
package service

import (
	"context"
	"gorm.io/gorm"
)

type SourceWeightUpdater struct{ db *gorm.DB }

func NewSourceWeightUpdater(db *gorm.DB) *SourceWeightUpdater {
	return &SourceWeightUpdater{db: db}
}

func (u *SourceWeightUpdater) Run(ctx context.Context) error {
	return u.db.WithContext(ctx).Exec(`
		UPDATE knowledge k
		SET source_weight = GREATEST(0.1, LEAST(2.0,
			1.0
			+ (SELECT COUNT(*) FROM source_feedback sf
			   WHERE sf.knowledge_id = k.id AND sf.feedback_type = 'accurate'
			     AND sf.created_at > NOW() - INTERVAL '30 days') * 0.02
			- (SELECT COUNT(*) FROM source_feedback sf
			   WHERE sf.knowledge_id = k.id AND sf.feedback_type = 'wrong'
			     AND sf.created_at > NOW() - INTERVAL '30 days') * 0.05
		))
		WHERE EXISTS (SELECT 1 FROM source_feedback sf WHERE sf.knowledge_id = k.id)
	`).Error
}

func (u *SourceWeightUpdater) MarkFreshnessFlags(ctx context.Context) error {
	return u.db.WithContext(ctx).Exec(`
		UPDATE knowledge SET freshness_flag = true
		WHERE id IN (
			SELECT DISTINCT knowledge_id FROM source_feedback
			WHERE feedback_type = 'expired' AND created_at > NOW() - INTERVAL '7 days'
		) AND freshness_flag = false
	`).Error
}
```

- [ ] 在 cmd/server/main.go 启动后添加每日调度（参考现有 goroutine 风格）：

```go
go func() {
    ticker := time.NewTicker(24 * time.Hour)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            bgCtx := context.Background()
            if err := weightUpdater.Run(bgCtx); err != nil {
                logger.Errorf(bgCtx, "source weight update error: %v", err)
            }
            weightUpdater.MarkFreshnessFlags(bgCtx)
        case <-ctx.Done():
            return
        }
    }
}()
```

- [ ] 编译：`go build ./...`

- [ ] Commit：`git add internal/application/service/source_weight_updater.go cmd/server/main.go && git commit -m "feat(service): add daily source weight updater"`

---

## Task 10: Frontend ConfidencePanel

- [ ] 确认 request 工具路径：

```bash
grep -rn "export.*request" frontend/src/utils/ | head -5
```

- [ ] 写入 frontend/src/api/confidence/index.ts：

```typescript
import { request } from '@/utils/request'

export interface EvidenceItem {
  evidence_id: string
  knowledge_id?: string
  vector_score: number
  keyword_hit: boolean
  rerank_score: number
  source_channel?: string
  source_url?: string
  is_cited: boolean
}

export interface ConfidenceDetail {
  message_id: string
  score: number
  level: 'high' | 'medium' | 'low' | 'insufficient'
  source_count: number
  evidence: EvidenceItem[]
}

export interface FeedbackRequest {
  evidence_id?: string
  knowledge_id?: string
  feedback_type: 'accurate' | 'partial' | 'wrong' | 'expired' | 'unclear' | 'correction'
  notes?: string
}

export const getConfidence = (messageID: string) =>
  request.get<ConfidenceDetail>(`/api/chat/answer/${messageID}/confidence`)

export const submitFeedback = (messageID: string, data: FeedbackRequest) =>
  request.post(`/api/chat/answer/${messageID}/feedback`, data)
```

- [ ] 写入 frontend/src/components/ConfidencePanel.vue：

```vue
<template>
  <div v-if="detail" class="confidence-panel">
    <div class="score-row">
      <t-progress :percentage="pct" :color="levelColor" size="small" :label="false" style="width:80px" />
      <span class="score-num" :style="{ color: levelColor }">{{ pct }}%</span>
      <span class="source-hint">综合了 {{ detail.source_count }} 个来源</span>
    </div>
    <div class="level-text">{{ levelLabel }}</div>
    <div class="evidence-list">
      <div v-for="ev in citedEvidence" :key="ev.evidence_id" class="ev-item">
        <t-icon name="file-1" size="13px" />
        <span class="ev-channel">{{ ev.source_channel || 'upload' }}</span>
        <t-tag size="small" :theme="ev.keyword_hit ? 'primary' : 'default'" variant="light">
          {{ ev.keyword_hit ? '关键词' : '语义' }}
        </t-tag>
      </div>
    </div>
    <div class="feedback-row">
      <t-button v-for="fb in feedbackOptions" :key="fb.type"
        size="small" variant="text" :disabled="submitted" @click="onFeedback(fb.type)">
        {{ fb.label }}
      </t-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { getConfidence, submitFeedback } from '@/api/confidence'
import type { ConfidenceDetail, FeedbackRequest } from '@/api/confidence'

const props = defineProps<{ messageId: string }>()
const detail = ref<ConfidenceDetail | null>(null)
const submitted = ref(false)
const pct = computed(() => Math.round((detail.value?.score ?? 0) * 100))

const levelColor = computed(() => ({
  high: '#00a870', medium: '#0052d9', low: '#ed7b2f', insufficient: '#8c8c8c',
}[detail.value?.level ?? 'insufficient']))

const levelLabel = computed(() => ({
  high: '高度可信，来自权威文档', medium: '可参考，建议核对来源',
  low: '仅供参考，知识库覆盖有限', insufficient: '知识库中未找到可靠依据',
}[detail.value?.level ?? 'insufficient']))

const citedEvidence = computed(() => detail.value?.evidence.filter(e => e.is_cited) ?? [])

const feedbackOptions = [
  { type: 'accurate' as const, label: '👍 准确' },
  { type: 'wrong' as const,    label: '✏️ 有误' },
  { type: 'expired' as const,  label: '📅 已过期' },
  { type: 'unclear' as const,  label: '❓ 难懂' },
]

async function onFeedback(type: FeedbackRequest['feedback_type']) {
  if (submitted.value) return
  submitted.value = true
  await submitFeedback(props.messageId, { feedback_type: type })
}

onMounted(async () => {
  try {
    const res = await getConfidence(props.messageId)
    detail.value = res.data
  } catch { /* non-critical */ }
})
</script>

<style scoped>
.confidence-panel { padding: 8px 12px; border-top: 1px solid var(--td-border-level-2-color); font-size: 12px; color: var(--td-text-color-secondary); }
.score-row { display: flex; align-items: center; gap: 8px; margin-bottom: 4px; }
.score-num { font-weight: 600; font-size: 13px; }
.level-text { margin-bottom: 6px; }
.evidence-list { margin-bottom: 6px; }
.ev-item { display: flex; align-items: center; gap: 5px; padding: 2px 0; }
.ev-channel { flex: 1; font-size: 11px; }
.feedback-row { display: flex; gap: 2px; flex-wrap: wrap; }
</style>
```

- [ ] 在 frontend/src/views/chat/index.vue 的 script setup 中：

```typescript
import ConfidencePanel from '@/components/ConfidencePanel.vue'
```

在 assistant 消息模板末尾（msg 为实际循环变量名）：

```vue
<ConfidencePanel v-if="msg.role === 'assistant' && msg.id" :message-id="msg.id" />
```

- [ ] 类型检查：`cd frontend && npm run type-check`

- [ ] 启动并验证：`npm run dev` — 发送 RAG 问答，确认置信度面板出现在回答下方

- [ ] Commit：`git add frontend/src/api/confidence/ frontend/src/components/ConfidencePanel.vue frontend/src/views/chat/index.vue && git commit -m "feat(frontend): add ConfidencePanel"`

---

## 自检清单

- [ ] answer_evidence / source_feedback 表存在，knowledge 有 source_weight 列
- [ ] RAG 问答后 `SELECT COUNT(*) FROM answer_evidence WHERE message_id='$MSG_ID'` > 0
- [ ] GET /confidence 返回 score 在 0-1 之间的有效 JSON
- [ ] POST /feedback 写入 source_feedback 表
- [ ] 前端 ConfidencePanel 在回答下方正确渲染
- [ ] `go build ./...` 无错误
- [ ] `npm run type-check` 无错误
