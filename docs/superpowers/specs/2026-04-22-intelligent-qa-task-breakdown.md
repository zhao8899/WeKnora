# 智能问答任务拆解单

**日期：** 2026-04-22  
**状态：** 研发任务草案  
**用途：** 用于排期、拆 Jira、定义验收

关联文档：

- [智能问答需求与架构评审](/D:/workSpace/WeKnora/docs/superpowers/specs/2026-04-22-intelligent-qa-review.md:1)
- [智能问答实施方案](/D:/workSpace/WeKnora/docs/superpowers/specs/2026-04-22-intelligent-qa-implementation-plan.md:1)

---

## 一、建议排期

建议按 4 个迭代批次推进。

### 批次 A

目标：

- 收敛无效参数与前后端契约漂移

内容：

- 删除前端 session 创建时的 `agent_config`
- 清理 `mcp_service_ids` 透传
状态：已完成

### 批次 B

目标：

- 为 assistant 消息补充 execution snapshot

内容：

- `messages.execution_meta`
- 创建时写基础快照
- 完成/停止/报错时补终态
状态：已完成

### 批次 C

目标：

- 置信度语义去误导化

内容：

- 新增 `evidence_strength_score`
- 新增 `source_health_score`
- 前端文案从“置信度”向“证据强度”迁移
状态：已完成第一版
说明：
- 后端双维度字段已落地
- 前端问答面板已展示双维度
- `confidence_score/confidence_label` 仍保留为兼容别名

### 批次 D

目标：

- 把已有数据能力快速变成可用运营视图

内容：

- 基于现有 analytics 与看板做一期上线
状态：已完成第一版
说明：
- `coverage_gaps` 已使用双维度口径
- `stale_documents` / `citation_heatmap` 已补 `source_health_score` 与 `health_status`

---

## 二、任务清单

## T1. 删除无效 `agent_config` 创建参数

### 目标

让 session 创建回到“仅创建会话容器”的模型。

### 影响范围

前端：

- [creatChat.vue](/D:/workSpace/WeKnora/frontend/src/views/creatChat/creatChat.vue:1)
- [api/chat/index.ts](/D:/workSpace/WeKnora/frontend/src/api/chat/index.ts:1)

后端：

- 无接口变更

### 具体动作

1. 删除 `createNewSession()` 中拼装的 `sessionData.agent_config`
2. 检查 `createSessions()` 是否仍传空对象或无效字段
3. 补注释说明：session 不保存运行期问答配置

### 前置依赖

- 无

### 可并行性

- 可独立做

### 验收标准

1. 新建会话成功
2. 首次发送消息后，问答行为与改动前一致
3. 网络请求中不再出现 `agent_config`

### 风险

- 极低

---

## T2. 清理 `mcp_service_ids` 透传

### 目标

去掉当前无效的会话级 MCP 控制，避免假配置。

### 影响范围

前端：

- [streame.ts](/D:/workSpace/WeKnora/frontend/src/api/chat/streame.ts:1)
- [chat/index.vue](/D:/workSpace/WeKnora/frontend/src/views/chat/index.vue:1)

后端：

- 无需变更请求结构

### 具体动作

1. 删除 `startStream()` 中的 `mcp_service_ids` 参数定义与 body 透传
2. 删除 `sendMsg()` 中读取 `selectedMCPServices` 并下发的逻辑
3. 如果 UI 中有“当前会话临时 MCP 选择”的文案，改为由 Agent 配置控制

### 前置依赖

- 无

### 可并行性

- 可与 T1 并行

### 验收标准

1. 问答请求体不再带 `mcp_service_ids`
2. Agent 仍能按其配置正常使用 MCP
3. 不出现用户界面可调但后端无效的字段

### 风险

- 低

---

## T3. 为 `messages` 增加 `execution_meta`

### 目标

为 assistant 消息补充最小执行快照，支撑审计、回放、运营分析与问题排查。

### 影响范围

数据库：

- `messages` 表新增 `execution_meta JSONB`

后端：

- [message.go](/D:/workSpace/WeKnora/internal/types/message.go:100)
- [json.go](/D:/workSpace/WeKnora/internal/types/json.go:1)
- [qa.go](/D:/workSpace/WeKnora/internal/handler/session/qa.go:1)
- [helpers.go](/D:/workSpace/WeKnora/internal/handler/session/helpers.go:168)
- [message.go](/D:/workSpace/WeKnora/internal/application/repository/message.go:1)
- [interfaces/message.go](/D:/workSpace/WeKnora/internal/types/interfaces/message.go:1)

### 具体动作

#### 数据层

1. 新增 migration：

