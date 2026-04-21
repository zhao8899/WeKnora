# Plan B: Knowledge Health Dashboard

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Plan A（answer_evidence + source_feedback）数据基础上，新增 document_access_log 表，并暴露四类分析接口（热点问题、覆盖空白、陈旧文档、引用热力图），前端在设置页面提供管理员专用知识健康看板。

**Architecture:** 扩展 PluginEvidenceCapture 同时写入 document_access_log；新建 AnalyticsRepository 做聚合查询；AnalyticsHandler 暴露四个 GET 接口；前端 KnowledgeHealthDashboard.vue 挂载至设置侧边栏（adminOnly）。看板数据来自三张日志表的聚合，不独立采集数据。

**Tech Stack:** Go, Gin, GORM, PostgreSQL, Vue 3, TypeScript, TDesign Vue Next

**前置依赖：** Plan A 全部完成（answer_evidence + source_feedback 表已建，PluginEvidenceCapture 已注册）

---

## 文件清单

新建：
- `migrations/versioned/000034_document_access_log.up.sql`
- `migrations/versioned/000034_document_access_log.down.sql`
- `internal/models/document_access_log.go`
- `internal/application/repository/analytics.go`
- `internal/application/service/analytics_service.go`
- `internal/handler/analytics.go`
- `frontend/src/api/analytics/index.ts`
- `frontend/src/views/settings/KnowledgeHealthDashboard.vue`

