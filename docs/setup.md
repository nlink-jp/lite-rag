# Setup Guide

## Prerequisites

| Tool | Version | Purpose |
|---|---|---|
| Go | 1.26+ | Build toolchain |
| make | any | Build automation |
| cc (clang/gcc) | any | CGo compilation (required by go-duckdb) |
| podman or docker | any | Linux cross-compilation (optional) |

> **Note:** The build uses a project-local Go module cache (`.go/`) to avoid
> polluting `~/go`. This is configured automatically by the Makefile.

## Quick Start

```sh
git clone <repo-url> lite-rag
cd lite-rag

# Copy and edit configuration
mkdir -p ~/.config/lite-rag
cp config.example.toml ~/.config/lite-rag/config.toml
$EDITOR ~/.config/lite-rag/config.toml

# Install git hooks and build tools
make setup

# Build
make build

# Run
./dist/lite-rag --help
```

## LM Studio Configuration

1. Start LM Studio and load the following models:
   - Embedding: `nomic-ai/nomic-embed-text-v1.5-GGUF`
   - Chat: `openai/gpt-oss-20b`
2. Enable the local server (default port: `1234`).
3. Confirm the API is reachable:
   ```sh
   curl http://localhost:1234/v1/models
   ```

## Security Scanning

`govulncheck` scans Go dependencies for known vulnerabilities and runs as part of `make check`.

```sh
# Install manually (also done by make setup)
go install golang.org/x/vuln/cmd/govulncheck@latest

# Run standalone
make vuln
```

## Git Hook Installation

Git hooks are managed under `scripts/hooks/`. To install them:

```sh
make setup
```

This copies `scripts/hooks/pre-commit` and `scripts/hooks/pre-push` to `.git/hooks/`
and makes them executable. Both hooks run `make check` (lint + test + build) before
allowing the operation.

## Environment Variables

All settings in the config file can be overridden via environment variables:

| Variable | Overrides |
|---|---|
| `LITE_RAG_API_BASE_URL` | `api.base_url` |
| `LITE_RAG_API_KEY` | `api.api_key` |
| `LITE_RAG_EMBEDDING_MODEL` | `models.embedding` |
| `LITE_RAG_CHAT_MODEL` | `models.chat` |
| `LITE_RAG_DB_PATH` | `database.path` |

The default config file path follows the XDG Base Directory Specification:
`$XDG_CONFIG_HOME/lite-rag/config.toml` (defaults to `~/.config/lite-rag/config.toml`).
Override with the `--config` flag: `lite-rag --config /path/to/config.toml`.

## Cross-Compilation

### darwin targets (from a macOS host)

Uses the macOS system `clang` with `-arch` for multi-architecture support.
No additional tools required.

```sh
make cross-build-darwin
# Produces: dist/lite-rag-darwin-arm64  dist/lite-rag-darwin-amd64
```

### linux targets (via container)

`go-duckdb` links a prebuilt static DuckDB library built with GCC/libstdc++.
Cross-compiling this from macOS requires a Linux environment with the matching
C++ runtime. The easiest approach is to run the build inside a container:

```sh
# Requires podman (brew install podman) or docker
make cross-build-linux
# Produces: dist/lite-rag-linux-amd64  dist/lite-rag-linux-arm64
```

The Makefile detects `podman` first, then falls back to `docker`.

### linux targets (on a Linux host)

If you are already on a Linux machine, build directly without a container:

```sh
# For arm64, install the cross-compiler first:
sudo apt-get install gcc-aarch64-linux-gnu

make cross-build-linux-native
```


## Project-Local Module Cache

The Makefile sets:

```
GOPATH=$(pwd)/.go
GOMODCACHE=$(pwd)/.go/pkg/mod
GOCACHE=$(pwd)/.go/cache
```

This keeps all downloaded modules and build artifacts inside the project
directory, which is useful in restricted environments. The `.go/` directory
is git-ignored.
