package database_test

import (
	"fmt"
	"math"
	"testing"
	"time"

	"lite-rag/internal/database"
)

// ── Helpers ────────────────────────────────────────────────────────────────

func newTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// ── FindDocumentByPath ─────────────────────────────────────────────────────

func TestFindDocumentByPath_NotFound(t *testing.T) {
	db := newTestDB(t)
	id, hash, _, err := db.FindDocumentByPath("/no/such/file.md")
	if err != nil {
		t.Fatalf("FindDocumentByPath: %v", err)
	}
	if id != "" || hash != "" {
		t.Errorf("expected empty id/hash, got id=%q hash=%q", id, hash)
	}
}

// ── ReplaceDocument ────────────────────────────────────────────────────────

func TestReplaceDocument_InsertAndFind(t *testing.T) {
	db := newTestDB(t)
	doc := database.DocumentRow{
		ID: "doc1", FilePath: "/docs/a.md", FileHash: "hash1",
		TotalChunks: 2, IndexedAt: time.Now().UTC(),
	}
	chunks := []database.ChunkRow{
		{ID: "c0", DocumentID: "doc1", ChunkIndex: 0, Content: "chunk zero",
			Embedding: []float32{1, 0, 0, 0}},
		{ID: "c1", DocumentID: "doc1", ChunkIndex: 1, Content: "chunk one",
			Embedding: []float32{0, 1, 0, 0}},
	}
	if err := db.ReplaceDocument(doc, chunks); err != nil {
		t.Fatalf("ReplaceDocument: %v", err)
	}

	id, hash, _, err := db.FindDocumentByPath("/docs/a.md")
	if err != nil {
		t.Fatalf("FindDocumentByPath: %v", err)
	}
	if id != "doc1" || hash != "hash1" {
		t.Errorf("got id=%q hash=%q", id, hash)
	}
}

func TestReplaceDocument_UpdateReplacesChunks(t *testing.T) {
	db := newTestDB(t)

	insert := func(docID, hash, content string) {
		doc := database.DocumentRow{
			ID: docID, FilePath: "/docs/a.md", FileHash: hash,
			TotalChunks: 1, IndexedAt: time.Now().UTC(),
		}
		ch := []database.ChunkRow{{
			ID: docID + "_c0", DocumentID: docID,
			ChunkIndex: 0, Content: content,
			Embedding: []float32{1, 0},
		}}
		if err := db.ReplaceDocument(doc, ch); err != nil {
			t.Fatalf("ReplaceDocument: %v", err)
		}
	}

	insert("doc1", "hash1", "original content")
	// Same file_path, different document ID and hash — simulates a file update.
	insert("doc2", "hash2", "updated content")

	hits, err := db.SimilarChunks([]float32{1, 0}, 10, "")
	if err != nil {
		t.Fatalf("SimilarChunks: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 chunk after update, got %d", len(hits))
	}
	if hits[0].Content != "updated content" {
		t.Errorf("content = %q, want 'updated content'", hits[0].Content)
	}
}

// ── SimilarChunks ──────────────────────────────────────────────────────────

func TestSimilarChunks_TopK(t *testing.T) {
	db := newTestDB(t)

	// Insert 3 chunks with orthogonal embeddings.
	doc := database.DocumentRow{
		ID: "d1", FilePath: "/f.md", FileHash: "h",
		TotalChunks: 3, IndexedAt: time.Now().UTC(),
	}
	chunks := []database.ChunkRow{
		{ID: "c0", DocumentID: "d1", ChunkIndex: 0, Content: "A", Embedding: []float32{1, 0, 0}},
		{ID: "c1", DocumentID: "d1", ChunkIndex: 1, Content: "B", Embedding: []float32{0, 1, 0}},
		{ID: "c2", DocumentID: "d1", ChunkIndex: 2, Content: "C", Embedding: []float32{0, 0, 1}},
	}
	if err := db.ReplaceDocument(doc, chunks); err != nil {
		t.Fatal(err)
	}

	// Query [1,0,0] — chunk A should rank first.
	hits, err := db.SimilarChunks([]float32{1, 0, 0}, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits (top_k=2), got %d", len(hits))
	}
	if hits[0].Content != "A" {
		t.Errorf("top hit = %q, want A", hits[0].Content)
	}
	if math.Abs(float64(hits[0].Score-1.0)) > 1e-5 {
		t.Errorf("top score = %v, want ~1.0", hits[0].Score)
	}
}

