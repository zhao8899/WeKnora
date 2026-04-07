# 模型管理指南

## 概述

WeKnora 支持三种模型管理方式：

| 类型 | 字段 | 说明 | 管理方式 |
|------|------|------|---------|
| **租户自有模型** | `is_builtin=false, is_platform=false` | 租户自行配置的模型，仅该租户可见 | 前端 UI / API |
| **平台共享模型** | `is_platform=true` | 平台管理员配置的共享模型，所有租户可用，按配额消耗 | 管理 API / SQL |
| **内置模型** | `is_builtin=true` | 系统预置的模型模板，所有租户可见，不可编辑 | SQL |

### 模型解析优先级

当租户发起对话时，系统按以下顺序解析使用哪个模型：

```
请求指定模型 → Agent配置模型 → 知识库配置模型 → 租户自有模型 → 平台共享模型 → 报错
```

新注册的租户即使没有配置任何模型，只要平台管理员配置了平台共享模型，就可以开箱即用。

## 平台共享模型（推荐）

平台共享模型是为解决"每个租户都需要单独配置模型"问题而设计的。平台管理员配置一次，所有租户均可使用。

### 特性

- **所有租户可用**：自动出现在所有租户的模型列表中
- **配额控制**：配合租户 Token 配额（`token_quota`）控制各租户用量
- **安全保护**：API Key、Base URL 等敏感信息对普通用户隐藏
- **管理 API**：通过 `/api/v1/models/platform` 路由进行 CRUD 管理（需超级管理员权限）

### 通过 API 管理平台模型

```bash
# 创建平台共享模型（需超级管理员 Token）
curl -X POST http://localhost:8080/api/v1/models/platform \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "GPT-4o",
    "type": "KnowledgeQA",
    "source": "remote",
    "description": "平台共享 LLM 模型",
    "parameters": {
      "base_url": "https://api.openai.com/v1",
      "api_key": "sk-xxx",
      "provider": "openai"
    },
    "is_default": true
  }'

# 查看所有平台模型
curl http://localhost:8080/api/v1/models/platform \
  -H "Authorization: Bearer <admin-token>"

# 更新平台模型
curl -X PUT http://localhost:8080/api/v1/models/platform/<model-id> \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"name": "GPT-4o-updated", ...}'

# 删除平台模型
curl -X DELETE http://localhost:8080/api/v1/models/platform/<model-id> \
  -H "Authorization: Bearer <admin-token>"
```

### 通过 SQL 管理平台模型

```sql
-- 插入平台共享 LLM 模型
INSERT INTO models (id, tenant_id, name, type, source, description, parameters, is_default, status, is_platform)
VALUES (
    'platform-llm-001', 10000, 'GPT-4o', 'KnowledgeQA', 'remote',
    '平台共享 LLM 模型',
    '{"base_url": "https://api.openai.com/v1", "api_key": "sk-xxx", "provider": "openai"}'::jsonb,
    true, 'active', true
) ON CONFLICT (id) DO NOTHING;

-- 插入平台共享 Embedding 模型
INSERT INTO models (id, tenant_id, name, type, source, description, parameters, is_default, status, is_platform)
VALUES (
    'platform-embedding-001', 10000, 'text-embedding-3-small', 'Embedding', 'remote',
    '平台共享 Embedding 模型',
    '{"base_url": "https://api.openai.com/v1", "api_key": "sk-xxx", "provider": "openai", "embedding_parameters": {"dimension": 1536}}'::jsonb,
    true, 'active', true
) ON CONFLICT (id) DO NOTHING;

-- 查看所有平台模型
SELECT id, name, type, is_platform, is_default, status FROM models WHERE is_platform = true;
```

## 内置模型（模板）

内置模型是系统预置的模型配置模板，主要用于提供统一的模型定义。

### 特性

- **所有租户可见**：对所有租户都可见，无需单独配置
- **安全保护**：敏感信息（API Key、Base URL）会被隐藏
- **只读保护**：不能被编辑或删除
- **统一管理**：由系统管理员通过 SQL 维护

## 如何添加内置模型

内置模型需要通过数据库直接插入。以下是添加内置模型的步骤：

### 1. 准备模型数据

首先，确保你已经有了要设置为内置模型的模型配置信息，包括：
- 模型名称（name）
- 模型类型（type）：`KnowledgeQA`、`Embedding`、`Rerank` 或 `VLLM`
- 模型来源（source）：`local` 或 `remote`
- 模型参数（parameters）：包括 base_url、api_key、provider 等
- 租户ID（tenant_id）：建议使用小于10000的租户ID，避免冲突

**支持的服务商（provider）**：`generic`（自定义）、`openai`、`aliyun`、`zhipu`、`volcengine`、`hunyuan`、`deepseek`、`minimax`、`mimo`、`siliconflow`、`jina`、`openrouter`、`gemini`、`modelscope`、`moonshot`、`qianfan`、`qiniu`、`longcat`、`gpustack`

