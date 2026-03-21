package retriever_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"lite-rag/internal/config"
	"lite-rag/internal/database"
	"lite-rag/internal/retriever"
)

// ── Test doubles ────────────────────────────────────────────────────────────

type mockEmbedder struct {
	vec []float32
	err error
}

func (m *mockEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = m.vec
	}
	return out, nil
}

type mockRewriter struct {
	results []string
	err     error
}

func (m *mockRewriter) RewriteQuery(_ context.Context, _ string) ([]string, error) {
	return m.results, m.err
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func openDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open("")
	if err != nil {
		t.Fatalf("Open DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func insertDoc(t *testing.T, db *database.DB, docID, filePath string, chunks []database.ChunkRow) {
	t.Helper()
	doc := database.DocumentRow{
		ID: docID, FilePath: filePath, FileHash: "h",
		TotalChunks: len(chunks), IndexedAt: time.Now().UTC(),
		EmbeddingModel: "test-model",
	}
	if err := db.ReplaceDocument(doc, chunks); err != nil {
		t.Fatalf("ReplaceDocument: %v", err)
	}
}

func cfg(topK, window int) config.RetrievalConfig {
	return config.RetrievalConfig{TopK: topK, ContextWindow: window, ChunkSize: 512, ChunkOverlap: 64}
}

// ── Tests ────────────────────────────────────────────────────────────────────

func TestRetrieve_ReturnsTopPassage(t *testing.T) {
	db := openDB(t)
	insertDoc(t, db, "d1", "/doc1.md", []database.ChunkRow{
		{ID: "c0", DocumentID: "d1", ChunkIndex: 0, Content: "alpha", Embedding: []float32{1, 0, 0}},
		{ID: "c1", DocumentID: "d1", ChunkIndex: 1, Content: "beta", Embedding: []float32{0, 1, 0}},
		{ID: "c2", DocumentID: "d1", ChunkIndex: 2, Content: "gamma", Embedding: []float32{0, 0, 1}},
	})

	// Query vector matches chunk 0 (alpha).
	emb := &mockEmbedder{vec: []float32{1, 0, 0}}
	ret := retriever.New(db, emb, nil, "test-model", cfg(1, 0))

	passages, err := ret.Retrieve(context.Background(), "query")
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(passages) != 1 {
		t.Fatalf("expected 1 passage, got %d", len(passages))
	}
	if passages[0].Content != "alpha" {
		t.Errorf("content = %q, want 'alpha'", passages[0].Content)
	}
	if passages[0].Score < 0.99 {
		t.Errorf("score = %v, want ~1.0", passages[0].Score)
	}
}

func TestRetrieve_ContextWindowExpandsAdjacent(t *testing.T) {
	db := openDB(t)
	insertDoc(t, db, "d1", "/doc1.md", []database.ChunkRow{
		{ID: "c0", DocumentID: "d1", ChunkIndex: 0, Content: "before", Embedding: []float32{0, 1, 0}},
		{ID: "c1", DocumentID: "d1", ChunkIndex: 1, Content: "hit", Embedding: []float32{1, 0, 0}},
		{ID: "c2", DocumentID: "d1", ChunkIndex: 2, Content: "after", Embedding: []float32{0, 0, 1}},
	})

	// Query matches chunk 1 (hit); window=1 should pull in chunks 0 and 2.
	emb := &mockEmbedder{vec: []float32{1, 0, 0}}
	ret := retriever.New(db, emb, nil, "test-model", cfg(1, 1))

	passages, err := ret.Retrieve(context.Background(), "query")
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(passages) != 1 {
		t.Fatalf("expected 1 passage, got %d", len(passages))
	}
	for _, want := range []string{"before", "hit", "after"} {
		if !strings.Contains(passages[0].Content, want) {
			t.Errorf("passage content should contain %q, got: %q", want, passages[0].Content)
		}
	}
}

func TestRetrieve_DeduplicatesOverlappingWindows(t *testing.T) {
	db := openDB(t)
	// Chunks 0–4; hits on indices 1 and 3 with window=1 → spans [0,2] and [2,4] → merged [0,4].
	embeddings := [][]float32{
		{0, 0, 0, 1},
		{1, 0, 0, 0}, // hit
		{0, 0, 0, 1},
		{1, 0, 0, 0}, // hit
		{0, 0, 0, 1},
	}
	contents := []string{"c0", "c1", "c2", "c3", "c4"}
	chunks := make([]database.ChunkRow, 5)
	for i := 0; i < 5; i++ {
		chunks[i] = database.ChunkRow{
			ID: fmt.Sprintf("ch%d", i), DocumentID: "d1", ChunkIndex: i,
			Content: contents[i], Embedding: embeddings[i],
		}
	}
	insertDoc(t, db, "d1", "/doc1.md", chunks)

	emb := &mockEmbedder{vec: []float32{1, 0, 0, 0}}
	ret := retriever.New(db, emb, nil, "test-model", cfg(2, 1))

	passages, err := ret.Retrieve(context.Background(), "q")
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	// Overlapping windows should be merged into a single passage.
	if len(passages) != 1 {
		t.Fatalf("expected 1 merged passage, got %d", len(passages))
	}
	for _, want := range contents {
		if !strings.Contains(passages[0].Content, want) {
			t.Errorf("merged passage missing %q", want)
		}
	}
}

func TestRetrieve_EmptyDB(t *testing.T) {
	db := openDB(t)
	emb := &mockEmbedder{vec: []float32{1, 0}}
	ret := retriever.New(db, emb, nil, "test-model", cfg(5, 1))

	passages, err := ret.Retrieve(context.Background(), "anything")
	if err != nil {
		t.Fatalf("Retrieve on empty DB: %v", err)
	}
	if len(passages) != 0 {
		t.Errorf("expected 0 passages on empty DB, got %d", len(passages))
	}
}

func TestRetrieve_MultipleDocuments(t *testing.T) {
	db := openDB(t)
	insertDoc(t, db, "d1", "/doc1.md", []database.ChunkRow{
		{ID: "d1c0", DocumentID: "d1", ChunkIndex: 0, Content: "doc1 chunk", Embedding: []float32{1, 0}},
	})
	insertDoc(t, db, "d2", "/doc2.md", []database.ChunkRow{
		{ID: "d2c0", DocumentID: "d2", ChunkIndex: 0, Content: "doc2 chunk", Embedding: []float32{0, 1}},
	})

	// Query matches d1 more than d2.
	emb := &mockEmbedder{vec: []float32{1, 0}}
	ret := retriever.New(db, emb, nil, "test-model", cfg(2, 0))

	passages, err := ret.Retrieve(context.Background(), "q")
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(passages) != 2 {
		t.Fatalf("expected 2 passages (one per doc), got %d", len(passages))
	}
	// Highest score should be first.
	if passages[0].Score < passages[1].Score {
		t.Error("passages not sorted by score descending")
	}
}

func TestRetrieve_EmbedError(t *testing.T) {
	db := openDB(t)
	emb := &mockEmbedder{err: fmt.Errorf("embed service down")}
	ret := retriever.New(db, emb, nil, "test-model", cfg(5, 1))

	_, err := ret.Retrieve(context.Background(), "query")
	if err == nil {
		t.Error("expected error when embed fails, got nil")
	}
}

func TestRetrieve_WindowClampsAtDocumentStart(t *testing.T) {
	db := openDB(t)
	insertDoc(t, db, "d1", "/doc1.md", []database.ChunkRow{
		{ID: "c0", DocumentID: "d1", ChunkIndex: 0, Content: "first", Embedding: []float32{1, 0}},
		{ID: "c1", DocumentID: "d1", ChunkIndex: 1, Content: "second", Embedding: []float32{0, 1}},
	})

	// Hit is chunk 0; window=2 should not panic or return negative indices.
	emb := &mockEmbedder{vec: []float32{1, 0}}
	ret := retriever.New(db, emb, nil, "test-model", cfg(1, 2))

	passages, err := ret.Retrieve(context.Background(), "q")
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(passages) == 0 {
		t.Fatal("expected at least one passage")
	}
	if !strings.Contains(passages[0].Content, "first") {
		t.Errorf("content should contain 'first', got: %q", passages[0].Content)
	}
}

// ── Query Rewriting ──────────────────────────────────────────────────────────

func TestRetrieve_QueryRewriteAddsExtraHits(t *testing.T) {
	db := openDB(t)
	// Two documents with orthogonal embeddings.
	insertDoc(t, db, "d1", "/doc1.md", []database.ChunkRow{
		{ID: "c0", DocumentID: "d1", ChunkIndex: 0, Content: "original hit", Embedding: []float32{1, 0}},
	})
	insertDoc(t, db, "d2", "/doc2.md", []database.ChunkRow{
		{ID: "c1", DocumentID: "d2", ChunkIndex: 0, Content: "rewrite hit", Embedding: []float32{0, 1}},
	})

	// Embedder alternates: first call (original query) → matches d1,
	// second call (rewritten query) → matches d2.
	var mu sync.Mutex
	callCount := 0
	emb := &funcEmbedder{fn: func(_ context.Context, _ []string) ([][]float32, error) {
		mu.Lock()
		n := callCount
		callCount++
		mu.Unlock()
		if n == 0 {
			return [][]float32{{1, 0}}, nil // original: matches c0
		}
		return [][]float32{{0, 1}}, nil // rewritten: matches c1
	}}

	rw := &mockRewriter{results: []string{"rewritten query text"}}
	ret := retriever.New(db, emb, rw, "test-model", cfg(1, 0))

	passages, err := ret.Retrieve(context.Background(), "q")
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	// Both documents should be represented after merging.
	if len(passages) != 2 {
		t.Fatalf("expected 2 passages (one from each search), got %d", len(passages))
	}
}

func TestRetrieve_QueryRewriteMultipleVariants(t *testing.T) {
	db := openDB(t)
	insertDoc(t, db, "d1", "/doc1.md", []database.ChunkRow{
		{ID: "c0", DocumentID: "d1", ChunkIndex: 0, Content: "original hit", Embedding: []float32{1, 0, 0}},
	})
	insertDoc(t, db, "d2", "/doc2.md", []database.ChunkRow{
		{ID: "c1", DocumentID: "d2", ChunkIndex: 0, Content: "japanese hit", Embedding: []float32{0, 1, 0}},
	})
	insertDoc(t, db, "d3", "/doc3.md", []database.ChunkRow{
		{ID: "c2", DocumentID: "d3", ChunkIndex: 0, Content: "english hit", Embedding: []float32{0, 0, 1}},
	})

	// Embedder maps call index → distinct vector per query type.
	var mu sync.Mutex
	callCount := 0
	emb := &funcEmbedder{fn: func(_ context.Context, _ []string) ([][]float32, error) {
		mu.Lock()
		n := callCount
		callCount++
		mu.Unlock()
		switch n {
		case 0:
			return [][]float32{{1, 0, 0}}, nil // original → d1
		case 1:
			return [][]float32{{0, 1, 0}}, nil // JA variant → d2
		default:
			return [][]float32{{0, 0, 1}}, nil // EN variant → d3
		}
	}}

	// Two variants: JA and EN.
	rw := &mockRewriter{results: []string{"日本語リライト", "English rewrite"}}
	ret := retriever.New(db, emb, rw, "test-model", cfg(3, 0))

	passages, err := ret.Retrieve(context.Background(), "q")
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	// All three documents should appear after merging original + JA + EN searches.
	if len(passages) != 3 {
		t.Fatalf("expected 3 passages (original + JA variant + EN variant), got %d", len(passages))
	}
}

func TestRetrieve_QueryRewriteErrorFallsBackToOriginal(t *testing.T) {
	db := openDB(t)
	insertDoc(t, db, "d1", "/doc1.md", []database.ChunkRow{
		{ID: "c0", DocumentID: "d1", ChunkIndex: 0, Content: "found", Embedding: []float32{1, 0}},
	})

	emb := &mockEmbedder{vec: []float32{1, 0}}
	rw := &mockRewriter{err: fmt.Errorf("LLM unavailable")}
	ret := retriever.New(db, emb, rw, "test-model", cfg(1, 0))

	passages, err := ret.Retrieve(context.Background(), "q")
	if err != nil {
		t.Fatalf("Retrieve should not fail when rewrite errors: %v", err)
	}
	if len(passages) != 1 || passages[0].Content != "found" {
		t.Errorf("expected original hit to be returned, got %v", passages)
	}
}

// funcEmbedder allows per-call control over embedding results.
type funcEmbedder struct {
	fn func(context.Context, []string) ([][]float32, error)
}

func (f *funcEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return f.fn(ctx, texts)
}
