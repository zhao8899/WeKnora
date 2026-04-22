# 智能问答需求与架构评审

**日期：** 2026-04-22  
**状态：** 评审稿  
**范围：** WeKnora Web 端智能问答、后端问答服务、检索与证据链、Agent 问答能力  

---

## 一、评审结论

当前项目中的“智能问答”已经形成了较完整的技术闭环，不是单一的 RAG 问答，而是由以下几层组合而成：

1. 普通问答链路：面向知识库问答与纯聊天场景。
2. Agent 问答链路：面向工具调用、多步推理、外部能力扩展场景。
3. 证据与反馈层：面向答案来源展示、置信度计算、来源级反馈与后续权重调整。
4. 健康与洞察层：面向 document access log、answer evidence、source feedback 的后续分析。

从技术成熟度上看，当前实现已经超过“一期企业知识库底座”的最低要求，具备较强扩展性；但从产品定位上看，也已经部分越过“一期以稳定检索、可信回答、权限清晰为核心”的边界，呈现出明显的“AI 平台化”倾向。

因此，本次评审的核心结论不是“要不要继续做智能问答”，而是：

- 当前能力基础是成立的。
- 当前产品表达存在过度暴露和层次混杂问题。
- 下一步应从“继续堆能力”转向“能力分层、契约收敛、可信增强、治理补齐”。

---

## 二、现状链路梳理

## 2.1 前端主流程

前端主要由以下模块组成：

- [chat/index.vue](/D:/workSpace/WeKnora/frontend/src/views/chat/index.vue:1)：承载聊天主页面、消息列表、SSE 接收、停止生成、继续流式传输。
- [creatChat.vue](/D:/workSpace/WeKnora/frontend/src/views/creatChat/creatChat.vue:1)：承载新建会话入口与推荐问题展示。
- [AgentStreamDisplay.vue](/D:/workSpace/WeKnora/frontend/src/views/chat/components/AgentStreamDisplay.vue:1)：承载 Agent 推理、工具调用、结果展示。
- [botmsg.vue](/D:/workSpace/WeKnora/frontend/src/views/chat/components/botmsg.vue:1)：承载普通回答、反馈、追问建议、证据入口。
- [ConfidencePanel.vue](/D:/workSpace/WeKnora/frontend/src/views/chat/components/ConfidencePanel.vue:1)：承载置信度与来源反馈。
- [streame.ts](/D:/workSpace/WeKnora/frontend/src/api/chat/streame.ts:1)：统一封装 SSE 请求与事件回调。

前端发送消息时会同时携带：

- 查询文本
- 当前选择的知识库与文件
- 是否开启 Agent 模式
- 是否开启 Web Search
- 是否开启 Memory
- 选中的 Agent
- @ 提及的知识库或文件
- 图片附件

这意味着当前前端把“快速问答”“知识问答”“深度研究”“工具扩展”统一折叠到了一个输入框中。

## 2.2 后端入口与模式分流

后端问答入口主要在：

- [qa.go](/D:/workSpace/WeKnora/internal/handler/session/qa.go:1)
- [types.go](/D:/workSpace/WeKnora/internal/handler/session/types.go:1)

当前后端分为两条主链路：

1. `knowledge-chat`
   面向普通模式，进入 KnowledgeQA。

2. `agent-chat`
   面向 Agent 模式，但实际仍会根据 Agent 配置决定是走 AgentQA 还是降级回 KnowledgeQA。

入口层已完成以下关键职责：

- 会话校验
- 自定义 Agent / 共享 Agent 解析
- @提及知识库与文件合并
- 图片保存与 SSRF 防护
- SSE 初始化
- 异步执行问答流程

## 2.3 普通问答链路

普通问答主逻辑位于：

- [session_knowledge_qa.go](/D:/workSpace/WeKnora/internal/application/service/session_knowledge_qa.go:1)

其关键特点如下：

- 若无 KB、无 Web Search，则走 pure chat。
- 若存在 KB 或开启 Web Search，则走 RAG 流水线。
- 流水线中已经包含 history、query understand、parallel search、rerank、chunk merge、evidence capture 等关键步骤。

主要流水线如下：

1. `LOAD_HISTORY`
2. `QUERY_UNDERSTAND`
3. `CHUNK_SEARCH_PARALLEL`
4. `CHUNK_RERANK`
5. `WEB_FETCH`
6. `CHUNK_MERGE`
7. `FILTER_TOP_K`
8. `DATA_ANALYSIS`
9. `INTO_CHAT_MESSAGE`
10. `EVIDENCE_CAPTURE`
11. `CHAT_COMPLETION_STREAM`

其中关键节点包括：

