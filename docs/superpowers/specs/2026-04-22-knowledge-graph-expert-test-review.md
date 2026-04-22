# 知识图谱专家测试流程评审与优化建议

**日期：** 2026-04-22  
**状态：** 评审稿  
**范围：** `builtin-knowledge-graph-expert`、`query_knowledge_graph` 工具、知识图谱启用链路、测试与验收流程

---

## 一、结论

当前“知识图谱专家”这条链路是**可运行**的，但若仅凭人工提问和答案主观判断，**不能认定测试已经完整通过**。

从代码现状看，它更接近：

- 图谱配置感知的 Agent
- 图谱前置校验后的混合检索
- 检索结果的图谱化展示

而不是严格意义上的：

- 基于图数据库关系遍历的图谱查询专家
- 可验证实体关系路径的图查询闭环

因此，当前测试流程如果只是“导入文档 -> 选择知识图谱专家 -> 提问 -> 答案看起来合理”，只能说明：

- 能力入口正常
- Agent 可以调工具
- 返回结果在产品层可用

但还不能说明：

- 查询真正使用了图谱关系，而不是普通 `HybridSearch`
- 图谱启用判定一致且可靠
- 前后端展示契约完全正确
- 回归时不会静默退化成普通检索

---

## 二、当前实现链路判断

### 2.1 Agent 配置是成立的

内置 Agent 已配置完成，见 [builtin_agents.yaml](/D:/workSpace/WeKnora/config/builtin_agents.yaml:155)：

- `id = builtin-knowledge-graph-expert`
- `agent_mode = smart-reasoning`
- `allowed_tools` 包含：
  - `query_knowledge_graph`
  - `knowledge_search`
  - `grep_chunks`
  - `list_knowledge_chunks`

对应系统提示词也已单独配置，见 [agent_system_prompt.yaml](/D:/workSpace/WeKnora/config/prompt_templates/agent_system_prompt.yaml:286)。

其中提示词明确要求：

- Graph-first
- 先 `query_knowledge_graph`
- 再结合文档检索进行佐证

这在“设计意图”上是正确的。

### 2.2 工具能力命名与实际实现有偏差

`query_knowledge_graph` 工具位于 [query_knowledge_graph.go](/D:/workSpace/WeKnora/internal/agent/tools/query_knowledge_graph.go:16)。

它当前的执行方式是：

1. 解析 `knowledge_base_ids`
2. 获取 KB
3. 判断 KB 是否配置了图谱抽取字段
4. 实际执行 `HybridSearch`
5. 将结果包装成 `graph_query_results`

关键点在 [query_knowledge_graph.go](/D:/workSpace/WeKnora/internal/agent/tools/query_knowledge_graph.go:152)：

```go
results, err := t.knowledgeService.HybridSearch(ctx, id, searchParams)
```

这意味着当前工具并没有进行真正的图关系查询，而是：

- 利用图谱配置作为前置条件
- 最终返回混合检索结果

所以它更准确的名字应该接近：

- `graph_aware_search`
- `search_with_graph_context`

而不是严格的 `query_knowledge_graph`

### 2.3 图谱启用语义不一致

知识图谱相关链路对“图谱是否启用”的判断不一致：

在 [extract_entity.go](/D:/workSpace/WeKnora/internal/application/service/chat_pipeline/extract_entity.go:110) 中，判定依据是：

```go
if kb.ExtractConfig != nil && kb.ExtractConfig.Enabled
```

但在 [query_knowledge_graph.go](/D:/workSpace/WeKnora/internal/agent/tools/query_knowledge_graph.go:144) 中，判定依据是：

```go
if kb.ExtractConfig == nil || (len(kb.ExtractConfig.Nodes) == 0 && len(kb.ExtractConfig.Relations) == 0)
```

这会导致一种风险：

- KB 的图谱能力逻辑上已关闭
- 但只要 `Nodes/Relations` 配置仍在
- 图谱专家工具仍可能把它当作“已配置图谱”

