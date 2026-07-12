## lite-rag Makefile
## Usage: make <target>

# Project settings
MODULE     := lite-rag
BINARY     := lite-rag
CMD        := ./cmd/lite-rag

# Go toolchain — use project-local GOPATH/cache to avoid polluting the system.
GOPATH     := $(CURDIR)/.go
GOMODCACHE := $(CURDIR)/.go/pkg/mod
GOCACHE    := $(CURDIR)/.go/cache
export GOPATH GOMODCACHE GOCACHE

# Tools
GOLANGCI     := $(GOPATH)/bin/golangci-lint
GOVULNCHECK  := $(GOPATH)/bin/govulncheck

# Version from git tag (fallback to "dev")
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS    := -ldflags "-X main.version=$(VERSION)"

# macOS Developer ID signing / notarization (see nlink-jp/.github
# CONVENTIONS.md §Code Signing). Defaults match any Developer ID
# Application cert in the keychain and the org-standard notary
# profile. Builds without these fall back to ad-hoc / un-notarized
# with a one-line warning — see scripts/codesign-darwin.sh. darwin
# release archives are .zip (per the org Release Archive Standard),
# which notarytool accepts directly, so the shipped zip is submitted
# to the notary service as-is (no temp-zip indirection).
CODESIGN_IDENTITY ?= Developer ID Application
NOTARY_PROFILE    ?= nlink-jp-notary

# Container runtime (podman preferred, docker as fallback)
CONTAINER  := $(shell command -v podman 2>/dev/null || command -v docker 2>/dev/null)
# Go image used for linux container builds — must match go.mod toolchain.
GO_IMAGE   := golang:1.26

# Distribution output directory
DIST_DIR   := dist

# Evaluation database — versioned by build date, symlinked to eval-current.db
EVAL_DATE  := $(shell date +%Y%m%d)
EVAL_DB    := testdata/db/lite-rag-docs-$(EVAL_DATE).db
EVAL_LINK  := testdata/db/eval-current.db

.PHONY: all build test lint vet vuln check setup \
        cross-build cross-build-darwin cross-build-linux cross-build-linux-native \
        dist dist-darwin dist-linux \
        eval-build-db eval \
        clean help

## all: build the binary for the current platform
all: build

## build: compile the binary for the current platform
build:
	@mkdir -p dist
	go build $(LDFLAGS) -o dist/$(BINARY) $(CMD)
	@scripts/codesign-darwin.sh dist/$(BINARY) "$(CODESIGN_IDENTITY)"

## test: run all tests
test:
	go test ./...

## vet: run go vet
vet:
	go vet ./...

## lint: run golangci-lint
lint: $(GOLANGCI)
	$(GOLANGCI) run ./...

## vuln: run govulncheck (security scan)
vuln: $(GOVULNCHECK)
	$(GOVULNCHECK) ./...

## check: run the full quality gate (vet + lint + test + build + vuln)
check: vet lint test build vuln
	@echo "✓ All checks passed."

## setup: install git hooks and required tools
setup:
	@echo "Installing git hooks..."
	cp scripts/hooks/pre-commit  .git/hooks/pre-commit
	cp scripts/hooks/pre-push    .git/hooks/pre-push
	chmod +x .git/hooks/pre-commit .git/hooks/pre-push
	@echo "Installing golangci-lint..."
	GOPATH=$(GOPATH) go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "Installing govulncheck..."
	GOPATH=$(GOPATH) go install golang.org/x/vuln/cmd/govulncheck@latest
	@echo "Setup complete."

# ── Cross-compilation ─────────────────────────────────────────────────────
#
# darwin targets: use macOS system clang (supports -arch for multi-arch).
# linux targets:  CGo + DuckDB require a Linux host; run inside a container
#                 via cross-build-linux (podman/docker).
#

## cross-build: compile binaries for all target platforms
cross-build: cross-build-darwin cross-build-linux

## cross-build-darwin: compile darwin/arm64 (macOS host only; arm64-only policy, no Intel)
cross-build-darwin:
	@mkdir -p dist
	@echo "Building darwin/arm64 (native)..."
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 \
		go build $(LDFLAGS) -o dist/$(BINARY)-darwin-arm64 $(CMD)
	@scripts/codesign-darwin.sh dist/$(BINARY)-darwin-arm64 "$(CODESIGN_IDENTITY)" "$(BINARY)"

## cross-build-linux: compile linux/amd64 and linux/arm64 inside a container
cross-build-linux:
	@if [ -z "$(CONTAINER)" ]; then \
		echo "Error: podman or docker is required for Linux cross-compilation."; \
		echo "Install podman (brew install podman) or run 'make cross-build-linux-native' on a Linux host."; \
		exit 1; \
	fi
	@mkdir -p bin
	@echo "Using container runtime: $(CONTAINER)"
	$(CONTAINER) run --rm \
		-v "$(CURDIR):/workspace:z" \
		-w /workspace \
		-e GOPATH=/workspace/.go \
		-e GOMODCACHE=/workspace/.go/pkg/mod \
		-e GOCACHE=/workspace/.go/cache \
		$(GO_IMAGE) \
		bash -c "apt-get update -qq && apt-get install -y -q gcc-aarch64-linux-gnu g++-aarch64-linux-gnu gcc-x86-64-linux-gnu g++-x86-64-linux-gnu && make cross-build-linux-native"

