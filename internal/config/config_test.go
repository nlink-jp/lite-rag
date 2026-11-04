package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lite-rag/internal/config"
)

func TestDefaults(t *testing.T) {
	// Load from a non-existent path should return defaults without error.
	cfg, err := config.Load("/nonexistent/config.toml")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.API.BaseURL != "http://localhost:1234/v1" {
		t.Errorf("BaseURL = %q, want default", cfg.API.BaseURL)
	}
	if cfg.Retrieval.TopK != 5 {
		t.Errorf("TopK = %d, want 5", cfg.Retrieval.TopK)
	}
	if cfg.Retrieval.ContextWindow != 1 {
		t.Errorf("ContextWindow = %d, want 1", cfg.Retrieval.ContextWindow)
	}
}

func TestLoadTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[api]
base_url = "http://example.com/v1"
api_key  = "test-key"

[retrieval]
top_k          = 10
context_window = 2
chunk_size     = 256
chunk_overlap  = 32
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.API.BaseURL != "http://example.com/v1" {
		t.Errorf("BaseURL = %q", cfg.API.BaseURL)
	}
	if cfg.API.APIKey != "test-key" {
		t.Errorf("APIKey = %q", cfg.API.APIKey)
	}
	if cfg.Retrieval.TopK != 10 {
		t.Errorf("TopK = %d", cfg.Retrieval.TopK)
	}
	if cfg.Retrieval.ContextWindow != 2 {
		t.Errorf("ContextWindow = %d", cfg.Retrieval.ContextWindow)
	}
}

func TestEnvOverride(t *testing.T) {
	t.Setenv("LITE_RAG_API_BASE_URL", "http://env-override/v1")
	t.Setenv("LITE_RAG_API_KEY", "env-key")

	cfg, err := config.Load("/nonexistent/config.toml")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.API.BaseURL != "http://env-override/v1" {
		t.Errorf("BaseURL = %q, want env override", cfg.API.BaseURL)
	}
	if cfg.API.APIKey != "env-key" {
		t.Errorf("APIKey = %q, want env override", cfg.API.APIKey)
	}
}

func TestEnvOverride_DBPath(t *testing.T) {
	t.Setenv("LITE_RAG_DB_PATH", "/tmp/custom.db")

	cfg, err := config.Load("/nonexistent/config.toml")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Database.Path != "/tmp/custom.db" {
		t.Errorf("Database.Path = %q, want env override", cfg.Database.Path)
	}
}

func TestInvalidTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.toml")
	if err := os.WriteFile(path, []byte("not valid toml :::"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(path); err == nil {
		t.Error("Load() expected error for invalid TOML, got nil")
	}
}

// A key the config does not know — a typo, or a setting from another tool —
// is refused with its name rather than silently ignored, so a setting that
// seems to do nothing cannot be one that was never read.
func TestLoadRefusesUnknownKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := "[retrieval]\ntop_k = 5\nquery_rewirte = true\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(path)
	if err == nil || !strings.Contains(err.Error(), "retrieval.query_rewirte") {
		t.Fatalf("Load = %v, want an error naming retrieval.query_rewirte", err)
	}
}

func TestEnvOverride_Models(t *testing.T) {
	t.Setenv("LITE_RAG_EMBEDDING_MODEL", "my-embed-model")
	t.Setenv("LITE_RAG_CHAT_MODEL", "my-chat-model")

	cfg, err := config.Load("/nonexistent/config.toml")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Models.Embedding != "my-embed-model" {
		t.Errorf("Embedding = %q, want env override", cfg.Models.Embedding)
	}
	if cfg.Models.Chat != "my-chat-model" {
		t.Errorf("Chat = %q, want env override", cfg.Models.Chat)
	}
}
