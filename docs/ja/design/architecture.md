# アーキテクチャ設計: lite-rag

## 1. 概要

`lite-rag` は Markdown ドキュメント向けの RAG（Retrieval-Augmented Generation）パイプラインを実装した CLI ツールです。
ドキュメントをローカルの DuckDB データベースにインデックスし、自然言語の質問に対して関連チャンクを取得して
ローカル LLM に渡し、OpenAI 互換 API 経由で回答を生成します。

---

## 2. システムアーキテクチャ

```
┌─────────────────────────────────────────────────────────┐
│                        CLI (cobra)                       │
│              cmd/lite-rag/main.go                        │
│         ┌──────────────┬─────────────────┐              │
│         │ index <dir>  │  ask "<query>"   │              │
└─────────┴──────┬───────┴────────┬────────┴──────────────┘
                 │                │
        ┌────────▼──────┐  ┌──────▼──────────┐
        │   Indexer     │  │    Retriever     │
        │ internal/     │  │  internal/       │
        │ indexer/      │  │  retriever/      │
        └────────┬──────┘  └──────┬───────────┘
                 │                │
        ┌────────▼────────────────▼───────────┐
        │           Database Layer             │
        │         internal/database/           │
        │          (DuckDB via CGo)            │
        └─────────────────────────────────────┘
                 │
        ┌────────▼──────────────────────────────┐
        │            LLM Client                  │
        │           internal/llm/                │
        │   (OpenAI 互換 HTTP クライアント)        │
        └───────────────────────────────────────┘
```

---

## 3. コンポーネント設計

### 3.1 設定 (`internal/config/`)

- デフォルトは `~/.config/lite-rag/config.toml`（XDG Base Directory 仕様: `$XDG_CONFIG_HOME/lite-rag/config.toml`）。`--config` フラグで上書き可能。
- 環境変数でファイル設定を上書き可能（プレフィックス: `LITE_RAG_`）。
- リファレンス設定として `config.example.toml` を提供。

**主要設定項目:**

```toml
[api]
base_url     = "http://localhost:1234/v1"
api_key      = "lm-studio"

[models]
embedding    = "nomic-ai/nomic-embed-text-v1.5-GGUF"
chat         = "openai/gpt-oss-20b"

[database]
path         = "./lite-rag.db"

[retrieval]
top_k           = 5    # ベクター検索ヒット数
context_window  = 1    # 各ヒットの前後に拡張するチャンク数（0 = 無効）
chunk_size      = 512  # チャンクあたりの目標トークン数
chunk_overlap   = 64   # 隣接チャンク間のオーバーラップ
```

### 3.2 Indexer (`internal/indexer/`)

責務：
1. 指定ディレクトリを再帰的に走査し `*.md` ファイルを収集。
2. 各ファイルの SHA-256 ハッシュを計算。
3. DuckDB に保存済みのハッシュと比較し、変更のないファイルをスキップ（冪等性）。
4. Markdown を解析してセマンティックチャンクに分割（§3.5 参照）。
5. 各チャンクの埋め込みAPIを呼び出し（可能な場合はバッチ処理）。
6. チャンクとベクターを DuckDB にアップサート。

### 3.3 Retriever (`internal/retriever/`)

責務：
1. 自然言語クエリを受け取る。
2. LLM によるクエリの多言語リライトを行う（オプション、§3.3.2 参照）。
3. 埋め込みAPIを呼び出してクエリベクターを生成。
4. DuckDB 内の全チャンクベクターに対してコサイン類似度検索を実行（バリアント毎）。
5. 全検索結果をマージ（チャンク ID ごとに最高スコアを採用）。
6. 各ヒットに対して同一ドキュメントの隣接チャンクを取得（§3.3.1 参照）。
7. 充実したチャンク群を LLM プロンプト構築用のコンテキストとして返す。

ベクター類似度検索は DuckDB の組み込み `list_cosine_similarity` 関数を使ったSQLクエリで実装。

