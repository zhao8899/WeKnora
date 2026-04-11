# 上游能力待吸收 Backlog

## 文档目的

本文档用于跟踪 `upstream/main` 中值得吸收的能力设计，服务于两种后续路径：

1. 合并上游代码时，快速定位需要重点检查的提交和文件。
2. 不直接合并上游代码时，只吸收其设计理念，结合当前 `custom-main` 的产品结构自行开发。

当前判断：

- `custom-main` 的主线是企业知识门户一期产品化。
- `upstream/main` 的主线是通用平台能力扩展。
- 后续建议优先吸收“能力设计”，而不是直接照搬前端结构。

## 使用方式

每项 backlog 都包含 6 个维度：

- 功能目标：上游这项能力解决什么问题。
- 上游提交：定位提交范围，便于后续复查 diff。
- 核心代码：最值得阅读的上游文件，不要求照抄。
- 代码说明：这些文件各自承担什么职责。
- 设计理念：应该吸收的抽象思路。
- 自研建议：如果不合并上游，建议如何在本分支中独立实现。

## 优先级定义

- `P0`：建议尽早纳入规划，和当前知识门户方向强相关。
- `P1`：重要但不是一期必须，可在主结构稳定后吸收。
- `P2`：偏扩展能力或特定场景，后置处理。

---

## P0-1 Azure OpenAI 全量支持

### 功能目标

补齐企业环境常见的模型接入诉求，让平台可以同时支持：

- Azure OpenAI Chat
- Azure OpenAI VLM
- Azure OpenAI Embedding
- Embedding dimensions 参数控制
- Azure 特殊连通性校验与部署名兼容

### 上游提交

- `8ceb0b19` feat: add Azure OpenAI provider constants and URL detection
- `c9001ef9` feat: add Azure OpenAI provider registration with metadata
- `22ec5757` feat: add Azure OpenAI support for chat models
- `3cfd034e` feat: add Azure OpenAI support for VLM models
- `f72b032c` feat: add Azure OpenAI to frontend provider list and i18n
- `2eb2dea6` feat: add Azure OpenAI support for embedding models
- `4e4118d4` fix: pass provider to connection test so Azure OpenAI uses correct endpoint
- `c6215405` fix: override AzureModelMapperFunc to preserve deployment name as-is
- `188f3172` fix: treat 400 errors as successful connection in model check
- `c663182a` feat: add support for dimensions parameter in Azure OpenAI and OpenAI embedding requests
- `5233394d` fix: gate Azure OpenAI dimensions support

### 核心代码

后端：

- [internal/models/provider/azure_openai.go](D:\workSpace\WeKnora\internal\models\provider\azure_openai.go)
- [internal/models/embedding/azure_openai.go](D:\workSpace\WeKnora\internal\models\embedding\azure_openai.go)
- [internal/models/provider/provider.go](D:\workSpace\WeKnora\internal\models\provider\provider.go)
- [internal/application/service/model.go](D:\workSpace\WeKnora\internal\application\service\model.go)
- [internal/handler/model.go](D:\workSpace\WeKnora\internal\handler\model.go)
- [internal/types/model.go](D:\workSpace\WeKnora\internal\types\model.go)

前端：

- [frontend/src/views/settings/ModelSettings.vue](D:\workSpace\WeKnora\frontend\src\views\settings\ModelSettings.vue)
- [frontend/src/api/model/index.ts](D:\workSpace\WeKnora\frontend\src\api\model\index.ts)
- [frontend/src/components/ModelEditorDialog.vue](D:\workSpace\WeKnora\frontend\src\components\ModelEditorDialog.vue)
- [frontend/src/components/ModelSelector.vue](D:\workSpace\WeKnora\frontend\src\components\ModelSelector.vue)
- [frontend/src/i18n/locales/zh-CN.ts](D:\workSpace\WeKnora\frontend\src\i18n\locales\zh-CN.ts)

### 代码说明

