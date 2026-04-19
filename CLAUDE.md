# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

WeKnora is an LLM-powered RAG (Retrieval-Augmented Generation) framework for document understanding and semantic retrieval. This is a **customized fork** of the open-source [Tencent/WeKnora](https://github.com/Tencent/WeKnora), tailored as a phase-one enterprise internal knowledge portal.

The customization scope is documented in `docs/企业内部知识库底座一期/`. The strategy is to **hide, not delete** upstream features that are out of scope for phase one (e.g., Agent, MCP, complex settings).

## Branch Convention

- `main` — tracks `upstream/main` (Tencent official). Never customize here.
- `custom-main` — the working branch for all local customization. Daily development happens here.

To sync upstream changes:
```bash
git checkout main && git fetch upstream && git reset --hard upstream/main
git checkout custom-main && git merge main && git push
```

## Development Commands

### Backend (Go, entry: `cmd/server/main.go`)

```bash
make build          # go build -o WeKnora ./cmd/server
make run            # build + run
make test           # go test -v ./...
make lint           # golangci-lint run
make fmt            # go fmt ./...
make docs           # regenerate Swagger docs (requires swag: make install-swagger)
```

Hot reload with Air (recommended for development):
```bash
go install github.com/air-verse/air@latest
make dev-app        # starts backend with air hot-reload
```

### Frontend (Vue 3 + TypeScript + Vite, at `frontend/`)

```bash
cd frontend
npm run dev         # Vite dev server on :5173
npm run build       # production build
npm run type-check  # vue-tsc type check
```

### Dev Environment (recommended three-terminal workflow)

```bash
# Terminal 1: start infrastructure (PostgreSQL, Redis, MinIO, Neo4j, DocReader via Docker)
make dev-start

# Terminal 2: backend with hot-reload
make dev-app        # backend on :8080

# Terminal 3: frontend
make dev-frontend   # frontend on :5173
```

### Database Migrations

```bash
make migrate-up
make migrate-down
make migrate-create name=your_migration_name
```

### Docker (production)

```bash
make docker-build-all   # build all three images (app, docreader, frontend)
docker-compose up       # run production stack
```

## Architecture

Three separate services compose the system:

| Service | Language | Port | Description |
|---------|----------|------|-------------|
| `cmd/server` | Go (Gin) | 8080 | Main API server |
| `frontend/` | Vue 3 / Vite | 80/5173 | Web UI |
| `docreader/` | Python (gRPC) | 50051 | Document parsing service |

Infrastructure dependencies: PostgreSQL, Redis, MinIO (or other S3-compatible), optional Neo4j for knowledge graph.

### Backend Package Layout (`internal/`)

- `handler/` — HTTP handlers, one file per domain (knowledge, chat, agent, etc.)
- `router/router.go` — all route registration in one place
- `agent/` — RAG engine: `engine.go` orchestrates `think.go` → `act.go` → `finalize.go` → `observe.go`
- `models/` — GORM models / data layer
- `infrastructure/` — adapters: `chunker/`, `docparser/`, `web_search/`, `web_fetch/`, vector DB drivers
- `application/` — service/use-case layer between handlers and infrastructure
- `container/container.go` — dependency injection wiring (dig/fx style)
- `config/` — config loading; runtime config is `config/config.yaml`
- `datasource/` — pluggable data source connectors (see `CONNECTOR_IMPLEMENTATION_GUIDE.md`)
- `im/` — IM bot adapters (WeCom, Feishu, Slack, Telegram, DingTalk, Mattermost)
- `mcp/` — MCP service integration

Prompt templates are YAML files in `prompt_templates/`; IDs are referenced in `config/config.yaml`.

### Frontend Structure (`frontend/src/`)

- `views/` — page-level Vue components grouped by feature (knowledge, chat, agent, settings, platform)
- `views/settings/nav.ts` — drives the settings sidebar navigation; controls which settings pages are visible
- `stores/` — Pinia stores
- `router/` — vue-router config
- `i18n/` — internationalization (vue-i18n)
- `api/` — Axios-based API client modules

UI component library: **TDesign Vue Next** (`tdesign-vue-next`).

### Key Configuration

- `.env` / `.env.example` — environment variables (DB, storage, model endpoints, ports)
- `config/config.yaml` — app-level tuning (RAG thresholds, chunking, prompts, conversation rounds)
- `GIN_MODE=release` disables Swagger UI; set to `debug` during development
- Swagger UI available at `http://localhost:8080/swagger/index.html` in debug mode

### Storage / Vector DB Backends

Switchable via `RETRIEVE_DRIVER` env var: `postgres` (default, uses pgvector), `elasticsearch_v7/v8`, `qdrant`, `milvus`, `weaviate`.

File storage via `STORAGE_TYPE`: `local`, `minio`, `cos`, `tos`, `s3`.

## Customization Notes (Phase One)

The phase-one customization focuses on hiding rather than removing upstream features:

- **Frontend**: Agent, MCP, and advanced settings entries are hidden in navigation but the underlying code is kept intact to ease future upstream merges.
- **Settings**: `frontend/src/views/settings/nav.ts` controls visible settings pages; modify here to show/hide settings sections.
- **Platform view**: `frontend/src/views/platform/index.vue` is the phase-one knowledge portal entry page.

Track upstream features pending adoption in `docs/上游能力待吸收Backlog.md`.
