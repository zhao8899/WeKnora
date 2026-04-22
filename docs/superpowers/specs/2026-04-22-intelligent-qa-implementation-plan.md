# 智能问答实施方案

**日期：** 2026-04-22  
**状态：** 实施方案草案  
**关联文档：**

- [智能问答需求与架构评审](/D:/workSpace/WeKnora/docs/superpowers/specs/2026-04-22-intelligent-qa-review.md:1)
- [知识智能全链路设计](/D:/workSpace/WeKnora/docs/superpowers/specs/2026-04-20-knowledge-intelligence-design.md:1)

---

## 一、目标

本方案只覆盖近期最值得落地的 `P0/P1` 项，不展开长期能力如片段级引用与大规模入口重构。

本轮实施目标有四个：

1. 收敛前后端问答契约，消除无效参数与漂移字段。
2. 为每条 assistant 消息补充可追溯的执行快照。
3. 将“置信度”语义拆解为更准确的工程指标，避免误导。
4. 尽量复用现有表结构、接口路径和前端组件，降低改造风险。

## 1.1 Current Status Update

As of 2026-04-22, the following parts have already landed in code:

1. `agent_config` has been removed from session creation flow.
2. `mcp_service_ids` has been removed from chat request flow.
3. `messages.execution_meta` has been added, including stop/error/panic completion paths.
4. `/chat/answer/:message_id/confidence` now returns:
   - `evidence_strength_score`
   - `evidence_strength_label`
   - `source_health_score`
   - `source_health_label`
   while keeping `confidence_score/confidence_label` for compatibility.
5. Analytics coverage and health dashboards now use dual-dimension evidence/source-health semantics.

Remaining work should treat the above as baseline, not planned-only items.

---

## 二、实施优先级

## 2.1 P0

1. 移除无效 `agent_config` 创建参数
2. 清理 `mcp_service_ids` 契约漂移
3. 在 `messages` 增加 `execution_meta`，记录执行快照

## 2.2 P1

1. 保留现有 `confidence_score` 兼容字段
2. 新增 `evidence_strength_score`
3. 新增 `source_health_score`
4. 调整前端展示文案，从“高置信度”改为“证据强度/来源健康度”
5. 轻量运营看板一期，直接复用现有 analytics 数据层

## 2.3 暂不纳入本轮

1. 片段级引用
2. 搜索 / 可信问答 / 深度研究三入口重构
3. AnswerEvidence 语句级定位字段
4. LLM 原生 citations 能力接入

---

## 三、P0 详细方案

## 3.1 移除无效 `agent_config` 创建参数

### 现状

前端 [creatChat.vue](/D:/workSpace/WeKnora/frontend/src/views/creatChat/creatChat.vue:1) 在创建 session 时会传入：

- `agent_config.enabled`
- `agent_config.max_iterations`
- `agent_config.temperature`
- `agent_config.knowledge_bases`
- `agent_config.knowledge_ids`
- `agent_config.allowed_tools`

但后端 [CreateSessionRequest](/D:/workSpace/WeKnora/internal/handler/session/types.go:1) 只接受：

- `title`
- `description`

且 [CreateSession](/D:/workSpace/WeKnora/internal/handler/session/handler.go:1) 明确将 session 视为容器，不接收问答执行配置。

### 目标

统一认知：

- session 创建不携带问答执行配置
- 真正的执行配置只随每次问答请求携带

### 改法

前端：

1. 删除 `createNewSession()` 中拼装的 `sessionData.agent_config`
2. 保留空对象或只保留 title/description
3. 不改变后续进入 chat 页面后的问答行为

后端：

- 无需改接口，只补充注释和文档说明

### 风险

- 极低

### 验证点

1. 创建新 session 仍成功
2. 首次发送消息时 Agent/KB/Web Search 行为不变
3. 不影响 `isFirstSession` 和首问自动发送逻辑

---

## 3.2 清理 `mcp_service_ids` 契约漂移

### 现状

前端 [streame.ts](/D:/workSpace/WeKnora/frontend/src/api/chat/streame.ts:1) 会在请求体中附带：

- `mcp_service_ids`

但后端 [CreateKnowledgeQARequest](/D:/workSpace/WeKnora/internal/handler/session/types.go:1) 中并没有该字段。

当前 Agent 链路的 MCP 选择实际上来自：