- `provider/azure_openai.go`：定义 Azure OpenAI 提供商识别、元数据、URL 适配等规则。
- `embedding/azure_openai.go`：处理 Azure Embedding 请求构造与响应适配。
- `service/model.go` + `handler/model.go`：把 provider 能力暴露到模型管理接口。
- `ModelSettings.vue`：承载提供商配置、模型列表、连接测试等前端入口。
- `ModelEditorDialog.vue`：决定 provider-specific 表单字段是否需要单独展示。

### 设计理念

- 不把 Azure 只当作“另一个 OpenAI URL”，而是当作单独 provider 处理。
- provider 差异要在后端建模，前端只是消费元数据和表单结构。
- 模型连通性校验不能只看 HTTP 200，要按 provider 语义判断。

### 自研建议

- 保留你当前设置中心结构，不直接照搬上游设置页布局。
- 在后端先抽象 `ProviderCapabilities` 或等价结构，统一声明：
  - 支持的模型类型
  - 是否需要 deployment name
  - 是否支持 dimensions
  - 如何校验连接
- 前端只在“模型管理”分组中增加 Azure OpenAI 配置项，不改变你的门户首页和主菜单。

### 推荐结论

- 建议吸收设计并优先自研。
- 不建议直接整体拷贝上游前端设置页。

---

## P0-2 Web Search Provider 扩展

### 功能目标

增加可配置的网络搜索提供商，使检索增强问答可以不依赖单一供应商，并支持 Ollama Web Search Provider。

### 上游提交

- `fd182641` feat(web-search): add ollama web search provider support

### 核心代码

后端：

- [internal/application/service/web_search_provider.go](D:\workSpace\WeKnora\internal\application\service\web_search_provider.go)
- [internal/infrastructure/web_search/ollama.go](D:\workSpace\WeKnora\internal\infrastructure\web_search\ollama.go)
- [internal/types/web_search_provider.go](D:\workSpace\WeKnora\internal\types\web_search_provider.go)
- [internal/router/router.go](D:\workSpace\WeKnora\internal\router\router.go)
- [migrations/versioned/000030_web_search_providers.up.sql](D:\workSpace\WeKnora\migrations\versioned\000030_web_search_providers.up.sql)

前端：

- [frontend/src/api/web-search-provider.ts](D:\workSpace\WeKnora\frontend\src\api\web-search-provider.ts)
- [frontend/src/views/settings/WebSearchSettings.vue](D:\workSpace\WeKnora\frontend\src\views\settings\WebSearchSettings.vue)
- [frontend/src/views/settings/Settings.vue](D:\workSpace\WeKnora\frontend\src\views\settings\Settings.vue)

### 代码说明

- `web_search_provider.go`：提供商的增删改查、启停、验证等业务逻辑入口。
- `ollama.go`：对接具体供应商协议，通常是 HTTP 请求构造、结果标准化。
- `types/web_search_provider.go`：定义 provider 配置结构和字段约束。
- `WebSearchSettings.vue`：配置页面，承载 provider 列表和编辑表单。

### 设计理念

- Web Search 应按 provider 插件化设计，而不是把搜索逻辑写死在单一调用路径里。
- 搜索 provider 的配置、启停、默认选择应落库管理。
- 前端只负责配置，不直接耦合具体搜索实现细节。

### 自研建议

- 保留你当前设置页侧栏结构，在设置中心新增“网络搜索”分组即可。
- 后端按接口抽象：
  - `Search(ctx, query)`
  - `ValidateConfig(ctx, config)`
  - `ProviderMeta()`
- 即使暂时只接一个 provider，也按多 provider 结构设计，避免后面重做。

### 推荐结论

- 建议优先吸收。
- 可参考上游落库和 provider 抽象方式，自行实现更贴合你当前产品结构的配置页面。

---

## P0-3 Notion 数据源连接器

### 功能目标

支持将 Notion 作为知识数据源导入平台，纳入知识库建设流程。

### 上游提交

- `9f35abaf` feat(notion): add Notion API client and type definitions
- `2cd979e1` feat(notion): implement Connector interface, markdown renderer, and DI registration

### 核心代码

