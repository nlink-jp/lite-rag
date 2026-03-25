package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── Helpers ─────────────────────────────────────────────────────────────────

// writeCfg creates a config.toml in dir pointing at apiURL and dbPath.
func writeCfg(t *testing.T, dir, apiURL, dbPath string) string {
	t.Helper()
	content := fmt.Sprintf(`
[api]
base_url = %q
api_key  = "test"

[models]
embedding = "test-embed"
chat      = "test-chat"

[database]
path = %q

[retrieval]
top_k          = 3
context_window = 1
chunk_size     = 512
chunk_overlap  = 64
`, apiURL, dbPath)
	p := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return p
}

// mockLLM returns a test server that handles /embeddings and /chat/completions.
func mockLLM(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/embeddings":
			var req struct {
				Input []string `json:"input"`
			}
			json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
			data := make([]map[string]any, len(req.Input))
			for i := range data {
				data[i] = map[string]any{
					"index":     i,
					"embedding": []float32{1, 0, 0, 0},
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"data": data}) //nolint:errcheck
		case "/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"answer\"},\"finish_reason\":null}]}\n")
			fmt.Fprint(w, "data: [DONE]\n")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// run executes rootCmd with the given arguments and returns the error.
// It suppresses cobra's built-in usage/error printing to keep test output clean.
func run(args ...string) error {
	// Reset package-level flag vars; cobra does not reset them between Execute calls.
	indexDir = ""
	indexFile = ""
	dbOverride = ""

	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	// Reset so subsequent tests start clean.
	rootCmd.SilenceUsage = false
	rootCmd.SilenceErrors = false
	return err
}

// ── index command ────────────────────────────────────────────────────────────