#### 3.3.1 コンテキストウィンドウ拡張

ベクター検索ヒットは意味的に最も近いチャンクを特定しますが、そのチャンクが途中で切れる場合があります。
連続性を回復するため、Retriever は同一ドキュメントの `±N` 隣接チャンクを取得してヒットの周囲にマージします。

```
ドキュメントのチャンク:  [ 0 ][ 1 ][ 2 ★ ][ 3 ][ 4 ][ 5 ]
                                    ↑ ベクター検索ヒット

context_window=1: [ 1 ][ 2 ★ ][ 3 ]           (ヒット ± 1)
context_window=2: [ 0 ][ 1 ][ 2 ★ ][ 3 ][ 4 ] (ヒット ± 2)
```

取得は `chunk_index` と `document_id` を使った1クエリで完結：

```sql
SELECT content, chunk_index, heading_path
FROM   chunks
WHERE  document_id = $1
  AND  chunk_index BETWEEN $hit_index - $window AND $hit_index + $window
ORDER  BY chunk_index ASC;
```

隣接チャンクは順序通りに連結され、1つの連続したパッセージとして LLM に渡されます。
ヒットチャンクの類似度スコアは関連性スコアとして引き継がれます。

**重複除去:** 同一ドキュメントの複数ヒットで拡張ウィンドウが重複する場合、重複部分はマージされ繰り返しを防ぎます。

#### 3.3.2 多言語クエリリライト

`query_rewrite = true` の場合、Retriever は**多言語ハイブリッドサーチ**を実行します：

1. `llm.Client.RewriteQuery` がクエリを日本語（`JA:`）と英語（`EN:`）の宣言文にリライト。
2. 3つのベクター検索を並列実行：
   - オリジナルクエリ（ユーザー入力のまま）
   - 日本語バリアント
   - 英語バリアント
3. 3つの結果をマージ（チャンク ID ごとに最高スコアを採用）してからコンテキスト拡張。

クエリの言語に関わらず、多言語ドキュメントを効率よく検索できます。

```
query "チャンク分割はどう動く？"
       │
       ├── オリジナル埋め込み ────────────────→ SimilarChunks → hits_orig
       │
       └── RewriteQuery ──→ JA: チャンク分割の仕組み  → embed → SimilarChunks → hits_ja
                         └→ EN: chunk splitting algorithm → embed → SimilarChunks → hits_en
                                                              ↓
                                              mergeHits(orig, ja, en)
                                                              ↓
                                              コンテキスト拡張 → passages
```

リライトの LLM 呼び出しが失敗した場合はオリジナルクエリのみで検索（非致命的フォールバック）。

**設定:**

```toml
[retrieval]
query_rewrite = true   # デフォルト: false
```

### 3.4 LLM クライアント (`internal/llm/`)

- OpenAI 互換 REST API の薄いラッパー。
- 標準ライブラリの `net/http` のみ使用（サードパーティ SDK 依存なし）。
- Server-Sent Events (SSE) による**ストリーミング**レスポンス対応（到着したトークンを `io.Writer` に書き込み）。
- 埋め込みエンドポイント用の `Embed()` メソッド（非ストリーミング）。
- `RewriteQuery()` はチャットモデルに `JA: ...` / `EN: ...` 2行フォーマットを要求し、
  両バリアントを `[]string` で返す。フォーマット不一致時は単一要素スライスにフォールバック。

### 3.5 チャンカー (`pkg/chunker/`)

`pkg/` に配置（I/O 依存なし、独立して再利用可能）。

チャンカーは**正規化済みテキスト**（§3.7 参照）に対して動作します。

アルゴリズム：
1. Markdown 見出し（`#`、`##`、`###`）で分割してセマンティック境界を保持。
2. 見出しセクションが `chunk_size` トークンを超える場合、以下の優先順位でさらに分割：
   a. 空行（段落区切り）— 言語非依存。
   b. 日本語文末句読点: `。` `．` `！` `？`（全角）。
   c. 英語文末句読点: `.` `!` `?` の後に空白。
