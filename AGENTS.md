# AGENTS.md — lite-rag

CLI-based RAG tool for Markdown documents using DuckDB vector storage.
Part of [lite-series](https://github.com/nlink-jp/lite-series).

## Rules

- Project rules (security, testing, docs, release, etc.): → [RULES.md](RULES.md)
- Series-wide conventions (config format, CLI, Makefile, etc.): → [../CONVENTIONS.md](https://github.com/nlink-jp/lite-series/blob/main/CONVENTIONS.md)

## Build & test

```sh
make build        # dist/lite-rag
make check        # vet → lint → test → build → govulncheck (full gate)
go test ./...     # tests only
make test-linux   # same suite on Linux (container)
make verify-release  # gate: notarized, fresh, runs at this version, clean linux archives (run before upload)
```

## Key structure

```
cmd/lite-rag/            ← CLI entry point (Cobra subcommands: index, ask, serve, reindex, docs, version; `--version` too — the org homebrew formula tests it)
internal/config/         ← Config struct, Load(), env overrides
internal/indexer/        ← Document chunking + embedding + DuckDB insert
internal/retriever/      ← Vector search + context expansion + query rewrite
internal/llm/            ← HTTP client for OpenAI-compatible API
internal/server/         ← HTTP API server (POST /api/ask SSE, GET /api/status) + Web UI
internal/database/       ← DuckDB connection management
internal/normalizer/     ← NFKC Unicode normalization for JP/EN mixed text
pkg/chunker/             ← Public Markdown chunker library
```

## Gotchas

- **CGO required**: DuckDB uses CGO (`CGO_ENABLED=1`). Pure-Go builds (`CGO_ENABLED=0`) will fail.
- **Driver**: `github.com/duckdb/duckdb-go/v2` (`marcboeker/go-duckdb/v2` is
  frozen). Its version encodes DuckDB's: v2.20000.x = DuckDB v2.0.x. The
  native libraries come from `go.duckdb.org/libduckdb/*` via the module proxy.
- **Index files**: a database created by DuckDB 2.0 cannot be opened by DuckDB
  1.x (lite-rag ≤ v0.3.x). Indexes created earlier still open here and stay
  readable by older versions.
- **Cross-compilation**: Linux binaries must be built inside a container (podman/docker) due to CGO. Use `make cross-build-linux`, not `make build-all`.
- **Module path**: bare `lite-rag` (no GitHub path) in `go.mod`. Internal imports use `lite-rag/internal/...`.
- **Config format**: sectioned TOML (`[api]`, `[models]`, `[database]`, `[retrieval]`, `[server]`). See `config.example.toml`.
- **No hosted CI**: quality gate runs locally via Git hooks. Run `make setup` once after cloning.