- [internal/datasource/connector/notion/client.go](D:\workSpace\WeKnora\internal\datasource\connector\notion\client.go)
- [internal/datasource/connector/notion/types.go](D:\workSpace\WeKnora\internal\datasource\connector\notion\types.go)
- [internal/datasource/connector/notion/connector.go](D:\workSpace\WeKnora\internal\datasource\connector\notion\connector.go)
- [internal/datasource/connector/notion/markdown.go](D:\workSpace\WeKnora\internal\datasource\connector\notion\markdown.go)
- [internal/application/service/datasource_service.go](D:\workSpace\WeKnora\internal\application\service\datasource_service.go)
- [internal/container/container.go](D:\workSpace\WeKnora\internal\container\container.go)
- [frontend/src/api/datasource/index.ts](D:\workSpace\WeKnora\frontend\src\api\datasource\index.ts)
- [frontend/src/views/knowledge/settings/DataSourceEditorDialog.vue](D:\workSpace\WeKnora\frontend\src\views\knowledge\settings\DataSourceEditorDialog.vue)

### 代码说明

- `client.go`：对接 Notion HTTP API，负责认证、分页和原始数据获取。
- `types.go`：把 Notion 的页面、块、数据库等结构映射到本地类型。
- `connector.go`：实现统一数据源接口，接入平台的数据导入管道。
- `markdown.go`：把 Notion 内容转成平台下游可消费的 Markdown 或中间文本。
- `datasource_service.go`：统一管理各类数据源的创建、执行、同步。

### 设计理念

- “连接器”是稳定边界，第三方源接入时不要把外部 API 细节散落到业务层。
- 数据接入的关键不是拿到原始 JSON，而是做内容标准化和结构降维。
- Markdown 或统一中间格式是非常实用的落地策略。

### 自研建议

- 直接学习上游 connector 接口边界，不必复制实现细节。
- 你可以定义自己的三层：
  - RemoteClient：拉取数据
  - Transformer：转统一中间结构
  - Importer：写入知识库
- 前端入口建议仍放在知识库设置页，不增加主导航。

### 推荐结论

- 强烈建议吸收，和知识门户方向高度一致。
- 最适合“只吸设计，自行开发”。

---

## P0-4 文档总结能力增强

### 功能目标

增强知识文档的摘要生成与展示能力，提升“先看摘要，再决定是否深入阅读”的使用体验。

### 上游提交

- `cb36570c` feat(summary): enhance document summarization capabilities and UI

### 核心代码

- [config/prompt_templates/generate_summary.yaml](D:\workSpace\WeKnora\config\prompt_templates\generate_summary.yaml)
- [internal/application/service/knowledge.go](D:\workSpace\WeKnora\internal\application\service\knowledge.go)
- [internal/application/service/knowledgebase.go](D:\workSpace\WeKnora\internal\application\service\knowledgebase.go)
- [frontend/src/views/knowledge/KnowledgeBase.vue](D:\workSpace\WeKnora\frontend\src\views\knowledge\KnowledgeBase.vue)
- [frontend/src/components/doc-content.vue](D:\workSpace\WeKnora\frontend\src\components\doc-content.vue)

### 代码说明

- `generate_summary.yaml`：提示词模板，决定总结输出结构和风格。
- `knowledge.go` / `knowledgebase.go`：决定何时生成、如何存储、何时返回摘要。
- `KnowledgeBase.vue`：知识库详情页承载摘要展示与交互。
- `doc-content.vue`：文档内容和摘要展示可能在这里耦合。

### 设计理念

- 摘要能力不是单独的 AI 按钮，而应该成为文档消费链路的一部分。
- 摘要最好作为结构化产物存储，而不是每次现算。
- UI 上要区分“原文”和“摘要”，避免误导用户。

### 自研建议

- 你的首页已经强调“统一查看、统一问答”，摘要能力适合直接并入知识库详情页。
- 先做后端摘要任务和结果缓存，再做前端展示。
- 设计上可以加：
  - 摘要状态
  - 最近生成时间
  - 手动重生成入口

