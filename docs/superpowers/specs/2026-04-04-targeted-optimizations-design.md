# WeKnora 针对性优化方案

**日期**: 2026-04-04
**范围**: RAG 检索质量、Agent 引擎健壮性、IM 并发性能、代码可维护性
**策略**: 小改动、大收益 — 每项改动独立可验证，全部向后兼容

---

## 1. 正确性修复

### 1.1 修复 `compositeScore` positionPrior 除零问题

**文件**: `internal/application/service/chat_pipeline/rerank.go` — `compositeScore()`

**问题**: 当 `EndAt=0`（如 web 搜索结果或无位置信息的 chunk）时，`positionPrior` 计算 `1.0 - StartAt/(EndAt+1)` 在 `StartAt=0, EndAt=0` 时为 `1.0`，被 clamp 到 `+0.05`，人为提升了无位置信息的 chunk 分数。

**修复**: 当 `EndAt <= 0` 时跳过 positionPrior 计算：

```go
positionPrior := 1.0
if sr.EndAt > 0 && sr.StartAt >= 0 {
    positionPrior += searchutil.ClampFloat(1.0-float64(sr.StartAt)/float64(sr.EndAt+1), -0.05, 0.05)
}
```

### 1.2 收紧瞬态错误匹配标记

**文件**: `internal/agent/const.go` — `transientErrorMarkers`

**问题**: 子串匹配 `"500"` 会错误命中 `"1500 tokens"`，导致非瞬态错误被重试，浪费 2-4 秒延迟。

**修复**: 使用更具上下文的模式：

```go
var transientErrorMarkers = []string{
    "429", "rate limit", "rate_limit",
    "status 500", "status 502", "status 503", "status 504",
    "http 500", "http 502", "http 503", "http 504",
    "overloaded", "timeout", "timed out",
    "connection refused", "connection reset",
    "server error", "temporarily unavailable",
    "internal server error", "bad gateway", "service unavailable",
}
```

### 1.3 修复最终答案合成中的工具结果格式

**文件**: `internal/agent/finalize.go` — `streamFinalAnswerToEventBus()`

**问题**: 工具结果被包装为多条 `role: "user"` 消息（`"Tool X returned: ..."`），LLM 会将其误认为用户输入，影响答案质量。

**修复**: 将所有工具结果合并为单条带标记的上下文消息，明确标注为非用户输入：

```go
var contextParts []string
for _, step := range state.RoundSteps {
    for _, toolCall := range step.ToolCalls {
        if toolCall.Result != nil && toolCall.Result.Output != "" {
            contextParts = append(contextParts, fmt.Sprintf("[Tool: %s]\n%s", toolCall.Name, toolCall.Result.Output))
        }
    }
}
if len(contextParts) > 0 {
    messages = append(messages, chat.Message{
        Role:    "user",
        Content: "The following are results from tool calls (not user input):\n\n" + strings.Join(contextParts, "\n\n---\n\n"),
    })
}
```

---

## 2. 性能优化

### 2.1 并行化 VLM 图片描述

**文件**: `internal/agent/engine.go` — `describeImages()`

**问题**: 图片串行处理，3-4 张图片需 6-12 秒。

**修复**: 使用 `errgroup` 并行调用 VLM，按原始顺序收集结果。单张失败不影响其他图片（`return nil` 而非 cancel）。

### 2.2 预编译 `extractPhrases` 正则表达式

**文件**: `internal/application/service/chat_pipeline/rerank.go`

**问题**: `extractPhrases` 每次调用都 `regexp.MustCompile`，rerank 流程中高频调用。

**修复**: 移至包级预编译变量 `reQuotedPhrase`，与同文件中的其他预编译正则一致。

### 2.3 扩展查询禁用向量匹配

**文件**: `internal/application/service/chat_pipeline/query_expansion.go` — `runQueryExpansion()`

**问题**: 每个 expansion variant × 每个 search target 都独立调用 `HybridSearch`，内部重新计算 embedding。而扩展查询的目的是补充关键词召回（原始向量搜索已完成），embedding 计算浪费资源且扩展变体的语义可能偏移。

**修复**: 设置 `DisableVectorMatch: true`，只做关键词搜索：

```go
paramsExp := types.SearchParams{
    QueryText:             q,
    KeywordThreshold:      expKwTh,
    MatchCount:            expTopK,
    DisableVectorMatch:    true,
    DisableKeywordsMatch:  false,
    SkipContextEnrichment: true,
}
```

### 2.4 环形缓冲区优化 qaQueue

**文件**: `internal/im/qaqueue.go`

**问题**: `dequeue()` 用 `q.queue = q.queue[1:]` 做头部移除，每次 O(n) 拷贝，高吞吐时 GC 压力大。

**修复**: 用固定容量的环形缓冲区替代 slice：
- `buf []*qaRequest`（容量 = maxSize）
- `head, tail, count int`
- `enqueueLocked()`：写入 `buf[tail]`，`tail = (tail+1) % cap`
- `dequeueLocked()`：读取 `buf[head]`，置 nil（允许 GC），`head = (head+1) % cap`
- `Remove()` 需遍历环形缓冲区查找并压缩

---

## 3. 可维护性改进

### 3.1 提取共享的 `emitToolCallEvents` 方法

**文件**: `internal/agent/act.go`

**问题**: `executeSingleToolCall`（160-203 ��）和 `executeToolCallsParallel`（119-157 行）包含完全相同的事件发送代码块（`EventAgentToolResult` + `EventAgentTool`），修改事件格式需编辑两处。

**修复**: 提取 `emitToolCallEvents(ctx, toolCall, iteration, sessionID)` 方法，两处调用简化为：

```go
step.ToolCalls = append(step.ToolCalls, toolCall)
e.emitToolCallEvents(ctx, toolCall, iteration, sessionID)
```

---

## 变更矩阵

| # | 文件 | 改动 | 类别 | 风险 |
|---|------|------|------|------|
| 1.1 | `rerank.go` | 修复 compositeScore positionPrior | 正确性 | 低 |
| 1.2 | `const.go` | 收紧瞬态错误标记 | 正确性 | 低 |
| 1.3 | `finalize.go` | 修复工具结果消息格��� | 正确性 | 低 |
| 2.1 | `engine.go` | 并行 VLM 图片描述 | 性能 | 低 |
| 2.2 | `rerank.go` | 预编译正则 | 性能 | 极低 |
| 2.3 | `query_expansion.go` | 扩展查询关键词优先 | 性能 | 低 |
| 2.4 | `qaqueue.go` | 环形缓冲区队列 | 性能 | 中 |
| 3.1 | `act.go` | 提取共享事件方法 | 可维护性 | 极低 |

**约束**: 全部向后兼容，无 API/配置变更，无数据库迁移。