3. 各チャンクに見出し階層パスをプレフィックスとして付与（例: `# ガイド > ## インストール`）。
4. 各チャンクはメタデータを保持: `source_file`、`heading_path`、`chunk_index`。

**混在言語テキストのトークン数推定:**

| スクリプト | 文字 | 係数 |
|---|---|---|
| CJK（漢字・ひらがな・カタカナ等） | `\p{Han}`、`\p{Hiragana}`、`\p{Katakana}` | 1文字 ≈ 2トークン |
| ASCII / ラテン文字 | `[A-Za-z0-9]` | 1単語 ≈ 1.3トークン |
| 句読点 / その他 | — | 0（無視可能） |

推定トークン数 = `(CJK文字数 × 2) + (単語数 × 1.3)`

**Nomic-embed プレフィックス注入:**

`nomic-embed-text-v1.5` は最良の検索品質を得るためにタスク固有のプレフィックスが必要です。
チャンカーはプレフィックスを追加しません — その責務は呼び出し元（IndexerとRetriever）に属し、
保存済みのチャンクコンテンツはプレフィックスなしで保持されます。

- Indexer: `search_document: {chunk_content}` としてラップ
- Retriever: `search_query: {question}` としてラップ

### 3.6 テキスト正規化 (`internal/normalizer/`)

ドキュメントは日本語と英語を混在させる場合があります。
検索品質を低下させる表面的なばらつきを軽減するため、チャンク分割・埋め込み前にテキストを正規化します。

**正規化パイプライン（順番通りに適用）:**

```
生の Markdown テキスト
       │
       ▼
1. Unicode NFKC 正規化   (golang.org/x/text/unicode/norm)
       │  • 全角ASCII → 半角  （Ａ→A, １→1, （→(）
       │  • 半角カタカナ → 全角  (ｶﾅ → カナ)
       │  • 互換合字の分解  (㍉→ミリ, ㎞→km)
       ▼
2. 空白正規化
       │  • 全角スペース U+3000 → U+0020
       │  • 連続スペース / タブ → 単一スペース
       │  • 改行コードを LF に統一
       ▼
3. 制御文字除去
       │  • C0/C1 制御文字を除去（LF と TAB を除く）
       ▼
4. Markdown アーティファクト除去  (埋め込み前のみ適用、チャンク分割前は適用しない)
       │  • 画像構文を除去  ![alt](url) → alt
       │  • リンク構文を折りたたむ  [text](url) → text
       │  • 生の HTML タグを除去
       │  • フェンスコードブロックのマーカーを除去（```行）; コード内容は保持
       │  • インラインコードのバッククォートのみ除去
       ▼
正規化済みテキスト（チャンク分割または埋め込みの準備完了）
```

### 3.7 データベース層 (`internal/database/`)

スキーマ：

```sql
CREATE TABLE documents (
    id              TEXT      PRIMARY KEY,  -- SHA-256(file_path + ":" + file_hash)
    file_path       TEXT      NOT NULL,
    file_hash       TEXT      NOT NULL,     -- SHA-256 of file content (変更検知)
    total_chunks    INTEGER   NOT NULL,     -- 境界クランプに使用
    indexed_at      TIMESTAMP NOT NULL,
    embedding_model TEXT      NOT NULL DEFAULT ''  -- 埋め込み生成に使用したモデル名
);

