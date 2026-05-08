# WeKnora 上游能力吸收计划

**日期：** 2026-05-07  
**上游仓库：** https://github.com/Tencent/WeKnora  
**本地分支：** `custom-main`

## 摘要

已查询并刷新原始仓库 `upstream=https://github.com/Tencent/WeKnora.git`。当前上游主线为 `0.5.1`，并已有 Unreleased 更新；本地 `custom-main` 相对上游分叉明显，且工作区有大量未提交改动，因此不建议直接整体合并，应按能力模块分批吸收。

上游主要新增方向包括：Wiki Mode、Langfuse 可观测性、按知识库配置索引/向量库、自适应分块、IM/小程序/桌面端、更多连接器、模型与存储扩展、Agent 工具增强。

## 优先级

### P0：优先吸收，直接提升商业化闭环

- Wiki Mode：自动把原始文档沉淀为结构化 Markdown Wiki，并带 WikiBrowser 和 Wiki 知识图谱。
- Langfuse Observability：追踪 Agent ReAct、LLM 调用、Token、工具调用、异步任务，支撑企业客户验收和运维。
- 自适应分块与调试面板：3 层 chunking 策略、分块预览接口、分块质量统计，直接影响 RAG 准确率。
- 自定义索引策略：每个知识库可独立启用 Vector、Keyword、Wiki、Knowledge Graph。
- Vector Store 管理：向量库 CRUD、连接测试、按知识库绑定不同向量库。

### P1：建议吸收，增强产品差异化

- Yuque / Notion 数据源连接器：补齐企业知识来源，和现有飞书/网页/RSS 形成连接器矩阵。
- 微信小程序：作为移动端轻量入口，适合企业内部试点和微信生态客户。
- IM 管理增强：租户级 IM Channel 总览、会话搜索、会话置顶、IM 来源标识。
- Agent 工具增强：`json_repair`、更强 data_analysis SQL 校验、Excel 多 sheet 分析。
- 附件处理能力：聊天中上传附件并注入问答上下文，适合临时资料分析场景。

### P2：选择性吸收，视商业路线决定

- Desktop client：适合本地私有知识助手或个人部署，但企业后台优先级较低。
- WeKnora Cloud Provider：若本项目要保留完全私有化定位，应谨慎吸收云托管依赖，只吸收 provider 抽象。
- Chrome Extension / ClawHub Skill：适合生态分发，但不是当前企业知识库闭环的首要能力。
- ASR 与音频转写：对会议纪要、培训资料场景有价值，可作为行业包能力。

## 实施计划

### 阶段 1：只读评估与差异拆分

- 建立上游能力清单：Wiki、Langfuse、Chunking、VectorStore、IndexingStrategy、Connectors、IM、小程序。
- 对每个能力标注影响面：数据库迁移、后端服务、前端页面、配置项、API、测试。
- 先排除与本地定制冲突大的模块：企业门户、角色权限、智能 QA、现有知识运营看板相关代码。

### 阶段 2：P0 能力分支吸收

- 先吸收自适应分块：包含 chunker 后端、`/api/v1/chunker/preview`、前端调试面板、相关配置。
- 再吸收 Vector Store 与索引策略：包含 migrations、types、handler、settings UI、知识库绑定逻辑。
- 再吸收 Langfuse：只接入可观测性链路，不改变现有业务逻辑。
- 最后吸收 Wiki Mode：先作为独立知识库类型/入口接入，避免干扰现有文档/FAQ 主链路。

### 阶段 3：P1 能力场景化吸收

- 数据源连接器按商业优先级接入：Yuque、Notion 优先，其他连接器后置。
- IM 增强只吸收管理、搜索、置顶、来源标识，不覆盖本地已有 IM 配置。
- 小程序作为独立客户端目录接入，先保持 API 兼容，不纳入核心后台验收。
- Agent 工具增强按工具逐个吸收，避免一次性重写 Agent 执行链。

### 阶段 4：产品化包装

- 将 Wiki Mode 包装为“自动知识沉淀”。
- 将 Langfuse 包装为“问答可观测与客户验收”。
- 将 Chunking Debug 包装为“知识质量调优工具”。
- 将 VectorStore/IndexingStrategy 包装为“企业级知识库治理与成本控制”。

## 验证计划

### 代码级验证

- 每个能力单独分支吸收，分别跑 `go test ./...`、`cd frontend && npm run type-check`、`cd frontend && npm run build`。
- 涉及迁移的能力，使用空库和已有库各跑一次 `make migrate-up`。

### 业务级验证

- Chunking：用中文制度文档、Markdown 手册、Excel、PDF 分别验证分块质量和召回。
- VectorStore：验证不同知识库绑定不同向量库，连接失败时不影响其他知识库。
- Wiki：验证文档导入、Wiki 页面生成、引用关系、图谱展示、问答引用。
- Langfuse：验证 Agent、普通问答、工具调用、异步任务都有 trace。
- 连接器：验证全量同步、增量同步、删除同步、凭证失败、单文档失败不阻断整体任务。

## 假设

- 不直接 merge `upstream/main`，因为本地 `custom-main` 有企业门户、权限、智能 QA 等定制改造，且工作区当前存在大量未提交改动。
- 优先服务商业运营目标：提升试点交付、问答质量、可观测性、知识沉淀和企业治理。
- 上游 `WeKnora Cloud` 相关能力默认不作为核心吸收对象，除非后续决定走云服务或混合云商业模式。
- 最佳吸收顺序为：Chunking -> VectorStore/IndexingStrategy -> Langfuse -> Wiki -> Connectors/IM/小程序。

## 当前落地状态

- 已开始阶段 2 的第一个 P0 切片：自适应分块与调试面板。
- 已引入自适应 chunker、`/api/v1/chunker/preview` 预览接口、前端分块策略/Token 上限/调试抽屉入口。
- 已验证：`go test ./internal/infrastructure/chunker ./internal/handler ./internal/application/service ./internal/types ./internal/router`、`cd frontend && npm run type-check`、`cd frontend && npm run build`。
- `go test ./...` 仍受本机环境影响：docreader client 测试需要 `localhost:50051` 服务；SSRF 测试受当前 DNS 将公开域名解析到 `198.18.0.0/15` 影响。
