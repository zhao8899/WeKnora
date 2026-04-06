# Proto 注册冲突解决方案

## 问题描述

WeKnora 同时依赖两个使用 Protocol Buffers 的客户端库：
- `github.com/qdrant/go-client` (Qdrant 向量数据库)
- `github.com/milvus-io/milvus/client/v2` (Milvus 向量数据库)

这两个库都注册了相同名称的 protobuf 文件（`common.proto`），导致 Go protobuf 运行时报错：

```
WARNING: proto: file "common.proto" is already registered
        previously from: "github.com/milvus-io/milvus-proto/go-api/v2/commonpb"
        currently from:  "github.com/qdrant/go-client/qdrant"
```

## 当前解决方案

在 `cmd/server/main.go` 中设置了环境变量：

```go
func ensureProtoRegistrationConflictMode() {
    if os.Getenv("GOLANG_PROTOBUF_REGISTRATION_CONFLICT") == "" {
        _ = os.Setenv("GOLANG_PROTOBUF_REGISTRATION_CONFLICT", "warn")
    }
}
```

`warn` 模式允许程序继续运行，仅在日志中输出警告信息。

## 影响范围

- **不影响**：程序正常运行，所有功能可用
- **仅影响**：启动时输出 WARNING 日志
- **无数据风险**：不会导致数据损坏或查询错误

## 长期解决方案

### 方案 1：使用独立的向量数据库（推荐）

根据实际需求选择一种向量数据库，避免同时引入多个客户端：

```yaml
# docker-compose.yml - 仅启用需要的向量数据库
services:
  # 选择 Qdrant
  qdrant:
    profiles: ["qdrant"]
  
  # 或选择 Milvus
  milvus:
    profiles: ["milvus"]
  
  # 不要同时启用
```

### 方案 2：使用 build tags 分离编译

创建不同的构建目标，每个目标只包含一个向量数据库客户端：

```go
// +build qdrant
package retriever

// 仅包含 Qdrant 相关代码
```

```bash
# 编译时指定 tag
go build -tags qdrant -o weknora ./cmd/server
```

### 方案 3：等待上游修复

关注 protobuf Go 实现的更新：
- https://github.com/golang/protobuf/issues/1395
- https://protobuf.dev/reference/go/faq#namespace-conflict

未来版本可能会提供更好的隔离机制。

## 配置建议

| 场景 | 推荐配置 |
|------|----------|
| 开发环境 | `GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn`（默认） |
| 生产环境 | `GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn` |
| 严格模式 | `GOLANG_PROTOBUF_REGISTRATION_CONFLICT=fatal`（会导致启动失败） |

## 验证方法

启动应用后检查日志：

```bash
docker logs WeKnora-app 2>&1 | grep -i "proto.*registered"
```

如果看到 WARNING 但没有 ERROR，说明配置正确。

## 相关资源

- [Protocol Buffers Go FAQ](https://protobuf.dev/reference/go/faq#namespace-conflict)
- [Qdrant Go Client](https://github.com/qdrant/go-client)
- [Milvus Go SDK](https://github.com/milvus-io/milvus/client/v2)