- custom agent config 中的 `MCPSelectionMode`
- custom agent config 中的 `MCPServices`

也就是说，用户在当前聊天界面上选中的 MCP 服务并未形成稳定闭环。

### 目标

先消除假控制，再决定是否补真能力。

### 两种可选方案

**方案 A：P0 直接移除前端字段**

适用条件：

- 当前产品并不希望“会话级临时覆盖 MCP 选择”

改法：

1. 删除前端 `mcp_service_ids` 透传
2. 在 UI 层隐藏或改写相关选择逻辑
3. MCP 完全以 Agent 配置为准

**方案 B：P0 补齐后端字段并明确优先级**

适用条件：

- 需要支持“当前对话临时启用部分 MCP”

改法：

1. 给 `CreateKnowledgeQARequest` 增加 `MCPServiceIDs []string`
2. 给 `types.QARequest` 增加 `MCPServiceIDs`
3. 在 `buildAgentConfig()` 中增加优先级规则：

优先级建议：

- request.MCPServiceIDs > customAgent.Config.MCPServices

前提限制：

- 仅在 `MCPSelectionMode == selected` 时允许覆盖

### 建议

本轮推荐 **方案 A**。

原因：

- 代码改动最小
- 认知最清晰
- 不会引入新的运行期权限与治理复杂度

### 风险

- 低

### 验证点

1. 现有 Agent 问答不受影响
2. MCP 工具仍按 Agent 配置可用
3. 前端不再出现“用户以为可控但实际无效”的状态

---

## 3.3 为 `messages` 增加 `execution_meta`

## 3.3.1 为什么落在 `messages`

从当前模型设计看，[Message](/D:/workSpace/WeKnora/internal/types/message.go:100) 已经承载：

- `agent_steps`
- `mentioned_items`
- `images`
- `rendered_content`
- `feedback`

因此 assistant 消息天然就是“本轮执行结果”的聚合承载对象。

如果新建单独快照表，收益不大，反而会增加：

- 事务复杂度
- 查询复杂度
- 继续流式恢复时的关联成本

所以本轮建议直接在 `messages` 增加：

- `execution_meta JSONB`

## 3.3.2 字段设计

建议在 [Message](/D:/workSpace/WeKnora/internal/types/message.go:100) 增加：

```go
ExecutionMeta JSON `json:"execution_meta,omitempty" gorm:"type:jsonb;column:execution_meta"`
```

建议结构：

```json
{
  "mode": "knowledge" | "agent",
  "requested_mode": "knowledge" | "agent",
  "final_mode": "knowledge" | "agent",
  "agent_id": "xxx",
  "agent_tenant_id": 1001,
  "model_id": "xxx",
  "kb_ids": ["kb1", "kb2"],
  "knowledge_ids": ["doc1"],
  "web_search_enabled": true,
  "memory_enabled": false,
  "channel": "web",
  "has_images": true,
  "requested_at": "2026-04-22T10:00:00Z",
  "completed_at": "2026-04-22T10:00:08Z",
  "stop_reason": "",
  "error_stage": ""
}
```

首版不建议加入过多字段，优先保留：

- mode
- agent_id
- model_id
- kb_ids
- knowledge_ids
- web_search_enabled
- memory_enabled
- channel

## 3.3.3 写入时机

不要只在 [completeAssistantMessage](/D:/workSpace/WeKnora/internal/handler/session/qa.go:695) 写。

建议分两段写入：

### 阶段一：创建 assistant message 后立即写基础快照

位置：

- [createAssistantMessage](/D:/workSpace/WeKnora/internal/handler/session/helpers.go:184) 之后
- 或 `executeQA()` 中 `assistantMessage` 创建完成后立即设置并 `UpdateMessage`

写入内容：

- requested_mode
- initial mode
- agent_id
- kb_ids
- knowledge_ids
- web_search_enabled
- memory_enabled
- channel
- requested_at

### 阶段二：完成 / 停止 / 错误时补最终状态

位置：

- [completeAssistantMessage](/D:/workSpace/WeKnora/internal/handler/session/qa.go:695)
- stop handler
- 错误事件处理链路

补充内容：

- final_mode
- completed_at
- stop_reason
- error_stage
- 最终使用的 model_id

这样做的原因是：

- stop 时也能保留快照
- error 时也能保留快照
- continue-stream 场景中已有 message 也有足够信息恢复

