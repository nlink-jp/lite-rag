// cmd/eval evaluates retrieval quality with and without query rewriting.
// It runs a fixed set of benchmark queries against the live database,
// measures cosine scores and source recall, and prints a Markdown report
// to stdout.
//
// Usage:
//
//	go run ./cmd/eval [-config path/to/config.toml]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"lite-rag/internal/config"
	"lite-rag/internal/database"
	"lite-rag/internal/llm"
	"lite-rag/internal/retriever"
)

// benchQuery is one benchmark item.
type benchQuery struct {
	label        string // short label for the table row
	text         string // natural-language question
	expectedFile string // substring of the file path that should appear in results
}

// runResult captures metrics for a single retrieval call.
type runResult struct {
	passages  int
	topScore  float32
	meanScore float32
	recall    bool  // expected file found in top passages?
	latencyMs int64
}

// evalResult holds the two runs (base vs rewrite) for one benchmark query.
type evalResult struct {
	base    runResult
	rewrite runResult
}

var benchmarks = []benchQuery{
	{
		label:        "chunk_size (数値)",
		text:         "デフォルトのチャンクサイズは何トークンですか？",
		expectedFile: "architecture",
	},
	{
		label:        "vector_db (コンポーネント)",
		text:         "ベクトルデータベースには何を使っていますか？",
		expectedFile: "architecture",
	},
	{
		label:        "overlap (アルゴリズム)",
		text:         "チャンク間のオーバーラップはどのように実装されていますか？",
		expectedFile: "architecture",
	},
	{
		label:        "index_cmd (使い方)",
		text:         "ドキュメントをインデックスするにはどのコマンドを使いますか？",
		expectedFile: "README",
	},
	{
		label:        "quality_gate (開発)",
		text:         "品質ゲートに含まれるチェック項目を教えてください",
		expectedFile: "setup",
	},
	{
		label:        "out_of_scope (範囲外)",
		text:         "Pythonでの実装方法を教えてください",
		expectedFile: "", // no expected file — should score low
	},
	{
		label:        "idempotent (冪等性)",
		text:         "同じファイルを2回インデックスするとどうなりますか？",
		expectedFile: "architecture",
	},
	{
		label:        "english_query (英語)",
		text:         "What embedding model does lite-rag use by default?",
		expectedFile: "setup",
	},
	{
		label:        "serve_cmd (Web UI)",
		text:         "HTTPサーバーを起動してWeb UIで検索するにはどうすればよいですか？",
		expectedFile: "README",
	},
}

func main() {
	cfgPath := flag.String("config", config.DefaultConfigPath(), "path to config file")
	dbPath := flag.String("db", "", "path to DuckDB file (overrides config database.path)")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	if *dbPath != "" {
		cfg.Database.Path = *dbPath
	}

	db, err := database.Open(cfg.Database.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open database: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "close database: %v\n", err)
		}
	}()

	client := llm.New(cfg.API.BaseURL, cfg.API.APIKey, cfg.Models.Embedding, cfg.Models.Chat)

	retCfg := cfg.Retrieval
	retCfg.QueryRewrite = false // controlled per run below

	ctx := context.Background()
	results := make([]evalResult, len(benchmarks))

	fmt.Fprintln(os.Stderr, "Running evaluation...")

	for i, q := range benchmarks {
		fmt.Fprintf(os.Stderr, "  [%d/%d] %s\n", i+1, len(benchmarks), q.label)

		// Base run (no rewriting)
		retBase := retriever.New(db, client, nil, cfg.Models.Embedding, retCfg)
		results[i].base = retrieve(ctx, retBase, q)

		// Rewrite run
		retRewrite := retriever.New(db, client, client, cfg.Models.Embedding, retCfg)
		results[i].rewrite = retrieve(ctx, retRewrite, q)
	}

	printReport(results)
}

func retrieve(ctx context.Context, ret *retriever.Retriever, q benchQuery) runResult {
	start := time.Now()
	passages, err := ret.Retrieve(ctx, q.text)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		fmt.Fprintf(os.Stderr, "    retrieve error: %v\n", err)
		return runResult{latencyMs: elapsed}
	}
	if len(passages) == 0 {
		return runResult{latencyMs: elapsed}
	}

	var total float32
	for _, p := range passages {
		total += p.Score
	}
	mean := total / float32(len(passages))

	var recall bool
	if q.expectedFile != "" {
		for _, p := range passages {
			if strings.Contains(p.FilePath, q.expectedFile) {
				recall = true
				break
			}
		}
	} else {
		// out-of-scope: "recall OK" if top score is low (< 0.60)
		recall = passages[0].Score < 0.60
	}

	return runResult{
		passages:  len(passages),
		topScore:  passages[0].Score,
		meanScore: mean,
		recall:    recall,
		latencyMs: elapsed,
	}
}

func printReport(results []evalResult) {
	fmt.Println("# クエリリライト性能評価レポート")
	fmt.Println()
	fmt.Printf("実行日時: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println()
	fmt.Println("## 結果サマリ")
	fmt.Println()

	baseWins, rewriteWins, ties := 0, 0, 0
	baseRecall, rewriteRecall := 0, 0
	for i, r := range results {
		if benchmarks[i].expectedFile != "" {
			switch {
			case r.rewrite.topScore > r.base.topScore+0.005:
				rewriteWins++
			case r.base.topScore > r.rewrite.topScore+0.005:
				baseWins++
			default:
				ties++
			}
		}
		if r.base.recall {
			baseRecall++
		}
		if r.rewrite.recall {
			rewriteRecall++
		}
	}

	fmt.Printf("- スコア優位（閾値 0.005）: ベース %d勝 / リライト %d勝 / 引き分け %d\n", baseWins, rewriteWins, ties)
	fmt.Printf("- ソース再現率: ベース %d/%d, リライト %d/%d\n",
		baseRecall, len(results), rewriteRecall, len(results))
	fmt.Println()
	fmt.Println("## 詳細結果")
	fmt.Println()
	fmt.Println("| クエリ | Base Top | RW Top | Base Mean | RW Mean | Base Recall | RW Recall | Base ms | RW ms |")
	fmt.Println("|--------|----------|--------|-----------|---------|-------------|-----------|---------|-------|")

	for i, r := range results {
		q := benchmarks[i]
		bRecall := "✗"
		rRecall := "✗"
		if r.base.recall {
			bRecall = "✓"
		}
		if r.rewrite.recall {
			rRecall = "✓"
		}

		marker := ""
		if q.expectedFile != "" {
			switch {
			case r.rewrite.topScore > r.base.topScore+0.005:
				marker = " ▲"
			case r.base.topScore > r.rewrite.topScore+0.005:
				marker = " ▼"
			}
		}

		fmt.Printf("| %s | %.3f | %.3f%s | %.3f | %.3f | %s | %s | %d | %d |\n",
			q.label,
			r.base.topScore,
			r.rewrite.topScore,
			marker,
			r.base.meanScore,
			r.rewrite.meanScore,
			bRecall,
			rRecall,
			r.base.latencyMs,
			r.rewrite.latencyMs,
		)
	}

	fmt.Println()
	fmt.Println("▲ = リライトが+0.005以上スコア向上, ▼ = ベースラインがスコア優位")
}