func TestSimilarChunks_SkipsNullEmbedding(t *testing.T) {
	db := newTestDB(t)
	doc := database.DocumentRow{
		ID: "d1", FilePath: "/f.md", FileHash: "h",
		TotalChunks: 2, IndexedAt: time.Now().UTC(),
	}
	chunks := []database.ChunkRow{
		{ID: "c0", DocumentID: "d1", ChunkIndex: 0, Content: "has embedding",
			Embedding: []float32{1, 0}},
		{ID: "c1", DocumentID: "d1", ChunkIndex: 1, Content: "no embedding",
			Embedding: nil},
	}
	if err := db.ReplaceDocument(doc, chunks); err != nil {
		t.Fatal(err)
	}

	hits, err := db.SimilarChunks([]float32{1, 0}, 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit (null embedding skipped), got %d", len(hits))
	}
}

// ── Error paths (closed DB) ────────────────────────────────────────────────

func TestReplaceDocument_ClosedDB(t *testing.T) {
	db := newTestDB(t)
	db.Close() // close before use

	doc := database.DocumentRow{ID: "d1", FilePath: "/f.md", FileHash: "h", TotalChunks: 0}
	if err := db.ReplaceDocument(doc, nil); err == nil {
		t.Error("expected error on closed DB, got nil")
	}
}

func TestSimilarChunks_ClosedDB(t *testing.T) {
	db := newTestDB(t)
	db.Close()

	if _, err := db.SimilarChunks([]float32{1, 0}, 5, ""); err == nil {
		t.Error("expected error on closed DB, got nil")
	}
}

func TestAdjacentChunks_ClosedDB(t *testing.T) {
	db := newTestDB(t)
	db.Close()

	if _, err := db.AdjacentChunks("d1", 0, 2); err == nil {
		t.Error("expected error on closed DB, got nil")
	}
}

// ── EmbeddingModel ─────────────────────────────────────────────────────────

func TestFindDocumentByPath_ReturnsEmbeddingModel(t *testing.T) {
	db := newTestDB(t)
	doc := database.DocumentRow{
		ID: "d1", FilePath: "/f.md", FileHash: "h",
		TotalChunks: 0, IndexedAt: time.Now().UTC(),
		EmbeddingModel: "nomic-v1.5",
	}
	if err := db.ReplaceDocument(doc, nil); err != nil {
		t.Fatal(err)
	}

	_, _, model, err := db.FindDocumentByPath("/f.md")
	if err != nil {
		t.Fatalf("FindDocumentByPath: %v", err)
	}
	if model != "nomic-v1.5" {
		t.Errorf("model = %q, want %q", model, "nomic-v1.5")
	}
}

func TestSimilarChunks_FiltersOtherModel(t *testing.T) {
	db := newTestDB(t)

	// Insert two documents with different embedding models.
	docA := database.DocumentRow{
		ID: "dA", FilePath: "/a.md", FileHash: "hA",
		TotalChunks: 1, IndexedAt: time.Now().UTC(),
		EmbeddingModel: "model-alpha",
	}
	docB := database.DocumentRow{
		ID: "dB", FilePath: "/b.md", FileHash: "hB",
		TotalChunks: 1, IndexedAt: time.Now().UTC(),
		EmbeddingModel: "model-beta",
	}
	if err := db.ReplaceDocument(docA, []database.ChunkRow{
		{ID: "cA", DocumentID: "dA", ChunkIndex: 0, Content: "alpha doc", Embedding: []float32{1, 0}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceDocument(docB, []database.ChunkRow{
		{ID: "cB", DocumentID: "dB", ChunkIndex: 0, Content: "beta doc", Embedding: []float32{1, 0}},
	}); err != nil {
		t.Fatal(err)
	}

	// Querying with "model-alpha" must only return the alpha document's chunk.
	hits, err := db.SimilarChunks([]float32{1, 0}, 10, "model-alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit filtered by model, got %d", len(hits))
	}
	if hits[0].Content != "alpha doc" {
		t.Errorf("content = %q, want 'alpha doc'", hits[0].Content)
	}
}

// ── ListDocuments ──────────────────────────────────────────────────────────

func TestListDocuments_Empty(t *testing.T) {
	db := newTestDB(t)
	docs, err := db.ListDocuments()
	if err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}
	if len(docs) != 0 {
		t.Errorf("expected 0 documents, got %d", len(docs))
	}
}

func TestListDocuments_OrderedByFilePath(t *testing.T) {
	db := newTestDB(t)

	for _, fp := range []string{"/c.md", "/a.md", "/b.md"} {
		doc := database.DocumentRow{
			ID: fp, FilePath: fp, FileHash: "h",
			TotalChunks: 1, IndexedAt: time.Now().UTC(),
			EmbeddingModel: "model-x",
		}
		if err := db.ReplaceDocument(doc, nil); err != nil {
			t.Fatalf("ReplaceDocument: %v", err)
		}
	}

	docs, err := db.ListDocuments()
	if err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}
	if len(docs) != 3 {
		t.Fatalf("expected 3 documents, got %d", len(docs))
	}
	want := []string{"/a.md", "/b.md", "/c.md"}
	for i, d := range docs {
		if d.FilePath != want[i] {
			t.Errorf("docs[%d].FilePath = %q, want %q", i, d.FilePath, want[i])
		}
	}
}

