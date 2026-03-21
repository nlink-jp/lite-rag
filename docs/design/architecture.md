# Architecture Design: lite-rag

## 1. Overview

`lite-rag` is a CLI tool implementing a Retrieval-Augmented Generation (RAG) pipeline
for Markdown documents. It indexes documents into a local DuckDB database and answers
natural-language questions by retrieving relevant chunks and passing them to a local LLM
via an OpenAI-compatible API.

---

## 2. System Architecture

```
┌─────────────────────────────────────────────────────────┐
│                        CLI (cobra)                       │
│              cmd/lite-rag/main.go                        │
│         ┌──────────────┬─────────────────┐              │
│         │ index <dir>  │  ask "<query>"   │              │
└─────────┴──────┬───────┴────────┬────────┴──────────────┘
                 │                │
        ┌────────▼──────┐  ┌──────▼──────────┐
        │   Indexer     │  │    Retriever     │
        │ internal/     │  │  internal/       │
        │ indexer/      │  │  retriever/      │
        └────────┬──────┘  └──────┬───────────┘
                 │                │
        ┌────────▼────────────────▼───────────┐
        │           Database Layer             │
        │         internal/database/           │
        │          (DuckDB via CGo)            │
        └─────────────────────────────────────┘
                 │
        ┌────────▼──────────────────────────────┐
        │            LLM Client                  │
        │           internal/llm/                │
        │   (OpenAI-compatible HTTP client)      │
        └───────────────────────────────────────┘
```

---

## 3. Component Design

### 3.1 Configuration (`internal/config/`)

- Loaded from `~/.config/lite-rag/config.toml` by default (XDG Base Directory
  Specification: `$XDG_CONFIG_HOME/lite-rag/config.toml`). Override with `--config`.
- Environment variables override file-based settings (prefix: `LITE_RAG_`).
- A `config.example.toml` is provided as the canonical reference.

**Key settings:**

```toml
[api]
base_url     = "http://localhost:1234/v1"
api_key      = "lm-studio"           # placeholder; not validated by LM Studio

[models]
embedding    = "nomic-ai/nomic-embed-text-v1.5-GGUF"
chat         = "openai/gpt-oss-20b"

[database]
path         = "./lite-rag.db"

[retrieval]
top_k           = 5    # number of vector search hits
context_window  = 1    # adjacent chunks to fetch around each hit (0 = disabled)
chunk_size      = 512  # target token count per chunk
chunk_overlap   = 64   # overlap between adjacent chunks (handles boundary edge cases)
```

### 3.2 Indexer (`internal/indexer/`)

Responsibilities:
1. Walk the given directory recursively and collect `*.md` files.
2. Compute a SHA-256 hash of each file's content.
3. Compare against stored hashes in DuckDB; skip files that have not changed
   (idempotency).
4. Parse Markdown and split into semantic chunks (see §3.5).
5. Call the embedding API for each chunk (batched where possible).
6. Upsert chunks and their vector embeddings into DuckDB.

### 3.3 Retriever (`internal/retriever/`)

Responsibilities:
1. Accept a natural-language query string.
2. Call the embedding API to get the query vector.
3. Execute a cosine-similarity search against all stored chunk vectors in DuckDB.
4. For each hit, expand the result window by fetching adjacent chunks from the same
   document (see §3.3.1).
5. Return the enriched chunks as the context for LLM prompt construction.

Vector similarity search is implemented as a DuckDB SQL query using the built-in
`list_cosine_similarity` function, which operates on `FLOAT[]` columns natively.

#### 3.3.1 Context Window Expansion

A vector search hit identifies the semantically closest chunk, but that chunk may
begin or end mid-thought. To recover continuity, the Retriever fetches the `±N`
adjacent chunks from the same document and merges them around the hit.

```
Document chunks:  [ 0 ][ 1 ][ 2 ★ ][ 3 ][ 4 ][ 5 ]
                               ↑ vector search hit

context_window=1: [ 1 ][ 2 ★ ][ 3 ]         (hit ± 1)
context_window=2: [ 0 ][ 1 ][ 2 ★ ][ 3 ][ 4 ] (hit ± 2)
```

The fetch is a single SQL query per hit, using `chunk_index` and `document_id`:

```sql
SELECT content, chunk_index, heading_path
FROM   chunks
WHERE  document_id = $1
  AND  chunk_index BETWEEN $hit_index - $window AND $hit_index + $window
ORDER  BY chunk_index ASC;
```

Adjacent chunks are concatenated in order; the result is presented to the LLM as one
continuous passage. The hit chunk's similarity score is carried forward as the
relevance score for ranking.

