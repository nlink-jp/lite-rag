# Project Directory Structure

```
lite-rag/
├── cmd/
│   ├── lite-rag/
│   │   ├── main.go          # CLI entry point; wires subcommands via cobra
│   │   ├── index.go         # `index <dir>` subcommand
│   │   ├── ask.go           # `ask <question>` subcommand
│   │   ├── serve.go         # `serve` subcommand — starts HTTP API + Web UI
│   │   ├── docs.go          # `docs` subcommand — list / show / delete
│   │   └── version.go       # `version` subcommand
│   └── eval/
│       └── main.go          # Retrieval quality evaluation harness (query-rewrite benchmark)
│
├── internal/                # Private application code (not importable externally)
│   ├── config/
│   │   └── config.go        # TOML + env-var configuration loading
│   ├── database/
│   │   └── db.go            # DuckDB connection and schema migration
│   ├── normalizer/
│   │   └── normalizer.go    # Unicode NFKC normalization; Markdown stripping;
│   │                        # mixed JP/EN token estimation
│   ├── indexer/
│   │   └── indexer.go       # Walk docs, normalize, chunk, embed, upsert into DuckDB
│   ├── retriever/
│   │   └── retriever.go     # Vector search + context window expansion + deduplication
│   ├── llm/
│   │   └── client.go        # OpenAI-compatible HTTP client (Embed + Chat/stream)
│   └── server/
│       ├── server.go        # Server struct, routing, graceful shutdown
│       ├── rag.go           # RAG query logic shared by ask command and HTTP handler
│       ├── handler_ask.go   # POST /api/ask — SSE streaming handler
│       ├── handler_status.go # GET /api/status
│       ├── embed.go         # //go:embed static/*
│       └── static/          # Embedded Web UI (index.html, app.js, style.css, marked.min.js)
│
├── pkg/                     # Public library code (reusable independently)
│   └── chunker/
│       └── chunker.go       # Heading-aware Markdown chunker (JP + EN boundaries)
│
├── api/                     # API contract files (placeholder; future HTTP frontend)
│
├── scripts/
│   └── hooks/
│       ├── pre-commit       # Runs `make check` before each commit
│       └── pre-push         # Runs `make check` before each push
│
│
├── docs/
│   ├── design/
│   │   ├── architecture.md  # System architecture and component design
│   │   └── plan.md          # Development phases and milestones
│   ├── eval/
│   │   └── query-rewrite.md # Query-rewrite feature evaluation report
│   ├── ja/                  # Japanese translations of all primary docs
│   ├── RFP.md               # Original requirements document
│   ├── authoring-guide.md   # How to write Markdown documents for best retrieval results
│   ├── dependencies.md      # Third-party dependency register (RULES.md §18)
│   ├── setup.md             # Installation and development environment setup
│   └── structure.md         # This file
│
├── .go/                     # Project-local Go module cache (git-ignored)
│   ├── pkg/mod/             # Downloaded module sources
│   └── cache/               # Build cache
│
├── bin/                     # Compiled binaries (git-ignored)
├── dist/                    # Release archives produced by `make dist` (git-ignored)
├── config.example.toml      # Reference configuration with all available settings
├── go.mod                   # Go module definition
├── go.sum                   # Module checksum database
├── Makefile                 # Build, test, lint, cross-compile targets
└── RULES.md                 # Project rules (all contributors must follow)
```

## Key Conventions

- **`internal/`** packages are not importable by external projects. Cross-package
  dependencies flow inward: `cmd` → `internal/*` → `pkg/*`.
- **`pkg/chunker`** has no I/O dependencies and may be imported by other projects.
- **`internal/normalizer`** is called by both the Indexer (before storing) and the
  Retriever (before embedding a query).
- **`.go/`** is the project-local GOPATH/GOMODCACHE/GOCACHE. It keeps downloaded
  modules inside the project directory, which is useful in network-restricted
  environments. The directory is git-ignored.
