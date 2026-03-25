# lite-rag

Markdown ドキュメント向けの CLI ベース RAG（Retrieval-Augmented Generation）ツールです。
ディレクトリ内の Markdown ファイルをローカルの [DuckDB](https://duckdb.org/) データベースにインデックスし、
OpenAI 互換 API（例：[LM Studio](https://lmstudio.ai/)）経由でローカル LLM に質問を投げて回答を得られます。

日英混在ドキュメントも NFKC 正規化・混合スクリプトのトークン推定により完全対応しています。

---

## 機能

- **インデックス**: ディレクトリを再帰的にスキャンして `*.md` ファイルをチャンク分割・埋め込み生成し、DuckDB に保存
- **質問応答**: 質問を埋め込み、関連チャンクを検索して隣接チャンクでコンテキストを拡張し、LLM の回答をストリーミング出力
- **サーバー**: ローカル HTTP API サーバー（`POST /api/ask` SSE、`GET /api/status`）と、Markdown レンダリング対応の組み込み Web UI を提供
- **差分更新**: SHA-256 ハッシュで変更されたファイルのみ再インデックス（冪等性）
- **日本語対応**: Unicode NFKC 正規化、日本語文境界でのチャンク分割、CJK/ASCII 混在トークン推定
- **コンテキストウィンドウ拡張**: ベクトル検索ヒットを前後 N チャンク分拡張し、重複する範囲は自動マージ

---

## 必要環境

| ツール | バージョン | 用途 |
|---|---|---|
| Go | 1.26+ | ビルドツールチェーン |
| make | 任意 | ビルド自動化 |
| cc（clang/gcc） | 任意 | CGo コンパイル（go-duckdb 必須） |
| LM Studio | 任意 | ローカル LLM 推論サーバー |

詳細なセットアップ手順は [docs/ja/setup.md](setup.md) を参照してください。

---

## クイックスタート

```sh
git clone <repo-url> lite-rag
cd lite-rag

# 設定ファイルをコピーして編集
mkdir -p ~/.config/lite-rag
cp config.example.toml ~/.config/lite-rag/config.toml
$EDITOR ~/.config/lite-rag/config.toml   # api.base_url、api.api_key、models.* を設定

# Git フックとリンターをインストール
make setup

# ビルド
make build

# ディレクトリをインデックス
./bin/lite-rag index --dir /path/to/docs

# シングルファイルをインデックス
./bin/lite-rag index --file /path/to/doc.md

# 質問する（CLI）
./bin/lite-rag ask "リトライポリシーの設定方法は？"

# または Web UI サーバーを起動する
./bin/lite-rag serve        # http://127.0.0.1:8080 で起動
```

---

## 設定

`config.example.toml` をコピーして編集してください：

```toml
[api]
base_url = "http://localhost:1234/v1"   # LM Studio のデフォルト
api_key  = "lm-studio"                 # 任意の値（LM Studio は検証しない）

[models]
embedding = "nomic-ai/nomic-embed-text-v1.5-GGUF"
chat      = "openai/gpt-oss-20b"

[database]
path = "./lite-rag.db"

[retrieval]
top_k          = 5     # ベクトル検索のヒット件数
context_window = 1     # 各ヒットの前後に拡張するチャンク数
chunk_size     = 512
chunk_overlap  = 64
query_rewrite  = false # LLM によるクエリリライトを有効化（再現率向上、+約 2 秒/クエリ）

[server]
addr      = "127.0.0.1:8080"  # `serve` コマンドのリッスンアドレス
log_level = "info"             # info | debug | warn | error
```

環境変数でファイル設定を上書きできます：

| 変数名 | 上書き対象 |
|---|---|
| `LITE_RAG_API_BASE_URL` | `api.base_url` |
| `LITE_RAG_API_KEY` | `api.api_key` |
| `LITE_RAG_EMBEDDING_MODEL` | `models.embedding` |
| `LITE_RAG_CHAT_MODEL` | `models.chat` |
| `LITE_RAG_DB_PATH` | `database.path` |

---

## 使い方

```
lite-rag [--config <path>] [--db <path>] <command>

コマンド:
  index   --dir <directory>  ディレクトリ以下の *.md ファイルをインデックス
          --file <file>       シングルファイルをインデックス（拡張子不問）
  ask     <question>         インデックス済みドキュメントを使って質問に回答
  serve                      HTTP API サーバーと組み込み Web UI を起動
  docs                       インデックス済みドキュメントを管理
  version                    バージョン情報を表示

グローバルフラグ:
  --config <path>   設定ファイルパス（デフォルト: ~/.config/lite-rag/config.toml）
  --db <path>       データベースファイルパス（config の database.path を上書き）
```

### index

```sh
# ディレクトリ以下の *.md ファイルをすべてインデックス（再帰的）
./bin/lite-rag index --dir ./docs

# シングルファイルをインデックス（拡張子不問）
./bin/lite-rag index --file ./docs/notes.md
./bin/lite-rag index --file ./release-notes.txt
```

- `--dir`: 指定ディレクトリを再帰的にスキャンし `*.md` ファイルのみ処理します
- `--file`: 拡張子に関わらず指定ファイルを直接インデックスします
- SHA-256 ハッシュが一致するファイルはスキップします（再埋め込みなし）
- `--dir` のファイル単位エラーはログに記録され全体の処理は継続しますが、`--file` のエラーは即時返されます

### ask

```sh
./bin/lite-rag ask "デフォルトのチャンクサイズは？"
./bin/lite-rag --config /etc/lite-rag.toml ask "インストール手順は？"
./bin/lite-rag --db ./project-b.db ask "デフォルトのチャンクサイズは？"

# JSON 出力（回答とソースを1つの JSON オブジェクトで出力）
./bin/lite-rag ask --json "デフォルトのチャンクサイズは？"
```

- 質問を埋め込み、DuckDB で上位 K 件の類似チャンクを検索します
- 各ヒットを ±`context_window` チャンク分拡張します
- LLM の回答を stdout にストリーミング出力します
- `query_rewrite = true` で LLM による**多言語クエリリライト**が有効になります。クエリを日本語・英語の宣言文にリライトして3並列検索を実行し、多言語ドキュメントの双方を効率よく検索します（88% のクエリでスコア向上、約 2 秒のオーバーヘッド）
- `--json`: 回答をバッファリングして1つの JSON オブジェクトとして出力します。進捗メッセージは抑制され、stdout は純粋な JSON になります。

```json
{
  "answer": "デフォルトのチャンクサイズは 512 トークンです。",
  "sources": [
    {"file_path": "docs/README.md", "heading_path": "Configuration", "score": 0.872}
  ]
}
```

### serve

```sh
./bin/lite-rag serve                     # 127.0.0.1:8080 でリッスン（デフォルト）
./bin/lite-rag serve --addr 0.0.0.0:9090
```

ローカル HTTP サーバーを起動します。ブラウザで `http://127.0.0.1:8080` を開いてください。

| エンドポイント | メソッド | 説明 |
|---|---|---|
| `/api/ask` | POST | SSE ストリーミングで回答を返す（`{"query":"..."}`）|
| `/api/status` | GET | ヘルスチェックとバージョン確認 |
| `/` | GET | 組み込み Web UI |

### docs

インデックスデータベースに保存されているドキュメントを管理します。

```sh
# インデックス済みドキュメントをテキスト一覧で表示
./bin/lite-rag docs list

# JSON 形式で出力（機械処理向け）
./bin/lite-rag docs list --json

# ドキュメント ID を指定してコンテンツを表示
./bin/lite-rag docs show <document-id>

# ドキュメントとその全チャンクを削除
./bin/lite-rag docs delete <document-id>
```

`<document-id>` は `docs list` で表示される 64 文字の SHA-256 ハッシュです。

---

## ビルド

```sh
# 現在のプラットフォーム向け
make build

# darwin 全アーキテクチャ（macOS ホスト）
make cross-build-darwin

# Linux 向け（podman または docker が必要。または Linux ホストで実行）
make cross-build-linux
```

バイナリは `bin/` に生成されます。クロスコンパイルの詳細は [docs/ja/setup.md](setup.md) を参照してください。

---

## 開発

```sh
make test    # テスト実行
make vet     # go vet
make lint    # golangci-lint
make check   # 品質ゲート全体：vet + lint + test + build
```

`make setup` でインストールされる Git フックが、コミット・プッシュ前に `make check` を自動実行します。

---

## アーキテクチャ

詳細は [docs/design/architecture.md](../design/architecture.md) を参照してください。

```
index コマンド
  └─ Indexer: ファイル走査 → 正規化 → チャンク分割 → 埋め込み → DuckDB

ask コマンド
  └─ Retriever: クエリ埋め込み → SimilarChunks → AdjacentChunks → LLM Chat

serve コマンド
  └─ HTTP サーバー: POST /api/ask (SSE) · GET /api/status · GET / (Web UI)
```

---

## ライセンス

MIT

---

*English documentation: [README.md](../../README.md)*
