package llm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lite-rag/internal/llm"
)

// failWriter is an io.Writer that always returns an error.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, fmt.Errorf("write failed") }

// ── Embed ──────────────────────────────────────────────────────────────────

func TestEmbed_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing or wrong Authorization header")
		}

		var req struct {
			Input []string `json:"input"`
			Model string   `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(req.Input) != 2 {
			t.Errorf("expected 2 inputs, got %d", len(req.Input))
		}

		// Return 2 mock vectors (dim=4 for brevity).
		resp := map[string]any{
			"data": []map[string]any{
				{"index": 0, "embedding": []float32{0.1, 0.2, 0.3, 0.4}},
				{"index": 1, "embedding": []float32{0.5, 0.6, 0.7, 0.8}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := llm.New(srv.URL, "test-key", "embed-model", "chat-model")
	vecs, err := c.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(vecs) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vecs))
	}
	if vecs[0][0] != 0.1 {
		t.Errorf("vecs[0][0] = %v, want 0.1", vecs[0][0])
	}
	if vecs[1][0] != 0.5 {
		t.Errorf("vecs[1][0] = %v, want 0.5", vecs[1][0])
	}
}

func TestEmbed_OutOfOrderIndex(t *testing.T) {
	// Server returns embeddings in reversed order; client must re-sort.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": []map[string]any{
				{"index": 1, "embedding": []float32{0.9}},
				{"index": 0, "embedding": []float32{0.1}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := llm.New(srv.URL, "", "m", "m")
	vecs, err := c.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if vecs[0][0] != 0.1 || vecs[1][0] != 0.9 {
		t.Errorf("wrong order: %v", vecs)
	}
}

func TestEmbed_NegativeIndex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": []map[string]any{
				{"index": -1, "embedding": []float32{0.1}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := llm.New(srv.URL, "", "m", "m")
	_, err := c.Embed(context.Background(), []string{"x"})
	if err == nil {
		t.Error("expected error for negative index, got nil")
	}
}

func TestEmbed_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := llm.New(srv.URL, "", "m", "m")
	_, err := c.Embed(context.Background(), []string{"x"})
	if err == nil {
		t.Error("expected error for 500 response, got nil")
	}
}

func TestEmbed_InvalidBaseURL(t *testing.T) {
	// A URL with a space makes http.NewRequestWithContext fail.
	c := llm.New("http://invalid host:9999", "", "m", "m")
	_, err := c.Embed(context.Background(), []string{"x"})
	if err == nil {
		t.Error("expected error for invalid base URL, got nil")
	}
}

func TestEmbed_InvalidJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	c := llm.New(srv.URL, "", "m", "m")
	_, err := c.Embed(context.Background(), []string{"x"})
	if err == nil {
		t.Error("expected error for invalid JSON response, got nil")
	}
}

func TestEmbed_MissingEmbeddingInResponse(t *testing.T) {
	// Server returns only 1 embedding for a 2-text request.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": []map[string]any{
				{"index": 0, "embedding": []float32{0.1}},
				// index 1 is absent
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := llm.New(srv.URL, "", "m", "m")
	_, err := c.Embed(context.Background(), []string{"a", "b"})
	if err == nil {
		t.Error("expected error for missing embedding, got nil")
	}
}

// ── Chat (SSE streaming) ───────────────────────────────────────────────────

func TestChat_Streaming(t *testing.T) {
	sse := `data: {"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}
data: {"choices":[{"delta":{"content":", "},"finish_reason":null}]}
data: {"choices":[{"delta":{"content":"world"},"finish_reason":null}]}
data: [DONE]
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var req struct {
			Stream bool `json:"stream"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if !req.Stream {
			t.Error("stream flag not set")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(sse))
	}))
	defer srv.Close()

	c := llm.New(srv.URL, "k", "e", "chat-model")
	var buf strings.Builder
	err := c.Chat(context.Background(), []llm.Message{
		{Role: "user", Content: "say hello"},
	}, &buf)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if buf.String() != "Hello, world" {
		t.Errorf("Chat() output = %q, want %q", buf.String(), "Hello, world")
	}
}

func TestChat_SSEWithKeepAliveComments(t *testing.T) {
	// Lines not starting with "data: " must be silently ignored.
	sse := `: keep-alive
data: {"choices":[{"delta":{"content":"ok"},"finish_reason":null}]}
data: [DONE]
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(sse))
	}))
	defer srv.Close()

	c := llm.New(srv.URL, "", "e", "c")
	var buf strings.Builder
	if err := c.Chat(context.Background(), nil, &buf); err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if buf.String() != "ok" {
		t.Errorf("got %q, want %q", buf.String(), "ok")
	}
}

