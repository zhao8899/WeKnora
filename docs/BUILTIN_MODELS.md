# 平台模型治理指南

> 历史说明：本文件保留原路径名仅用于兼容旧链接。模型治理已收敛为两层：`平台共享模型 + 租户自有模型`。
> `models.is_builtin` 已退出模型域运行时逻辑，不应再新增或依赖。

## 概述

WeKnora 当前模型治理只有两层：

| 类型 | 字段 | 说明 | 管理方式 |
|------|------|------|---------|
| **租户自有模型** | `is_platform=false` | 租户自行配置，仅本租户可见和使用 | 前端 UI / API |
| **平台共享模型** | `is_platform=true` | 超级管理员统一配置，所有租户可用 | 平台管理 API / SQL |

## 运行时解析优先级

系统在模型域按以下顺序解析模型：

```text
请求指定模型 -> Agent配置模型 -> 知识库配置模型 -> 租户自有模型 -> 平台共享模型 -> 报错
```

这意味着：

- 租户如果配置了自己的模型，优先使用租户模型
- 租户没有自有模型时，回退到平台共享模型
- 不再存在“内置模型”作为正式第三层治理概念

## 平台共享模型

平台共享模型用于解决“每个租户都要重复录入模型配置”的问题。

### 特性

- 所有租户可见可用
- 敏感信息对普通租户隐藏
- 仅超级管理员可编辑、删除
- 可作为租户未显式配置时的默认回退

### 通过 API 管理平台模型

```bash
# 创建平台共享模型（需超级管理员）
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

# 查看平台模型
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
    is_platform
) VALUES (
    'platform-llm-001',
    10000,
    'GPT-4o',
    'KnowledgeQA',
    'remote',
    '平台共享 LLM 模型',
    '{"base_url": "https://api.openai.com/v1", "api_key": "sk-xxx", "provider": "openai"}'::jsonb,
    true,
    'active',
    true
) ON CONFLICT (id) DO NOTHING;

SELECT id, name, type, is_platform, is_default, status
FROM models
WHERE is_platform = true;
```

## 租户自有模型

租户自有模型用于：

- 租户使用自己的 API Key
- 租户接入自定义供应商或专有模型
- 租户需要和平台默认配置隔离

租户模型由租户管理员维护，仅本租户可见。

## 默认策略建议

推荐策略：

1. 平台预先配置一套可用的平台共享模型
2. 新租户未配置模型时，直接使用平台共享模型
3. 租户有自定义需求时，再补充租户自有模型

这样可以把用户心智保持为：

- “我没配，就用平台提供的”
- “我自己配了，就优先用我自己的”

## 历史兼容说明

旧版本中的 `models.is_builtin`：

- 已不再参与模型域运行时逻辑
- 已不再作为正式治理概念展示给租户
- 仅可能出现在历史迁移、回滚脚本或旧文档上下文中

如果你的环境仍存在旧数据，应先将其归一到 `is_platform=true`，再执行字段下线迁移。
