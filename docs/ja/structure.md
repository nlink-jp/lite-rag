# プロジェクトディレクトリ構造

```
lite-rag/
├── cmd/
│   ├── lite-rag/
│   │   ├── main.go          # CLI エントリーポイント；cobra でサブコマンドを登録
│   │   ├── index.go         # `index <dir>` サブコマンド
│   │   ├── ask.go           # `ask <question>` サブコマンド
│   │   ├── serve.go         # `serve` サブコマンド — HTTP API + Web UI 起動
│   │   ├── docs.go          # `docs` サブコマンド — list / show / delete
│   │   └── version.go       # `version` サブコマンド
│   └── eval/
│       └── main.go          # 検索品質評価ハーネス（クエリリライトベンチマーク）
│
├── internal/                # プライベートアプリケーションコード（外部インポート不可）
│   ├── config/
│   │   └── config.go        # TOML + 環境変数による設定読み込み
│   ├── database/
│   │   └── db.go            # DuckDB 接続とスキーママイグレーション
│   ├── normalizer/
│   │   └── normalizer.go    # Unicode NFKC 正規化；Markdown 除去；
│   │                        # 日英混在トークン数推定
│   ├── indexer/
│   │   └── indexer.go       # ファイル走査・正規化・チャンク分割・埋め込み・DuckDB 書き込み
│   ├── retriever/
│   │   └── retriever.go     # ベクトル検索 + コンテキストウィンドウ拡張 + 重複除去
│   ├── llm/
│   │   └── client.go        # OpenAI 互換 HTTP クライアント（Embed + Chat ストリーミング）
│   └── server/
│       ├── server.go        # Server 構造体・ルーティング・graceful shutdown
│       ├── rag.go           # ask コマンドと HTTP ハンドラが共用する RAG クエリロジック
│       ├── handler_ask.go   # POST /api/ask — SSE ストリーミングハンドラ
│       ├── handler_status.go # GET /api/status
│       ├── embed.go         # //go:embed static/*
│       └── static/          # 埋め込み Web UI（index.html, app.js, style.css, marked.min.js）
│
├── pkg/                     # パブリックライブラリコード（外部から再利用可能）
│   └── chunker/
│       └── chunker.go       # 見出し対応 Markdown チャンカー（日英文境界対応）
│
├── api/                     # API コントラクトファイル（将来の HTTP フロントエンド用プレースホルダー）
│
├── scripts/
│   └── hooks/
│       ├── pre-commit       # コミット前に `make check` を実行
│       └── pre-push         # プッシュ前に `make check` を実行
│
│
├── docs/
│   ├── design/
│   │   ├── architecture.md  # システムアーキテクチャとコンポーネント設計
│   │   └── plan.md          # 開発フェーズとマイルストーン
│   ├── eval/
│   │   └── query-rewrite.md # クエリリライト機能の性能評価レポート
│   ├── ja/                  # 全一次ドキュメントの日本語訳
│   ├── RFP.md               # 元要件定義書
│   ├── authoring-guide.md   # 検索品質を最大化する Markdown ドキュメントの書き方
│   ├── dependencies.md      # サードパーティ依存関係レジスター（RULES.md §18）
│   ├── setup.md             # インストールと開発環境セットアップ
│   └── structure.md         # このファイル
│
├── .go/                     # プロジェクトローカル Go モジュールキャッシュ（.gitignore 済み）
│   ├── pkg/mod/             # ダウンロード済みモジュールソース
│   └── cache/               # ビルドキャッシュ
│
├── bin/                     # コンパイル済みバイナリ（.gitignore 済み）
├── dist/                    # `make dist` が生成するリリースアーカイブ（.gitignore 済み）
├── config.example.toml      # 全設定項目を網羅したリファレンス設定
├── go.mod                   # Go モジュール定義
├── go.sum                   # モジュールチェックサムデータベース
├── Makefile                 # ビルド・テスト・lint・クロスコンパイルターゲット
└── RULES.md                 # プロジェクトルール（全コントリビューター必読）
```

## 主要な規約

- **`internal/`** パッケージは外部プロジェクトからインポートできません。パッケージ間の依存は
  内向きに流れます：`cmd` → `internal/*` → `pkg/*`
- **`pkg/chunker`** は I/O 依存なしで、他プロジェクトからインポート可能です。
- **`internal/normalizer`** は Indexer（保存前）と Retriever（クエリ埋め込み前）の両方から呼ばれます。
- **`.go/`** はプロジェクトローカルの GOPATH/GOMODCACHE/GOCACHE です。
  ネットワーク制限のある環境でも依存関係をキャッシュできます。.gitignore に登録済みです。