func TestChat_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()

	c := llm.New(srv.URL, "", "e", "c")
	err := c.Chat(context.Background(), nil, &strings.Builder{})
	if err == nil {
		t.Error("expected error for 400 response, got nil")
	}
}

func TestChat_WriteError(t *testing.T) {
	// Writer that always fails; Chat must propagate the error.
	sse := `data: {"choices":[{"delta":{"content":"hi"},"finish_reason":null}]}
data: [DONE]
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(sse))
	}))
	defer srv.Close()

	c := llm.New(srv.URL, "", "e", "c")
	err := c.Chat(context.Background(), nil, failWriter{})
	if err == nil {
		t.Error("expected error when writer fails, got nil")
	}
}

// ── RewriteQuery ────────────────────────────────────────────────────────────

func TestRewriteQuery_ReturnsJAAndENVariants(t *testing.T) {
	// Server returns the expected two-line JA/EN format (newline must be JSON-escaped).
	// Two SSE chunks: line 1 (JA), then line 2 (EN).
	sse := "data: {\"choices\":[{\"delta\":{\"content\":\"JA: \\u65e5\\u672c\\u8a9e\\u306e\\u30af\\u30a8\\u30ea\\nEN: English query\"},\"finish_reason\":null}]}\ndata: [DONE]\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(sse)) //nolint:errcheck
	}))
	defer srv.Close()

	c := llm.New(srv.URL, "", "e", "chat-model")
	variants, err := c.RewriteQuery(context.Background(), "original question?")
	if err != nil {
		t.Fatalf("RewriteQuery: %v", err)
	}
	if len(variants) != 2 {
		t.Fatalf("expected 2 variants (JA+EN), got %d: %v", len(variants), variants)
	}
	if variants[0] != "日本語のクエリ" {
		t.Errorf("JA variant = %q, want %q", variants[0], "日本語のクエリ")
	}
	if variants[1] != "English query" {
		t.Errorf("EN variant = %q, want %q", variants[1], "English query")
	}
}

func TestRewriteQuery_FallsBackToSingleVariant(t *testing.T) {
	// Server returns a single-language string (no JA:/EN: format) — fallback path.
	sse := `data: {"choices":[{"delta":{"content":"rewritten query"},"finish_reason":null}]}` + "\ndata: [DONE]\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(sse)) //nolint:errcheck
	}))
	defer srv.Close()

	c := llm.New(srv.URL, "", "e", "chat-model")
	variants, err := c.RewriteQuery(context.Background(), "original question?")
	if err != nil {
		t.Fatalf("RewriteQuery: %v", err)
	}
	if len(variants) != 1 {
		t.Fatalf("expected 1 fallback variant, got %d: %v", len(variants), variants)
	}
	if variants[0] != "rewritten query" {
		t.Errorf("variant = %q, want %q", variants[0], "rewritten query")
	}
}

func TestRewriteQuery_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := llm.New(srv.URL, "", "e", "chat-model")
	_, err := c.RewriteQuery(context.Background(), "query")
	if err == nil {
		t.Error("expected error on server 500, got nil")
	}
}

func TestChat_MalformedSSELineSkipped(t *testing.T) {
	// A malformed JSON line must not abort the stream.
	sse := `data: not-json
data: {"choices":[{"delta":{"content":"fine"},"finish_reason":null}]}
data: [DONE]
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(sse))
	}))
	defer srv.Close()

	c := llm.New(srv.URL, "", "e", "c")
	var buf strings.Builder
	if err := c.Chat(context.Background(), nil, &buf); err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if buf.String() != "fine" {
		t.Errorf("got %q, want %q", buf.String(), "fine")
	}
}
