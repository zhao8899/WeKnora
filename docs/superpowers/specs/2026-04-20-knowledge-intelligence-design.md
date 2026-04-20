# 知识智能全链路设计文档

**日期：** 2026-04-20  
**版本：** v2（根据深度评审意见修订）  
**状态：** 已确认，待实施  
**定位：** 将 WeKnora 从"知识检索工具"升级为企业"AI Executive Assistant"，三层递进形成完整飞轮

---

## 一、背景与目标

### 核心问题

1. **知识入库成本高**：员工手动上传文件，缺乏自动从网站、API、企业系统拉取的能力
2. **答案可信度不透明**：用户无法判断 AI 回答是否准确，缺少置信度和溯源信号，影响采纳率
3. **管理员无法度量价值**：看不到哪些知识被频繁引用、哪些文档从未被检索、知识库有哪些覆盖空白

### 设计定位（Karpathy EA 思路）

把系统定位为组织的 **EA（Executive Assistant）**：它能访问所有组织知识，在用户提问时跨源综合回答，并能主动告知"我在这方面的知识有限"。

### 三层架构目标

```
[入库层] 自动拉取 + 智能摘要  →  知识库持续"活"起来
     ↓
[解释层] 答案证据模型 + 来源反馈  →  用户信任每一个回答
     ↓
[洞察层] 三张日志表 → 知识健康看板  →  管理员看清 EA 的知识边界
```

---

## 二、整体架构

```
┌─────────────────────────────────────────────────────────┐
│                  WeKnora 知识智能全链路                    │
├─────────────────────────────────────────────────────────┤
│  [入库层] KnowledgeConnector Framework                   │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐               │
│  │ Web/RSS  │ │ REST API │ │ 企业系统  │               │
│  │（优先）   │ │（声明式） │ │（飞书→更多）│               │
│  └────┬─────┘ └────┬─────┘ └────┬─────┘               │
│       └────────────┴────────────┘                       │
│       ConnectorJob（调度 + 增量变更 + archived 归档）      │
│       自动摘要生成 + 实体标签提取                           │
├─────────────────────────────────────────────────────────┤
│  [解释层] Answer Explanation Layer                       │
│  复用现有 vector_score / rerank_score / keyword_hit      │
│  写入 answer_evidence → /confidence 接口 → 前端展示       │
│  来源级反馈 source_feedback → source_weight 更新          │
├─────────────────────────────────────────────────────────┤
│  [洞察层] 数据基础 → 看板                                 │
│  answer_evidence + document_access_log + source_feedback │
│  → 热点问题 / 覆盖空白 / 陈旧文档预警 / 引用热力图          │
└─────────────────────────────────────────────────────────┘
```

**关键设计原则：**
- **不动召回**：解释层只在 finalize 阶段写入证据，不修改检索逻辑
- **数据先于 UI**：三张日志表建好，看板才有意义
- **连接器通用化**：Web/RSS 验证整条链路，REST API 做声明式配置，不再复制飞书式定制 connector
- **先修偏差再扩功能**：config 加密、归档语义、版本更新三个问题先解决

---

## 三、先修实现偏差（前置工作）

在扩展任何新功能之前，必须先解决三个现有问题，否则后续所有功能都建在不稳定的基础上。

### 3.1 data_sources.config 加解密

**现状**：连接器配置（含 API Key、Token）以明文存入数据库。  
**风险**：数据库泄露 = 所有第三方连接认证全部暴露。  
**修复**：对 `data_sources.config` 字段实施 AES-256 应用层加密，密钥通过环境变量注入，不落库。

### 3.2 删除同步改为知识归档

**现状**：第三方来源同步时，若远端文档消失，仅记日志，知识条目状态不变。  
**风险**：已归档文档仍会被检索命中，影响答案质量；引用历史、source_weight 也无法正确反映文档状态。  
**修复**：远端消失 → 将知识条目状态改为 `archived`（禁用检索，保留引用历史），不做物理删除。