### 推荐结论

- 建议尽快规划吸收。
- 这是和当前门户价值直接挂钩的能力。

---

## P1-1 Vector Store 抽象

### 功能目标

把向量存储从隐含实现提升为显式实体，给后续知识检索扩展和多向量库支持打基础。

### 上游提交

- `69785ad0` feat: add VectorStore entity, repository, and migrations
- `9d021a29` feat(elasticsearch): enhance ID field handling with dynamic suffix detection

### 核心代码

- [internal/application/repository/vectorstore.go](D:\workSpace\WeKnora\internal\application\repository\vectorstore.go)
- [internal/types/vectorstore.go](D:\workSpace\WeKnora\internal\types\vectorstore.go)
- [internal/types/interfaces/vectorstore.go](D:\workSpace\WeKnora\internal\types\interfaces\vectorstore.go)
- [internal/application/repository/retriever/elasticsearch/v7/repository.go](D:\workSpace\WeKnora\internal\application\repository\retriever\elasticsearch\v7\repository.go)
- [internal/application/repository/retriever/elasticsearch/v8/repository.go](D:\workSpace\WeKnora\internal\application\repository\retriever\elasticsearch\v8\repository.go)
- [migrations/versioned/000032_vector_stores.up.sql](D:\workSpace\WeKnora\migrations\versioned\000032_vector_stores.up.sql)

### 代码说明

- `vectorstore.go`：把向量存储的生命周期和元数据独立出来。
- `interfaces/vectorstore.go`：明确 repository 或 service 的抽象边界。
- `elasticsearch/*/repository.go`：说明现有检索实现如何和向量存储抽象衔接。

### 设计理念

- 当检索系统变复杂时，向量存储必须成为显式概念，而不是隐含在知识库里。
- “知识库”和“向量索引”不一定是一对一永久绑定关系。
- 未来切换或并存多个后端时，元数据层抽象很关键。

### 自研建议

- 如果当前业务暂时不需要多向量库，先只做抽象层，不一定做完整 UI。
- 先建库表和后端接口，再决定是否需要管理员配置界面。
- 自研时重点看上游的实体拆分思路，不必复制具体字段。

### 推荐结论

- 适合第二批吸收。
- 偏后端基础建设，前台价值不如 P0 直接。

---

## P1-2 ASR 语音识别

### 功能目标

让平台支持音频转文本，为问答、文档处理或多模态场景提供语音入口。

### 上游提交

- `624e24ba` feat(asr): integrate Automatic Speech Recognition (ASR) support
- `c93f4e94` feat(asr): add ASR model connection check and update related components
- `4ea58bc6` feat(assets): add ASR test audio file and embed it in the application

### 核心代码

- [internal/models/asr/asr.go](D:\workSpace\WeKnora\internal\models\asr\asr.go)
- [internal/models/asr/openai.go](D:\workSpace\WeKnora\internal\models\asr\openai.go)
- [internal/assets/embed.go](D:\workSpace\WeKnora\internal\assets\embed.go)
- [internal/handler/model.go](D:\workSpace\WeKnora\internal\handler\model.go)
- [frontend/src/views/settings/ModelSettings.vue](D:\workSpace\WeKnora\frontend\src\views\settings\ModelSettings.vue)
- [frontend/src/components/Input-field.vue](D:\workSpace\WeKnora\frontend\src\components\Input-field.vue)

### 代码说明

- `asr.go`：语音识别模型抽象定义。
- `openai.go`：某个 ASR provider 的具体实现。
- `embed.go`：嵌入测试资源，用于模型检查或演示。
- `Input-field.vue`：如果上游把语音输入挂到聊天输入区，这里是重点观察对象。

### 设计理念

- ASR 应视为模型能力的一种，而不是临时附件处理逻辑。
- 模型管理侧要能校验 ASR provider 配置是否可用。
- 聊天输入、文件上传、后台处理三者要分开设计，不要糊成一个入口。

### 自研建议

- 如果近期目标还是知识门户优先，先只做模型层和服务层，不急着改前端入口。
- 后续若接聊天输入，可在提问区增加独立“语音转文本”动作，不要挤占主提问流程。

