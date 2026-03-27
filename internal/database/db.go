// Package database provides DuckDB connectivity and schema management for lite-rag.
package database

import (
	"database/sql"
	"fmt"

	_ "github.com/marcboeker/go-duckdb/v2"
)

// DB wraps a DuckDB connection.
type DB struct {
	conn *sql.DB
}

// Open opens (or creates) the DuckDB database at path and applies migrations.
func Open(path string) (*DB, error) {
	conn, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, fmt.Errorf("open duckdb %s: %w", path, err)
	}
	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, err
	}
	return db, nil
}

// Close closes the underlying database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Ping verifies the database connection is alive.
func (db *DB) Ping() error {
	return db.conn.Ping()
}

// QueryRaw executes a raw SQL query and returns the resulting rows.
// Intended for tests and diagnostic use only.
func (db *DB) QueryRaw(query string, args ...any) (*sql.Rows, error) {
	return db.conn.Query(query, args...)
}

// DocumentRecord holds metadata for a single indexed document.
type DocumentRecord struct {
	ID             string
	FilePath       string
	FileHash       string
	TotalChunks    int
	IndexedAt      string
	EmbeddingModel string
}

// ChunkRecord holds a single chunk with its text content.
type ChunkRecord struct {
	ChunkIndex  int
	HeadingPath string
	Content     string
}

// ListDocuments returns all documents ordered by file_path.
func (db *DB) ListDocuments() ([]DocumentRecord, error) {
	rows, err := db.conn.Query(`
		SELECT id, file_path, file_hash, total_chunks,
		       strftime(indexed_at, '%Y-%m-%d %H:%M:%S'), embedding_model
		FROM documents
		ORDER BY file_path
	`)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer rows.Close()

	var docs []DocumentRecord
	for rows.Next() {
		var d DocumentRecord
		if err := rows.Scan(&d.ID, &d.FilePath, &d.FileHash,
			&d.TotalChunks, &d.IndexedAt, &d.EmbeddingModel); err != nil {
			return nil, fmt.Errorf("scan document: %w", err)
		}
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

// DeleteDocument removes a document and all its chunks. Returns an error if the
// document does not exist.
func (db *DB) DeleteDocument(id string) error {
	// Verify the document exists first.
	var count int
	if err := db.conn.QueryRow(`SELECT COUNT(*) FROM documents WHERE id = ?`, id).
		Scan(&count); err != nil {
		return fmt.Errorf("check document: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("document %q not found", id)
	}

	if _, err := db.conn.Exec(`DELETE FROM chunks WHERE document_id = ?`, id); err != nil {
		return fmt.Errorf("delete chunks: %w", err)
	}
	if _, err := db.conn.Exec(`DELETE FROM documents WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	return nil
}

// ListStaleDocuments returns documents whose embedding_model differs from
// currentModel, ordered by file_path. Used by the reindex command to find
// documents that need re-embedding after a model change.
func (db *DB) ListStaleDocuments(currentModel string) ([]DocumentRecord, error) {
	rows, err := db.conn.Query(`
		SELECT id, file_path, file_hash, total_chunks,
		       strftime(indexed_at, '%Y-%m-%d %H:%M:%S'), embedding_model
		FROM documents
		WHERE embedding_model != ?
		ORDER BY file_path
	`, currentModel)
	if err != nil {
		return nil, fmt.Errorf("list stale documents: %w", err)
	}
	defer rows.Close()

	var docs []DocumentRecord
	for rows.Next() {
		var d DocumentRecord
		if err := rows.Scan(&d.ID, &d.FilePath, &d.FileHash,
			&d.TotalChunks, &d.IndexedAt, &d.EmbeddingModel); err != nil {
			return nil, fmt.Errorf("scan document: %w", err)
		}
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

// UpdateDocumentEmbeddings replaces the embedding vectors for all chunks of a
// document and updates the document's embedding_model, all in one transaction.
// chunks must be ordered by chunk_index and len(chunks) must equal len(embeddings).
func (db *DB) UpdateDocumentEmbeddings(docID, newModel string, chunks []ChunkRecord, embeddings [][]float32) error {
	if len(chunks) != len(embeddings) {
		return fmt.Errorf("chunk count (%d) != embedding count (%d)", len(chunks), len(embeddings))
	}
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for i, c := range chunks {
		if _, err := tx.Exec(
			`UPDATE chunks SET embedding = ? WHERE document_id = ? AND chunk_index = ?`,
			embeddings[i], docID, c.ChunkIndex,
		); err != nil {
			return fmt.Errorf("update chunk %d embedding: %w", c.ChunkIndex, err)
		}
	}
	if _, err := tx.Exec(
		`UPDATE documents SET embedding_model = ? WHERE id = ?`, newModel, docID,
	); err != nil {
		return fmt.Errorf("update document model: %w", err)
	}
	return tx.Commit()
}

// DocumentChunks returns all chunks for a document ordered by chunk_index.
func (db *DB) DocumentChunks(id string) ([]ChunkRecord, error) {
	// Verify the document exists first.
	var filePath string
	if err := db.conn.QueryRow(`SELECT file_path FROM documents WHERE id = ?`, id).
		Scan(&filePath); err == sql.ErrNoRows {
		return nil, fmt.Errorf("document %q not found", id)
	} else if err != nil {
		return nil, fmt.Errorf("check document: %w", err)
	}

	rows, err := db.conn.Query(`
		SELECT chunk_index, COALESCE(heading_path, ''), content
		FROM chunks
		WHERE document_id = ?
		ORDER BY chunk_index
	`, id)
	if err != nil {
		return nil, fmt.Errorf("query chunks: %w", err)
	}
	defer rows.Close()

	var chunks []ChunkRecord
	for rows.Next() {
		var c ChunkRecord
		if err := rows.Scan(&c.ChunkIndex, &c.HeadingPath, &c.Content); err != nil {
			return nil, fmt.Errorf("scan chunk: %w", err)
		}
		chunks = append(chunks, c)
	}
	return chunks, rows.Err()
}

// migrate creates all required tables if they do not already exist and applies
// incremental schema changes for pre-existing databases.
func (db *DB) migrate() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS documents (
    id              TEXT      PRIMARY KEY,
    file_path       TEXT      NOT NULL,
    file_hash       TEXT      NOT NULL,
    total_chunks    INTEGER   NOT NULL,
    indexed_at      TIMESTAMP NOT NULL,
    embedding_model TEXT      NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS chunks (
    id           TEXT     PRIMARY KEY,
    document_id  TEXT     NOT NULL,  -- logical FK to documents(id); enforced by app
    chunk_index  INTEGER  NOT NULL,
    heading_path TEXT,
    content      TEXT     NOT NULL,
    embedding    FLOAT[]            -- []float32 stored as DuckDB list; requires go-duckdb v2
);
`
	if _, err := db.conn.Exec(ddl); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	// Add embedding_model to pre-existing databases that lack the column.
	const alterDDL = `ALTER TABLE documents ADD COLUMN IF NOT EXISTS embedding_model TEXT DEFAULT '';`
	if _, err := db.conn.Exec(alterDDL); err != nil {
		return fmt.Errorf("migrate alter: %w", err)
	}
	return nil
}