CREATE TABLE chunks (
    id           TEXT    PRIMARY KEY,  -- SHA-256(document_id + ":" + chunk_index)
    document_id  TEXT    NOT NULL,     -- documents(id) への論理 FK (アプリで強制)
    chunk_index  INTEGER NOT NULL,     -- ドキュメント内の 0 始まり位置
    heading_path TEXT,                 -- 例: "# ガイド > ## インストール"
    content      TEXT    NOT NULL,     -- 正規化済みテキスト（プレフィックスなし）
    embedding    FLOAT[]               -- []float32; go-duckdb v2 でネイティブ保存
);
```

類似度検索は DuckDB の組み込み `list_cosine_similarity` 関数を使用：

```sql
SELECT c.id, c.document_id, c.chunk_index, c.heading_path, c.content,
       list_cosine_similarity(c.embedding, ?) AS score
FROM   chunks c
JOIN   documents d ON d.id = c.document_id
WHERE  c.embedding IS NOT NULL AND len(c.embedding) > 0
  AND  d.embedding_model = ?
ORDER  BY score DESC
LIMIT  ?
```

上位 K 件のヒットを見つけた後、Retriever は各ヒットの隣接チャンクを取得し、
重複するスパンをマージしてから呼び出し元にパッセージを返します（§3.3.1）。

---

## 4. CLI インターフェース

```
lite-rag [--config <path>] [--db <path>] <command>

コマンド:
  index   --dir <directory>  ディレクトリ以下の *.md ファイルをインデックス
          --file <file>       シングルファイルをインデックス（拡張子不問）
  ask     <question>         インデックス済みドキュメントを使って質問に回答
          --json              回答とソースを JSON オブジェクトで出力
  serve                      HTTP API サーバーと組み込み Web UI を起動
  docs                       インデックス済みドキュメントを管理（list / show / delete）
  version                    バージョン情報を表示

グローバルフラグ:
  --config <path>   設定ファイル（デフォルト: ~/.config/lite-rag/config.toml、XDG）
  --db <path>       データベースファイルパス（config の database.path を上書き）
```

`--db` を使うと設定ファイルを編集せずに複数のインデックス済みコレクションを切り替えられます:

```sh
lite-rag --db ./docs-en.db ask "How does chunking work?"
lite-rag --db ./docs-ja.db ask "チャンク分割はどう動く？"
```

---

## 5. 対象プラットフォーム

| OS      | アーキテクチャ | サポート |
|---------|---------|-----------|
| linux   | amd64   | ○ |
| linux   | arm64   | ○ |
| darwin  | amd64   | ○ |
| darwin  | arm64   | ○ |
| windows | amd64   | **なし** — `go-duckdb` の CGo 制約によりドロップ |

クロスコンパイルの詳細は `docs/setup.md` を参照。

---

## 6. 依存関係

| パッケージ | 目的 | 採用理由 |
|---|---|---|
| `github.com/marcboeker/go-duckdb/v2` | DuckDB ドライバー | 公式 Go ドライバー; 組み込み DB で外部サーバー不要; v2 で `[]float32` SQL パラメーター対応 |
| `github.com/spf13/cobra` | CLI フレームワーク | Go CLI の標準; 構造化サブコマンドサポート |
| `github.com/BurntSushi/toml` | TOML 設定解析 | 軽量・推移的依存なし・広く使用されている |
| `golang.org/x/text` | Unicode 正規化 (NFKC) | 標準ライブラリ拡張; 信頼できる日本語テキスト処理に必須 |

---

## 7. エラーハンドリング

- すべてのエラーは上位に伝播し、構造化出力（`log/slog`）でログ記録。
- CLI は致命的エラーで非ゼロステータスコードで終了。
- Indexer はファイル単位のエラーをログに記録し、全体の実行を中断しない（回復可能）。
- LLM API エラー（タイムアウト、5xx）は生のステータスコードとともに即座に表示。

---

## 8. セキュリティ考慮事項

- API キーはログに記録しない。
- データベースや設定テンプレートにシークレットを保存しない。
- データベースファイルパスはデフォルトでカレントディレクトリ; 必要に応じてファイルパーミッションを制限すること。

---

*一次言語: 英語。原文: `docs/design/architecture.md`*