### 推荐结论

- 可吸收，但不建议抢在 P0 前面。

---

## P1-3 音频预览

### 功能目标

支持在前端直接预览音频类资源，提升多媒体知识内容的可访问性。

### 上游提交

- `155fc690` feat(audio): add audio file preview support and enhance UI components

### 核心代码

- [frontend/src/components/document-preview.vue](D:\workSpace\WeKnora\frontend\src\components\document-preview.vue)
- [frontend/src/components/doc-content.vue](D:\workSpace\WeKnora\frontend\src\components\doc-content.vue)
- [frontend/src/views/chat/index.vue](D:\workSpace\WeKnora\frontend\src\views\chat\index.vue)

### 代码说明

- `document-preview.vue`：高概率承载不同文件类型预览切换。
- `doc-content.vue`：知识文档内容展示层，可能新增音频渲染分支。
- `chat/index.vue`：如果消息附件支持音频预览，这里需要打通展示逻辑。

### 设计理念

- 文件预览应按 MIME 或资源类型做统一分发，不要每个页面单独判断。
- 音频预览不只是播放器，还应考虑元信息、下载、转写联动。

### 自研建议

- 如果你要自研，建议先抽一个统一的 `PreviewResolver` 思路：
  - 文本
  - 图片
  - 音频
  - 视频
  - 其他附件
- 这样后面接视频多模态也不会返工。

### 推荐结论

- 可与 ASR 一起评估。
- 本身不是门户一期的核心阻塞项。

---

## P1-4 权限与知识边界增强

### 功能目标

增强知识权限控制和资源清理，减少越权访问与垃圾资源残留。

### 上游提交

- `b1fe7abb` feat(knowledge): enhance knowledge tag batch update with authorization checks
- `3756c7c7` feat(session): restrict @mentions to agent's allowed knowledge base scope
- `33e919cc` fix(knowledge): delete extracted images from storage when knowledge is removed

### 核心代码

- [internal/application/service/knowledge.go](D:\workSpace\WeKnora\internal\application\service\knowledge.go)
- [internal/application/service/session_agent_qa.go](D:\workSpace\WeKnora\internal\application\service\session_agent_qa.go)
- [internal/application/service/session_knowledge_qa.go](D:\workSpace\WeKnora\internal\application\service\session_knowledge_qa.go)
- [internal/application/service/session_qa_helpers.go](D:\workSpace\WeKnora\internal\application\service\session_qa_helpers.go)
- [internal/application/service/knowledge_image_cleanup_test.go](D:\workSpace\WeKnora\internal\application\service\knowledge_image_cleanup_test.go)

### 代码说明

- `knowledge.go`：知识对象操作权限和删除副作用处理。
- `session_*`：会话问答过程中如何限制知识范围。
- `knowledge_image_cleanup_test.go`：验证删除知识时资源清理链路。

### 设计理念

- 权限控制不应只发生在前端可见性层面，必须落在 service 层。
- 删除知识资源时要考虑附属产物的生命周期，如抽取图片、索引、缓存。
- agent 可访问的知识范围应是显式约束，不是调用方自觉遵守。

### 自研建议

- 这部分非常适合直接自研，因为它更接近你的业务规则。
- 可以优先定义 3 类约束：
  - 知识可见范围
  - agent 可引用范围
  - 删除后的副作用清理范围

### 推荐结论

- 建议尽快吸收后端策略。
- 即使前端暂不改，后端也值得先补齐。

---

## P2-1 视频多模态

### 功能目标

支持视频资源的抽帧、信息提取和后续多模态理解。

### 上游提交

- `b80bdb10` feat: add video multimodal functionality
- `46ff7a7d` fix: 修正视频相关迁移文件版本号冲突，从 000032 改为 000033

### 核心代码

