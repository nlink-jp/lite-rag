# Query Rewrite Evaluation Report

## Revision history

| Date | Dataset | Queries | Notes |
|------|---------|---------|-------|
| 2026-03-21 (v2) | `docs/` — 16 files, full project documentation | 9 | Current; added `serve_cmd` query |
| 2026-03-21 (v1) | `testdata/docs/` — 5 purpose-built files | 8 | Previous; archived below |

---

## v2 Evaluation (2026-03-21) — docs/ dataset

### Overview

Nine benchmark queries were run against a database built from the full project
documentation (`docs/`, 16 files, 165 chunks). Results compare baseline retrieval
(no rewriting) against hybrid retrieval (LLM-assisted query rewriting).

The evaluation database is stored at `testdata/db/lite-rag-docs-20260321.db`.
It was built from the documentation at this point in time; `docs/en/eval/` is included
but reflects results from the v1 evaluation run.

### Environment

| Setting | Value |
|---------|-------|
| Embedding model | `text-embedding-nomic-embed-text-v1.5` |
| Chat model | `openai/gpt-oss-20b` (LM Studio) |
| Database | `testdata/db/lite-rag-docs-20260321.db` (16 files from `docs/`) |
| top_k | 5 |
| context_window | 1 |
| chunk_size | 512 |
| chunk_overlap | 64 |

### Benchmark Queries

| # | Label | Query | Expected file |
|---|-------|-------|---------------|
| 1 | chunk_size (numeric) | デフォルトのチャンクサイズは何トークンですか？ | architecture |
| 2 | vector_db (component) | ベクトルデータベースには何を使っていますか？ | architecture |
| 3 | overlap (algorithm) | チャンク間のオーバーラップはどのように実装されていますか？ | architecture |
| 4 | index_cmd (usage) | ドキュメントをインデックスするにはどのコマンドを使いますか？ | README |
| 5 | quality_gate (dev) | 品質ゲートに含まれるチェック項目を教えてください | setup |
| 6 | out_of_scope | Pythonでの実装方法を教えてください | — (low score expected) |
| 7 | idempotent | 同じファイルを2回インデックスするとどうなりますか？ | architecture |
| 8 | english_query | What embedding model does lite-rag use by default? | setup |
| 9 | serve_cmd (Web UI) | HTTPサーバーを起動してWeb UIで検索するにはどうすればよいですか？ | README |

### Metrics

| Metric | Description |
|--------|-------------|
| Top Score | Cosine similarity of the highest-ranked passage |
| Mean Score | Average cosine similarity across all returned passages |
| Recall | Whether the expected file appeared in any returned passage (out_of_scope: "pass" if top score < 0.60) |
| Latency (ms) | Wall time of `Retrieve()` including embedding, optional LLM rewrite, and vector search |

### Results

| Query | Base Top | RW Top | Base Mean | RW Mean | Base Recall | RW Recall | Base ms | RW ms |
|-------|----------|--------|-----------|---------|-------------|-----------|---------|-------|
| chunk_size (numeric) | 0.704 | **0.799** ▲ | 0.688 | **0.720** | ✓ | ✓ | 48 | 3795 |
| vector_db (component) | 0.750 | 0.750 | 0.745 | 0.717 | ✗ | ✓ | 11 | 2078 |
| overlap (algorithm) | 0.710 | **0.757** ▲ | 0.696 | **0.740** | ✗ | ✗ | 12 | 2435 |
| index_cmd (usage) | 0.767 | **0.778** ▲ | 0.722 | **0.748** | ✗ | ✓ | 12 | 2159 |
| quality_gate (dev) | 0.710 | **0.757** ▲ | 0.708 | **0.753** | ✗ | ✗ | 10 | 2310 |
| out_of_scope | 0.732 | 0.732 | 0.715 | 0.700 | ✗ | ✗ | 12 | 1483 |
| idempotent | 0.731 | **0.778** ▲ | 0.695 | **0.739** | ✓ | ✓ | 12 | 2357 |
| english_query | 0.811 | **0.821** ▲ | 0.787 | 0.782 | ✓ | ✓ | 11 | 1988 |
| serve_cmd (Web UI) | 0.711 | **0.747** ▲ | 0.704 | **0.732** | ✗ | ✗ | 12 | 1139 |

▲ = rewrite improved top score by ≥ 0.005

### Summary

| Metric | Result |
|--------|--------|
| Score wins (threshold 0.005) | Baseline 0 / Rewrite **7** / Tie 1 (of 8 non-out_of_scope queries) |
| Source recall | Baseline 3/9 (33%) / Rewrite **5/9 (56%)** |
| Mean latency — baseline | ~16 ms |
| Mean latency — rewrite | ~2,194 ms |
| Mean latency increase | **+2,178 ms** |

