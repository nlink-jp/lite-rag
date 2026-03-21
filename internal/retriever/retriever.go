// Package retriever implements the retrieval stage of the RAG pipeline.
// It embeds a query, finds the top-K similar chunks via DuckDB, expands each
// hit with adjacent chunks (context window), and deduplicates overlapping spans.
// Optionally a QueryRewriter rewrites the query in parallel for hybrid search.
package retriever

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"lite-rag/internal/config"
	"lite-rag/internal/database"
	"lite-rag/internal/normalizer"
)

// Embedder is the subset of llm.Client used by the Retriever.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// QueryRewriter rewrites a query into one or more forms more likely to match
// document text. Returning multiple variants (e.g. Japanese and English) causes
// the Retriever to run a parallel search per variant and merge the results.
// If the implementation returns an error, only the original query is searched.
type QueryRewriter interface {
	RewriteQuery(ctx context.Context, query string) ([]string, error)
}

// DBReader is the subset of database.DB used by the Retriever.
type DBReader interface {
	SimilarChunks(query []float32, topK int, embeddingModel string) ([]database.ScoredChunk, error)
	AdjacentChunks(documentID string, lo, hi int) ([]database.ChunkRow, error)
}

// Passage is a retrieved, context-expanded text passage ready for LLM consumption.
type Passage struct {
	Content     string  // merged content of the expanded window
	Score       float32 // cosine similarity of the best matching chunk in this window
	HeadingPath string  // heading hierarchy of the highest-scoring chunk
	DocumentID  string
	FilePath    string // source file path of the document
}

// Retriever executes the vector search and context expansion steps.
type Retriever struct {
	db             DBReader
	emb            Embedder
	rewriter       QueryRewriter // nil = disabled
	topK           int
	contextWindow  int
	embeddingModel string
}

// New creates a Retriever configured from cfg.
// rewriter may be nil to disable query rewriting.
// embeddingModel must match the model used during indexing so that vector
// comparisons are only made between embeddings from the same model.
func New(db DBReader, emb Embedder, rewriter QueryRewriter, embeddingModel string, cfg config.RetrievalConfig) *Retriever {
	return &Retriever{
		db:             db,
		emb:            emb,
		rewriter:       rewriter,
		topK:           cfg.TopK,
		contextWindow:  cfg.ContextWindow,
		embeddingModel: embeddingModel,
	}
}

// Retrieve embeds query, searches for the top-K similar chunks, expands each hit
// by ±contextWindow adjacent chunks, deduplicates overlapping spans, and returns
// the resulting passages sorted by score descending.
//
// When a QueryRewriter is configured, two searches run concurrently:
//   - original query embedding → SimilarChunks
//   - rewritten query embedding → SimilarChunks
//
// Results are merged (max score per chunk ID) before context expansion.
func (r *Retriever) Retrieve(ctx context.Context, query string) ([]Passage, error) {
	hits, err := r.search(ctx, query)
	if err != nil {
		return nil, err
	}
	if len(hits) == 0 {
		return nil, nil
	}
	return r.expand(hits)
}

// search runs concurrent vector searches for the original query and any rewritten
// variants, then merges the results (max score per chunk ID).
func (r *Retriever) search(ctx context.Context, query string) ([]database.ScoredChunk, error) {
	type searchResult struct {
		hits []database.ScoredChunk
		err  error
	}

	// Goroutine 1: embed the original query and search immediately.
	origCh := make(chan searchResult, 1)
	go func() {
		vecs, err := r.emb.Embed(ctx, []string{"search_query: " + normalizer.StripMarkdown(query)})
		if err != nil {
			origCh <- searchResult{err: fmt.Errorf("embed query: %w", err)}
			return
		}
		hits, err := r.db.SimilarChunks(vecs[0], r.topK, r.embeddingModel)
		if err != nil {
			origCh <- searchResult{err: fmt.Errorf("similar chunks: %w", err)}
			return
		}
		origCh <- searchResult{hits: hits}
	}()

	// Goroutine 2: rewrite the query into variants (e.g. JA + EN), then search
	// each variant in parallel and send the merged result (optional).
	rewriteCh := make(chan searchResult, 1)
	if r.rewriter != nil {
		go func() {
			variants, err := r.rewriter.RewriteQuery(ctx, query)
			if err != nil {
				slog.Warn("query rewrite failed, skipping", "error", err)
				rewriteCh <- searchResult{}
				return
			}
			for i, v := range variants {
				slog.Debug("query rewritten", "original", query, "variant", i, "rewritten", v)
			}

			// Search all variants in parallel.
			type varResult struct{ hits []database.ScoredChunk }
			varChs := make([]chan varResult, len(variants))
			for i, v := range variants {
				ch := make(chan varResult, 1)
				varChs[i] = ch
				go func(v string, ch chan varResult) {
					vecs, err := r.emb.Embed(ctx, []string{"search_query: " + normalizer.StripMarkdown(v)})
					if err != nil {
						slog.Warn("embed rewritten query failed, skipping", "variant", v, "error", err)
						ch <- varResult{}
						return
					}
					hits, err := r.db.SimilarChunks(vecs[0], r.topK, r.embeddingModel)
					if err != nil {
						slog.Warn("similar chunks (rewritten) failed, skipping", "variant", v, "error", err)
						ch <- varResult{}
						return
					}
					ch <- varResult{hits: hits}
				}(v, ch)
			}

			var merged []database.ScoredChunk
			for _, ch := range varChs {
				merged = mergeHits(merged, (<-ch).hits)
			}
			rewriteCh <- searchResult{hits: merged}
		}()
	} else {
		rewriteCh <- searchResult{} // disabled — send empty immediately
	}

	orig := <-origCh
	if orig.err != nil {
		return nil, orig.err
	}
	rewrite := <-rewriteCh // rewrite errors are non-fatal; already logged above

	return mergeHits(orig.hits, rewrite.hits), nil
}