- [knowledgebase_search.go](/D:/workSpace/WeKnora/internal/application/service/knowledgebase_search.go:1)：混合检索、向量 + 关键词融合。
- [search_parallel.go](/D:/workSpace/WeKnora/internal/application/service/chat_pipeline/search_parallel.go:1)：并行执行 chunk search 与 entity search。
- [into_chat_message.go](/D:/workSpace/WeKnora/internal/application/service/chat_pipeline/into_chat_message.go:1)：把检索结果装配成最终发给模型的上下文。
- [evidence_capture.go](/D:/workSpace/WeKnora/internal/application/service/chat_pipeline/evidence_capture.go:1)：持久化 answer evidence 与 document access log。

## 2.4 Agent 问答链路

Agent 主逻辑位于：

- [session_agent_qa.go](/D:/workSpace/WeKnora/internal/application/service/session_agent_qa.go:1)
- [internal/agent/engine.go](/D:/workSpace/WeKnora/internal/agent/engine.go:1)
- [agent_stream_handler.go](/D:/workSpace/WeKnora/internal/handler/session/agent_stream_handler.go:1)

其关键特点如下：

- 运行期动态构建 AgentConfig。
- 支持 KB、Web Search、MCP、Skills、Memory、Multi-turn。
- 通过 EventBus + StreamManager 将 thinking、tool_call、tool_result、references、final_answer 以事件流方式推给前端。
- 前端能恢复历史 Agent 过程，而不仅是最终答案。

从能力设计上看，这已经接近“深度研究/工具型问答”而不是传统企业知识问答。

## 2.5 证据、置信度与反馈

当前系统已经具备可信问答的基础数据层：

- [answer_evidence.go](/D:/workSpace/WeKnora/internal/types/answer_evidence.go:1)
- [answer_evidence.go](/D:/workSpace/WeKnora/internal/application/repository/answer_evidence.go:1)
- [confidence_service.go](/D:/workSpace/WeKnora/internal/application/service/confidence_service.go:1)
- [confidence.go](/D:/workSpace/WeKnora/internal/handler/confidence.go:1)

已落地能力包括：

- 对每次回答记录来源证据
- 记录 retrieved / reranked / cited 文档访问轨迹
- 按来源做 up/down/expired 反馈
- 将来源反馈映射到 `source_weight`
- 在前端展示置信度与来源列表

这部分是当前系统最有价值的能力之一，说明项目已经开始从“能答”转向“可信、可治理、可持续优化”。

---

## 三、与需求设计的符合度评估

## 3.1 与一期“企业知识库底座”定位的符合度

参考文档：

- [01-一期建设方案.md](/D:/workSpace/WeKnora/docs/企业内部知识库底座一期/01-一期建设方案.md:1)
- [02-差异分析.md](/D:/workSpace/WeKnora/docs/企业内部知识库底座一期/02-差异分析.md:1)

符合点：

- 文档型知识库与 FAQ 型知识库并存。
- 支持搜索、问答、多轮会话。
- 支持引用来源与推荐问题。
- 支持组织/共享/租户隔离。
- 支持后续分析与治理基础设施。

偏离点：

- 前端暴露 Agent、MCP、Web Search、Memory 等过多高级能力。
- 普通用户入口心智偏“AI 平台”，而非“知识问答入口”。
- 问答配置项分散在多个层级，产品表达不够收敛。

结论：

当前实现“能力上满足一期”，但“产品表达上不够一期”。

## 3.2 与知识智能全链路设计的符合度

参考文档：

- [2026-04-20-knowledge-intelligence-design.md](/D:/workSpace/WeKnora/docs/superpowers/specs/2026-04-20-knowledge-intelligence-design.md:1)

符合点：

- 已实现 answer evidence 数据层。
- 已实现 source feedback。
- 已实现 source weight 调整机制。
- 已实现 document access log。
- 已在前端提供 confidence panel。

仍有差距：

- 证据仍然偏“检索结果级”，不是“最终答案语句级”。
- 置信度算法仍偏启发式，不是完整的可信判断模型。
- 来源冲突、时效性衰减、覆盖度不足等问题尚未被系统性建模。

结论：

当前实现已经进入“解释层落地期”，但还没有进入“强可信问答”的稳定阶段。

---

## 四、当前主要问题

## 4.1 产品层问题

### 4.1.1 单入口承载过多模式

当前输入区同时承载：

- 普通聊天
- 知识问答
- Agent 问答
- Web Search
- Memory
- MCP
- 图片问答

问题在于：

- 普通用户不知道什么时候该开 Agent。
- 用户无法预判当前答案是“快速回答”还是“深度研究”。
- 功能越多，默认路径越不清晰。

这类设计对高级用户灵活，对企业落地却不一定友好。

### 4.1.2 搜索与问答心智没有分层