func TestListDocuments_FieldsPopulated(t *testing.T) {
	db := newTestDB(t)
	doc := database.DocumentRow{
		ID: "doc-id", FilePath: "/x.md", FileHash: "abc123",
		TotalChunks: 3, IndexedAt: time.Now().UTC(),
		EmbeddingModel: "nomic-v1",
	}
	if err := db.ReplaceDocument(doc, nil); err != nil {
		t.Fatal(err)
	}

	docs, err := db.ListDocuments()
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}
	d := docs[0]
	if d.ID != "doc-id" {
		t.Errorf("ID = %q, want %q", d.ID, "doc-id")
	}
	if d.FilePath != "/x.md" {
		t.Errorf("FilePath = %q, want %q", d.FilePath, "/x.md")
	}
	if d.FileHash != "abc123" {
		t.Errorf("FileHash = %q, want %q", d.FileHash, "abc123")
	}
	if d.TotalChunks != 3 {
		t.Errorf("TotalChunks = %d, want 3", d.TotalChunks)
	}
	if d.EmbeddingModel != "nomic-v1" {
		t.Errorf("EmbeddingModel = %q, want %q", d.EmbeddingModel, "nomic-v1")
	}
	if d.IndexedAt == "" {
		t.Error("IndexedAt is empty")
	}
}

// ── DeleteDocument ─────────────────────────────────────────────────────────

func TestDeleteDocument_RemovesDocumentAndChunks(t *testing.T) {
	db := newTestDB(t)
	doc := database.DocumentRow{
		ID: "d1", FilePath: "/f.md", FileHash: "h",
		TotalChunks: 2, IndexedAt: time.Now().UTC(),
	}
	chunks := []database.ChunkRow{
		{ID: "c0", DocumentID: "d1", ChunkIndex: 0, Content: "alpha", Embedding: []float32{1, 0}},
		{ID: "c1", DocumentID: "d1", ChunkIndex: 1, Content: "beta", Embedding: []float32{0, 1}},
	}
	if err := db.ReplaceDocument(doc, chunks); err != nil {
		t.Fatal(err)
	}

	if err := db.DeleteDocument("d1"); err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}

	// document must be gone
	docs, err := db.ListDocuments()
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 0 {
		t.Errorf("expected 0 documents after delete, got %d", len(docs))
	}

	// chunks must be gone (SimilarChunks returns nothing)
	hits, err := db.SimilarChunks([]float32{1, 0}, 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Errorf("expected 0 chunks after delete, got %d", len(hits))
	}
}

func TestDeleteDocument_NotFound(t *testing.T) {
	db := newTestDB(t)
	if err := db.DeleteDocument("nonexistent-id"); err == nil {
		t.Error("expected error for nonexistent document, got nil")
	}
}