### 3.3 更新从"删除重建"改为版本更新

**现状**：文档内容变化时，删除旧知识条目、重建新条目，导致 ID 变化。  
**风险**：之前积累的 source_weight、反馈记录、引用计数全部归零，无法做来源权重和热力图。  
**修复**：通过 `external_id` 匹配已有知识条目，做内容和向量的原地更新，ID 不变。

---

## 四、解释层设计（原"置信度引擎"修订）

> **核心修订**：不新建检索系统，而是在现有召回链路末端增加"证据写入"，把已有得分结构持久化为可查询的答案证据。

### 4.1 答案证据模型

在 `finalize.go` 或 `observe.go` 阶段，把召回过程中已计算的得分写入 `answer_evidence` 表：

```sql
CREATE TABLE answer_evidence (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id      UUID NOT NULL REFERENCES sessions(id),
    message_id      UUID NOT NULL,           -- 关联到具体回答消息
    knowledge_id    UUID REFERENCES knowledge(id),
    chunk_id        UUID,                    -- 具体 chunk 粒度
    vector_score    FLOAT,                   -- 向量相似度（现有，直接写入）
    keyword_hit     BOOLEAN,                 -- 关键词命中（现有，直接写入）
    rerank_score    FLOAT,                   -- rerank 得分（现有，直接写入）
    match_type      VARCHAR(20),             -- 'vector' | 'keyword' | 'hybrid'
    source_url      TEXT,                    -- 来源链接（连接器入库时记录）
    source_channel  VARCHAR(50),             -- 来源渠道（web/feishu/api/upload）
    is_cited        BOOLEAN DEFAULT false,   -- 是否被最终引用进回答
    created_at      TIMESTAMPTZ DEFAULT NOW()
);
```

**不新增召回逻辑**：`vector_score`、`keyword_hit`、`rerank_score` 均来自现有 chat_pipeline，只是从内存写到数据库。

### 4.2 综合置信度计算

置信度在 `/confidence` 接口按需计算，不存储计算结果（权重可调，不影响历史数据）：

```
ConfidenceScore = w1×VectorSim + w2×KeywordHit + w3×SourceWeight + w4×Freshness

默认权重：
  w1 = 0.40  向量相似度
  w2 = 0.25  关键词命中
  w3 = 0.20  来源权重（来自 source_weight 字段，默认 1.0，由反馈调整）
  w4 = 0.15  时效性（基于 knowledge.updated_at 衰减）
```

**多源综合加成：**
- 2+ 个来源结论一致（is_cited=true 的 evidence 数量）→ 置信度 +10%，上限 100%
- 来源间存在矛盾（高分但结论冲突）→ 标注"存在不同说法"
- 仅有低分证据（max rerank_score < 0.4）→ 主动声明"知识库中未找到可靠依据"

### 4.3 新增后端接口

```
GET  /api/chat/answer/{message_id}/confidence
  → 返回：综合置信度、evidence 列表（含各维度得分）、来源元信息

POST /api/chat/answer/{message_id}/feedback
  → 写入 source_feedback 表（见 4.4）

GET  /api/source/{knowledge_id}/weight-history
  → 来源权重变化历史（管理员）
```

### 4.4 前端展示策略

| 置信度区间 | 颜色 | 展示文案 |
|-----------|------|---------|
| 85–100% | 绿色 | 高度可信，来自权威文档 |
| 60–84% | 蓝色 | 可参考，建议核对来源 |
| 40–59% | 橙色 | 仅供参考，知识库覆盖有限 |
| < 40% | 灰色 | 知识库中未找到可靠依据 |

**答案下方展示：**
```
● 置信度  ████████░░  82%   综合了 3 个来源

引用来源：
├─ 《员工手册 2024》第 12 页  ▶ 查看原文
├─ 《HR 政策更新通知》2024-03  ▶ 查看原文
└─ FAQ #47 年假相关

[👍 准确]  [✏️ 有误，我来纠正]  [❓ 来源看不懂]  [📅 文档已过期]
```