这是测试流程里非常容易漏掉的隐性问题。

### 2.4 前后端结构化结果契约存在漂移

后端返回的是：

- `graph_configs`
- `graph_data`
- `has_graph_config`

见 [query_knowledge_graph.go](/D:/workSpace/WeKnora/internal/agent/tools/query_knowledge_graph.go:352)

但前端类型 [tool-results.ts](/D:/workSpace/WeKnora/frontend/src/types/tool-results.ts:123) 和组件 [GraphQueryResults.vue](/D:/workSpace/WeKnora/frontend/src/views/chat/components/tool-results/GraphQueryResults.vue:4) 使用的是：

- `graph_config`

这说明：

- 工具返回结构与前端渲染契约并不完全一致
- 当前测试如果只看文字答案，很可能漏掉工具面板展示错误

### 2.5 所谓“图谱可视化”当前并不是关系图验证

`buildGraphVisualizationData()` 当前只构造了结果节点，不构造边：

- `nodes` 有内容
- `edges` 恒为空

见 [query_knowledge_graph.go](/D:/workSpace/WeKnora/internal/agent/tools/query_knowledge_graph.go:364)

所以现阶段它更像“搜索结果节点列表”，而不是“可验证关系路径的图谱可视化”。

---

## 三、当前测试流程是否正常

### 3.1 可以判定为“基本正常”的条件

若测试流程包含以下步骤，则可判定为“主链路正常”：

1. 启动 Neo4j，并确认环境变量生效。
2. 在知识库侧开启图谱抽取配置。
3. 导入文档，等待实体/关系抽取完成。
4. 直接在 Neo4j 中验证节点与关系已生成。
5. 在前端选择“知识图谱专家”进行问答。
6. 确认 Agent 确实调用了 `query_knowledge_graph`。
7. 确认最终答案结合了图谱结果与文档证据。
8. 确认工具结果卡片和答案内容不冲突。

如果上述步骤完整跑通，可以认为：

- 环境正常
- 图谱构建正常
- Agent 配置正常
- 工具调用正常
- 结果呈现基本正常

### 3.2 不能判定为“完整正确”的情况

如果测试只是：

1. 上传文档
2. 选择知识图谱专家
3. 问几个问题
4. 看回答好像对

那么这个流程不够。

因为它无法区分：

- 回答来自图谱
- 回答来自普通检索
- 回答来自模型已有知识

这类测试只能验证“有答案”，不能验证“图谱专家真的工作正确”。

---

## 四、当前流程的主要缺口

### 4.1 缺少“图谱路径被实际使用”的验证

当前最关键的测试缺口不是“答得对不对”，而是：

- 是否真的走了 `query_knowledge_graph`
- 是否因为图谱配置异常退化成普通检索
- 是否出现了“工具叫图谱查询，实际是检索”的误判

建议在测试记录中明确保留：

- 工具调用序列
- 每一步工具输入
- 每一步工具输出
- 最终答案中的证据来源

### 4.2 缺少负例测试

至少应补以下负例：

1. KB 未启用图谱抽取
2. KB 有 `Nodes/Relations` 但 `Enabled=false`
3. 图谱已启用但 Neo4j 无数据
4. 查询实体不存在
5. 多 KB 混查时仅部分 KB 有图谱
6. 图谱工具失败但普通搜索仍可返回结果

当前代码路径对这些情况有不同处理方式，但没有看到专门自动化测试覆盖。

### 4.3 缺少结构化结果一致性验证

需要单独验证：

- 后端返回 `graph_configs`
- 前端是否正确解析
- 卡片中的实体类型/关系类型是否按预期展示
- 无图谱配置时是否展示合适 fallback 文案

否则会出现：

- 后端正确
- 面板空白
- 人工测试却只看聊天正文而误判通过

### 4.4 缺少“图谱 vs 检索”能力边界说明

当前提示词要求 Graph-first，但工具实现仍是 `HybridSearch`。

