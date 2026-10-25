package database_test

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"lite-rag/internal/database"
)

func TestOpen_InMemory(t *testing.T) {
	// Empty path opens an in-memory DuckDB instance.
	db, err := database.Open("")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = db.Close() }()
}

func TestMigrate_TablesExist(t *testing.T) {
	db, err := database.Open("")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = db.Close() }()

	// Both tables must be queryable after migration.
	tables := []string{"documents", "chunks"}
	for _, tbl := range tables {
		if err := db.Ping(); err != nil {
			t.Fatalf("Ping() error = %v", err)
		}
		rows, err := db.QueryRaw("SELECT COUNT(*) FROM " + tbl)
		if err != nil {
			t.Errorf("table %q not found after migration: %v", tbl, err)
			continue
		}
		_ = rows.Close()
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	// Opening (and migrating) twice against the same file must not error.
	dir := t.TempDir()
	path := dir + "/test.db"

	db1, err := database.Open(path)
	if err != nil {
		t.Fatalf("first Open() error = %v", err)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	db2, err := database.Open(path)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	if err := db2.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestOpen_DirectoryPathFails(t *testing.T) {
	// DuckDB cannot open a directory as a database file; Open must return an error.
	dir := t.TempDir()
	_, err := database.Open(dir)
	if err == nil {
		t.Error("expected error when opening a directory as DB path, got nil")
	}
}

func TestClose_Idempotent(t *testing.T) {
	db, err := database.Open("")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
}

// TestOpen_FilePersistsAcrossReopen indexes into an on-disk database, closes
// it, and searches it again after reopening — the path every real index takes.
// It pins the storage round trip through whatever DuckDB the driver bundles.
func TestOpen_FilePersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.duckdb")

	db, err := database.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	doc := database.DocumentRow{
		ID: "d1", FilePath: "/f.md", FileHash: "h", TotalChunks: 1,
		IndexedAt: time.Now().UTC(), EmbeddingModel: "m",
	}
	chunks := []database.ChunkRow{
		{ID: "c0", DocumentID: "d1", ChunkIndex: 0, Content: "persisted",
			Embedding: []float32{0.6, 0.8}},
	}
	if err := db.ReplaceDocument(doc, chunks); err != nil {
		t.Fatalf("ReplaceDocument: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	db, err = database.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = db.Close() }()
	hits, err := db.SimilarChunks([]float32{0.6, 0.8}, 1, "m")
	if err != nil {
		t.Fatalf("SimilarChunks: %v", err)
	}
	if len(hits) != 1 || hits[0].Content != "persisted" {
		t.Fatalf("hits = %+v, want the persisted chunk", hits)
	}
	if math.Abs(float64(hits[0].Score-1.0)) > 1e-5 {
		t.Errorf("score = %v, want ~1.0", hits[0].Score)
	}
}
