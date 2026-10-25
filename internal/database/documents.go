package database

import (
	"database/sql"
	"fmt"
	"time"
)

// DocumentRow represents a row in the documents table.
type DocumentRow struct {
	ID             string
	FilePath       string
	FileHash       string
	TotalChunks    int
	IndexedAt      time.Time
	EmbeddingModel string // model used to generate the chunk embeddings
}

// ChunkRow represents a row in the chunks table.
type ChunkRow struct {
	ID          string
	DocumentID  string
	ChunkIndex  int
	HeadingPath string
	Content     string
	Embedding   []float32
}

// ScoredChunk is a ChunkRow with an associated similarity score and source path.
type ScoredChunk struct {
	ChunkRow
	Score    float32
	FilePath string // file_path of the parent document
}

// FindDocumentByPath returns the ID, file hash, and embedding model for the
// document with the given file path. Returns ("", "", "", nil) if not found.
func (db *DB) FindDocumentByPath(filePath string) (id, hash, model string, err error) {
	row := db.conn.QueryRow(
		"SELECT id, file_hash, embedding_model FROM documents WHERE file_path = ?", filePath)
	err = row.Scan(&id, &hash, &model)
	if err == sql.ErrNoRows {
		return "", "", "", nil
	}
	return
}

// ReplaceDocument atomically replaces a document and all its chunks.
// If a document with the same file_path already exists it is removed first.
func (db *DB) ReplaceDocument(doc DocumentRow, chunks []ChunkRow) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Look up existing document ID by file_path to clean up any previous version.
	var existingID string
	row := tx.QueryRow("SELECT id FROM documents WHERE file_path = ?", doc.FilePath)
	if err := row.Scan(&existingID); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("lookup existing document: %w", err)
	}
	if existingID != "" {
		// Delete chunks before document (logical FK enforced by app code).
		if _, err := tx.Exec(
			"DELETE FROM chunks WHERE document_id = ?", existingID); err != nil {
			return fmt.Errorf("delete old chunks: %w", err)
		}
		if _, err := tx.Exec(
			"DELETE FROM documents WHERE id = ?", existingID); err != nil {
			return fmt.Errorf("delete old document: %w", err)
		}
	}

	// Insert the new document row.
	if _, err := tx.Exec(
		`INSERT INTO documents (id, file_path, file_hash, total_chunks, indexed_at, embedding_model)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		doc.ID, doc.FilePath, doc.FileHash, doc.TotalChunks, doc.IndexedAt, doc.EmbeddingModel); err != nil {
		return fmt.Errorf("insert document: %w", err)
	}

	// Insert chunk rows with []float32 embeddings passed directly to DuckDB (v2).
	for _, c := range chunks {
		if _, err := tx.Exec(
			`INSERT INTO chunks
			   (id, document_id, chunk_index, heading_path, content, embedding)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			c.ID, c.DocumentID, c.ChunkIndex, c.HeadingPath, c.Content,
			c.Embedding); err != nil {
			return fmt.Errorf("insert chunk %d: %w", c.ChunkIndex, err)
		}
	}

	return tx.Commit()
}

// SimilarChunks returns the top-k chunks ranked by cosine similarity to the
// query vector, filtered to documents indexed with the given embeddingModel.
// Computed inside DuckDB using list_cosine_similarity.
func (db *DB) SimilarChunks(query []float32, topK int, embeddingModel string) ([]ScoredChunk, error) {
	rows, err := db.conn.Query(
		`SELECT c.id, c.document_id, c.chunk_index, c.heading_path, c.content,
		        list_cosine_similarity(c.embedding, ?) AS score,
		        d.file_path
		 FROM   chunks c
		 JOIN   documents d ON d.id = c.document_id
		 WHERE  c.embedding IS NOT NULL AND len(c.embedding) > 0
		   AND  d.embedding_model = ?
		 ORDER  BY score DESC
		 LIMIT  ?`,
		query, embeddingModel, topK)
	if err != nil {
		return nil, fmt.Errorf("similar chunks query: %w", err)
	}
	defer func() { _ = rows.Close() }() // read-only; rows.Err reports failures

	var result []ScoredChunk
	for rows.Next() {
		var sc ScoredChunk
		if err := rows.Scan(
			&sc.ID, &sc.DocumentID, &sc.ChunkIndex, &sc.HeadingPath,
			&sc.Content, &sc.Score, &sc.FilePath); err != nil {
			return nil, fmt.Errorf("scan scored chunk: %w", err)
		}
		result = append(result, sc)
	}
	return result, rows.Err()
}

// AdjacentChunks returns the chunks of documentID whose chunk_index is in
// [lo, hi], ordered by chunk_index. Used for context window expansion.
func (db *DB) AdjacentChunks(documentID string, lo, hi int) ([]ChunkRow, error) {
	rows, err := db.conn.Query(
		`SELECT id, document_id, chunk_index, heading_path, content
		 FROM chunks
		 WHERE document_id = ?
		   AND chunk_index BETWEEN ? AND ?
		 ORDER BY chunk_index ASC`,
		documentID, lo, hi)
	if err != nil {
		return nil, fmt.Errorf("query adjacent chunks: %w", err)
	}
	defer func() { _ = rows.Close() }() // read-only; rows.Err reports failures

	var result []ChunkRow
	for rows.Next() {
		var c ChunkRow
		if err := rows.Scan(
			&c.ID, &c.DocumentID, &c.ChunkIndex, &c.HeadingPath, &c.Content); err != nil {
			return nil, fmt.Errorf("scan adjacent chunk: %w", err)
		}
		result = append(result, c)
	}
	return result, rows.Err()
}