这会导致测试者误以为：

- 关系已经来自图遍历

实际上更可能是：

- 结果来自 chunk 检索

如果不在测试说明里把这一点写清楚，验收结论会偏乐观。

---

## 五、更合理的测试方案

建议把“知识图谱专家测试”拆成四层。

### 5.1 L0 环境与构建层

目标：验证图谱基础设施确实可用

检查项：

1. `NEO4J_ENABLE=true`
2. Neo4j 可连通
3. 知识库图谱抽取配置保存成功
4. 文档导入后可在 Neo4j 中看到节点和关系

通过标准：

- Neo4j 中能看到预期实体和关系

### 5.2 L1 工具层

目标：验证 `query_knowledge_graph` 本身行为正确

检查项：

1. 参数为空时报错
2. 超过 10 个 KB 报错
3. 未配置图谱的 KB 返回明确错误或 fallback 信息
4. 部分 KB 失败时，其他 KB 结果仍返回
5. 结构化字段完整返回

通过标准：

- 工具输出稳定
- 错误可解释
- 结构化数据可消费

### 5.3 L2 Agent 层

目标：验证知识图谱专家真的优先走图谱工具

检查项：

1. 简单实体关系问题是否先调用 `query_knowledge_graph`
2. 复杂问题是否形成：
   - `query_knowledge_graph`
   - `knowledge_search` / `list_knowledge_chunks`
   - `final_answer`
3. 工具失败时是否降级但不胡编

通过标准：

- 工具调用顺序符合提示词预期

### 5.4 L3 产品层

目标：验证前端用户感知一致

检查项：

1. 工具面板展示正常
2. 图谱配置卡片正常
3. 无图谱时展示 fallback
4. 答案、引用、工具结果三者语义一致

通过标准：

- 用户能看懂系统当前到底是在查图谱，还是已退化为检索

---

## 六、最值得立刻优化的地方

### P0

1. 统一图谱启用判定逻辑  
   当前 `ExtractConfig.Enabled` 与 `Nodes/Relations` 的判断不一致，应统一。

2. 修正前后端结果契约  
   `graph_configs` / `graph_config` 要统一，否则人工测试结论不可靠。

3. 在工具结果中显式返回查询模式  
   建议新增：
   - `query_mode = graph | fallback_search`
   - `fallback_reason`
   - `graph_enabled`

### P1

1. 给 `query_knowledge_graph` 增加专门单测  
   覆盖参数校验、图谱未配置、部分失败、多 KB、结构化输出。

2. 给知识图谱专家增加 Agent 回归测试  
   验证是否优先调用图谱工具。

3. 前端增加图谱状态说明  
   明确告诉用户当前是：
   - 图谱结果
   - 图谱失败后的检索 fallback

### P2

1. 将 `query_knowledge_graph` 从 `HybridSearch` 中解耦  
   引入独立 `QueryGraph` service/interface。

2. 支持真正的图关系路径返回  
   如：
   - 实体节点
   - 关系边
   - hop 数
   - 关系路径说明

3. 补真正的图谱可视化  
   当前 `edges` 为空，不足以称为图谱可视化验证。

---

## 七、推荐验收口径

建议把当前“知识图谱专家测试”结果定性为：

**可用，但尚未达到“图谱查询能力完全验真”的验收标准。**

更准确的表述应为：

- Agent 与工具链路可运行
- 图谱配置可参与问答流程
- 当前能力更偏图谱增强检索
- 尚需补齐查询语义、契约一致性和自动化验证

---

## 八、建议的下一步

最优先的不是马上重写图谱能力，而是先把测试口径收紧。

推荐顺序：

1. 先补专项测试方案和用例矩阵
2. 再修 `graph_config(s)` 契约漂移
3. 再统一图谱启用判定
4. 最后再决定是否投入真正图查询能力重构

这样收益最高，风险最低，也最容易让后续每次“知识图谱专家测试”结果具备可比性。
