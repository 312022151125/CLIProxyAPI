# Repository Guidelines

## Project Overview

CLIProxyAPI is a Go 1.26 proxy server exposing OpenAI-, Gemini-, Claude-, and compatible provider APIs. It supports OAuth and API-key credentials, multi-account scheduling and failover, streaming and WebSocket transports, multimodal requests, model catalog updates, and an embeddable Go SDK with a plugin system.

## Architecture & Data Flow

- `cmd/server/main.go` is the executable entry point. It loads `.env`, parses CLI flags, selects config/auth storage, initializes logging, starts model/catalog updates, and launches server, TUI, login, or standalone modes.
- `sdk/cliproxy.Builder` and `sdk/cliproxy.Service` provide dependency injection and lifecycle management. They assemble config, auth stores, access managers, watchers, plugin hosts, executors, and hooks before creating the HTTP server.
- `internal/api/server.go` builds the Gin server, middleware, management routes, plugin routes, and authenticated provider routes.
- Requests flow through `sdk/api/handlers` into the auth `Manager`, which selects credentials through scheduler/round-robin or fill-first policy, handles refresh/retry/cooldown state, and invokes a `ProviderExecutor`.
- Provider executors in `internal/runtime/executor/` perform provider-specific HTTP or WebSocket calls. Keep executor code focused on execution; place support helpers under `internal/runtime/executor/helps/`.
- `sdk/translator/registry.go` and `sdk/translator/pipeline.go` translate request/response protocols and run plugin hooks. Preserve the canonical translation pipeline and provider-specific adapters; do not add isolated translator conventions.
- `internal/registry/` tracks model metadata, provider availability, embedded catalogs, remote updates, and registration hooks.
- `internal/store/` and `sdk/cliproxy/auth/store.go` abstract local, Git, Postgres, object-store, and plugin-auth persistence.
- Streaming uses `context.Context`, goroutines, channels, and explicit terminal errors. Do not introduce post-connection network timeouts except existing documented liveness/session exceptions.

## Key Directories

- `cmd/server/` — production server entry point.
- `cmd/` — catalog fetch/validation and other maintenance commands.
- `internal/api/` — Gin server, middleware, handlers, and management endpoints.
- `internal/runtime/executor/` — provider executors and executor tests.
- `internal/thinking/` — canonical reasoning/thinking config normalization and provider application.
- `internal/registry/` — model registry and catalog updater.
- `internal/store/` — persistence backends and secret resolution.
- `internal/watcher/` — config and auth hot reload.
- `internal/wsrelay/` — WebSocket relay sessions.
- `internal/tui/` — Bubble Tea terminal UI.
- `sdk/cliproxy/` — embeddable service and builder lifecycle.
- `sdk/api/handlers/` — SDK-facing execution flow and protocol request types.
- `sdk/translator/` — shared translation registry and middleware pipeline.
- `test/` — cross-module tests and fixtures.
- `examples/plugin/` — Go, C, and Rust plugin examples plus plugin build Makefile.
- `docs/` — SDK documentation; verify version references because some docs still mention v6 while the module is v7.

## Development Commands

```bash
# Run the server with config.yaml or the default config path
go run ./cmd/server

# Required compile smoke check
go build -o test-output ./cmd/server && rm -f test-output

# Format changed Go files
gofmt -w path/to/changed.go

# Run all Go tests or one focused test
go test ./...
go test -v -run TestName ./path/to/package

# Static analysis when needed
go vet ./...

# Refresh and validate embedded model catalogs
bash .github/scripts/refresh-model-catalogs.sh

go run ./cmd/validate_codex_models --file path/to/catalog.json

# Build plugin examples
make -C examples/plugin list
make -C examples/plugin build
make -C examples/plugin clean

# Compose development
./docker-build.sh
docker compose up -d --remove-orphans --no-build
docker compose logs -f
```

There is no root `Makefile`; the only Makefile is `examples/plugin/Makefile`. Catalog refresh requires network access and may modify files under `internal/registry/models/`.

## Code Conventions & Common Patterns