func TestIndexCmd_HappyPath(t *testing.T) {
	srv := mockLLM(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	cfg := writeCfg(t, dir, srv.URL, dbPath)

	docDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(docDir, "a.md"), []byte("# Hello\n\nWorld."), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := run("--config", cfg, "index", "--dir", docDir); err != nil {
		t.Errorf("index command failed: %v", err)
	}
}

func TestIndexCmd_SingleFile(t *testing.T) {
	srv := mockLLM(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	cfg := writeCfg(t, dir, srv.URL, dbPath)

	filePath := filepath.Join(dir, "single.md")
	if err := os.WriteFile(filePath, []byte("# Single\n\nFile content."), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := run("--config", cfg, "index", "--file", filePath); err != nil {
		t.Errorf("index --file command failed: %v", err)
	}
}

func TestIndexCmd_MutuallyExclusive(t *testing.T) {
	srv := mockLLM(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	cfg := writeCfg(t, dir, srv.URL, dbPath)

	if err := run("--config", cfg, "index", "--dir", dir, "--file", dbPath); err == nil {
		t.Error("expected error when both --dir and --file are specified, got nil")
	}
}

func TestIndexCmd_NoFlags(t *testing.T) {
	srv := mockLLM(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	cfg := writeCfg(t, dir, srv.URL, dbPath)

	if err := run("--config", cfg, "index"); err == nil {
		t.Error("expected error when neither --dir nor --file is specified, got nil")
	}
}

func TestIndexCmd_NonexistentDir(t *testing.T) {
	srv := mockLLM(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	cfg := writeCfg(t, dir, srv.URL, dbPath)

	if err := run("--config", cfg, "index", "--dir", "/nonexistent/path/xyz"); err == nil {
		t.Error("expected error for nonexistent directory, got nil")
	}
}

func TestIndexCmd_InvalidConfig(t *testing.T) {
	// Malformed TOML must cause config.Load to fail and the command to return an error.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bad.toml")
	if err := os.WriteFile(cfgPath, []byte("not valid toml [[[["), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run("--config", cfgPath, "index", "--dir", dir); err == nil {
		t.Error("expected error for invalid TOML config, got nil")
	}
}

func TestIndexCmd_DBError(t *testing.T) {
	srv := mockLLM(t)
	dir := t.TempDir()
	// Use a directory as the DB path — DuckDB will fail to open it.
	cfg := writeCfg(t, dir, srv.URL, dir)

	docDir := t.TempDir()
	if err := run("--config", cfg, "index", "--dir", docDir); err == nil {
		t.Error("expected error when DB path is a directory, got nil")
	}
}

// ── ask command ──────────────────────────────────────────────────────────────

func TestAskCmd_EmptyDB(t *testing.T) {
	srv := mockLLM(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	cfg := writeCfg(t, dir, srv.URL, dbPath)

	// No documents indexed — ask should complete without error.
	var out strings.Builder
	rootCmd.SetOut(&out)
	defer rootCmd.SetOut(nil)

	if err := run("--config", cfg, "ask", "what is this?"); err != nil {
		t.Errorf("ask on empty DB should not error, got: %v", err)
	}
}

func TestAskCmd_AfterIndex(t *testing.T) {
	srv := mockLLM(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	cfg := writeCfg(t, dir, srv.URL, dbPath)

	// Index a document first.
	docDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(docDir, "doc.md"), []byte("# Topic\n\nSome content."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run("--config", cfg, "index", "--dir", docDir); err != nil {
		t.Fatalf("index failed: %v", err)
	}

	// Now ask a question.
	if err := run("--config", cfg, "ask", "what is the topic?"); err != nil {
		t.Errorf("ask command failed: %v", err)
	}
}

func TestAskCmd_DBError(t *testing.T) {
	srv := mockLLM(t)
	dir := t.TempDir()
	// Use a directory as the DB path.
	cfg := writeCfg(t, dir, srv.URL, dir)

	if err := run("--config", cfg, "ask", "question"); err == nil {
		t.Error("expected error when DB path is a directory, got nil")
	}
}

// ── --db global flag ─────────────────────────────────────────────────────────

// TestDBFlag_OverridesConfigPath verifies that --db takes precedence over the
// database.path in the config file.
func TestDBFlag_OverridesConfigPath(t *testing.T) {
	srv := mockLLM(t)
	dir := t.TempDir()

	configDB := filepath.Join(dir, "config.db")
	flagDB := filepath.Join(dir, "flag.db")

	// Config points to configDB.
	cfg := writeCfg(t, dir, srv.URL, configDB)

	docDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(docDir, "doc.md"), []byte("# Topic\n\nContent."), 0o644); err != nil {
		t.Fatal(err)
	}

	// Index into flagDB via --db (configDB should remain untouched).
	if err := run("--config", cfg, "--db", flagDB, "index", "--dir", docDir); err != nil {
		t.Fatalf("index with --db failed: %v", err)
	}

	// flagDB must exist; configDB must not.
	if _, err := os.Stat(flagDB); err != nil {
		t.Errorf("--db target %s not created: %v", flagDB, err)
	}
	if _, err := os.Stat(configDB); err == nil {
		t.Errorf("config db %s should not have been created", configDB)
	}
}

// TestDBFlag_IndexAndAsk verifies end-to-end: index into a --db path, then ask
// against the same --db path (config db is different and empty throughout).
func TestDBFlag_IndexAndAsk(t *testing.T) {
	srv := mockLLM(t)
	dir := t.TempDir()

	configDB := filepath.Join(dir, "config.db")
	flagDB := filepath.Join(dir, "flag.db")
	cfg := writeCfg(t, dir, srv.URL, configDB)

	docDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(docDir, "doc.md"), []byte("# Topic\n\nContent."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run("--config", cfg, "--db", flagDB, "index", "--dir", docDir); err != nil {
		t.Fatalf("index with --db failed: %v", err)
	}

	if err := run("--config", cfg, "--db", flagDB, "ask", "what is the topic?"); err != nil {
		t.Errorf("ask with --db failed: %v", err)
	}
}

// TestDBFlag_InvalidPath verifies that an unwritable --db path produces an error.
func TestDBFlag_InvalidPath(t *testing.T) {
	srv := mockLLM(t)
	dir := t.TempDir()
	cfg := writeCfg(t, dir, srv.URL, filepath.Join(dir, "config.db"))

	// Passing a directory as --db should fail when DuckDB tries to open it.
	if err := run("--config", cfg, "--db", dir, "ask", "question"); err == nil {
		t.Error("expected error for --db pointing at a directory, got nil")
	}
}