### Analysis

#### Score improvement (7 of 8 in-scope queries)

Query rewriting improved cosine similarity by ≥ 0.005 in 7 of 8 in-scope queries,
a notable improvement over the v1 result of 5/7. The three largest gains:

- **chunk_size**: +0.095 (0.704 → 0.799) — the largest gain observed across both
  evaluations. The interrogative "何トークンですか" was likely rewritten into a
  declarative statement that closely matches the TOML snippet in `architecture.md`.
- **overlap / idempotent / quality_gate**: all +0.047 — consistent improvements
  across technical and procedural queries.
- **serve_cmd**: +0.036 (0.711 → 0.747) — positive but recall remained ✗; the
  serve subcommand is only briefly mentioned in README sections.

The one tie (vector_db, 0.750 = 0.750) still produced a recall improvement (✗ → ✓),
suggesting the rewritten query surfaced a different but correct passage.

#### Source recall (baseline 3/9 → rewrite 5/9)

Rewriting lifted recall from 33% to 56% — a meaningful improvement. Persistent
recall failures:

1. **overlap**: `architecture.md` was not in the top passages despite being the
   correct source. The overlap section is a short paragraph surrounded by other
   architecture content; larger `context_window` would likely help.
2. **quality_gate**: `setup.md` describes `make check` in the Git hooks section,
   but the phrase "品質ゲート" (quality gate) does not appear verbatim. A terminology
   match issue; authoring guide recommends using the exact term readers would search for.
3. **out_of_scope**: top score 0.732 exceeds the 0.60 low-relevance threshold. The
   larger docs/ dataset raises the floor for all scores. A threshold around 0.70 may
   be more appropriate for this dataset size.
4. **serve_cmd**: `serve` is mentioned in README files but without a dedicated section;
   adding a standalone serve usage document would fix this.

#### Comparison with v1 (testdata/docs/ dataset)

| Metric | v1 (5 files) | v2 (16 files) |
|--------|-------------|--------------|
| Rewrite score wins | 5/7 (71%) | 7/8 (88%) |
| Rewrite recall | 5/8 (62.5%) | 5/9 (56%) |
| Baseline recall | 5/8 (62.5%) | 3/9 (33%) |
| Mean latency increase | +1,433 ms | +2,178 ms |

The lower recall in v2 is expected: 16 files produce more competitive retrieval,
making it harder for the exact expected file to surface. Score improvements are
stronger because the larger corpus provides more relevant content for the rewriter
to leverage. Latency increase reflects longer concurrent search over a larger index.

### Recommendations

| Use case | Setting | Reason |
|----------|---------|--------|
| Accuracy-first (batch / async) | `query_rewrite = true` | Score improved in 88% of queries; recall +23 pp |
| Speed-first (interactive chat) | `query_rewrite = false` | ~16 ms vs ~2,200 ms per query |
| Predominantly English queries | `query_rewrite = true` | english_query recall held; small latency is acceptable |

### Reproducing the evaluation

```sh
# Build the binary
make build

# Index docs/ into a versioned evaluation database
make eval-build-db
# → testdata/db/lite-rag-docs-YYYYMMDD.db + testdata/db/eval-current.db symlink

# Run the evaluation (Markdown table to stdout)
make eval
```

The evaluation harness is implemented in `cmd/eval/main.go`. The `-db` flag
overrides the database path; the `-config` flag selects the config file (used for
LLM connection settings).

---

## v1 Evaluation (archived) — testdata/docs/ dataset

The original evaluation used five purpose-built documents in `testdata/docs/`.
Results are preserved here for reference.

### Environment

| Setting | Value |
|---------|-------|
| Database | `lite-rag.db` (5 files from `testdata/docs/` indexed) |

Indexed documents: `overview.md`, `architecture.md`, `configuration.md`,
`usage.md`, `development.md`.

### Results

| Query | Base Top | RW Top | Base Recall | RW Recall |
|-------|----------|--------|-------------|-----------|
| chunk_size (numeric) | 0.693 | **0.709** ▲ | ✓ | ✓ |
| vector_db (component) | 0.683 | 0.686 | ✓ | ✓ |
| overlap (algorithm) | 0.623 | **0.665** ▲ | ✓ | ✓ |
| index_cmd (usage) | 0.662 | **0.720** ▲ | ✓ | ✓ |
| quality_gate (dev) | 0.672 | **0.708** ▲ | ✗ | ✗ |
| out_of_scope | 0.681 | 0.696 | ✗ | ✗ |
| idempotent | 0.651 | **0.678** ▲ | ✓ | ✓ |
| english_query | 0.785 | 0.790 | ✗ | ✗ |

Summary: Rewrite 5/7 wins · Recall 5/8 each · Mean latency +1,433 ms