修改：
- `internal/application/service/chat_pipeline/evidence_capture.go` — 新增异步写入 document_access_log
- `internal/container/container.go` — 注入 AnalyticsHandler
- `internal/router/router.go` — 注册 /api/analytics/* 路由
- `frontend/src/views/settings/nav.ts` — 新增 knowledge-health adminOnly 条目

---

## Task 1: DB Migration — document_access_log

**Files:**
- Create: `migrations/versioned/000034_document_access_log.up.sql`
- Create: `migrations/versioned/000034_document_access_log.down.sql`

- [ ] **Step 1: 写入 up migration**

```sql
-- migrations/versioned/000034_document_access_log.up.sql
DO $$ BEGIN RAISE NOTICE '[Migration 000034] Creating document_access_log table'; END $$;

CREATE TABLE IF NOT EXISTS document_access_log (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    knowledge_id UUID        REFERENCES knowledge(id) ON DELETE SET NULL,
    session_id   UUID,
    message_id   TEXT,
    access_type  VARCHAR(20) NOT NULL CHECK (access_type IN ('retrieved', 'reranked', 'cited')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dal_knowledge_id ON document_access_log(knowledge_id);
CREATE INDEX IF NOT EXISTS idx_dal_created_at   ON document_access_log(created_at);
CREATE INDEX IF NOT EXISTS idx_dal_access_type  ON document_access_log(access_type);

COMMENT ON TABLE document_access_log IS '文档访问日志：记录每次检索/rerank/引用事件，用于热力图和覆盖分析';
COMMENT ON COLUMN document_access_log.access_type IS 'retrieved=进入召回结果, reranked=rerank后仍在top-k, cited=被引用进最终回答';

DO $$ BEGIN RAISE NOTICE '[Migration 000034] document_access_log created successfully'; END $$;
```

- [ ] **Step 2: 写入 down migration**

```sql
-- migrations/versioned/000034_document_access_log.down.sql
DROP TABLE IF EXISTS document_access_log;
```

- [ ] **Step 3: 执行迁移**

```bash
make migrate-up
```

Expected: migration 000034 applied without error.

- [ ] **Step 4: 验证表结构**

```bash
psql $DATABASE_URL -c "\d document_access_log"
```

Expected: 看到 id, knowledge_id, session_id, message_id, access_type, created_at 列。

- [ ] **Step 5: Commit**

```bash
git add migrations/versioned/000034_document_access_log.up.sql migrations/versioned/000034_document_access_log.down.sql
git commit -m "feat(migration): add document_access_log table (000034)"
```

---

## Task 2: GORM Model — DocumentAccessLog

**Files:**
- Create: `internal/models/document_access_log.go`

- [ ] **Step 1: 写入模型文件**

```go
// internal/models/document_access_log.go
package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	AccessTypeRetrieved = "retrieved"
	AccessTypeReranked  = "reranked"
	AccessTypeCited     = "cited"
)

type DocumentAccessLog struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	KnowledgeID *uuid.UUID `gorm:"type:uuid;index"`
	SessionID   *uuid.UUID `gorm:"type:uuid"`
	MessageID   string     `gorm:"type:text"`
	AccessType  string     `gorm:"type:varchar(20);not null"`
	CreatedAt   time.Time  `gorm:"not null;default:now()"`
}
```

- [ ] **Step 2: 确认编译**

```bash
make build 2>&1 | head -20
```

Expected: build 成功，无新增错误。

- [ ] **Step 3: Commit**

```bash
git add internal/models/document_access_log.go
git commit -m "feat(models): add DocumentAccessLog GORM model"
```

---

## Task 3: 扩展 PluginEvidenceCapture — 写入 document_access_log

**Files:**
- Modify: `internal/application/service/chat_pipeline/evidence_capture.go`

Plan A Task 6 已创建此文件。本 Task 在其异步 goroutine 中追加 document_access_log 写入。

- [ ] **Step 1: 在 PluginEvidenceCapture 中新增 db 字段**

找到 evidence_capture.go 中 `PluginEvidenceCapture` struct，添加 `db *gorm.DB`：

```go
type PluginEvidenceCapture struct {
	repo *repository.AnswerEvidenceRepository
	db   *gorm.DB
}

func NewPluginEvidenceCapture(
	eventManager *EventManager,
	repo *repository.AnswerEvidenceRepository,
	db *gorm.DB,
) *PluginEvidenceCapture {
	res := &PluginEvidenceCapture{repo: repo, db: db}
	eventManager.Register(res)
	return res
}
```

- [ ] **Step 2: 新增 buildAccessLogs 辅助函数**

在 evidence_capture.go 末尾追加：

```go
func buildAccessLogs(cm *types.ChatManage, evidences []models.AnswerEvidence) []models.DocumentAccessLog {
	if len(evidences) == 0 {
		return nil
	}

	sessionUUID, err := uuid.Parse(string(cm.SessionID))
	if err != nil {
		return nil
	}

	var logs []models.DocumentAccessLog
	for _, ev := range evidences {
		if ev.KnowledgeID == nil {
			continue
		}
		accessType := models.AccessTypeRetrieved
		if ev.RerankScore != nil && *ev.RerankScore > 0 {
			accessType = models.AccessTypeReranked
		}
		if ev.IsCited {
			accessType = models.AccessTypeCited
		}
		logs = append(logs, models.DocumentAccessLog{
			KnowledgeID: ev.KnowledgeID,
			SessionID:   &sessionUUID,
			MessageID:   cm.MessageID,
			AccessType:  accessType,
		})
	}
	return logs
}
```

- [ ] **Step 3: 在 OnEvent 异步 goroutine 中追加写入 document_access_log**

在现有 goroutine 中 `repo.BulkCreate(bgCtx, evidences)` 调用之后追加：

```go
go func() {
	if err := p.repo.BulkCreate(bgCtx, evidences); err != nil {
		pipelineWarn(bgCtx, "EvidenceCapture", "bulk_create_error", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	// 写入 document_access_log
	accessLogs := buildAccessLogs(cm, evidences)
	if len(accessLogs) > 0 {
		if err := p.db.WithContext(bgCtx).Create(&accessLogs).Error; err != nil {
			pipelineWarn(bgCtx, "EvidenceCapture", "access_log_write_error", map[string]interface{}{
				"error": err.Error(),
			})
		}
	}
}()
```

- [ ] **Step 4: 确认编译**

```bash
make build 2>&1 | head -20
```

Expected: build 成功。

- [ ] **Step 5: Commit**

```bash
git add internal/application/service/chat_pipeline/evidence_capture.go
git commit -m "feat(pipeline): write document_access_log from EvidenceCapture"
```

---

## Task 4: AnalyticsRepository — 四类聚合查询

**Files:**
- Create: `internal/application/repository/analytics.go`

- [ ] **Step 1: 写入 repository 文件**

```go
// internal/application/repository/analytics.go
package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// HotQuestion holds a message with its retrieval evidence count.
type HotQuestion struct {
	MessageID     string    `json:"message_id"`
	SessionID     string    `json:"session_id"`
	EvidenceCount int64     `json:"evidence_count"`
	MaxScore      float64   `json:"max_score"`
	CreatedAt     time.Time `json:"created_at"`
}

// CoverageGap holds a message that returned low-confidence evidence.
type CoverageGap struct {
	MessageID string    `json:"message_id"`
	SessionID string    `json:"session_id"`
	MaxScore  float64   `json:"max_score"`
	CreatedAt time.Time `json:"created_at"`
}

// StaleDocument holds a knowledge document that hasn't been updated recently.
type StaleDocument struct {
	KnowledgeID string    `json:"knowledge_id"`
	Title       string    `json:"title"`
	UpdatedAt   time.Time `json:"updated_at"`
	DaysSince   int       `json:"days_since"`
}

// CitationHeat holds the citation count for a document.
type CitationHeat struct {
	KnowledgeID    string `json:"knowledge_id"`
	Title          string `json:"title"`
	CitedCount     int64  `json:"cited_count"`
	RetrievedCount int64  `json:"retrieved_count"`
}

// AnalyticsRepository runs aggregation queries for the health dashboard.
type AnalyticsRepository struct {
	db *gorm.DB
}

// NewAnalyticsRepository creates a new AnalyticsRepository.
func NewAnalyticsRepository(db *gorm.DB) *AnalyticsRepository {
	return &AnalyticsRepository{db: db}
}

// HotQuestions returns the top N messages ranked by evidence retrieval count.
// These represent frequently-asked questions that triggered the most chunk recalls.
func (r *AnalyticsRepository) HotQuestions(ctx context.Context, limit int) ([]HotQuestion, error) {
	var results []HotQuestion
	err := r.db.WithContext(ctx).Raw(`
		SELECT
			ae.message_id,
			ae.session_id::text,
			COUNT(*)               AS evidence_count,
			MAX(ae.rerank_score)   AS max_score,
			MIN(ae.created_at)     AS created_at
		FROM answer_evidence ae
		WHERE ae.created_at >= NOW() - INTERVAL '30 days'
		GROUP BY ae.message_id, ae.session_id
		ORDER BY evidence_count DESC
		LIMIT ?
	`, limit).Scan(&results).Error
	return results, err
}

// CoverageGaps returns messages where the best rerank score was below threshold,
// indicating the knowledge base could not confidently answer the question.
func (r *AnalyticsRepository) CoverageGaps(ctx context.Context, threshold float64, limit int) ([]CoverageGap, error) {
	var results []CoverageGap
	err := r.db.WithContext(ctx).Raw(`
		SELECT
			ae.message_id,
			ae.session_id::text,
			MAX(ae.rerank_score) AS max_score,
			MIN(ae.created_at)   AS created_at
		FROM answer_evidence ae
		WHERE ae.created_at >= NOW() - INTERVAL '30 days'
		GROUP BY ae.message_id, ae.session_id
		HAVING MAX(ae.rerank_score) < ? OR MAX(ae.rerank_score) IS NULL
		ORDER BY created_at DESC
		LIMIT ?
	`, threshold, limit).Scan(&results).Error
	return results, err
}

// StaleDocuments returns knowledge documents that haven't been updated in staleDays days.
func (r *AnalyticsRepository) StaleDocuments(ctx context.Context, staleDays int, limit int) ([]StaleDocument, error) {
	var results []StaleDocument
	err := r.db.WithContext(ctx).Raw(`
		SELECT
			k.id::text                                    AS knowledge_id,
			COALESCE(k.name, k.filename, '')              AS title,
			k.updated_at,
			EXTRACT(DAY FROM NOW() - k.updated_at)::int  AS days_since
		FROM knowledge k
		WHERE k.deleted_at IS NULL
		  AND k.status NOT IN ('archived', 'processing')
		  AND k.updated_at < NOW() - (? * INTERVAL '1 day')
		ORDER BY k.updated_at ASC
		LIMIT ?
	`, staleDays, limit).Scan(&results).Error
	return results, err
}

// CitationHeatmap returns citation and retrieval counts per document, ordered by citations.
func (r *AnalyticsRepository) CitationHeatmap(ctx context.Context, limit int) ([]CitationHeat, error) {
	var results []CitationHeat
	err := r.db.WithContext(ctx).Raw(`
		SELECT
			dal.knowledge_id::text,
			COALESCE(k.name, k.filename, '')                      AS title,
			COUNT(*) FILTER (WHERE dal.access_type = 'cited')     AS cited_count,
			COUNT(*) FILTER (WHERE dal.access_type = 'retrieved') AS retrieved_count
		FROM document_access_log dal
		LEFT JOIN knowledge k ON k.id = dal.knowledge_id
		WHERE dal.created_at >= NOW() - INTERVAL '30 days'
		  AND dal.knowledge_id IS NOT NULL
		GROUP BY dal.knowledge_id, k.name, k.filename
		ORDER BY cited_count DESC
		LIMIT ?
	`, limit).Scan(&results).Error
	return results, err
}
```

- [ ] **Step 2: 确认编译**

```bash
make build 2>&1 | head -20
```

Expected: build 成功。

- [ ] **Step 3: Commit**

```bash
git add internal/application/repository/analytics.go
git commit -m "feat(repo): add AnalyticsRepository for health dashboard queries"
```

---

## Task 5: AnalyticsService

**Files:**
- Create: `internal/application/service/analytics_service.go`

- [ ] **Step 1: 写入 service 文件**

```go
// internal/application/service/analytics_service.go
package service

import (
	"context"

	"github.com/Tencent/WeKnora/internal/application/repository"
)

const (
	defaultHotQuestionsLimit   = 20
	defaultCoverageGapsLimit   = 50
	defaultStaleDocumentsLimit = 50
	defaultHeatmapLimit        = 100
	coverageGapThreshold       = 0.40
	staleDocumentsDays         = 90
)

// AnalyticsService provides knowledge health metrics for the admin dashboard.
type AnalyticsService struct {
	repo *repository.AnalyticsRepository
}

// NewAnalyticsService creates a new AnalyticsService.
func NewAnalyticsService(repo *repository.AnalyticsRepository) *AnalyticsService {
	return &AnalyticsService{repo: repo}
}

// HotQuestions returns top 20 most-retrieved question messages.
func (s *AnalyticsService) HotQuestions(ctx context.Context) ([]repository.HotQuestion, error) {
	return s.repo.HotQuestions(ctx, defaultHotQuestionsLimit)
}

// CoverageGaps returns messages where knowledge base confidence was below 0.40.
func (s *AnalyticsService) CoverageGaps(ctx context.Context) ([]repository.CoverageGap, error) {
	return s.repo.CoverageGaps(ctx, coverageGapThreshold, defaultCoverageGapsLimit)
}

// StaleDocuments returns documents not updated in the last 90 days.
func (s *AnalyticsService) StaleDocuments(ctx context.Context) ([]repository.StaleDocument, error) {
	return s.repo.StaleDocuments(ctx, staleDocumentsDays, defaultStaleDocumentsLimit)
}

// CitationHeatmap returns citation counts per document for the last 30 days.
func (s *AnalyticsService) CitationHeatmap(ctx context.Context) ([]repository.CitationHeat, error) {
	return s.repo.CitationHeatmap(ctx, defaultHeatmapLimit)
}
```

- [ ] **Step 2: 确认编译**

```bash
make build 2>&1 | head -20
```

- [ ] **Step 3: Commit**

```bash
git add internal/application/service/analytics_service.go
git commit -m "feat(service): add AnalyticsService for health dashboard"
```

---

## Task 6: AnalyticsHandler — HTTP 端点

**Files:**
- Create: `internal/handler/analytics.go`

- [ ] **Step 1: 查看现有 handler 的 JSON 应答模式**

```bash
grep -n "c.JSON\|gin.H" internal/handler/knowledge.go | head -10
```

确认实际用的应答方式（`c.JSON(http.StatusOK, gin.H{...})`）。

- [ ] **Step 2: 写入 analytics handler**

```go
// internal/handler/analytics.go
package handler

import (
	"net/http"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/gin-gonic/gin"
)

// AnalyticsHandler exposes knowledge health dashboard endpoints.
type AnalyticsHandler struct {
	svc *service.AnalyticsService
}

// NewAnalyticsHandler creates a new AnalyticsHandler.
func NewAnalyticsHandler(svc *service.AnalyticsService) *AnalyticsHandler {
	return &AnalyticsHandler{svc: svc}
}

// GetHotQuestions godoc
// @Summary      Hot questions
// @Description  Top 20 messages ranked by evidence retrieval count (last 30 days)
// @Tags         analytics
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Router       /api/analytics/hot-questions [get]
func (h *AnalyticsHandler) GetHotQuestions(c *gin.Context) {
	data, err := h.svc.HotQuestions(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// GetCoverageGaps godoc
// @Summary      Coverage gaps
// @Description  Messages where max rerank_score < 0.40, indicating insufficient knowledge coverage
// @Tags         analytics
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Router       /api/analytics/coverage-gaps [get]
func (h *AnalyticsHandler) GetCoverageGaps(c *gin.Context) {
	data, err := h.svc.CoverageGaps(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// GetStaleDocuments godoc
// @Summary      Stale documents
// @Description  Documents not updated in the last 90 days
// @Tags         analytics
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Router       /api/analytics/stale-documents [get]
func (h *AnalyticsHandler) GetStaleDocuments(c *gin.Context) {
	data, err := h.svc.StaleDocuments(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// GetCitationHeatmap godoc
// @Summary      Citation heatmap
// @Description  Citation and retrieval counts per document (last 30 days)
// @Tags         analytics
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Router       /api/analytics/citation-heatmap [get]
func (h *AnalyticsHandler) GetCitationHeatmap(c *gin.Context) {
	data, err := h.svc.CitationHeatmap(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}
```

- [ ] **Step 3: 确认编译**

```bash
make build 2>&1 | head -20
```

- [ ] **Step 4: Commit**

```bash
git add internal/handler/analytics.go
git commit -m "feat(handler): add AnalyticsHandler with four health dashboard endpoints"
```

---

## Task 7: Router + Container 注入

**Files:**
- Modify: `internal/router/router.go`
- Modify: `internal/container/container.go`

- [ ] **Step 1: 在 RouterParams struct 中添加 AnalyticsHandler 字段**

在 `internal/router/router.go` 中找到 `RouterParams` struct，添加：

```go
AnalyticsHandler *handler.AnalyticsHandler
```

- [ ] **Step 2: 在 SetupRouter 函数中注册路由**

找到现有 API 路由注册区域，在 admin 认证中间件下添加（参照 system、tenant 路由所在 group）：

```go
// Knowledge health dashboard — admin only
analytics := api.Group("/analytics")
{
    analytics.GET("/hot-questions",    params.AnalyticsHandler.GetHotQuestions)
    analytics.GET("/coverage-gaps",    params.AnalyticsHandler.GetCoverageGaps)
    analytics.GET("/stale-documents",  params.AnalyticsHandler.GetStaleDocuments)
    analytics.GET("/citation-heatmap", params.AnalyticsHandler.GetCitationHeatmap)
}
```

- [ ] **Step 3: 在 container.go 中注入 AnalyticsRepository、AnalyticsService、AnalyticsHandler**

找到 `internal/container/container.go` 中 `Provide` 调用列表，添加（与现有 repository/service/handler 导入一致）：

```go
// Analytics (health dashboard)
dig.Provide(repository.NewAnalyticsRepository),
dig.Provide(service.NewAnalyticsService),
dig.Provide(handler.NewAnalyticsHandler),
```

- [ ] **Step 4: 确认编译**

```bash
make build 2>&1 | head -20
```

Expected: build 成功。

- [ ] **Step 5: 启动并手工测试接口**

```bash
make run &
curl -s http://localhost:8080/api/analytics/hot-questions | python -m json.tool
```

Expected: `{"data": [...]}` 或 `{"data": null}`（表初始无数据时返回 null 可接受）。

- [ ] **Step 6: Commit**

```bash
git add internal/router/router.go internal/container/container.go
git commit -m "feat(router): register analytics endpoints and wire dependencies"
```

---

## Task 8: 前端 API 模块

**Files:**
- Create: `frontend/src/api/analytics/index.ts`

- [ ] **Step 1: 查看现有 API 模块的 request 导入方式**

```bash
head -5 frontend/src/api/knowledge/index.ts
```

确认 axios 实例导入路径（`import request from '@/utils/request'` 或类似）。

- [ ] **Step 2: 写入 analytics API 模块**

```typescript
// frontend/src/api/analytics/index.ts
import request from '@/utils/request'

export interface HotQuestion {
  message_id: string
  session_id: string
  evidence_count: number
  max_score: number
  created_at: string
}

export interface CoverageGap {
  message_id: string
  session_id: string
  max_score: number
  created_at: string
}

export interface StaleDocument {
  knowledge_id: string
  title: string
  updated_at: string
  days_since: number
}

export interface CitationHeat {
  knowledge_id: string
  title: string
  cited_count: number
  retrieved_count: number
}

export const getHotQuestions = () =>
  request.get<{ data: HotQuestion[] }>('/analytics/hot-questions')

export const getCoverageGaps = () =>
  request.get<{ data: CoverageGap[] }>('/analytics/coverage-gaps')

export const getStaleDocuments = () =>
  request.get<{ data: StaleDocument[] }>('/analytics/stale-documents')

export const getCitationHeatmap = () =>
  request.get<{ data: CitationHeat[] }>('/analytics/citation-heatmap')
```

- [ ] **Step 3: 类型检查**

```bash
cd frontend && npm run type-check 2>&1 | tail -20
```

Expected: 无新增错误。

- [ ] **Step 4: Commit**

```bash
git add frontend/src/api/analytics/index.ts
git commit -m "feat(frontend): add analytics API module"
```

---

## Task 9: 前端看板页面 — KnowledgeHealthDashboard.vue

**Files:**
- Create: `frontend/src/views/settings/KnowledgeHealthDashboard.vue`

看板分四个 Panel：热点问题、覆盖空白、陈旧文档预警、引用热力图。使用 TDesign Vue Next 的 `t-card`、`t-table`、`t-loading`。

- [ ] **Step 1: 写入 Vue 组件**

```vue
<!-- frontend/src/views/settings/KnowledgeHealthDashboard.vue -->
<template>
  <div class="health-dashboard">
    <h2 class="dashboard-title">知识健康看板</h2>
    <p class="dashboard-subtitle">
      数据范围：最近 30 天&nbsp;|&nbsp;仅管理员可见
    </p>

    <div class="panels">
      <t-card title="热点问题 Top20" class="panel">
        <t-loading :loading="loadingHot">
          <t-table
            :data="hotQuestions"
            :columns="hotColumns"
            row-key="message_id"
            size="small"
            stripe
          />
        </t-loading>
      </t-card>

      <t-card title="覆盖空白（置信度 < 40%）" class="panel">
        <t-loading :loading="loadingGaps">
          <t-table
            :data="coverageGaps"
            :columns="gapColumns"
            row-key="message_id"
            size="small"
            stripe
          />
        </t-loading>
      </t-card>

      <t-card title="陈旧文档预警（超过 90 天未更新）" class="panel">
        <t-loading :loading="loadingStale">
          <t-table
            :data="staleDocuments"
            :columns="staleColumns"
            row-key="knowledge_id"
            size="small"
            stripe
          />
        </t-loading>
      </t-card>

      <t-card title="引用热力图（最近 30 天）" class="panel">
        <t-loading :loading="loadingHeat">
          <t-table
            :data="citationHeat"
            :columns="heatColumns"
            row-key="knowledge_id"
            size="small"
            stripe
          />
        </t-loading>
      </t-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import {
  getHotQuestions,
  getCoverageGaps,
  getStaleDocuments,
  getCitationHeatmap,
  type HotQuestion,
  type CoverageGap,
  type StaleDocument,
  type CitationHeat,
} from '@/api/analytics/index'

const hotQuestions  = ref<HotQuestion[]>([])
const coverageGaps  = ref<CoverageGap[]>([])
const staleDocuments = ref<StaleDocument[]>([])
const citationHeat  = ref<CitationHeat[]>([])

const loadingHot   = ref(false)
const loadingGaps  = ref(false)
const loadingStale = ref(false)
const loadingHeat  = ref(false)

const hotColumns = [
  { colKey: 'evidence_count', title: '检索次数', width: 90 },
  {
    colKey: 'max_score',
    title: '最高得分',
    width: 90,
    cell: (_: unknown, { row }: { row: HotQuestion }) =>
      row.max_score != null ? row.max_score.toFixed(3) : '-',
  },
  { colKey: 'message_id', title: '消息 ID', ellipsis: true },
  {
    colKey: 'created_at',
    title: '首次触发',
    width: 170,
    cell: (_: unknown, { row }: { row: HotQuestion }) =>
      new Date(row.created_at).toLocaleString(),
  },
]

const gapColumns = [
  {
    colKey: 'max_score',
    title: '最高得分',
    width: 90,
    cell: (_: unknown, { row }: { row: CoverageGap }) =>
      row.max_score != null ? row.max_score.toFixed(3) : '无',
  },
  { colKey: 'message_id', title: '消息 ID', ellipsis: true },
  {
    colKey: 'created_at',
    title: '时间',
    width: 170,
    cell: (_: unknown, { row }: { row: CoverageGap }) =>
      new Date(row.created_at).toLocaleString(),
  },
]

const staleColumns = [
  { colKey: 'title', title: '文档标题', ellipsis: true },
  { colKey: 'days_since', title: '未更新天数', width: 110 },
  {
    colKey: 'updated_at',
    title: '最后更新',
    width: 170,
    cell: (_: unknown, { row }: { row: StaleDocument }) =>
      new Date(row.updated_at).toLocaleDateString(),
  },
]

const heatColumns = [
  { colKey: 'title', title: '文档标题', ellipsis: true },
  { colKey: 'cited_count', title: '被引用次数', width: 110 },
  { colKey: 'retrieved_count', title: '被检索次数', width: 110 },
]

async function load<T>(
  loadingRef: { value: boolean },
  dataRef: { value: T[] },
  fetcher: () => Promise<{ data: { data: T[] } }>,
) {
  loadingRef.value = true
  try {
    const res = await fetcher()
    dataRef.value = res.data?.data ?? []
  } finally {
    loadingRef.value = false
  }
}

onMounted(() => {
  load(loadingHot,   hotQuestions,   getHotQuestions)
  load(loadingGaps,  coverageGaps,   getCoverageGaps)
  load(loadingStale, staleDocuments, getStaleDocuments)
  load(loadingHeat,  citationHeat,   getCitationHeatmap)
})
</script>

<style scoped>
.health-dashboard {
  padding: 24px;
}
.dashboard-title {
  font-size: 20px;
  font-weight: 600;
  margin-bottom: 4px;
}
.dashboard-subtitle {
  color: var(--td-text-color-secondary);
  margin-bottom: 24px;
  font-size: 13px;
}
.panels {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 20px;
}
.panel {
  min-height: 240px;
}
@media (max-width: 1100px) {
  .panels {
    grid-template-columns: 1fr;
  }
}
</style>
```

- [ ] **Step 2: 类型检查**

```bash
cd frontend && npm run type-check 2>&1 | tail -20
```

Fix any type errors before continuing.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/views/settings/KnowledgeHealthDashboard.vue
git commit -m "feat(frontend): add KnowledgeHealthDashboard settings page"
```

---

## Task 10: 设置导航 + 路由注册

**Files:**
- Modify: `frontend/src/views/settings/nav.ts`
- Modify: settings router file (locate with step below)

- [ ] **Step 1: 查找 settings 路由配置文件**

```bash
grep -rn "knowledge-health\|KnowledgeHealth\|settings" frontend/src/router/ | head -20
```

确认 settings 子路由注册位置。

- [ ] **Step 2: 在 nav.ts 中添加看板入口**

在 `frontend/src/views/settings/nav.ts` 的 `allItems` 数组末尾（`api` 条目之后）添加：

```typescript
{
  key: 'knowledge-health',
  label: '知识健康看板',
  icon: 'chart-bar',
  adminOnly: true,
},
```

- [ ] **Step 3: 注册 settings 子路由**

找到 settings 路由配置文件，添加：

```typescript
{
  path: 'knowledge-health',
  name: 'settings-knowledge-health',
  component: () => import('@/views/settings/KnowledgeHealthDashboard.vue'),
},
```

如果 settings 页面使用 key-based 动态 component 而非静态路由，则参照现有 `system`、`tenant` 页面的挂载模式，以相同方式添加 `knowledge-health` case。

- [ ] **Step 4: 类型检查**

```bash
cd frontend && npm run type-check 2>&1 | tail -20
```

- [ ] **Step 5: 启动前端开发服务器，验证导航**

```bash
cd frontend && npm run dev
```

以管理员账户登录 → 设置 → 确认左侧导航出现"知识健康看板"条目 → 点击后显示四个面板（数据为空时显示空表格，属正常行为）。

- [ ] **Step 6: Commit**

```bash
git add frontend/src/views/settings/nav.ts frontend/src/router/
git commit -m "feat(frontend): add knowledge-health nav entry and route in settings"
```

---

## Self-Review

### Spec coverage

| 规格要求 | 对应 Task |
|---------|----------|
| document_access_log 表 | Task 1 |
| DocumentAccessLog GORM 模型 | Task 2 |
| EvidenceCapture 写入 access_log | Task 3 |
| 热点问题查询（Top 20） | Task 4 HotQuestions |
| 覆盖空白查询（score < 0.4） | Task 4 CoverageGaps |
| 陈旧文档预警（> 90 天） | Task 4 StaleDocuments |
| 引用热力图（cited 次数） | Task 4 CitationHeatmap |
| AnalyticsService 封装 | Task 5 |
| HTTP 端点 × 4 | Task 6 |
| Router + DI 注入 | Task 7 |
| 前端 TypeScript API 模块 | Task 8 |
| 看板 Vue 页面（四 Panel） | Task 9 |
| adminOnly 设置导航 + 路由 | Task 10 |

### Placeholder scan

无 TBD/TODO/填写说明。所有 step 含实际代码或可执行命令。

### Type consistency

- `AnalyticsRepository` → `AnalyticsService` → `AnalyticsHandler` 命名链路一致
- Repository 返回 `[]HotQuestion` 等具体类型，Service 透传，Handler 包装为 `gin.H{"data": data}`
- 前端 `HotQuestion` interface 字段与 Go struct json tag 对应（均为 snake_case）
- `models.AccessTypeRetrieved/Reranked/Cited` 常量值与 migration CHECK 约束值一致
- `buildAccessLogs` 依赖 Plan A 中 `AnswerEvidence.IsCited bool` 和 `AnswerEvidence.RerankScore *float64`——Task 3 实现前须确认 Plan A Task 6 已完成

---

## Execution Notes

**数据积累说明：** 看板在数据积累 2 周之前，四个表格将显示空数据或少量数据，这是正常行为。建议在 Plan A 上线 2 周后才向管理员推广此看板。

**数据库兼容：** 所有 RAW SQL 使用 PostgreSQL 语法（`gen_random_uuid()`、`INTERVAL`、`FILTER (WHERE ...)`），仅适用于 `RETRIEVE_DRIVER=postgres` 环境。
