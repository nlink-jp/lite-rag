package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type askRequest struct {
	Query string `json:"query"`
}

type sourceEntry struct {
	FilePath    string  `json:"file_path"`
	HeadingPath string  `json:"heading_path,omitempty"`
	Score       float32 `json:"score"`
}

// sseWriter wraps an http.ResponseWriter and formats each Write call as an
// SSE token event. The caller must set SSE headers before first use.
type sseWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

func newSSEWriter(w http.ResponseWriter) (*sseWriter, bool) {
	f, ok := w.(http.Flusher)
	if !ok {
		return nil, false
	}
	return &sseWriter{w: w, f: f}, true
}

func (s *sseWriter) Write(p []byte) (int, error) {
	// JSON-encode the token so that newlines and other special characters
	// never break the SSE "data:" line format.
	encoded, err := json.Marshal(string(p))
	if err != nil {
		return 0, err
	}
	if _, err := fmt.Fprintf(s.w, "event: token\ndata: %s\n\n", encoded); err != nil {
		return 0, err
	}
	s.f.Flush()
	return len(p), nil
}

func (s *sseWriter) sendJSON(event string, v any) {
	b, _ := json.Marshal(v)
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, b)
	s.f.Flush()
}

func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req askRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Query == "" {
		http.Error(w, "invalid request body: 'query' field required", http.StatusBadRequest)
		return
	}

	sw, ok := newSSEWriter(w)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering if present

	s.mu.RLock()
	defer s.mu.RUnlock()

	start := time.Now()
	result, err := runRAG(r.Context(), req.Query, s.db, s.client, s.cfg, sw)
	latency := time.Since(start)

	if err != nil {
		if errors.Is(err, context.Canceled) {
			slog.Info("ask cancelled", "latency_ms", latency.Milliseconds())
			slog.Debug("ask cancelled query", "query", req.Query)
			return
		}
		slog.Error("ask failed", "error", err, "latency_ms", latency.Milliseconds())
		slog.Debug("ask failed query", "query", req.Query)
		sw.sendJSON("error", map[string]string{"message": err.Error()})
		return
	}

	if result == nil || len(result.passages) == 0 {
		slog.Info("ask no results", "latency_ms", latency.Milliseconds())
		slog.Debug("ask no results query", "query", req.Query)
		sw.sendJSON("error", map[string]string{"message": "この質問に答える情報はドキュメントに含まれていません。"})
		sw.sendJSON("done", struct{}{})
		return
	}

	// Deduplicate sources by file path, keeping the highest-score passage per file.
	type seenEntry struct {
		entry sourceEntry
	}
	seen := make(map[string]seenEntry, len(result.passages))
	var order []string
	for _, p := range result.passages {
		if prev, ok := seen[p.FilePath]; !ok || p.Score > prev.entry.Score {
			if !ok {
				order = append(order, p.FilePath)
			}
			seen[p.FilePath] = seenEntry{entry: sourceEntry{
				FilePath:    p.FilePath,
				HeadingPath: p.HeadingPath,
				Score:       p.Score,
			}}
		}
	}
	sources := make([]sourceEntry, 0, len(order))
	for _, fp := range order {
		sources = append(sources, seen[fp].entry)
	}

	topScore := float32(0)
	if len(result.passages) > 0 {
		topScore = result.passages[0].Score
	}
	slog.Info("ask completed",
		"passages", len(result.passages),
		"top_score", topScore,
		"latency_ms", latency.Milliseconds(),
	)
	slog.Debug("ask completed query", "query", req.Query)

	sw.sendJSON("sources", sources)
	sw.sendJSON("done", struct{}{})
}
