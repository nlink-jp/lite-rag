# lite-rag

A CLI-based Retrieval-Augmented Generation (RAG) tool for Markdown documents.
Index a directory of Markdown files into a local [DuckDB](https://duckdb.org/) database,
then ask natural-language questions answered by a local LLM via an OpenAI-compatible API
(e.g., [LM Studio](https://lmstudio.ai/)).

Japanese and English mixed documents are fully supported with NFKC normalization and
mixed-script token estimation.

---

## Features

- **Index**: recursively scans a directory for `*.md` files, chunks them, embeds with a
  local model, and stores vectors in DuckDB.
- **Ask**: embeds your question, finds the most relevant chunks, expands context with
  adjacent chunks, and streams an LLM-generated answer.
- **Serve**: starts a local HTTP API server (`POST /api/ask` SSE, `GET /api/status`) with
  an embedded Web UI that renders Markdown answers in the browser.
- **Incremental updates**: re-indexing only re-processes files whose content has changed
  (SHA-256 hash check).
- **Japanese support**: Unicode NFKC normalization, Japanese sentence boundaries for
  chunking, mixed CJK/ASCII token estimation.
- **Context window expansion**: each vector-search hit is expanded by ±N adjacent chunks
  for continuity; overlapping spans are deduplicated automatically.

---

## Requirements

| Tool | Version | Purpose |
|---|---|---|
| Go | 1.26+ | Build toolchain |
| make | any | Build automation |
| cc (clang/gcc) | any | CGo (required by go-duckdb) |
| LM Studio | any | Local LLM inference server |

See [docs/setup.md](docs/setup.md) for full setup instructions.

---

## Quick Start

```sh
git clone <repo-url> lite-rag
cd lite-rag

# Copy and configure
mkdir -p ~/.config/lite-rag
cp config.example.toml ~/.config/lite-rag/config.toml
$EDITOR ~/.config/lite-rag/config.toml   # set api.base_url, api.api_key, models.*

# Install git hooks and linter
make setup

# Build
make build

# Index a directory of Markdown files
./bin/lite-rag index --dir /path/to/docs

# Index a single file
./bin/lite-rag index --file /path/to/doc.md

# Ask a question (CLI)
./bin/lite-rag ask "How do I configure the retry policy?"

# Or start the Web UI server
./bin/lite-rag serve        # opens at http://127.0.0.1:8080
```

---

## Configuration

Copy `config.example.toml` to `~/.config/lite-rag/config.toml` and adjust:

```toml
[api]
base_url = "http://localhost:1234/v1"   # LM Studio default
api_key  = "lm-studio"                 # arbitrary; LM Studio does not validate

[models]
embedding = "nomic-ai/nomic-embed-text-v1.5-GGUF"
chat      = "openai/gpt-oss-20b"

[database]
path = "./lite-rag.db"

[retrieval]
top_k          = 5     # vector search hits
context_window = 1     # adjacent chunks to expand around each hit
chunk_size     = 512
chunk_overlap  = 64
query_rewrite  = false # enable LLM-assisted query rewriting (improves recall, +~2 s/query)

[server]
addr      = "127.0.0.1:8080"  # listen address for `serve`
log_level = "info"             # info | debug | warn | error
```

Environment variables override file settings:

| Variable | Overrides |
|---|---|
| `LITE_RAG_API_BASE_URL` | `api.base_url` |
| `LITE_RAG_API_KEY` | `api.api_key` |
| `LITE_RAG_EMBEDDING_MODEL` | `models.embedding` |
| `LITE_RAG_CHAT_MODEL` | `models.chat` |
| `LITE_RAG_DB_PATH` | `database.path` |

---

## Usage

```
lite-rag [--config <path>] <command>

Commands:
  index   --dir <directory>   Index all *.md files under a directory
          --file <file>        Index a single file (any extension)
  ask     <question>           Answer a question using the indexed documents
  serve                 Start the HTTP API server with embedded Web UI
  docs                  Manage indexed documents
  version               Print version information
```

### index

```sh
# Index all *.md files under a directory (recursive)
./bin/lite-rag index --dir ./docs

# Index a single file (any extension)
./bin/lite-rag index --file ./docs/notes.md
./bin/lite-rag index --file ./release-notes.txt
```

- `--dir`: walks the directory recursively; processes only `*.md` files.
- `--file`: indexes the specified file directly, regardless of extension.
- Skips files whose SHA-256 hash matches the stored value (no re-embedding).
- `--dir` per-file errors are logged and do not abort the overall run; `--file` errors are returned immediately.

### ask

```sh
./bin/lite-rag ask "What is the default chunk size?"
./bin/lite-rag --config /etc/lite-rag.toml ask "Installation steps?"

# JSON output (answer + sources as a single JSON object)
./bin/lite-rag ask --json "What is the default chunk size?"
```

- Embeds the question, searches DuckDB for the top-K similar chunks.
- Expands each hit by ±`context_window` adjacent chunks.
- Streams the LLM answer to stdout.
- Set `query_rewrite = true` to enable **multilingual query rewriting**: the LLM
  rewrites the query into both Japanese and English declarative statements, and three
  parallel vector searches (original + JA + EN) are merged, improving recall across
  multilingual document collections (improves score in ~88% of queries; adds ~2 s latency).
- `--json`: buffers the full answer and outputs a single JSON object.
  Progress messages are suppressed; stdout contains only valid JSON.

```json
{
  "answer": "The default chunk size is 512 tokens.",
  "sources": [
    {"file_path": "docs/README.md", "heading_path": "Configuration", "score": 0.872}
  ]
}
```

### serve

```sh
./bin/lite-rag serve                     # listen on 127.0.0.1:8080 (default)
./bin/lite-rag serve --addr 0.0.0.0:9090
```

Starts a local HTTP server. Open `http://127.0.0.1:8080` in a browser.

| Endpoint | Method | Description |
|---|---|---|
| `/api/ask` | POST | SSE-streamed answer (`{"query":"..."}`) |
| `/api/status` | GET | Health check and version |
| `/` | GET | Embedded Web UI |

### docs

Manage documents stored in the index database.

```sh
# List all indexed documents (text table)
./bin/lite-rag docs list

# List as JSON (machine-readable)
./bin/lite-rag docs list --json

# Show reconstructed content of a document by ID
./bin/lite-rag docs show <document-id>

# Delete a document and all its chunks
./bin/lite-rag docs delete <document-id>
```

`<document-id>` is the 64-character SHA-256 hex shown in `docs list`.

---

## Building

```sh
# Current platform
make build

# All darwin platforms (macOS host)
make cross-build-darwin

# Linux platforms (requires podman or docker, or run on a Linux host)
make cross-build-linux
```

Binaries are placed in `bin/`. See [docs/setup.md](docs/setup.md) for cross-compilation details.

---

## Development

```sh
make test    # run all tests
make vet     # go vet
make lint    # golangci-lint
make check   # full quality gate: vet + lint + test + build
```

Git hooks installed by `make setup` run `make check` automatically before each
commit and push.

---

## Architecture

See [docs/design/architecture.md](docs/design/architecture.md) for the full design.

```
index command
  └─ Indexer: walk → normalize → chunk → embed → DuckDB

ask command
  └─ Retriever: embed query → SimilarChunks → AdjacentChunks → LLM Chat
```

---

## License

MIT

---

*日本語ドキュメント: [docs/ja/README.md](docs/ja/README.md)*