## 3.3.4 model_id 获取建议

当前真正的 model 解析发生在 service 层：

- 普通问答见 [session_knowledge_qa.go](/D:/workSpace/WeKnora/internal/application/service/session_knowledge_qa.go:1)
- Agent 问答见 [session_agent_qa.go](/D:/workSpace/WeKnora/internal/application/service/session_agent_qa.go:1)

因此建议：

1. 先在创建时写 `requested_model_id` 或 `summary_model_id`
2. 再在 service 层解析出 `effective_model_id`
3. 在完成时回写 execution_meta 中的 `model_id`

### 最小实现方案

本轮先允许 `model_id` 为空或只记录 request 侧模型，不阻塞 P0。

---

## 四、P1 详细方案

## 4.1 置信度拆维度，但保留兼容字段

### 现状

当前 [confidence_service.go](/D:/workSpace/WeKnora/internal/application/service/confidence_service.go:1) 直接输出：

- `confidence_score`
- `confidence_label`

这个值实际更接近：

- 检索证据强度

而不是严格意义上的：

- 最终答案可信度

### 问题

直接把它对用户叫“高置信度”，会产生认知偏差。

### 目标

把一个混合指标拆成两个更清晰的工程指标：

1. `evidence_strength_score`
   表示当前回答被检索证据支撑的强度。

2. `source_health_score`
   表示来源健康度，主要由来源反馈与 freshness 决定。

### 接口兼容策略

本轮不要移除旧字段。

建议返回：

```json
{
  "confidence_score": 0.72,
  "confidence_label": "medium",
  "evidence_strength_score": 0.72,
  "evidence_strength_label": "medium",
  "source_health_score": 0.88,
  "source_health_label": "healthy"
}
```

约定：

- `confidence_score = evidence_strength_score`
- `confidence_label = evidence_strength_label`

### 已落地说明

当前代码已完成这一兼容迁移，后续不应再把 `confidence_score` 当作独立业务语义扩展。
所有新能力应优先基于：

- `evidence_strength_score`
- `source_health_score`

仅用于兼容旧前端和旧 analytics。

### 后端改法

位置：

- [AnswerConfidenceResponse](/D:/workSpace/WeKnora/internal/types/answer_evidence.go:1)
- [confidence_service.go](/D:/workSpace/WeKnora/internal/application/service/confidence_service.go:1)

新增字段：

- `EvidenceStrengthScore`
- `EvidenceStrengthLabel`
- `SourceHealthScore`
- `SourceHealthLabel`

### 计算逻辑建议

**evidence_strength_score**

复用当前 `computeConfidenceScore()` 主体逻辑，后续再逐步增强。

**source_health_score**

建议首版简化为来源级聚合：

1. 取当前 answer evidence 关联的 unique knowledge
2. 读取 `source_weight`
3. 如果 `freshness_flag == true`，降权
4. 如果来源收到 `expired/down` 反馈，进一步降权

首版示例：

```text
base = avg(normalized(source_weight))
if freshness_flag => -0.15
if expired feedback exists => -0.10
if down feedback exists => -0.10
clamp to [0,1]
```

### 前端改法

位置：

- [ConfidencePanel.vue](/D:/workSpace/WeKnora/frontend/src/views/chat/components/ConfidencePanel.vue:1)

建议：

1. 主展示改为“证据强度”
2. 展开区域加一个“来源健康度”
3. 不立刻改所有布局，只补一行摘要信息

### 文案建议

- `answerConfidence` 改为 `evidenceStrength`
- `confidenceHigh/Medium/Low` 可暂时复用，但文案需换成“证据强/中/弱”

---

## 4.2 运营看板前移为 P1-

### 现状

当前其实已经有轻量看板基础：

- [analytics.go](/D:/workSpace/WeKnora/internal/application/repository/analytics.go:1)
- [KnowledgeHealthDashboard.vue](/D:/workSpace/WeKnora/frontend/src/views/settings/KnowledgeHealthDashboard.vue:1)

现有维度包括：

- 热门问题
- 覆盖缺口
- 陈旧文档
- 引用热力

### 判断

这部分没必要等到 `P2`。

因为：

- 数据表已经有
- SQL 已经有
- 前端页面已经有

本轮只要把 `coverage_gaps` 从“低置信度”语义微调为“低证据强度”，就已经能形成第一版运营视图。

### 建议

把“运营看板”拆两步：

