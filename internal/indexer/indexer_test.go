package indexer_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"lite-rag/internal/config"
	"lite-rag/internal/database"
	"lite-rag/internal/indexer"
)

// ── Test double ────────────────────────────────────────────────────────────

// mockEmbedder returns a fixed-dimension zero vector for every input.
type mockEmbedder struct {
	dim   int
	calls int
	err   error // if non-nil, returned on every Embed call
}

func (m *mockEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if m.err != nil {
		return nil, m.err
	}
	m.calls++
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = make([]float32, m.dim)
	}
	return out, nil
}

// ── Helpers ────────────────────────────────────────────────────────────────

func testCfg() config.RetrievalConfig {
	return config.RetrievalConfig{
		ChunkSize: 512, ChunkOverlap: 64,
	}
}

func openDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open("")
	if err != nil {
		t.Fatalf("Open DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
	return path
}

// ── Tests ──────────────────────────────────────────────────────────────────

func TestIndexDir_IndexesMarkdownFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "# Hello\n\nWorld content here.")
	writeFile(t, dir, "b.md", "# Second\n\nAnother document.")
	writeFile(t, dir, "skip.txt", "not a markdown file")

	db := openDB(t)
	emb := &mockEmbedder{dim: 4}
	idx := indexer.New(db, emb, "test-model", testCfg())

	if err := idx.IndexDir(context.Background(), dir); err != nil {
		t.Fatalf("IndexDir: %v", err)
	}

	// Both .md files should be indexed.
	for _, name := range []string{"a.md", "b.md"} {
		id, hash, _, err := db.FindDocumentByPath(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("FindDocumentByPath %s: %v", name, err)
		}
		if id == "" || hash == "" {
			t.Errorf("%s: not found in DB", name)
		}
	}

	// .txt file must not have been indexed.
	id, _, _, _ := db.FindDocumentByPath(filepath.Join(dir, "skip.txt"))
	if id != "" {
		t.Error("skip.txt should not be indexed")
	}

	// Embed must have been called.
	if emb.calls == 0 {
		t.Error("Embed was never called")
	}
}

func TestIndexDir_SkipsUnchangedFiles(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "doc.md", "# Static\n\nContent that does not change.")

	db := openDB(t)
	emb := &mockEmbedder{dim: 4}
	idx := indexer.New(db, emb, "test-model", testCfg())

	// First run.
	if err := idx.IndexDir(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	callsAfterFirst := emb.calls

	// Second run — content unchanged, Embed must not be called again.
	if err := idx.IndexDir(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if emb.calls != callsAfterFirst {
		t.Errorf("Embed called %d extra time(s) on unchanged file", emb.calls-callsAfterFirst)
	}
	_ = path
}

func TestIndexDir_ReindexesWhenModelChanges(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "# Static\n\nContent that does not change.")

	db := openDB(t)
	emb := &mockEmbedder{dim: 4}

	// Index with model-v1.
	idx1 := indexer.New(db, emb, "model-v1", testCfg())
	if err := idx1.IndexDir(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	callsAfterFirst := emb.calls

	// Re-index with model-v2 — same content but different model must trigger re-index.
	idx2 := indexer.New(db, emb, "model-v2", testCfg())
	if err := idx2.IndexDir(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if emb.calls == callsAfterFirst {
		t.Error("Embed should be called again when embedding model changes")
	}
}

func TestIndexDir_ReindexesChangedFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "doc.md", "# Original\n\nFirst version.")

	db := openDB(t)
	emb := &mockEmbedder{dim: 4}
	idx := indexer.New(db, emb, "test-model", testCfg())

	if err := idx.IndexDir(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	_, hashBefore, _, _ := db.FindDocumentByPath(path)

	// Overwrite with new content.
	writeFile(t, dir, "doc.md", "# Updated\n\nSecond version with more content.")

	if err := idx.IndexDir(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	_, hashAfter, _, _ := db.FindDocumentByPath(path)

	if hashBefore == hashAfter {
		t.Error("file hash should have changed after reindex")
	}

	// Only one document row must exist.
	hits, err := db.SimilarChunks([]float32{1, 0, 0, 0}, 100, "test-model")
	if err != nil {
		t.Fatal(err)
	}
	docIDs := map[string]struct{}{}
	for _, c := range hits {
		docIDs[c.DocumentID] = struct{}{}
	}
	if len(docIDs) != 1 {
		t.Errorf("expected 1 document after reindex, got %d", len(docIDs))
	}
}

func TestIndexDir_EmbedErrorIsPerFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "# A\n\nOK content.")
	writeFile(t, dir, "b.md", "# B\n\nAlso OK.")

	db := openDB(t)

	// Embed always errors — IndexDir should not propagate the error.
	emb := &mockEmbedder{dim: 4, err: fmt.Errorf("embed service down")}
	idx := indexer.New(db, emb, "test-model", testCfg())

	if err := idx.IndexDir(context.Background(), dir); err != nil {
		t.Errorf("IndexDir should not return error for per-file failures, got: %v", err)
	}
}

func TestIndexDir_DBErrorIsPerFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "# Test\n\nContent.")

	db := openDB(t)
	db.Close() // close before indexing to trigger FindDocumentByPath error

	emb := &mockEmbedder{dim: 4}
	idx := indexer.New(db, emb, "test-model", testCfg())

	// Per-file DB errors must not propagate from IndexDir.
	if err := idx.IndexDir(context.Background(), dir); err != nil {
		t.Errorf("IndexDir should not return error for per-file DB failures, got: %v", err)
	}
	// Embed must not have been called (failure happens before embedding).
	if emb.calls != 0 {
		t.Errorf("Embed should not be called when DB lookup fails, got %d call(s)", emb.calls)
	}
}