- Format Go with `gofmt`; keep comments in English. Preserve the language of existing user-facing strings.
- Use `logrus` for structured logging. Never use `log.Fatal` or `log.Fatalf`; return errors and log at the owning boundary.
- Wrap errors with context using `%w`. Use status/request-scoped error types when retry or HTTP status behavior depends on classification.
- Propagate `context.Context` through request, auth, executor, watcher, and stream paths. Respect cancellation and close response bodies.
- Use `sync.Mutex`/`sync.RWMutex`, atomics, and guarded state for concurrent managers. Prefer existing scheduler, cache, registry, and store abstractions over new parallel state.
- Keep dependency injection explicit through `Builder`, service options, server options, and manager constructors. Avoid factories/interfaces with one implementation.
- Preserve the canonical thinking flow: parse suffixes, normalize to `ThinkingConfig`, validate centrally, then apply provider-specific output through an applier.
- Preserve executor boundaries: executors execute provider calls; translators convert protocols; registry tracks model capability/availability.
- Do not leak tokens, API keys, credentials, or sensitive request content in logs.
- Avoid panics in HTTP handlers; return meaningful status codes and log unexpected failures.
- Do not make standalone changes only in `internal/translator/`. The repository has a path guard for that directory; check repository permissions before translator-only work.

## Important Files

- `cmd/server/main.go` — flags, startup modes, config loading, storage selection, and service startup.
- `config.example.yaml` — server, auth, routing, retry, plugin, logging, and provider configuration reference.
- `.env.example` / `.env.cluster.example` — remote-store and Home JWT environment examples.
- `sdk/cliproxy/service.go` — SDK lifecycle and server startup/shutdown.
- `sdk/cliproxy/builder.go` — dependency assembly and auth manager construction.
- `internal/api/server.go` — middleware and route registration.
- `sdk/api/handlers/model_execution.go` — model execution request/stream flow.
- `sdk/cliproxy/auth/conductor.go` — `ProviderExecutor`, auth manager, scheduling, retry, and cooldown state.
- `sdk/translator/registry.go` / `sdk/translator/pipeline.go` — protocol translation and plugin hooks.
- `internal/registry/model_registry.go` — model registration and provider availability.
- `internal/config/config.go` — YAML/runtime config schema.
- `Dockerfile` / `docker-compose.yml` / `docker-compose.cluster.yml` — container and deployment behavior.
- `.github/workflows/pr-test-build.yml` — PR catalog refresh and build gate.
- `.github/scripts/refresh-model-catalogs.sh` — model catalog synchronization and validation.

## Runtime/Tooling Preferences

- Required runtime: Go 1.26.0 or newer compatible with `go.mod`; CI currently uses Go 1.26.4 and Docker uses `golang:1.26-bookworm`.
- Root project is one Go module: `github.com/router-for-me/CLIProxyAPI/v7`. Plugin examples under `examples/plugin/*/go` are nested modules and commonly use a local `replace` directive.
- `.env` is loaded from the working directory with `godotenv`; YAML config defaults to `config.yaml`. Runtime auth material defaults to `~/.cli-proxy-api` (`internal/config/config.go:26`, `internal/util/util.go:74-94`); repository `auths/` is only a checked-in placeholder. 
- Local file storage is default. Optional Postgres, Git, object-store, and Home JWT modes are selected through environment variables/flags; see `.env.example`, `.env.cluster.example`, and `cmd/server/main.go`.
- Normal Docker builds are CGO-enabled Debian/glibc images. Plugin builds require matching target `GOOS/GOARCH`; Go plugins use `-buildmode=c-shared`, C uses CMake, and Rust uses Cargo.
- No Node/Bun package manager is used for the core project.

## Testing & QA

- Tests use Go's standard `testing` package; no external assertion/mock framework is declared.
- Package tests live beside code under `internal/`, `sdk/`, and `cmd/`. Cross-module and fixture-backed tests live under `test/`.
- Use `httptest`, temporary directories, controlled environment variables, table-driven tests, and subtests in the existing style. Windows-specific tests use `//go:build windows`.
- Benchmarks exist in translator, executor, and signature packages. No repository coverage policy or fuzz-test suite is configured.
- CI currently refreshes model catalogs and runs the server build on pull requests. Docker and release workflows build multi-platform artifacts; they do not run the project-wide test, vet, lint, race, or coverage suites.
- Before yielding a non-trivial change, run the narrowest relevant package test plus the required server build. Run `go test ./...` when changing shared contracts or cross-package behavior.
- `.github/workflows/agents-md-guard.yml` rejects PRs that modify `AGENTS.md`; update this file only when explicitly requested or when repository policy is intentionally changed.
