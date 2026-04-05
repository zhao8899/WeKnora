# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is WeKnora

WeKnora is an LLM-powered enterprise knowledge management and Q&A framework built on RAG (Retrieval-Augmented Generation) and ReACT Agent. It supports multi-source document ingestion, semantic retrieval, and Q&A delivery over IM channels (WeCom, Feishu, Slack, Telegram, DingTalk, Mattermost).

- **Backend**: Go 1.24, Gin, GORM, uber/dig (DI)
- **Frontend**: Vue 3 + TypeScript + Vite + TDesign
- **Infrastructure**: PostgreSQL, Redis, MinIO, Neo4j, DocReader (Python gRPC service)
- **Vector backends**: Qdrant, Milvus, Weaviate, Elasticsearch v7/v8, SQLite-vec, pgvector

## Development Workflow

### Local development (recommended)

```bash
# 1. Start infrastructure (Postgres, Redis, MinIO, Neo4j, DocReader, Jaeger)
make dev-start

# 2. In a separate terminal — run the Go backend
make dev-app

# 3. In a separate terminal — run the Vite dev server (hot-reload)
make dev-frontend
```

Access: frontend at `http://localhost:5173`, backend API at `http://localhost:8080`.

For backend hot-reload, install `air` (`go install github.com/air-verse/air@latest`) and run `air` in the repo root — `.air.toml` is already configured.

### Common commands

| Command | Purpose |
|---|---|
| `make build` | Build the Go binary |
| `make test` | Run all Go tests (`go test -v ./...`) |
| `go test -v ./internal/some/pkg/...` | Run tests in a single package |
| `make lint` | Run golangci-lint |
| `make fmt` | Format Go code |
| `make migrate-up` | Apply database migrations |
| `make migrate-down` | Roll back migrations |
| `make migrate-create name=<name>` | Create a new migration file |
| `make docs` | Regenerate Swagger docs (requires `swag`) |
| `make install-swagger` | Install the `swag` CLI tool |

### Smoke testing

After the backend is running with a valid tenant API key and a knowledge base:

```bash
make qa-mode-smoke API_KEY=sk-... KB_ID=<kb-id> BASE_URL=http://127.0.0.1:8080/api/v1
```

This validates the three core Q&A routing paths: `chat`, `rag_fast`, `rag_deep`.

### Health / diagnostics endpoints

```bash
curl http://localhost:8080/health
curl -H "X-API-Key: sk-..." http://localhost:8080/api/v1/system/diagnostics
```

## Architecture

### Entry point and DI

`cmd/server/` is the Go entry point. All dependency wiring lives in `internal/container/container.go` using **uber/dig**. Every service, repository, and handler is registered there — this is the first place to look when tracing how a component is constructed.

### Layer structure

```
internal/
  handler/        # HTTP handlers (Gin) — thin layer, delegates to services
  router/         # Route registration, middleware wiring
  application/
    service/      # Business logic
    repository/   # Data access (GORM for Postgres, vendor-specific for vector DBs)
  agent/          # ReACT agent engine (think/act/observe/finalize loop)
  im/             # IM channel adapters (wecom, feishu, slack, telegram, dingtalk, mattermost)
  datasource/     # External data source connectors (e.g., Feishu sync)
  event/          # In-process event bus for observability hooks
  stream/         # SSE/streaming response management
  infrastructure/ # Low-level impls: chunkers, web search, docparser wrappers
  middleware/     # Gin middleware (auth, rate-limit, tracing)
  config/         # Typed config structs bound from config/config.yaml
  types/
    interfaces/   # Service and repository interfaces — the contract layer
```

### Q&A pipeline

There are two modes:

1. **RAG (`rag_fast` / `rag_deep`)** — implemented in `internal/application/service/chat_pipeline/`. Stages: query rewrite → embedding retrieval → keyword retrieval → graph retrieval → rerank → LLM generation. Each stage is a plugin-style step.

2. **Agent (`chat`)** — implemented in `internal/agent/`. Runs a ReACT loop: Think → Act (tool calls: knowledge retrieval, MCP tools, web search) → Observe → Finalize. The agent engine is in `engine.go`; tools live under `internal/agent/tools/`.

### Configuration and prompts

Runtime config is loaded from `config/config.yaml` (and `.env`). Prompt templates are separate YAML files under `config/prompt_templates/` and referenced by ID in `config.yaml`. To change a prompt, edit the template file; the `_id` fields in `config.yaml` map names to template files.

### Adding a new data source connector

Follow `internal/datasource/CONNECTOR_IMPLEMENTATION_GUIDE.md`. New connectors go under `internal/datasource/connector/` and implement the `connector.Connector` interface.

### Database migrations

Migration files live in `migrations/versioned/`. Use `make migrate-create name=<name>` to scaffold a new one. Never edit existing migration files — always create a new migration.

### IM channel adapters

Each IM platform has its own subdirectory under `internal/im/` (e.g., `wecom/`, `feishu/`, `telegram/`). The shared slash-command system (`command.go`, `command_registry.go`) and QA worker queue (`qaqueue.go`) are at the `internal/im/` level.

### Frontend

Vue 3 SPA in `frontend/`. Proxies API calls to `http://localhost:8080` via Vite config. UI component library is TDesign (`tdesign-vue-next`). Build with `npm run build` inside `frontend/`, or via `make docker-build-frontend`.

### MCP server

`mcp-server/` is a standalone MCP server that exposes WeKnora knowledge retrieval as an MCP tool, usable by external agents.

### docreader

`docreader/` is a Python service (pyproject.toml) that handles document parsing (PDF, Word, images, etc.) and communicates with the Go backend over gRPC. It runs as a Docker container in the dev environment.