头部厂商普遍将“搜索”和“问答”分层：

- 搜索用于快速找信息
- 问答用于组织答案
- 深度研究用于复杂跨源推理

当前系统虽然已有 [KnowledgeSearch.vue](/D:/workSpace/WeKnora/frontend/src/views/knowledge/KnowledgeSearch.vue:1)，但主使用心智仍然偏向单一聊天入口，导致搜索层价值没有被突出。

## 4.2 架构层问题

### 4.2.1 会话是容器，但前端仍试图在创建时塞配置

[CreateSession](/D:/workSpace/WeKnora/internal/handler/session/handler.go:1) 当前只接收基础 session 信息，明确把 session 定义为容器；但 [creatChat.vue](/D:/workSpace/WeKnora/frontend/src/views/creatChat/creatChat.vue:1) 仍然传入 `agent_config`。

这说明前后端对“会话到底是否持有执行配置”认知不一致。

影响：

- 会话创建参数不可信。
- 历史复盘时拿不到一次问答真正的执行快照。
- 后续做审计、回放、比对、AB 实验会很难。

### 4.2.2 前后端契约存在漂移

前端会传：

- `mcp_service_ids`

但后端 `CreateKnowledgeQARequest` 未定义该字段，当前这部分选择并未形成稳定闭环。

这类契约漂移会带来两个问题：

- 用户以为自己在控制能力，实际没有生效。
- 代码复杂度持续上升，但产品结果不可预测。

### 4.2.3 普通链路与 Agent 链路边界尚不够稳定

当前 `agent-chat` 请求仍可能被降级回普通链路，普通链路中又可承载 web search、memory、image 等增强能力。

这在工程上是灵活的，但在产品和运维上会形成几个风险：

- 无法清晰定义 SLA
- 无法对不同模式做成本与性能预算
- 无法清晰配置权限边界

## 4.3 可信性问题

### 4.3.1 证据层还停留在“来源列表”

当前证据更像：

- 这条答案关联了哪些来源

但还不是：

- 这句话来自哪一段
- 哪部分是推断，哪部分是原文支撑

这会使“置信度”看起来存在，但说服力仍然有限。

### 4.3.2 置信度算法偏工程启发式

[confidence_service.go](/D:/workSpace/WeKnora/internal/application/service/confidence_service.go:1) 当前主要基于：

- retrieval score
- rerank score
- source weight
- 来源数和来源类型数

未系统考虑：

- 来源之间是否互相冲突
- 来源是否过期
- 回答是否覆盖用户问题
- 回答中真正被引用的部分占比多少

因此当前更适合称为“证据强度分”，而不是严格意义上的“答案可信度”。

## 4.4 治理与运营问题

### 4.4.1 缺少执行快照与审计视图

当前虽然有 evidence 和 access log，但还缺：

- 本轮实际使用了哪个模型
- 实际命中的 KB scope
- 是否开启 web search
- 是否使用了 agent / memory / MCP
- prompt 版本是什么

缺少这些字段，会限制后续：

- 问题排查
- 用户投诉追溯
- 质量评估
- 版本回归分析

### 4.4.2 高级能力没有清晰治理策略

Web Search、MCP、Agent、Memory 当前更偏“功能开关”，而不是“治理策略”。

缺少：

- 管理员级策略
- 用户组级策略
- 默认关闭与灰度开放机制
- 审计与成本视图

---

## 五、头部厂商设计启示

## 5.1 OpenAI：快问答与深研究分层

截至 2025-06-04，OpenAI 在 ChatGPT Enterprise/Edu 中已将能力分为：

- Chat Connectors：实时访问、行内引用
- Deep Research Connectors：长报告、内部与外部综合
- Synced Connectors：预索引，提升召回质量与速度
- Custom Connectors via MCP：管理员发布、自定义扩展

启示：

- 不同问答深度应有不同入口与成本模型。
- 预索引与实时查询应双轨共存。
- 连接器与扩展能力必须纳入管理员治理。

来源：

- https://help.openai.com/en/articles/10128477-chatgpt-enterprise-edu-release-notes
- https://openai.com/index/introducing-deep-research/

## 5.2 Microsoft：搜索是组织层，Chat 是执行层

Microsoft 365 Copilot Search 的公开设计非常明确：

- Search 是统一搜索层
- Chat 是深度交互层
- Search 可跳转到 Chat 做进一步操作
- Web grounding 有管理员开关、查询引用和审计日志

启示：

- 搜索和问答不应完全混同。
- 外部搜索必须是可治理、可审计的企业能力。
- 用户应知道系统到底基于“组织内数据”还是“组织外数据”作答。

来源：

- https://learn.microsoft.com/en-us/microsoft-365/copilot/microsoft-365-copilot-search
- https://learn.microsoft.com/en-us/microsoft-365/copilot/manage-public-web-access

