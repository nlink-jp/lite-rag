# Changelog

All notable changes to this project will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.2.1] — 2026-03-21

### Added

- **Multilingual query rewriting** — when `query_rewrite = true`, the LLM rewrites the
  query into both Japanese (`JA:`) and English (`EN:`) declarative statements in a single
  API call. Three vector searches run concurrently (original + JA + EN) and their results
  are merged (max score per chunk ID) before context expansion. This improves recall across
  multilingual document collections regardless of the query's input language.

---

## [0.2.0] — 2026-03-21

### Breaking Changes

- **`index` command**: positional `<directory>` argument replaced with explicit flags.
  - `lite-rag index --dir <directory>` — recursively index all `*.md` files (previous behaviour)
  - `lite-rag index --file <file>` — index a single file, any extension
  - Exactly one of `--dir` or `--file` is required; specifying both is an error.

### Added

- **`index --file <file>`** — single-file ingestion for external integrations.
  Accepts any file extension. Errors are returned immediately (unlike `--dir` which
  logs per-file errors and continues).

- **`ask --json`** — outputs the answer and deduplicated sources as a single JSON object.
  Progress messages are suppressed; stdout contains only valid JSON. Useful for
  scripting and external integrations.

  ```json
  {
    "answer": "...",
    "sources": [{"file_path": "...", "heading_path": "...", "score": 0.872}]
  }
  ```

---

## [0.1.2] — 2026-03-21

### Fixed

- **XSS in Web UI**: LLM responses rendered via `marked.parse()` could contain raw HTML
  (e.g. via prompt injection in indexed documents). The marked renderer now escapes raw
  HTML blocks instead of passing them through to `innerHTML`.

---

## [0.1.1] — 2026-03-21

### Added

- **`docs` subcommand** — manage documents stored in the index database.
  - `docs list [--json]` — list all indexed documents as a text table or JSON array.
    Each entry includes document ID (SHA-256), file path, chunk count, embedding model,
    and indexed timestamp.
  - `docs show <id>` — reconstruct and print the stored text of a document from its
    chunks; heading hierarchy is shown as `<!-- heading -->` comment separators.
  - `docs delete <id>` — remove a document and all its chunks from the database.

---

## [0.1.0] — 2026-03-21

### Added

- **`index <directory>`** — recursively indexes `*.md` files into a local DuckDB database.
  SHA-256-based change detection; unchanged files are skipped (idempotent).
  Unicode NFKC normalization, heading-aware Markdown chunker with JP/EN sentence
  boundary support. Embeddings stored as `FLOAT[]` via go-duckdb v2.

- **`ask <question>`** — answers natural-language questions using indexed documents.
  Cosine similarity search via DuckDB `list_cosine_similarity`. Context window
  expansion (±N adjacent chunks per hit). Overlapping windows deduplicated.
  LLM answer streamed to stdout.

- **`serve`** — starts a local HTTP API server with an embedded Web UI.
  - `POST /api/ask` — SSE-streamed RAG answer
  - `GET /api/status` — health check and version
  - `GET /` — embedded Web UI with Markdown rendering and Raw/Rendered toggle
  - `--addr` flag overrides the listen address at runtime
  - Configurable via `server.addr` and `server.log_level` in config file

- **`version`** — prints build-time version string.

- **Query rewriting** (`query_rewrite = true`) — LLM-assisted hybrid search rewrites
  the user query into a declarative retrieval statement and merges both result sets
  (max score per chunk). Improves top-score in 88% of benchmark queries on the full
  documentation corpus; adds ~2 s latency per query.

- **Prompt injection defense** — nonce-tagged XML context blocks with collision
  detection; prevents user content from escaping the document context boundary.

- **XDG config path** — default config location: `$XDG_CONFIG_HOME/lite-rag/config.toml`
  (fallback: `~/.config/lite-rag/config.toml`). Override with `--config`.

- **Environment variable overrides** for all key settings:
  `LITE_RAG_API_BASE_URL`, `LITE_RAG_API_KEY`, `LITE_RAG_EMBEDDING_MODEL`,
  `LITE_RAG_CHAT_MODEL`, `LITE_RAG_DB_PATH`.

- **Privacy-aware structured logging** — `log/slog` JSON to stderr.
  Query content logged at DEBUG only; INFO logs metadata (passages, score, latency).

- **`make dist`** — packages release archives (`tar.gz`) for all four platforms.

- **Cross-compilation** via `make cross-build`:
  - darwin/arm64, darwin/amd64 — macOS system `clang -arch`
  - linux/amd64, linux/arm64 — Podman/Docker container with GCC cross-compilers

- **Authoring guide** (`docs/authoring-guide.md`) — how to write Markdown documents
  for best RAG retrieval results, with Japanese translation.

- **Query-rewrite evaluation report** (`docs/eval/query-rewrite.md`) — benchmark
  results comparing baseline vs. hybrid retrieval on the full documentation corpus.

### Platform support

| OS    | Arch  | Archive |
|-------|-------|---------|
| macOS | arm64 | `lite-rag-0.1.0-darwin-arm64.tar.gz` |
| macOS | amd64 | `lite-rag-0.1.0-darwin-amd64.tar.gz` |
| Linux | amd64 | `lite-rag-0.1.0-linux-amd64.tar.gz` |
| Linux | arm64 | `lite-rag-0.1.0-linux-arm64.tar.gz` |

Windows is not supported due to CGo constraints from `go-duckdb`.

[0.2.1]: https://github.com/magifd2/lite-rag/releases/tag/v0.2.1
[0.2.0]: https://github.com/magifd2/lite-rag/releases/tag/v0.2.0
[0.1.2]: https://github.com/magifd2/lite-rag/releases/tag/v0.1.2
[0.1.1]: https://github.com/magifd2/lite-rag/releases/tag/v0.1.1
[0.1.0]: https://github.com/magifd2/lite-rag/releases/tag/v0.1.0