### 2. 执行 SQL 插入语句

使用以下 SQL 语句插入内置模型：

```sql
-- 示例：插入一个 LLM 内置模型
INSERT INTO models (
    id,
    tenant_id,
    name,
    type,
    source,
    description,
    parameters,
    is_default,
    status,
    is_builtin
) VALUES (
    'builtin-llm-001',                    -- 使用固定ID，建议使用 builtin- 前缀
    10000,                                -- 租户ID（使用第一个租户）
    'GPT-4',                              -- 模型名称
    'KnowledgeQA',                        -- 模型类型
    'remote',                             -- 模型来源
    '内置 LLM 模型',                       -- 描述
    '{"base_url": "https://api.openai.com/v1", "api_key": "sk-xxx", "provider": "openai"}'::jsonb,  -- 参数（JSON格式）
    false,                                -- 是否默认
    'active',                             -- 状态
    true                                  -- 标记为内置模型
) ON CONFLICT (id) DO NOTHING;

-- 示例：插入一个 Embedding 内置模型
INSERT INTO models (
    id,
    tenant_id,
    name,
    type,
    source,
    description,
    parameters,
    is_default,
    status,
    is_builtin
) VALUES (
    'builtin-embedding-001',
    10000,
    'text-embedding-ada-002',
    'Embedding',
    'remote',
    '内置 Embedding 模型',
    '{"base_url": "https://api.openai.com/v1", "api_key": "sk-xxx", "provider": "openai", "embedding_parameters": {"dimension": 1536, "truncate_prompt_tokens": 0}}'::jsonb,
    false,
    'active',
    true
) ON CONFLICT (id) DO NOTHING;

-- 示例：插入一个 ReRank 内置模型
INSERT INTO models (
    id,
    tenant_id,
    name,
    type,
    source,
    description,
    parameters,
    is_default,
    status,
    is_builtin
) VALUES (
    'builtin-rerank-001',
    10000,
    'bge-reranker-base',
    'Rerank',
    'remote',
    '内置 ReRank 模型',
    '{"base_url": "https://api.jina.ai/v1", "api_key": "jina-xxx", "provider": "jina"}'::jsonb,
    false,
    'active',
    true
) ON CONFLICT (id) DO NOTHING;

-- 示例：插入一个 VLLM 内置模型
INSERT INTO models (
    id,
    tenant_id,
    name,
    type,
    source,
    description,
    parameters,
    is_default,
    status,
    is_builtin
) VALUES (
    'builtin-vllm-001',
    10000,
    'gpt-4-vision',
    'VLLM',
    'remote',
    '内置 VLLM 模型',
    '{"base_url": "https://dashscope.aliyuncs.com/compatible-mode/v1", "api_key": "sk-xxx", "provider": "aliyun"}'::jsonb,
    false,
    'active',
    true
) ON CONFLICT (id) DO NOTHING;
```

### 3. 验证插入结果

执行以下 SQL 查询验证内置模型是否成功插入：

```sql
SELECT id, name, type, is_builtin, status 
FROM models 
WHERE is_builtin = true
ORDER BY type, created_at;
```

## 租户 Token 配额

平台共享模型配合 Token 配额体系，可以控制每个租户的使用量。

```sql
-- 设置租户 Token 配额（100万 token/月），0 = 不限制
UPDATE tenants SET token_quota = 1000000 WHERE id = 10001;

-- 查看租户用量
SELECT id, name, token_quota, token_used, quota_reset_at FROM tenants;

-- 重置租户用量
UPDATE tenants SET token_used = 0, quota_reset_at = NOW() + INTERVAL '30 days' WHERE id = 10001;
```

当租户的 `token_used >= token_quota` 时，QA 请求会被拒绝并返回配额超限错误。

## 注意事项

1. **ID 命名规范**：建议使用 `platform-{type}-{序号}` 或 `builtin-{type}-{序号}` 的格式
2. **租户ID**：平台/内置模型可以属于任意租户，建议使用第一个租户ID（通常是 10000）
3. **参数格式**：`parameters` 字段必须是有效的 JSON 格式
4. **幂等性**：使用 `ON CONFLICT (id) DO NOTHING` 确保重复执行不会报错
5. **安全性**：平台模型和内置模型的 API Key 和 Base URL 在前端会被自动隐藏
6. **优先推荐平台模型**：新部署建议优先使用平台共享模型（`is_platform=true`），而非内置模型

## 将现有模型升级为平台模型

```sql
-- 将已有模型升级为平台共享模型
UPDATE models SET is_platform = true WHERE id = '模型ID';

-- 将内置模型升级为平台模型（同时具备两种标记）
UPDATE models SET is_platform = true WHERE is_builtin = true AND id = '模型ID';
```

## 移除平台/内置标记

```sql
-- 恢复为普通模型
UPDATE models SET is_platform = false, is_builtin = false WHERE id = '模型ID';
```

注意：移除标记后，该模型将恢复为普通租户模型，仅该租户可见。