1. `P1-`
   直接复用现有 analytics，看板继续可用

2. `P2`
   等 `source_health_score` 稳定后，再增加来源健康维度看板

---

## 五、接口与数据结构改动清单

## 5.1 后端表结构

建议新增 migration：

```sql
ALTER TABLE messages
ADD COLUMN IF NOT EXISTS execution_meta JSONB DEFAULT NULL;
```

## 5.2 Go 类型

### `types.Message`

新增：

- `ExecutionMeta JSON`

### `types.AnswerConfidenceResponse`

新增：

- `EvidenceStrengthScore`
- `EvidenceStrengthLabel`
- `SourceHealthScore`
- `SourceHealthLabel`

## 5.3 前端类型

### `frontend/src/api/chat/index.ts`

补充 confidence 响应类型定义，避免后续字段继续隐式使用。

### `frontend/src/api/analytics/index.ts`

保留：

- `confidence_score`
- `confidence_label`

待第二阶段再扩展：

- `evidence_strength_score`
- `source_health_score`

---

## 六、建议实施顺序

## 6.1 第一批提交

目标：

- 收敛契约，不改变用户体验

内容：

1. 前端删除 session 创建中的 `agent_config`
2. 前端删除 `mcp_service_ids` 透传
3. 补充文档注释与接口说明

## 6.2 第二批提交

目标：

- 落 execution_meta

内容：

1. migration 新增 `messages.execution_meta`
2. `types.Message` 增加字段
3. 创建 assistant message 后写基础快照
4. 完成 / 停止 / 错误时补最终状态

## 6.3 第三批提交

目标：

- 修正置信度语义但不破坏兼容

内容：

1. `AnswerConfidenceResponse` 新增双维度字段
2. 保留旧 `confidence_score` 兼容输出
3. 前端文案改为“证据强度”
4. 面板内增加来源健康度摘要

## 6.4 第四批提交

目标：

- 复用现有健康看板，完成 P1- 收益释放

内容：

1. `coverage_gaps` 文案改为“低证据强度”
2. 看板保持现有结构
3. 不等待片段级引用

---

## 七、回归风险点

## 7.1 `UpdateMessage` 全量更新风险

[UpdateMessage](/D:/workSpace/WeKnora/internal/application/repository/message.go:1) 当前是 struct 级 `Updates(message)`。

风险：

- 如果 execution_meta 的写入与别的字段更新交织，容易出现覆盖或空值写回。

建议：

- 首版可以继续沿用
- 但更稳的是补一个专用方法：
  - `UpdateMessageExecutionMeta(ctx, sessionID, messageID, executionMeta)`

## 7.2 停止生成场景

stop 路径在：

- [setupStopEventHandler](/D:/workSpace/WeKnora/internal/handler/session/helpers.go:205)

这里会把 assistant 内容写成“用户停止了本次对话”，然后直接 complete。

若 execution_meta 只在正常完成写，则 stop 会丢失状态。

必须覆盖：

- `stop_reason = user_requested`

## 7.3 错误场景

当前 error 事件和完成事件不是完全同一条路径。

若要保证 execution_meta 可用于审计，必须在 error 时记录：

- `error_stage`
- `completed_at`

否则日志里有错误，消息元数据里却没有。

## 7.4 前端兼容风险

直接替换 `confidence_score` 会影响：

- [ConfidencePanel.vue](/D:/workSpace/WeKnora/frontend/src/views/chat/components/ConfidencePanel.vue:1)
- [KnowledgeHealthDashboard.vue](/D:/workSpace/WeKnora/frontend/src/views/settings/KnowledgeHealthDashboard.vue:1)
- analytics 类型定义

因此必须走“新增字段 + 渐进替换”。

---

## 八、最终建议

结合当前代码现状，最值得马上做的不是“大改架构”，而是先把最容易、最基础、最解锁后续能力的事情做完。

建议近期执行顺序如下：

1. 去掉无效 `agent_config`
2. 清理 `mcp_service_ids`
3. 给 `messages` 补 `execution_meta`
4. 将 `confidence_score` 渐进拆成 `evidence_strength_score + source_health_score`
5. 直接复用现有 analytics 做第一版运营视图

这样做的收益是：

- 风险低
- 改动集中
- 不打断现有问答主流程
- 为后续片段级引用、入口分层、治理与审计能力打下稳定基础