**Deduplication:** when multiple hits come from the same document and their expanded
windows overlap, the overlapping region is merged rather than repeated.

**Configuration:**

```toml
[retrieval]
top_k          = 5   # number of vector search hits
context_window = 1   # chunks to expand before and after each hit (0 = no expansion)
```

Setting `context_window = 0` disables expansion and returns bare hit chunks,
which is useful for benchmarking retrieval quality in isolation.

### 3.4 LLM Client (`internal/llm/`)

- A thin HTTP client wrapping the OpenAI-compatible Chat Completions endpoint.
- Uses `net/http` from the standard library — no third-party SDK dependency.
- Supports **streaming** responses via Server-Sent Events (SSE); the client writes
  tokens to `io.Writer` as they arrive.
- Separate `Embed()` method for the Embeddings endpoint (non-streaming).

### 3.5 Chunker (`pkg/chunker/`)

Placed in `pkg/` because it has no I/O dependencies and may be reused independently.

The chunker operates on **normalized text** (see §3.7) — raw Markdown is normalized
before chunking begins.

Algorithm:
1. Split on Markdown headings (`#`, `##`, `###`) to preserve semantic boundaries.
2. If a heading section exceeds `chunk_size` tokens, further split using these
   boundaries in priority order:
   a. Blank lines (paragraph breaks) — language-agnostic.
   b. Japanese sentence endings: `。` `．` `！` `？` (full-width punctuation).
   c. Western sentence endings: `.` `!` `?` followed by whitespace.
3. Prepend the heading hierarchy path to each chunk for context
   (e.g., `# Guide > ## Installation`).
4. Each chunk carries metadata: `source_file`, `heading_path`, `byte_offset`.

**Token count estimation for mixed-language text:**

Japanese text has no word boundaries and tokenizes differently from English.
A simple `word_count × 1.3` estimate grossly undercounts Japanese content.

The estimator classifies each character and applies per-script coefficients:

| Script | Characters | Coefficient |
|---|---|---|
| CJK (Kanji, Hiragana, Katakana, etc.) | `\p{Han}`, `\p{Hiragana}`, `\p{Katakana}` | 1 char ≈ 2 tokens |
| ASCII / Latin | `[A-Za-z0-9]` | 1 word ≈ 1.3 tokens |
| Punctuation / other | — | counted as 0 (negligible) |

Estimated tokens = `(cjk_char_count × 2) + (word_count × 1.3)`

This is a conservative heuristic; the exact count depends on the embedding model's
tokenizer. The goal is to prevent oversized chunks from being silently truncated.

**Nomic-embed prefix injection:**

`nomic-embed-text-v1.5` requires task-specific prefixes to achieve advertised retrieval
quality. The chunker does **not** add prefixes — that responsibility belongs to the
caller (Indexer and Retriever), so the stored chunk content remains prefix-free.

- Indexer wraps chunks as: `search_document: {chunk_content}`
- Retriever wraps queries as: `search_query: {question}`

### 3.6 Text Normalization (`internal/normalizer/`)

Documents may mix Japanese and English. Raw text must be normalized before chunking
and before generating embeddings, to reduce superficial variance that would degrade
retrieval quality.

**Normalization pipeline (applied in order):**

```
Raw Markdown text
       │
       ▼
1. Unicode NFKC normalization   (golang.org/x/text/unicode/norm)
       │  • Full-width ASCII → half-width  （Ａ→A, １→1, （→(）
       │  • Half-width Katakana → full-width  (ｶﾅ → カナ)
       │  • Compatibility ligatures decomposed  (㍉→ミリ, ㎞→km)
       ▼
2. Whitespace normalization
       │  • Full-width space U+3000 → U+0020
       │  • Consecutive spaces / tabs → single space
       │  • Normalize line endings to LF
       ▼
3. Control-character removal
       │  • Strip C0/C1 control characters (except LF and TAB)
       ▼
4. Markdown artifact stripping  (applied only before embedding, not before chunking)
       │  • Remove image syntax  ![alt](url) → alt
       │  • Collapse link syntax  [text](url) → text
       │  • Remove raw HTML tags
       │  • Remove fenced code block markers (``` lines); keep code content
       │  • Keep inline code content (strip backticks only)
       ▼