- [internal/application/service/video_multimodal.go](D:\workSpace\WeKnora\internal\application\service\video_multimodal.go)
- [internal/models/video/extractor.go](D:\workSpace\WeKnora\internal\models\video\extractor.go)
- [internal/models/video/ffmpeg_extractor.go](D:\workSpace\WeKnora\internal\models\video\ffmpeg_extractor.go)
- [internal/types/chunk.go](D:\workSpace\WeKnora\internal\types\chunk.go)
- [migrations/versioned/000033_add_video_info_to_chunks.up.sql](D:\workSpace\WeKnora\migrations\versioned\000033_add_video_info_to_chunks.up.sql)
- [docker/Dockerfile.app](D:\workSpace\WeKnora\docker\Dockerfile.app)

### 代码说明

- `video_multimodal.go`：视频处理业务入口。
- `extractor.go`：定义视频抽取接口。
- `ffmpeg_extractor.go`：基于 ffmpeg 的具体实现。
- `chunk.go` + migration：说明视频信息最终如何落到知识分块或元数据层。
- `Dockerfile.app`：提示运行时依赖可能需要 ffmpeg。

### 设计理念

- 视频能力不是简单“允许上传 mp4”，而是完整的信息抽取链路。
- 抽取器应可替换，ffmpeg 只是其中一种实现。
- 视频元数据最好和 chunk / 文档结构关联，而不是散落为临时字段。

### 自研建议

- 如果没有明确视频业务场景，先不要重投入。
- 但可以提前把预览层和处理层拆开，避免后面接入时重构。

### 推荐结论

- 后置处理。
- 适合有明确业务需求时再做。

---

## P2-2 IM / 企业微信引用上下文

### 功能目标

让 IM 接入链路识别引用消息、处理 @mention，并把引用上下文注入到问答提示中，减少上下文缺失。

### 上游提交

- `9790cb48` feat(im): add QuotedMessage type and Quote field to IncomingMessage
- `3587ddb1` feat(wecom): implement quote extraction and populate in handleCallback
- `4e7922c8` feat(im): add QuotedContext to shared types and IM service layer
- `dc5046ad` feat(pipeline): inject QuotedContext at LLM prompt stage
- `b6655274` fix(im): anti-hallucination for non-text quotes and unprocessable media messages
- `39831fb6` fix(wecom): strip @mention from group chat messages to fix slash commands

### 核心代码

- [internal/im/adapter.go](D:\workSpace\WeKnora\internal\im\adapter.go)
- [internal/im/service.go](D:\workSpace\WeKnora\internal\im\service.go)
- [internal/im/wecom/quote.go](D:\workSpace\WeKnora\internal\im\wecom\quote.go)
- [internal/im/wecom/webhook_adapter.go](D:\workSpace\WeKnora\internal\im\wecom\webhook_adapter.go)
- [internal/application/service/chat_pipeline/into_chat_message.go](D:\workSpace\WeKnora\internal\application\service\chat_pipeline\into_chat_message.go)
- [internal/types/context_helpers.go](D:\workSpace\WeKnora\internal\types\context_helpers.go)

### 代码说明

- `quote.go`：企业微信消息中引用信息的提取逻辑。
- `service.go`：IM 消息进入业务层后的统一加工。
- `into_chat_message.go`：最终把引用上下文带入 LLM 输入链路。
- `context_helpers.go`：上下文拼装和传递辅助结构。

### 设计理念

- IM 消息不是普通纯文本输入，必须保留消息来源结构。
- 引用上下文应该进入 prompt 组装层，而不是只停留在 webhook 层。
- 非文本媒体引用需要保守处理，避免模型幻觉。

### 自研建议

- 如果后续要做企业微信深度集成，先学上游的“上下文对象贯穿全链路”做法。
- 不建议只在 webhook 层打补丁，应从入站类型定义开始设计。

### 推荐结论

- 当前可保留为研究项。
- 有明确 IM 需求时再投入。

---

## P2-3 基础稳定性与可配置项

### 功能目标

补齐一批低成本但有实际收益的稳定性增强项。

### 上游提交