## cross-build-linux-native: compile linux/amd64 and linux/arm64 (Linux host only)
## Detects the host architecture at runtime to select the correct cross-compiler:
##   amd64 host → native gcc for amd64; aarch64-linux-gnu-gcc for arm64
##   arm64 host → x86_64-linux-gnu-gcc for amd64; native gcc for arm64
cross-build-linux-native:
	@mkdir -p bin
	@echo "Building linux/amd64..."
	@if [ "$$(uname -m)" = "aarch64" ]; then \
		GOOS=linux GOARCH=amd64 CGO_ENABLED=1 \
			CC=x86_64-linux-gnu-gcc CXX=x86_64-linux-gnu-g++ \
			go build $(LDFLAGS) -o dist/$(BINARY)-linux-amd64 $(CMD); \
	else \
		GOOS=linux GOARCH=amd64 CGO_ENABLED=1 \
			go build $(LDFLAGS) -o dist/$(BINARY)-linux-amd64 $(CMD); \
	fi
	@echo "Building linux/arm64..."
	@if [ "$$(uname -m)" = "x86_64" ]; then \
		GOOS=linux GOARCH=arm64 CGO_ENABLED=1 \
			CC=aarch64-linux-gnu-gcc CXX=aarch64-linux-gnu-g++ \
			go build $(LDFLAGS) -o dist/$(BINARY)-linux-arm64 $(CMD); \
	else \
		GOOS=linux GOARCH=arm64 CGO_ENABLED=1 \
			go build $(LDFLAGS) -o dist/$(BINARY)-linux-arm64 $(CMD); \
	fi

# ── Distribution packages ─────────────────────────────────────────────────
#
# Each archive contains:
#   lite-rag             — the compiled binary (canonical name)
#   README.md            — project readme
#   LICENSE              — license
#   config.example.toml  — reference configuration
#
# Naming (org Release Archive Standard): lite-rag-v<VERSION>-<OS>-<ARCH>.<ext>
#   darwin → .zip, linux → .tar.gz
#

## dist: build all platform binaries and package them as release archives
dist: dist-darwin dist-linux

## dist-darwin: package the notarized darwin/arm64 .zip (macOS host only)
# darwin ships arm64 only as a .zip, per the org Release Archive Standard.
# The .zip is the shipped artifact and notarytool accepts it directly, so
# there is no temp-zip indirection any more.
dist-darwin: cross-build-darwin
	@mkdir -p $(DIST_DIR)
	@echo "Packaging darwin/arm64..."
	@rm -rf $(DIST_DIR)/_pkg && mkdir -p $(DIST_DIR)/_pkg
	@cp dist/$(BINARY)-darwin-arm64 $(DIST_DIR)/_pkg/$(BINARY)
	@cp config.example.toml README.md LICENSE $(DIST_DIR)/_pkg/
	@( cd $(DIST_DIR)/_pkg && zip -q "../$(BINARY)-$(VERSION)-darwin-arm64.zip" * )
	@rm -rf $(DIST_DIR)/_pkg
	@echo "Notarizing darwin/arm64..."
	@scripts/notarize-darwin.sh $(DIST_DIR)/$(BINARY)-$(VERSION)-darwin-arm64.zip "$(NOTARY_PROFILE)"

## dist-linux: package linux/amd64 and linux/arm64 archives (requires container or Linux host)
dist-linux: cross-build-linux
	@mkdir -p $(DIST_DIR)
	@for arch in amd64 arm64; do \
		echo "Packaging linux/$$arch..."; \
		rm -rf $(DIST_DIR)/_pkg && mkdir -p $(DIST_DIR)/_pkg; \
		cp dist/$(BINARY)-linux-$$arch $(DIST_DIR)/_pkg/$(BINARY); \
		cp config.example.toml README.md LICENSE $(DIST_DIR)/_pkg/; \
		( cd $(DIST_DIR)/_pkg && tar -czf "../$(BINARY)-$(VERSION)-linux-$$arch.tar.gz" * ); \
		rm -rf $(DIST_DIR)/_pkg; \
	done

# ── Evaluation ────────────────────────────────────────────────────────────
#
# eval-build-db indexes docs/ into a date-stamped DuckDB file and updates
# the eval-current.db symlink used by the eval target.
#
# The database filename encodes the source and date so it is clear which
# version of the documentation was used:
#   testdata/db/lite-rag-docs-YYYYMMDD.db
#
# docs/eval/ is intentionally included even though it describes results from
# an earlier run; the retriever should still score those documents correctly.
#

## eval-build-db: index docs/ into a versioned test database
eval-build-db: build
	@mkdir -p testdata/db
	@echo "Indexing docs/ into $(EVAL_DB) ..."
	LITE_RAG_DB_PATH=$(EVAL_DB) ./dist/$(BINARY) index docs/
	@ln -sf lite-rag-docs-$(EVAL_DATE).db $(EVAL_LINK)
	@echo "Symlink updated: $(EVAL_LINK) -> lite-rag-docs-$(EVAL_DATE).db"

## eval: run retrieval quality evaluation against eval-current.db
eval:
	@if [ ! -e "$(EVAL_LINK)" ]; then \
		echo "Error: $(EVAL_LINK) not found. Run 'make eval-build-db' first."; \
		exit 1; \
	fi
	go run ./cmd/eval -db $(EVAL_LINK)

## clean: remove build artifacts
clean:
	rm -rf $(DIST_DIR)/

## help: show this help
help:
	@grep -E '^## ' Makefile | sed 's/## //'

# Install tools if not present
$(GOLANGCI):
	GOPATH=$(GOPATH) go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

$(GOVULNCHECK):
	GOPATH=$(GOPATH) go install golang.org/x/vuln/cmd/govulncheck@latest

# Homebrew tap generation (see scripts/release-brew.mk). After `make package`,
# `make brew` generates this formula from the built darwin-arm64 zip into the
# local nlink-jp/homebrew-tap checkout. The package target is unchanged.
BREW_KIND := formula
BREW_DESC := RAG CLI for Markdown documents using a local LLM
include scripts/release-brew.mk