Normalized text (ready for chunking or embedding)
```

**Design notes:**

- Steps 1–3 are applied **before** chunking so that chunk boundaries are computed
  on normalized text and stored content is already clean.
- Step 4 (Markdown artifact stripping) is applied **again** just before sending text
  to the embedding API, so that structural Markdown syntax does not pollute the
  semantic vector space.
- The original raw content is **not** stored; only normalized content is written to
  DuckDB. The normalization is deterministic, so re-indexing the same file produces
  the same result.
- Normalization is exposed as a pure function (`normalizer.Normalize(text string) string`)
  with no I/O, making it straightforward to unit-test.

### 3.7 Database Layer (`internal/database/`)

Schema:

```sql
CREATE TABLE documents (
    id              TEXT      PRIMARY KEY,  -- SHA-256 of (file_path + ":" + file_hash)
    file_path       TEXT      NOT NULL,
    file_hash       TEXT      NOT NULL,     -- SHA-256 of file content (change detection)
    total_chunks    INTEGER   NOT NULL,     -- total chunk count; used for boundary clamping
    indexed_at      TIMESTAMP NOT NULL,
    embedding_model TEXT      NOT NULL DEFAULT ''  -- model name used to produce embeddings
);

CREATE TABLE chunks (
    id           TEXT    PRIMARY KEY,  -- SHA-256 of (document_id + ":" + chunk_index)
    document_id  TEXT    NOT NULL REFERENCES documents(id),
    chunk_index  INTEGER NOT NULL,     -- 0-based position within the document
    heading_path TEXT,                 -- e.g. "# Guide > ## Installation"
    content      TEXT    NOT NULL,     -- normalized text (prefix-free)
    embedding    FLOAT[]               -- []float32; stored natively via go-duckdb v2
);
```

`total_chunks` in `documents` allows the context expansion query to clamp the
`BETWEEN` range to `[0, total_chunks - 1]` without a subquery, avoiding boundary
errors for the first and last chunks of a document.

Similarity search uses DuckDB's built-in `list_cosine_similarity` function:

```sql
SELECT c.id, c.document_id, c.chunk_index, c.heading_path, c.content,
       list_cosine_similarity(c.embedding, ?) AS score
FROM   chunks c
JOIN   documents d ON d.id = c.document_id
WHERE  c.embedding IS NOT NULL AND len(c.embedding) > 0
  AND  d.embedding_model = ?
ORDER  BY score DESC
LIMIT  ?
```

After finding the top-K hits, the Retriever fetches adjacent chunks for each hit
and merges overlapping spans before returning passages to the caller (§3.3.1).

---

## 4. CLI Interface

```
lite-rag [--config <path>] <command>

Commands:
  index   --dir <directory>   Index all *.md files under a directory
          --file <file>        Index a single file (any extension)
  ask     <question>    Answer a question using the indexed documents
          --json         Output answer and sources as a JSON object
  serve                 Start the HTTP API server with embedded Web UI
  docs                  Manage indexed documents (list / show / delete)
  version               Print version information
```

Global flag `--config` defaults to `~/.config/lite-rag/config.toml` (XDG).

---

## 5. Target Platforms

| OS      | Arch  | Supported |
|---------|-------|-----------|
| linux   | amd64 | Yes       |
| linux   | arm64 | Yes       |
| darwin  | amd64 | Yes       |
| darwin  | arm64 | Yes       |
| windows | amd64 | **No** — dropped due to CGo constraints from `go-duckdb` |

Cross-compilation strategy:

- **darwin targets**: macOS system `clang` with `-arch` flag; no extra tools needed.
- **linux targets**: require a Linux C/C++ runtime. The Makefile runs the build inside
  a container (`podman` / `docker`) with the appropriate cross-compiler packages
  (`gcc-x86-64-linux-gnu`, `gcc-aarch64-linux-gnu`). On a native Linux host use
  `make cross-build-linux-native` directly.

---

## 6. Dependencies

| Package | Purpose | Rationale |
|---|---|---|
| `github.com/marcboeker/go-duckdb` | DuckDB driver | Official Go driver; embedded DB avoids an external server |
| `github.com/spf13/cobra` | CLI framework | Standard Go CLI library; structured subcommand support |
| `github.com/BurntSushi/toml` | TOML config parsing | Minimal, widely used, no transitive deps |
| `golang.org/x/text` | Unicode normalization (NFKC) | Standard library extension; required for reliable Japanese text handling |

All dependencies are documented in `docs/dependencies.md` (Rule 18).

---

## 7. Error Handling

- All errors are propagated up and logged with structured output (`log/slog`).
- The CLI exits with a non-zero status code on any fatal error.
- The indexer reports per-file errors without aborting the entire run (recoverable).
- LLM API errors (timeouts, 5xx) are surfaced immediately with the raw status code.

---

## 8. Security Considerations

- API keys are never logged.
- No secrets are stored in the database or configuration template.
- The database file path defaults to the current directory; users should restrict
  file permissions as appropriate.

---

*Primary language: English. Japanese translation: `docs/ja/design/architecture.md`*