### 4.5 来源级反馈（source_feedback 表）

> **核心修订**：保留现有 message_feedback 的 like/dislike 作为轻反馈，新增 source_feedback 做来源粒度的精细反馈。

```sql
CREATE TABLE source_feedback (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id      UUID NOT NULL REFERENCES sessions(id),
    message_id      UUID NOT NULL,
    evidence_id     UUID REFERENCES answer_evidence(id),  -- 关联到具体证据
    knowledge_id    UUID REFERENCES knowledge(id),
    chunk_id        UUID,
    feedback_type   VARCHAR(30) NOT NULL,
    -- 'accurate' | 'partial' | 'wrong' | 'expired' | 'unclear' | 'correction'
    notes           TEXT,                    -- 用户可选填写说明
    user_id         UUID REFERENCES users(id),
    created_at      TIMESTAMPTZ DEFAULT NOW()
);
```

**反馈类型与权重影响：**

| 反馈类型 | source_weight 影响 | 后续动作 |
|---------|-------------------|---------|
| accurate（准确） | +0.02 | 无 |
| partial（部分正确） | 0 | 记录待审核 |
| wrong（有误） | -0.05 | 触发管理员审核通知 |
| expired（文档过期） | 0 | 标记 freshness_flag，触发重新拉取 |
| unclear（来源难懂） | 0 | 记录，供编辑优化参考 |
| correction（我来纠正） | -0.03 | 记录用户补充，待管理员审核 |

**source_weight 字段**：新增在 `knowledge` 表，初始值 1.0，由每日定时任务根据 source_feedback 聚合更新，不触发向量索引重建。

---

## 五、洞察层设计（数据基础先于看板）

> **核心修订**：先建三张日志表，看板是这三张表的聚合查询，不独立开发数据采集逻辑。

### 5.1 三张核心日志表

**① answer_evidence**（见第四节 4.1，同时服务解释层和洞察层）

**② document_access_log**

```sql
CREATE TABLE document_access_log (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    knowledge_id    UUID REFERENCES knowledge(id),
    session_id      UUID,
    message_id      UUID,
    access_type     VARCHAR(20),  -- 'retrieved' | 'reranked' | 'cited'
    created_at      TIMESTAMPTZ DEFAULT NOW()
);
```

三个 access_type 含义：
- `retrieved`：进入召回结果
- `reranked`：经过 rerank 仍在 top-k
- `cited`：被最终引用进回答（is_cited=true）

**③ source_feedback**（见第四节 4.5，同时服务反馈闭环和洞察层）

### 5.2 看板四类指标的数据来源

| 指标 | SQL 逻辑 |
|------|---------|
| 热点问题 Top20 | `sessions` 表提问内容聚类（可用 LLM 归类） |
| 覆盖空白 | `answer_evidence` 中 max(rerank_score) < 0.4 的问答 |
| 陈旧文档预警 | `knowledge.updated_at < NOW() - INTERVAL '90 days'` |
| 引用热力图 | `document_access_log` 按 knowledge_id 统计 cited 次数 |

### 5.3 看板上线时机

三张表数据积累 **2 周以上** 后，看板才有实际参考价值。建议在第二阶段末期上线看板前端页面。

---

## 六、入库层设计

### 6.1 连接器优先级调整

> **核心修订**：Web/RSS 优先于企业私有连接器。原因：飞书 connector 是定制式实现，继续做 Notion/Confluence 会导致认证、分页、权限模型分散；Web/RSS 可以更快验证完整产品链路。

**实施顺序：**
1. **Web/RSS 连接器**（优先）：验证"抓取 → 摘要 → 入库 → 变更检测 → archived 归档"完整链路
2. **REST API 连接器**（声明式，无需写代码）：覆盖企业自建系统、语雀、钉钉开放平台等
3. **企业私有连接器**（插件式扩展）：在通用链路验证后，再逐步扩展 Notion、Confluence

### 6.2 统一 Connector 接口