```sql
ALTER TABLE messages
ADD COLUMN IF NOT EXISTS execution_meta JSONB DEFAULT NULL;
```

2. `types.Message` 增加：

```go
ExecutionMeta JSON `json:"execution_meta,omitempty" gorm:"type:jsonb;column:execution_meta"`
```

#### 仓储层

建议新增专用更新方法，避免全量 `Updates(message)` 的覆盖风险：

- `UpdateMessageExecutionMeta(ctx, sessionID, messageID string, executionMeta types.JSON) error`

如果想先省改动，也可先复用 `UpdateMessage`，但不推荐作为长期方案。

#### Handler / 业务层

1. 在 `executeQA()` 中 assistant message 创建后立即写基础快照
2. 在 `completeAssistantMessage()` 中补齐终态
3. 在 stop 路径中补 `stop_reason`
4. 在错误路径中补 `error_stage`

### 推荐 execution_meta 首版字段

```json
{
  "requested_mode": "knowledge",
  "final_mode": "knowledge",
  "agent_id": "",
  "model_id": "",
  "kb_ids": [],
  "knowledge_ids": [],
  "web_search_enabled": false,
  "memory_enabled": false,
  "channel": "web",
  "requested_at": "2026-04-22T10:00:00Z",
  "completed_at": "2026-04-22T10:00:05Z",
  "stop_reason": "",
  "error_stage": ""
}
```

### 前置依赖

- 无

### 可并行性

- 数据 migration 与后端类型定义可并行
- 写入逻辑与仓储方法建议串行

### 验收标准

1. 正常问答完成后，assistant message 有 execution_meta
2. stop 后的 message 也有 execution_meta，且 `stop_reason = user_requested`
3. 出错消息也能写入 `error_stage`
4. continue-stream 场景不影响
5. 不影响 `agent_steps`、`mentioned_items`、`rendered_content`

### 风险

- 中

### 重点回归

1. 普通问答完成
2. Agent 问答完成
3. 用户手动 stop
4. 途中报错
5. 刷新页面后继续流式恢复

---

## T4. 记录最终 `model_id`

### 目标

让 execution_meta 真正具备可审计性，而不是只有 request 侧快照。

### 影响范围

后端：

- [session_knowledge_qa.go](/D:/workSpace/WeKnora/internal/application/service/session_knowledge_qa.go:1)
- [session_agent_qa.go](/D:/workSpace/WeKnora/internal/application/service/session_agent_qa.go:1)

### 具体动作

1. 在普通问答链路中，拿到 `chatModelID` 后，将其透出到 assistant message 的终态 metadata
2. 在 Agent 链路中，拿到 `effectiveModelID` 后，将其透出到终态 metadata

### 前置依赖

- T3

### 可并行性

- 不建议与 T3 完全并行，依赖 execution_meta 基础结构

### 验收标准

1. 普通问答可看到实际模型 ID
2. Agent 问答可看到实际模型 ID
3. request 传入模型为空时，也能记录最终选中的模型

### 风险

- 低到中

---

## T5. Answer confidence 增加双维度字段

### 目标

把当前“置信度”拆成：

- 证据强度
- 来源健康度

同时不破坏现有接口兼容性。

### 影响范围

后端：

- [answer_evidence.go](/D:/workSpace/WeKnora/internal/types/answer_evidence.go:1)
- [confidence_service.go](/D:/workSpace/WeKnora/internal/application/service/confidence_service.go:1)
- [confidence.go](/D:/workSpace/WeKnora/internal/handler/confidence.go:1)

前端：

- [ConfidencePanel.vue](/D:/workSpace/WeKnora/frontend/src/views/chat/components/ConfidencePanel.vue:1)
- [api/chat/index.ts](/D:/workSpace/WeKnora/frontend/src/api/chat/index.ts:1)

### 具体动作

状态：已完成

#### 后端类型

在 `AnswerConfidenceResponse` 中新增：

- `evidence_strength_score`
- `evidence_strength_label`
- `source_health_score`
- `source_health_label`

保留：

- `confidence_score`
- `confidence_label`

兼容映射：

- `confidence_score = evidence_strength_score`
- `confidence_label = evidence_strength_label`

#### 后端计算

1. 把当前 `computeConfidenceScore()` 产物作为 `evidence_strength_score`
2. 新增 `computeSourceHealthScore()`，基于：
   - `source_weight`
   - `freshness_flag`
   - 当前 answer 相关 evidence 的来源反馈

### 前置依赖

- 无

### 可并行性

- 后端计算和前端展示可并行

### 验收标准

1. 原接口字段仍可正常返回
2. 新字段正常返回
3. 前端不因新字段缺失而报错
4. 旧前端不升级也能继续工作