- `54da98fc` feat: add docx max pages env config
- `8a375749` fix(i18n): use WEKNORA_LANGUAGE env for prompt language fallback instead of hardcoded zh-CN
- `8daecd34` fix(frontend): add null checks for markdown code block text parameter
- `d68c94a1` fix(stream): use io.EOF instead of errors.New("EOF") in SSEReader
- `29de7dfb` fix: allow MINIO_ENDPOINT to be configured via environment variable
- `f63c9f6d` fix: resolve tool name duplication in streaming tool calls
- `d5ecc150` feat(agent): support customizable LLM call timeout and add docker-compose mapping
- `bfa3341d` Fix: Dockerfile build error (duplicate libsqlite3-0 and ffmpeg installation)
- `2cbd98e3` fix(migrations): correct migration numbers and remove broken trigger

### 核心代码

- [docreader/config.py](D:\workSpace\WeKnora\docreader\config.py)
- [docreader/parser/docx_parser.py](D:\workSpace\WeKnora\docreader\parser\docx_parser.py)
- [internal/middleware/language.go](D:\workSpace\WeKnora\internal\middleware\language.go)
- [internal/models/chat/sse_reader.go](D:\workSpace\WeKnora\internal\models\chat\sse_reader.go)
- [internal/application/service/file/s3.go](D:\workSpace\WeKnora\internal\application\service\file\s3.go)
- [docker-compose.yml](D:\workSpace\WeKnora\docker-compose.yml)
- [docker/Dockerfile.app](D:\workSpace\WeKnora\docker\Dockerfile.app)
- [.env.example](D:\workSpace\WeKnora\.env.example)

### 代码说明

- `docx_parser.py`：文档导入的边界控制，防止超大 docx 处理失控。
- `language.go`：语言回退逻辑，影响 prompt 或国际化体验。
- `sse_reader.go`：流式读取鲁棒性。
- `s3.go`：对象存储配置兼容性。
- `docker-compose.yml` / `Dockerfile.app`：部署体验和运行依赖修正。

### 设计理念

- 这类改动单点不大，但长期会显著降低运行成本和维护风险。
- 可配置项应尽量显式进入 `.env.example` 和文档。

### 自研建议

- 适合作为持续性 maintenance backlog，不必集中一次性完成。
- 每次碰到相关模块时顺手吸收即可。

### 推荐结论

- 按需滚动吸收。
- 不需要专门大规模合并。

---

## 建议执行顺序

如果后续不直接合并上游，而是“参考设计、自主开发”，建议顺序如下：

1. `P0-1` Azure OpenAI 全量支持
2. `P0-2` Web Search Provider 扩展
3. `P0-3` Notion 数据源连接器
4. `P0-4` 文档总结能力增强
5. `P1-4` 权限与知识边界增强
6. `P1-1` Vector Store 抽象
7. `P1-2` ASR 语音识别
8. `P1-3` 音频预览
9. `P2-3` 基础稳定性与可配置项
10. `P2-1` 视频多模态
11. `P2-2` IM / 企业微信引用上下文

## 对当前分支的落地原则

- 保持首页、FAQ、知识库、共享空间这套企业知识门户结构不变。
- 上游能力优先往设置中心、知识库详情、问答链路里吸收，不反向重做主导航。
- 优先吸收后端抽象和接口设计，前端只做最小必要入口。
- 对上游代码的使用原则是：
  - 看抽象边界
  - 看数据结构
  - 看失败处理
  - 不盲目复制 UI 结构

## 后续更新建议

每次决定吸收某项能力时，在本文件对应条目下追加：

- 状态：未开始 / 设计中 / 开发中 / 已上线 / 已放弃
- 实施分支：例如 `feat/azure-openai-support`
- 本地实现文件：记录你自己的落地文件
- 差异说明：如果与上游设计不同，说明为什么不同

建议格式：

```md
### 本地实施记录

- 状态：开发中
- 实施分支：`feat/xxx`
- 本地实现文件：
  - `internal/...`
  - `frontend/...`
- 与上游差异：
  - 只吸收 provider 抽象，不复用其设置页 UI
  - 将能力入口挂到现有设置中心二级导航
```