// mergeHits combines two ScoredChunk slices, deduplicating by chunk ID and
// keeping the higher score for any chunk that appears in both.
// The result is sorted by score descending.
func mergeHits(a, b []database.ScoredChunk) []database.ScoredChunk {
	if len(b) == 0 {
		return a
	}
	seen := make(map[string]database.ScoredChunk, len(a)+len(b))
	for _, h := range a {
		seen[h.ID] = h
	}
	for _, h := range b {
		if prev, ok := seen[h.ID]; !ok || h.Score > prev.Score {
			seen[h.ID] = h
		}
	}
	merged := make([]database.ScoredChunk, 0, len(seen))
	for _, h := range seen {
		merged = append(merged, h)
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].Score > merged[j].Score })
	return merged
}

// expand groups hits by document, computes ±contextWindow spans, merges
// overlapping spans, fetches adjacent chunks, and builds final Passages.
func (r *Retriever) expand(hits []database.ScoredChunk) ([]Passage, error) {
	// Group hits by documentID; also record the file path for each document.
	type hitInfo struct {
		chunkIndex  int
		score       float32
		headingPath string
	}
	docHits := make(map[string][]hitInfo, len(hits))
	docFilePath := make(map[string]string, len(hits))
	for _, h := range hits {
		docHits[h.DocumentID] = append(docHits[h.DocumentID], hitInfo{
			chunkIndex:  h.ChunkIndex,
			score:       h.Score,
			headingPath: h.HeadingPath,
		})
		docFilePath[h.DocumentID] = h.FilePath
	}

	var passages []Passage

	for docID, docHitList := range docHits {
		// Compute expanded spans.
		type span struct {
			lo, hi      int
			score       float32
			headingPath string
		}
		spans := make([]span, len(docHitList))
		for i, h := range docHitList {
			lo := h.chunkIndex - r.contextWindow
			if lo < 0 {
				lo = 0
			}
			spans[i] = span{
				lo:          lo,
				hi:          h.chunkIndex + r.contextWindow,
				score:       h.score,
				headingPath: h.headingPath,
			}
		}

		// Sort by lo, then merge overlapping / adjacent spans.
		sort.Slice(spans, func(i, j int) bool { return spans[i].lo < spans[j].lo })
		merged := []span{spans[0]}
		for _, s := range spans[1:] {
			last := &merged[len(merged)-1]
			if s.lo <= last.hi+1 {
				if s.hi > last.hi {
					last.hi = s.hi
				}
				if s.score > last.score {
					last.score = s.score
					last.headingPath = s.headingPath
				}
			} else {
				merged = append(merged, s)
			}
		}

		// Fetch chunks for each merged span.
		for _, m := range merged {
			chunks, err := r.db.AdjacentChunks(docID, m.lo, m.hi)
			if err != nil {
				return nil, fmt.Errorf("adjacent chunks for %s [%d,%d]: %w", docID, m.lo, m.hi, err)
			}
			var sb strings.Builder
			for i, c := range chunks {
				if i > 0 {
					sb.WriteByte('\n')
				}
				sb.WriteString(c.Content)
			}
			passages = append(passages, Passage{
				Content:     sb.String(),
				Score:       m.score,
				HeadingPath: m.headingPath,
				DocumentID:  docID,
				FilePath:    docFilePath[docID],
			})
		}
	}

	// Return passages sorted by score descending.
	sort.Slice(passages, func(i, j int) bool { return passages[i].Score > passages[j].Score })
	return passages, nil
}
