# Repository Guidelines

## Project Structure & Module Organization
`cmd/server` contains the Go entrypoint. Core backend code lives under `internal/`, grouped by domain (`application`, `handler`, `agent`, `datasource`, `models`, `middleware`). The Vue 3 frontend is in `frontend/src`, with API clients in `frontend/src/api`, shared UI in `frontend/src/components`, and views in `frontend/src/views`. The Python document parsing service lives in `docreader/`. Database migrations are under `migrations/`, runtime config under `config/`, automation scripts under `scripts/`, and longer-form design/API docs under `docs/`.

## Build, Test, and Development Commands
Use the repo `Makefile` for common workflows:

- `make dev-start`: start local dependencies with Docker for development.
- `make dev-app`: run the Go backend locally.
- `make dev-frontend`: start the Vite frontend with hot reload.
- `make build`: compile the backend binary from `./cmd/server`.
- `make test`: run `go test -v ./...`.
- `make lint`: run `golangci-lint`.
- `make migrate-up`: apply database migrations.
- `cd frontend && npm run type-check`: run Vue/TypeScript checks.
- `cd frontend && npm run build`: produce a frontend production build.

## Coding Style & Naming Conventions
Follow standard Go formatting: run `go fmt ./...` and keep code lint-clean with `golangci-lint` (`gofmt`, `gofumpt`, `govet`, `revive`, max line length 120). Keep Go packages lowercase and file names descriptive, using `_test.go` for tests. In the frontend, prefer PascalCase for Vue components (`AgentList.vue`), camelCase for composables/utilities (`useTheme.ts`), and keep API modules grouped by feature.

## Testing Guidelines
Backend tests are standard Go tests colocated with the code. Prefer focused runs while iterating, for example `go test ./internal/agent/...` or `go test ./internal/application/service/...`. For frontend changes, at minimum run `cd frontend && npm run type-check` and `cd frontend && npm run build`. When changes touch migrations, validate with `make migrate-up` against a local dev stack.

## Commit & Pull Request Guidelines
Recent history follows Conventional Commit style, often with scopes: `feat: ...`, `fix(frontend): ...`, `docs(spec): ...`. Keep commits focused and descriptive. PRs should use `.github/pull_request_template.md`: summarize the change, mark the affected scope, list test steps, link issues with `Fixes #123`, and attach screenshots or recordings for UI work. Call out schema, config, or deployment impacts explicitly.

## Security & Configuration Tips
Start from `.env.example`; do not commit real secrets in `.env`. Review `SECURITY.md` before changing auth, storage, or remote fetch behavior. Treat changes under `internal/crypto`, `internal/sandbox`, and web-fetch/search integrations as security-sensitive and verify them carefully.
