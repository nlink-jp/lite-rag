# Development Plan: lite-rag

## Phases and Milestones

---

### Phase 0: Project Scaffold  *(complete)*

Goal: Clean working skeleton with quality gates in place before any feature code.

- [x] Reset `go.mod` with correct module name and Go version
- [x] Add all dependencies (`go get`) and generate `go.sum`
- [x] Write `Makefile` with `build`, `test`, `lint`, `check`, `setup` targets
- [x] Write `config.example.toml`
- [x] Write `scripts/hooks/pre-commit` and `scripts/hooks/pre-push`
- [x] Write `docs/en/setup.md` (hook installation, prerequisites)
- [x] Write `docs/en/dependencies.md`
- [x] Write `docs/en/structure.md`

Completion criteria: `make check` runs cleanly (nothing to lint/test yet, but the
tooling itself works).

---

### Phase 1: Core Infrastructure  *(complete)*

Goal: Configuration loading and DuckDB connectivity working with tests.

- [x] `internal/config/` — load from TOML + env override
- [x] `internal/database/` — open/close DuckDB, run migrations (CREATE TABLE)
- [x] Unit tests for both packages

Completion criteria: `go test ./...` passes.

---

### Phase 2: Text Normalization + Chunker  *(complete)*

Goal: Normalization and chunking logic, fully tested in isolation.

- [x] `internal/normalizer/` — NFKC, whitespace, control-char, Markdown artifact stripping
- [x] Unit tests for normalizer: full-width→half-width, half-width kana→full-width, mixed JP/EN
- [x] `pkg/chunker/` — heading-aware split + paragraph/sentence fallback (JP + EN)
- [x] Token estimator: CJK character count × 2 + word count × 1.3
- [x] Unit tests with representative Markdown fixtures (JP-only, EN-only, mixed)

Completion criteria: `go test ./internal/normalizer/... ./pkg/chunker/...` passes
with edge cases covered (empty input, CJK-only, ASCII-only, mixed).

---

### Phase 3: LLM Client  *(complete)*

Goal: Embedding and chat completion calls working against LM Studio.

- [x] `internal/llm/` — `Embed()` and `Chat()` (streaming) methods
- [x] Integration test (skipped if `LITE_RAG_INTEGRATION=0`)

Completion criteria: Manual smoke test against LM Studio succeeds.

---

### Phase 4: Indexer  *(complete)*

Goal: `index` command end-to-end.

- [x] `internal/indexer/` — walk, hash, chunk, embed, upsert
- [x] `cmd/lite-rag/` — wire up `index` subcommand via cobra
- [x] Integration test with a small fixture directory

Completion criteria: Running `lite-rag index ./docs` populates the DuckDB file.

---

### Phase 5: Retriever + Ask Command  *(complete)*

Goal: `ask` command end-to-end, including context window expansion.

- [x] `internal/retriever/` — embed query, cosine similarity SQL, return top-K hits
- [x] Context window expansion: fetch adjacent chunks (±N) per hit via `chunk_index`
- [x] Deduplication: merge overlapping windows from hits in the same document
- [x] Wire up `ask` subcommand
- [x] Unit test: expansion logic with mocked DB results (boundary cases: first/last chunk)
- [x] Integration test: index fixtures, ask a question, assert non-empty answer
- [x] Query rewrite: LLM-assisted hybrid search (`query_rewrite` config flag)

Completion criteria: `lite-rag ask "What is lite-rag?"` returns a streamed answer
with surrounding context correctly merged.

---

### Phase 6: Cross-Compilation & Release  *(complete)*

Goal: Binaries for all four target platforms.

- [x] Makefile `cross-build` targets: darwin via `clang -arch`; linux via container
      (`podman`/`docker`) with `gcc-x86-64-linux-gnu` / `gcc-aarch64-linux-gnu`
- [x] `make dist` packages release archives (`tar.gz`) per platform
- [x] Verify binaries build on each platform (linux via container)
- [x] `CHANGELOG.md` with release entries
- [x] Git tag `v0.1.0`, `v0.1.1`; GitHub releases published

---

### Phase 7: HTTP API Server + Web UI  *(complete)*

Goal: Local `serve` subcommand with HTTP API and embedded browser UI.

- [x] `internal/server/` — HTTP server, graceful shutdown, SSE streaming handler
- [x] `POST /api/ask` — SSE-streamed RAG answer
- [x] `GET /api/status` — health check + version
- [x] Embedded Web UI (`index.html`, `app.js`, `style.css`, `marked.min.js`)
- [x] Markdown rendering with Raw/Rendered toggle
- [x] `cmd/lite-rag/serve.go` — cobra subcommand, `--addr` flag
- [x] Privacy-aware structured logging (`log/slog` JSON, query text at DEBUG only)

---

### Phase 8: Documentation  *(complete)*

Goal: All documentation requirements from RULES.md satisfied.

- [x] `README.md` — setup, usage, configuration reference
- [x] `docs/ja/` — Japanese translations of all primary docs
- [x] `docs/en/authoring-guide.md` — how to write Markdown for best retrieval results
- [x] `docs/en/eval/query-rewrite.md` — query-rewrite feature evaluation report
- [x] XDG default config path (`~/.config/lite-rag/config.toml`)

---

### Phase 9: Document Management (`docs` subcommand)  *(complete)*

Goal: Inspect and manage indexed documents without direct DB access.

- [x] `db.ListDocuments()` — returns all document records ordered by file path
- [x] `db.DeleteDocument(id)` — removes document and all its chunks; errors if not found
- [x] `db.DocumentChunks(id)` — returns chunks ordered by index for content reconstruction
- [x] `cmd/lite-rag/docs.go` — `docs list [--json]`, `docs show <id>`, `docs delete <id>`
- [x] Unit tests for all three DB methods (9 test cases)
- [x] Documentation updated (README, architecture.md, structure.md, usage.md)

---

## Out of Scope (v0.1.x)

- Windows support (dropped due to CGo constraints from `go-duckdb`)
- Multi-user or concurrent indexing
- Incremental embedding updates (whole-file re-index on change)
- Web UI document ingestion (indexing is a local CLI operation)
