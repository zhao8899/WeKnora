# Regression Smoke

用于当前混合部署模式的快速回归：
- 前端本地运行：`http://localhost:5173`
- 后端 Docker 运行：`http://localhost:8080`

执行方式：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\regression_smoke.ps1
```

如果当前不检查本地前端运行态，只验证静态构建和 Docker 后端：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\regression_smoke.ps1 -SkipFrontendRuntime
```

默认检查项：
- `frontend npm run type-check`
- `frontend npm run build`
- `http://localhost:5173` 可达
- `http://localhost:8080/health`
- `WeKnora-app` Docker health
- 未登录鉴权边界：
  - `/api/v1/chat/answer/test/confidence`
  - `/api/v1/analytics/hot-questions`
  - `/api/v1/datasource/types`

通过标准：
- 前端类型检查和构建都通过
- 后端健康检查返回 `{"status":"ok"}`
- `WeKnora-app` 为 `healthy`
- 上述 3 个核心接口在未登录时返回 `401`
