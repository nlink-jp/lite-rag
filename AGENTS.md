# AGENTS.md — lite-rag

CLI-based RAG tool for Markdown documents using DuckDB vector storage.
Part of [lite-series](https://github.com/nlink-jp/lite-series).

## Rules

- Project rules (security, testing, docs, release, etc.): → [RULES.md](RULES.md)
- Series-wide conventions (config format, CLI, Makefile, etc.): → [../CONVENTIONS.md](https://github.com/nlink-jp/lite-series/blob/main/CONVENTIONS.md)

## Build & test

```sh
make build        # bin/lite-rag
make check        # vet → lint → test → build → govulncheck (full gate)
go test ./...     # tests only
```

## Key structure

```
cmd/lite-rag/            ← CLI entry point (Cobra subcommands: index, ask, serve, reindex, docs)
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
- **Cross-compilation**: Linux binaries must be built inside a container (podman/docker) due to CGO. Use `make cross-build-linux`, not `make build-all`.
- **Module path**: bare `lite-rag` (no GitHub path) in `go.mod`. Internal imports use `lite-rag/internal/...`.
- **Config format**: sectioned TOML (`[api]`, `[models]`, `[database]`, `[retrieval]`, `[server]`). See `config.example.toml`.
- **No hosted CI**: quality gate runs locally via Git hooks. Run `make setup` once after cloning.