```go
type Connector interface {
    Meta() ConnectorMeta
    Validate(ctx context.Context, cfg Config) error
    Fetch(ctx context.Context, cfg Config, since time.Time) ([]RawDocument, error)
    SupportedContentTypes() []string
}

type RawDocument struct {
    ExternalID  string            // 来源系统唯一 ID，用于变更检测和版本更新
    Title       string
    Content     string
    ContentType string
    SourceURL   string
    SourceChannel string          // 'web' | 'rss' | 'api' | 'feishu' 等
    PublishedAt time.Time
    Metadata    map[string]string
}
```

### 6.3 Web/RSS 连接器

- **输入**：单页 URL / 站点地图 URL / RSS Feed
- **抓取策略**：Readability 正文提取，站点地图展开按 lastmod 增量同步
- **变更检测**：ETag / Last-Modified / 内容 Hash 三级对比
- **入库行为**：
  - 新增 → 建知识条目 + 向量化
  - 变更 → 通过 external_id 原地更新内容和向量（不改 ID）
  - 消失 → 状态改为 `archived`，不物理删除

### 6.4 REST API 连接器（声明式配置）

不写新 Go 代码，通过配置驱动：

```yaml
connector_type: rest_api
endpoint: "https://api.example.com/docs"
auth:
  type: bearer_token
  token_env: EXAMPLE_API_TOKEN
pagination:
  mode: cursor            # page-number | cursor | link-header
  cursor_field: "next_cursor"
field_mapping:
  title: "name"
  content: "body.text"
  external_id: "id"
  published_at: "created_time"
```

### 6.5 调度与同步

**触发方式：**
- 手动触发（管理员立即同步）
- 定时调度（Cron，默认每 6 小时）
- Webhook（企业系统推送变更通知）

**入库后自动处理：**
- 调用 LLM 生成文档摘要（写入 `knowledge.summary`）
- 自动提取实体标签（人名、产品名、部门名）
- 写入 `document_access_log`（access_type = 'ingested'，供后续新鲜度分析）

---

## 七、实施阶段规划（修订版）

| 阶段 | 内容 | 前置条件 |
|------|------|---------|
| **前置** | 修复三个实现偏差（加密 / 归档 / 版本更新） | 无 |
| **第一阶段** | answer_evidence 表 + /confidence 接口 + 前端展示 | 前置完成 |
| **第二阶段** | source_feedback 表 + 来源反馈 UI + source_weight 更新任务 | 第一阶段完成 |
| **第三阶段** | document_access_log + 知识健康看板 | 第二阶段数据积累 2 周 |
| **第四阶段** | Web/RSS 连接器 + REST API 声明式连接器 | 前置完成（可与第一阶段并行） |
| **第五阶段** | 企业私有连接器扩展（Notion / 钉钉 / Confluence） | 第四阶段链路验证完成 |

### 不纳入本次范围
- ASR 语音输入
- 视频多模态处理
- MCP 大规模接入
- GraphRAG 生产启用

---

## 八、关键设计决策记录

| 决策点 | 选择 | 理由 |
|--------|------|------|
| 置信度架构 | 答案解释层（复用现有得分） | 不动召回逻辑，风险最低 |
| 置信度计算时机 | 接口按需计算 | 支持权重热调整，不存冗余数据 |
| 反馈粒度 | 双层（message 轻反馈 + source 精细反馈） | 保留旧接口，新增来源运营资产 |
| 反馈写回方式 | 异步 + 每日批量 | 不影响检索实时性能 |
| 消失文档处理 | archived 状态，不物理删除 | 保留引用历史，避免对话断链 |
| 文档更新方式 | external_id 原地版本更新 | 保留 source_weight 和引用历史连续性 |
| 连接器扩展顺序 | Web/RSS → REST API 声明式 → 企业私有 | 先验证通用链路，再扩展复杂认证 |
| 看板上线时机 | 数据积累 2 周后 | 数据量不足时看板无实际价值 |
