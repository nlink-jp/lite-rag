# 使い方

## コマンド一覧

```
lite-rag [--config <path>] <command>
```

### index — ドキュメントをインデックス

```sh
lite-rag index <directory>
```

指定ディレクトリ以下の `*.md` ファイルを再帰的に走査してインデックスします。

- ファイルの SHA-256 ハッシュと埋め込みモデル名をチェックし、変更がなければスキップします。
- ファイル単位でエラーが発生しても処理は継続されます（ログに警告を出力）。
- インデックス結果は `config.toml` の `database.path` に指定した DuckDB ファイルに保存されます。

**使用例:**

```sh
# カレントディレクトリの docs/ フォルダをインデックス
lite-rag index ./docs

# 別の設定ファイルを使う
lite-rag --config /path/to/config.toml index ./docs
```

### ask — 質問に回答

```sh
lite-rag ask "<質問>"
```

インデックス済みドキュメントをもとに質問に回答します。回答はストリーミングで標準出力へ出力されます。

**使用例:**

```sh
# 日本語で質問
lite-rag ask "lite-rag のチャンク分割アルゴリズムを教えてください"

# 英語でも可
lite-rag ask "What embedding model does lite-rag use?"
```

インデックスが空の場合は「No relevant documents found.」と表示して終了します。

### serve — HTTP API サーバーと Web UI を起動

```sh
lite-rag serve [--addr <host:port>]
```

HTTP API サーバーを起動し、ブラウザから利用できる簡易 Web UI を提供します。
デフォルトのリッスンアドレスは `127.0.0.1:8080`（ローカルホスト専用）。

**エンドポイント:**

| エンドポイント | メソッド | 内容 |
|---|---|---|
| `/api/ask` | POST | SSE ストリーミングで回答を返す |
| `/api/status` | GET | サーバー稼働確認・バージョン取得 |
| `/` | GET | 組み込み Web UI |

**使用例:**

```sh
# デフォルトアドレスで起動（127.0.0.1:8080）
lite-rag serve

# アドレスを変更して起動
lite-rag serve --addr 127.0.0.1:9090
```

ブラウザで `http://127.0.0.1:8080` を開くと検索フォームが表示されます。
回答はストリーミングで表示され、Markdown のレンダリングも対応しています。

サーバーは `Ctrl+C`（SIGINT）または SIGTERM でグレースフルシャットダウンします。

**ログレベル設定（`config.toml` の `server.log_level`）:**

| レベル | 出力内容 |
|---|---|
| `info`（デフォルト） | メタデータのみ（クエリ内容は出力しない） |
| `debug` | クエリ内容も出力（開発用） |
| `warn` | 警告とエラーのみ |
| `error` | エラーのみ |

### docs — インデックス済みドキュメントの管理

```sh
lite-rag docs list [--json]
lite-rag docs show <document-id>
lite-rag docs delete <document-id>
```

インデックスデータベースに保存されているドキュメントを管理します。

- `list` — インデックス済みドキュメントの一覧をテキストまたは JSON (`--json`) で表示します
- `show` — ドキュメント ID を指定して、データベース内に保存されているコンテンツを表示します
- `delete` — ドキュメントとその全チャンクをデータベースから削除します

`<document-id>` は `docs list` で表示される 64 文字の SHA-256 ハッシュです。

### version — バージョン表示

```sh
lite-rag version
```

ビルド時に埋め込まれたバージョン文字列を表示します。タグなしビルドは `dev` と表示されます。

## 典型的なワークフロー

```sh
# 1. 設定ファイルを用意
mkdir -p ~/.config/lite-rag
cp config.example.toml ~/.config/lite-rag/config.toml
$EDITOR ~/.config/lite-rag/config.toml  # モデル名・DBパスを確認

# 2. ビルド
make build

# 3. ドキュメントをインデックス
./bin/lite-rag index ./docs

# 4. 質問する
./bin/lite-rag ask "このプロジェクトの概要を教えてください"
```

## nomic-embed のプレフィックス

`nomic-embed-text-v1.5` はタスク固有のプレフィックスを付与することで検索精度が向上します。
lite-rag は内部で自動的に付与するため、ユーザーが意識する必要はありません。

| フェーズ    | プレフィックス     |
|-----------|-----------------|
| インデックス | `search_document:` |
| クエリ      | `search_query:`    |