func TestDeleteDocument_OnlyDeletesTargetDocument(t *testing.T) {
	db := newTestDB(t)

	for _, id := range []string{"d1", "d2"} {
		doc := database.DocumentRow{
			ID: id, FilePath: "/" + id + ".md", FileHash: "h",
			TotalChunks: 1, IndexedAt: time.Now().UTC(),
		}
		if err := db.ReplaceDocument(doc, []database.ChunkRow{
			{ID: id + "_c0", DocumentID: id, ChunkIndex: 0, Content: id, Embedding: []float32{1}},
		}); err != nil {
			t.Fatal(err)
		}
	}

	if err := db.DeleteDocument("d1"); err != nil {
		t.Fatal(err)
	}

	docs, err := db.ListDocuments()
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].ID != "d2" {
		t.Errorf("expected only d2 to remain, got %v", docs)
	}
}

// ── DocumentChunks ─────────────────────────────────────────────────────────

func TestDocumentChunks_OrderedByIndex(t *testing.T) {
	db := newTestDB(t)
	doc := database.DocumentRow{
		ID: "d1", FilePath: "/f.md", FileHash: "h",
		TotalChunks: 3, IndexedAt: time.Now().UTC(),
	}
	// Insert in reverse order to verify ORDER BY chunk_index.
	chunks := []database.ChunkRow{
		{ID: "c2", DocumentID: "d1", ChunkIndex: 2, Content: "third"},
		{ID: "c0", DocumentID: "d1", ChunkIndex: 0, Content: "first"},
		{ID: "c1", DocumentID: "d1", ChunkIndex: 1, Content: "second"},
	}
	if err := db.ReplaceDocument(doc, chunks); err != nil {
		t.Fatal(err)
	}

	got, err := db.DocumentChunks("d1")
	if err != nil {
		t.Fatalf("DocumentChunks: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(got))
	}
	want := []string{"first", "second", "third"}
	for i, c := range got {
		if c.Content != want[i] {
			t.Errorf("chunks[%d].Content = %q, want %q", i, c.Content, want[i])
		}
		if c.ChunkIndex != i {
			t.Errorf("chunks[%d].ChunkIndex = %d, want %d", i, c.ChunkIndex, i)
		}
	}
}

func TestDocumentChunks_HeadingPath(t *testing.T) {
	db := newTestDB(t)
	doc := database.DocumentRow{
		ID: "d1", FilePath: "/f.md", FileHash: "h",
		TotalChunks: 2, IndexedAt: time.Now().UTC(),
	}
	chunks := []database.ChunkRow{
		{ID: "c0", DocumentID: "d1", ChunkIndex: 0, HeadingPath: "# Intro", Content: "intro text"},
		{ID: "c1", DocumentID: "d1", ChunkIndex: 1, HeadingPath: "", Content: "no heading"},
	}
	if err := db.ReplaceDocument(doc, chunks); err != nil {
		t.Fatal(err)
	}

	got, err := db.DocumentChunks("d1")
	if err != nil {
		t.Fatal(err)
	}
	if got[0].HeadingPath != "# Intro" {
		t.Errorf("HeadingPath = %q, want %q", got[0].HeadingPath, "# Intro")
	}
	if got[1].HeadingPath != "" {
		t.Errorf("HeadingPath = %q, want empty", got[1].HeadingPath)
	}
}

func TestDocumentChunks_NotFound(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.DocumentChunks("nonexistent-id"); err == nil {
		t.Error("expected error for nonexistent document, got nil")
	}
}

// ── AdjacentChunks ─────────────────────────────────────────────────────────

func TestAdjacentChunks(t *testing.T) {
	db := newTestDB(t)
	doc := database.DocumentRow{
		ID: "d1", FilePath: "/f.md", FileHash: "h",
		TotalChunks: 5, IndexedAt: time.Now().UTC(),
	}
	var chunks []database.ChunkRow
	for i := 0; i < 5; i++ {
		chunks = append(chunks, database.ChunkRow{
			ID: fmt.Sprintf("c%d", i), DocumentID: "d1", ChunkIndex: i,
			Content: fmt.Sprintf("chunk %d", i),
		})
	}
	if err := db.ReplaceDocument(doc, chunks); err != nil {
		t.Fatal(err)
	}

	adj, err := db.AdjacentChunks("d1", 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(adj) != 3 {
		t.Fatalf("expected 3 adjacent chunks, got %d", len(adj))
	}
	for i, c := range adj {
		if c.ChunkIndex != i+1 {
			t.Errorf("adj[%d].ChunkIndex = %d, want %d", i, c.ChunkIndex, i+1)
		}
	}
}