func TestIndexDir_NonexistentDir(t *testing.T) {
	db := openDB(t)
	emb := &mockEmbedder{dim: 4}
	idx := indexer.New(db, emb, "test-model", testCfg())

	err := idx.IndexDir(context.Background(), "/nonexistent/directory")
	if err == nil {
		t.Error("expected error for nonexistent directory, got nil")
	}
}

// ── IndexFile tests ────────────────────────────────────────────────────────

func TestIndexFile_IndexesSingleFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "single.md", "# Single\n\nThis is a single file.")

	db := openDB(t)
	emb := &mockEmbedder{dim: 4}
	idx := indexer.New(db, emb, "test-model", testCfg())

	if err := idx.IndexFile(context.Background(), path); err != nil {
		t.Fatalf("IndexFile: %v", err)
	}

	id, hash, _, err := db.FindDocumentByPath(path)
	if err != nil {
		t.Fatalf("FindDocumentByPath: %v", err)
	}
	if id == "" || hash == "" {
		t.Error("file not found in DB after IndexFile")
	}
	if emb.calls == 0 {
		t.Error("Embed was never called")
	}
}

func TestIndexFile_NonMarkdownExtension(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "notes.txt", "# Notes\n\nPlain text file indexed directly.")

	db := openDB(t)
	emb := &mockEmbedder{dim: 4}
	idx := indexer.New(db, emb, "test-model", testCfg())

	// IndexFile must accept any extension.
	if err := idx.IndexFile(context.Background(), path); err != nil {
		t.Fatalf("IndexFile on .txt: %v", err)
	}

	id, _, _, _ := db.FindDocumentByPath(path)
	if id == "" {
		t.Error("non-.md file not found in DB")
	}
}

func TestIndexFile_SkipsUnchangedFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "doc.md", "# Stable\n\nContent.")

	db := openDB(t)
	emb := &mockEmbedder{dim: 4}
	idx := indexer.New(db, emb, "test-model", testCfg())

	if err := idx.IndexFile(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	callsAfterFirst := emb.calls

	if err := idx.IndexFile(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	if emb.calls != callsAfterFirst {
		t.Errorf("Embed called %d extra time(s) on unchanged file", emb.calls-callsAfterFirst)
	}
}

// ── Reindex tests ─────────────────────────────────────────────────────────

func TestReindex_ReembedsStaleDocs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "# Alpha\n\nContent A.")
	writeFile(t, dir, "b.md", "# Beta\n\nContent B.")

	db := openDB(t)
	emb := &mockEmbedder{dim: 4}

	// Index both files with model-v1.
	idx1 := indexer.New(db, emb, "model-v1", testCfg())
	if err := idx1.IndexDir(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	callsAfterV1 := emb.calls

	// Reindex with model-v2 — should re-embed both docs using stored text.
	idx2 := indexer.New(db, emb, "model-v2", testCfg())
	stats, err := idx2.Reindex(context.Background())
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	if stats.Reindexed != 2 {
		t.Errorf("Reindexed = %d, want 2", stats.Reindexed)
	}
	if stats.Errors != 0 {
		t.Errorf("Errors = %d, want 0", stats.Errors)
	}
	if emb.calls <= callsAfterV1 {
		t.Error("Embed should have been called again during Reindex")
	}
}

func TestReindex_NoStaleDocsIsNoop(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "# Doc\n\nContent.")

	db := openDB(t)
	emb := &mockEmbedder{dim: 4}
	idx := indexer.New(db, emb, "model-v1", testCfg())

	if err := idx.IndexDir(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	callsBefore := emb.calls

	// Reindex with same model — nothing stale, Embed must not be called.
	stats, err := idx.Reindex(context.Background())
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	if stats.Reindexed != 0 {
		t.Errorf("Reindexed = %d, want 0", stats.Reindexed)
	}
	if emb.calls != callsBefore {
		t.Errorf("Embed called %d extra time(s) when no stale docs", emb.calls-callsBefore)
	}
}

func TestReindex_WorksWithoutSourceFile(t *testing.T) {
	// Index a file, then delete it from disk, then reindex — must succeed using DB text.
	dir := t.TempDir()
	path := writeFile(t, dir, "tmp.md", "# Temp\n\nContent.")

	db := openDB(t)
	emb := &mockEmbedder{dim: 4}

	idx1 := indexer.New(db, emb, "model-v1", testCfg())
	if err := idx1.IndexFile(context.Background(), path); err != nil {
		t.Fatal(err)
	}

	// Remove the source file.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	// Reindex with model-v2 — should work even though file is gone.
	idx2 := indexer.New(db, emb, "model-v2", testCfg())
	stats, err := idx2.Reindex(context.Background())
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	if stats.Reindexed != 1 {
		t.Errorf("Reindexed = %d, want 1", stats.Reindexed)
	}
}

func TestIndexFile_ReturnsErrorDirectly(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "doc.md", "# Test\n\nContent.")

	db := openDB(t)
	db.Close() // force DB error

	emb := &mockEmbedder{dim: 4}
	idx := indexer.New(db, emb, "test-model", testCfg())

	// Unlike IndexDir, IndexFile must return the error directly.
	if err := idx.IndexFile(context.Background(), path); err == nil {
		t.Error("expected error when DB is closed, got nil")
	}
}
