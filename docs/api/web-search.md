# Web Search API

## 概览

网络搜索配置通过 `/api/v1/web-search-providers` 管理，不再依赖旧的静态 provider 配置。

当前 provider 解析优先级：

1. 智能体或请求显式指定的 `web_search_provider_id`
2. 当前租户默认 provider：`is_default = true`
3. 平台共享默认 provider：`is_platform = true` 且 `is_default = true`
4. 兼容旧字段 `WebSearchConfig.Provider`

支持的 provider 类型：

- `duckduckgo`
- `bing`
- `google`
- `tavily`
- `serpapi`

## 获取可选类型

`GET /api/v1/web-search-providers/types`

返回前端动态表单所需元数据，例如：

- 是否需要 `api_key`
- 是否需要 `engine_id`
- 官方文档地址

## 获取配置列表

`GET /api/v1/web-search-providers`

返回当前租户可见的全部配置：

- 租户自己的 provider
- 平台共享 provider

说明：

- 普通租户查看平台共享 provider 时，返回结果会自动隐藏平台 API Key
- 超级管理员可看到完整配置

## 创建配置

`POST /api/v1/web-search-providers`

示例：

```json
{
  "name": "Platform SerpAPI",
  "provider": "serpapi",
  "parameters": {
    "api_key": "xxx"
  },
  "is_default": true,
  "is_platform": true
}
```

规则：

- `is_platform = true` 仅超级管理员可设置
- 普通租户只能创建自己的租户级 provider
- 设置 `is_default = true` 时，只会清理同作用域下旧的默认项
  - 租户级默认互斥
  - 平台级默认互斥

## 更新与删除

`PUT /api/v1/web-search-providers/{id}`

`DELETE /api/v1/web-search-providers/{id}`

权限规则：

- 普通租户只能修改、删除自己的 provider
- 平台共享 provider 对普通租户只读
- 超级管理员可以管理租户级和平台级 provider

## 实际行为

如果租户没有单独配置网络搜索，系统会自动回退到平台默认搜索引擎。这保证了：

- 普通租户开箱即用，不配置也能使用联网搜索
- 租户仍然可以按需新增自己的搜索配置
- 超级管理员可以统一管理平台级默认参数