## 5.3 Claude / Anthropic：把引用做成模型原生能力

Anthropic 公开强调两点：

- 引用不应依赖脆弱的 prompt engineering
- 应该尽可能做到具体文本片段级引用

启示：

- 当前 answer evidence 可以继续保留
- 但长期应向“答案片段级引用”升级，而不是停留在来源列表级

来源：

- https://claude.com/blog/introducing-citations-api
- https://support.claude.com/en/articles/11088779-using-google-drive-cataloging-on-the-enterprise-plan

## 5.4 Glean / Notion：统一搜索入口，但严格权限继承

Glean 和 Notion 的共同特征是：

- 一个统一入口
- 多连接器整合
- 答案建立在权限继承之上
- 优先解决“找得到”和“信得过”

启示：

- 当前共享 Agent / 共享 KB 的租户隔离方向是正确的
- 后续要继续优先保证权限正确，再扩展更复杂的智能能力

来源：

- https://docs.glean.com/connectors/native/gdrive/security/permissions
- https://www.notion.com/feature/enterprise-search
- https://www.notion.com/releases/2025-05-13

---

## 六、目标态建议

建议将智能问答产品能力重构为三个清晰层级。

## 6.1 第一层：知识搜索

定位：

- 快速定位文档、FAQ、相关片段

关键特征：

- 以搜索结果与摘要为主
- 不默认进入长答案生成
- 明确展示来源与命中范围

适用场景：

- 找制度
- 找文件
- 找 FAQ
- 找具体片段

## 6.2 第二层：可信问答

定位：

- 基于组织内知识给出可引用、可反馈的答案

关键特征：

- 默认入口
- 强制来源展示
- 强化证据层
- 支持来源级反馈
- 不默认开放复杂工具链

适用场景：

- 日常知识问答
- FAQ 问答
- 基于多个文档的归纳总结

## 6.3 第三层：深度研究 / Agent

定位：

- 面向复杂任务、跨源检索、工具调用、多步推理

关键特征：

- 单独入口或显式开关
- 成本更高
- 审计更强
- 能力灰度开放

适用场景：

- 跨多个系统的信息整合
- 复杂调研
- 结构化工具调用

---

## 七、迭代建议

## 7.1 P0：先收敛契约与产品边界

1. 统一“会话只是容器”的实现和前端认知，移除无效的 `agent_config` 创建参数。
2. 清理前后端漂移字段，补齐或删除 `mcp_service_ids` 这类未闭环参数。
3. 将入口明确分层为“搜索”“可信问答”“深度研究/Agent”，不要全部堆在默认输入区。
4. 管理员策略上默认关闭高风险高成本能力，如 Web Search、MCP、深度 Agent。

## 7.2 P1：增强可信问答能力

1. 把 evidence 从“来源列表级”升级到“回答片段级”。
2. 将置信度拆成两个维度：
   - 答案可信度
   - 来源健康度
3. 在前端区分“组织内来源”“Web 来源”“FAQ 来源”。
4. 为来源冲突、来源过期、证据不足增加明确提示语义。

## 7.3 P1：补执行快照与审计

建议新增一次问答执行快照表或扩展消息元数据，记录：

- mode
- agent_id
- model_id
- kb_ids / knowledge_ids
- web_search_enabled
- memory_enabled
- mcp_scope
- prompt_version
- retrieval_tenant_id

这样后续才能做：

- 复盘
- 回归
- 数据分析
- SLA 和成本分层

## 7.4 P2：优化检索与连接器策略

1. 将稳定知识源尽量预索引化。
2. 将强时效知识源尽量实时检索化。
3. 对不同来源建立不同新鲜度策略。
4. 将 connector / MCP 的接入纳入统一治理，而不是前端裸暴露。

## 7.5 P2：构建运营闭环

基于已有 `answer_evidence`、`document_access_logs`、`source_feedback`：

1. 做低置信问答看板
2. 做过期来源看板
3. 做高频被引用知识看板
4. 做反馈驱动的来源治理机制

---

## 八、最终判断

当前智能问答的基础架构是成立的，且已经具备较强的可扩展性与可信问答雏形。主要问题不在“技术链路缺失”，而在：

- 产品层次混杂
- 部分前后端契约漂移
- 可信能力尚未做到答案片段级
- 治理策略仍弱于能力扩展速度

因此，后续最优路线不是继续横向叠加功能，而是按以下顺序推进：

1. 收敛默认入口与模式边界
2. 修正契约与执行快照
3. 提升证据层与可信度
4. 再继续开放更强的 Agent / Connector 能力

这条路线更符合企业知识库产品落地规律，也更接近头部厂商在 2025 年后的公开设计共识。