### 风险

- 中

### 重点回归

1. 无 evidence 的回答
2. recovered / degraded / missing 三种 evidence 状态
3. 有 source feedback 的回答
4. FAQ / document / web 混合来源

---

## T6. 前端文案迁移：从“置信度”到“证据强度”

### 目标

先修正认知，不等待后端大改完毕。

### 影响范围

前端：

- [ConfidencePanel.vue](/D:/workSpace/WeKnora/frontend/src/views/chat/components/ConfidencePanel.vue:1)
- i18n 文案文件

### 具体动作

状态：已完成第一版
说明：
- 问答面板主标题已迁移到“证据强度”
- 来源健康度已作为第二指标展示
- 部分历史 i18n 文案仍保留兼容键

1. `answerConfidence` 文案改为“证据强度”
2. `confidenceHigh/Medium/Low` 文案改为“证据强/中/弱”
3. 展开面板中新增一行来源健康度摘要

### 前置依赖

- T5 最佳
- 也可先只改文案，后补字段

### 可并行性

- 可与 T5 并行

### 验收标准

1. 面板主标题不再误导用户理解为“最终答案真实性”
2. i18n 中英文至少同步
3. UI 布局不破坏

### 风险

- 低

---

## T7. 轻量运营看板一期

### 目标

先用已有 analytics 能力兑现业务价值，不等高级模型能力。

### 影响范围

后端：

- [analytics.go](/D:/workSpace/WeKnora/internal/application/repository/analytics.go:1)
- [document_access_log.go](/D:/workSpace/WeKnora/internal/types/document_access_log.go:1)

前端：

- [KnowledgeHealthDashboard.vue](/D:/workSpace/WeKnora/frontend/src/views/settings/KnowledgeHealthDashboard.vue:1)
- [api/analytics/index.ts](/D:/workSpace/WeKnora/frontend/src/api/analytics/index.ts:1)

### 具体动作

1. `CoverageGap` 语义从 `confidence_score` 转为“低证据强度”
2. 看板文案改为：
   - 覆盖缺口
   - 低证据强度回答
   - 来源健康风险
3. 首版先不改 SQL 结构，避免扩大范围

### 前置依赖

- T5/T6 推荐完成后再做

### 可并行性

- 可与前端文案迁移串行进行

### 验收标准

1. 现有看板数据可继续展示
2. 术语不再使用“低置信度”误导业务侧
3. 不影响 hot questions / citation heat / stale docs

### 风险

- 低

---

## 三、建议并行方式

### 可并行组 1

- T1 删除无效 `agent_config`
- T2 清理 `mcp_service_ids`

### 可并行组 2

- T3 migration 与类型定义
- T6 前端文案准备

### 串行组 3

- T3 写入逻辑
- T4 最终 model_id 回写

### 可并行组 4

- T5 双维度评分
- T6 前端面板改造

### 收尾组

- T7 轻量运营看板一期

---

## 四、测试清单

## 4.1 后端测试

建议新增或补充：

1. `execution_meta` 写入单测
2. stop 场景 metadata 单测
3. error 场景 metadata 单测
4. `computeSourceHealthScore()` 单测
5. confidence 兼容字段单测

## 4.2 前端验证

1. 新建会话
2. 首问自动发送
3. 普通问答
4. Agent 问答
5. 刷新后继续流式恢复
6. stop 按钮
7. ConfidencePanel 展开收起
8. KnowledgeHealthDashboard 正常加载

## 4.3 回归命令建议

后端：

```bash
go test ./internal/handler/session/...
go test ./internal/application/service/...
go test ./internal/application/repository/...
```

前端：

```bash
cd frontend && npm run type-check
cd frontend && npm run build
```

---

## 五、建议 Jira 拆分

建议拆为 7 个 ticket：

1. `CHAT-101` 删除 session 创建时无效 agent_config
2. `CHAT-102` 清理 mcp_service_ids 契约漂移
3. `CHAT-103` messages.execution_meta migration 与模型定义
4. `CHAT-104` assistant execution_meta 写入链路
5. `CHAT-105` answer confidence 双维度评分
6. `CHAT-106` 前端 ConfidencePanel 文案与字段兼容改造
7. `CHAT-107` 知识健康看板一期语义调整

---

## 六、最终建议

最值得马上排进开发的是：

1. T1
2. T2
3. T3

这三项收益最高、风险最低，并且能为后续所有智能问答治理能力打基础。

如果团队本周只做一个中等规模需求，优先做：

- `T3 + T4`

因为 execution snapshot 一旦落地，后续排查、分析、回放和数据治理都会明显顺畅。
