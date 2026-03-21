# 開発計画: lite-rag

## フェーズとマイルストーン

---

### Phase 0: プロジェクトスキャフォールド *(実装前)*

目標: 機能コードを書く前に品質ゲートが機能するクリーンな作業骨格を用意する。

- [x] 正しいモジュール名と Go バージョンで `go.mod` をリセット
- [x] 全依存関係を追加（`go get`）し `go.sum` を生成
- [x] `build`、`test`、`lint`、`check`、`setup` ターゲットを持つ `Makefile` を作成
- [x] `config.example.toml` を作成
- [x] `scripts/hooks/pre-commit` と `scripts/hooks/pre-push` を作成
- [x] `docs/setup.md`（フックインストール、前提条件）を作成
- [x] `docs/dependencies.md` を作成
- [x] `docs/structure.md` を作成

完了基準: `make check` がクリーンに実行される（まだ lint/test 対象はないが、ツール自体が動作する）。

---

### Phase 1: コアインフラ

目標: 設定読み込みと DuckDB 接続をテスト付きで動作させる。

- [x] `internal/config/` — TOML + 環境変数オーバーライドによる読み込み
- [x] `internal/database/` — DuckDB の開閉、マイグレーション実行（CREATE TABLE）
- [x] 両パッケージのユニットテスト

完了基準: `go test ./...` がパスする。

---

### Phase 2: テキスト正規化 + チャンカー

目標: 正規化とチャンク分割ロジックを独立してフルテスト。

- [x] `internal/normalizer/` — NFKC、空白、制御文字、Markdown アーティファクト除去
- [x] normalizer のユニットテスト: 全角→半角、半角カナ→全角カナ、日英混在
- [x] `pkg/chunker/` — 見出し対応分割 + 段落/文フォールバック（日本語 + 英語）
- [x] トークン推定器: CJK 文字数 × 2 + 単語数 × 1.3
- [x] 代表的な Markdown フィクスチャを使ったユニットテスト（日本語のみ、英語のみ、混在）

完了基準: エッジケース（空入力、CJK のみ、ASCII のみ、混在）をカバーした `go test ./internal/normalizer/... ./pkg/chunker/...` がパスする。

---

### Phase 3: LLM クライアント

目標: LM Studio に対して埋め込みとチャット補完の呼び出しが動作する。

- [x] `internal/llm/` — `Embed()` と `Chat()`（ストリーミング）メソッド
- [x] ユニットテスト（httptest.Server による mock）

完了基準: LM Studio に対する手動スモークテストが成功する。

---

### Phase 4: Indexer

目標: `index` コマンドのエンドツーエンド動作。

- [x] `internal/indexer/` — 走査、ハッシュ、チャンク分割、埋め込み、アップサート
- [x] `cmd/lite-rag/` — cobra 経由で `index` サブコマンドを配線
- [x] 小さなフィクスチャディレクトリを使った統合テスト

完了基準: `lite-rag index ./docs` 実行で DuckDB ファイルが作成される。

---

### Phase 5: Retriever + Ask コマンド  *(完了)*

目標: コンテキストウィンドウ拡張を含む `ask` コマンドのエンドツーエンド動作。

- [x] `internal/retriever/` — クエリ埋め込み、DuckDB でコサイン類似度 SQL、上位 K 件ヒット返却
- [x] コンテキストウィンドウ拡張: `chunk_index` でヒット毎に隣接チャンク（±N）を取得
- [x] 重複除去: 同一ドキュメントのヒットで重複するウィンドウをマージ
- [x] `ask` サブコマンドを配線
- [x] ユニットテスト: mock DB 結果で拡張ロジック（境界ケース: 先頭/末尾チャンク）
- [x] 統合テスト: フィクスチャをインデックスし、質問し、非空の回答を確認
- [x] クエリリライト: LLM によるハイブリッドサーチ（`query_rewrite` 設定フラグ）

完了基準: `lite-rag ask "lite-rag とは？"` が周囲のコンテキストを正しくマージしたストリーミング回答を返す。

---

### Phase 6: クロスコンパイル & リリース  *(完了)*

目標: 全 4 対象プラットフォーム向けバイナリ。

- [x] darwin ターゲット: `clang -arch` による Makefile `cross-build-darwin` ターゲット
- [x] linux ターゲット: Podman/Docker コンテナ内の `gcc-x86-64-linux-gnu` / `gcc-aarch64-linux-gnu` による `cross-build-linux` ターゲット
- [x] `make dist` でリリースアーカイブ（tar.gz）を生成
- [x] `CHANGELOG.md` にリリースエントリを記載
- [x] Git タグ `v0.1.0`、`v0.1.1`；GitHub リリース公開済み

---

### Phase 7: HTTP API サーバー + Web UI  *(完了)*

目標: ローカル `serve` サブコマンド（HTTP API + 組み込みブラウザ UI）。

- [x] `internal/server/` — HTTP サーバー、グレースフルシャットダウン、SSE ストリーミングハンドラ
- [x] `POST /api/ask` — SSE ストリーミング RAG 回答
- [x] `GET /api/status` — ヘルスチェック + バージョン
- [x] 組み込み Web UI（`index.html`、`app.js`、`style.css`、`marked.min.js`）
- [x] Markdown レンダリング、Raw/Rendered 切り替え
- [x] `cmd/lite-rag/serve.go` — cobra サブコマンド、`--addr` フラグ
- [x] プライバシー対応構造化ログ（`log/slog` JSON、クエリ内容は DEBUG のみ）

---

### Phase 8: ドキュメント  *(完了)*

目標: RULES.md のすべてのドキュメント要件を満たす。

- [x] `README.md` — セットアップ、使い方、設定リファレンス
- [x] `docs/ja/` — 全一次ドキュメントの日本語訳
- [x] `docs/authoring-guide.md` — 検索品質を最大化する Markdown の書き方
- [x] `docs/eval/query-rewrite.md` — クエリリライト機能の性能評価レポート
- [x] XDG デフォルト設定パス（`~/.config/lite-rag/config.toml`）

---

### Phase 9: ドキュメント管理（`docs` サブコマンド）  *(完了)*

目標: DB への直接アクセスなしでインデックス済みドキュメントを参照・管理できる。

- [x] `db.ListDocuments()` — ファイルパス順に全ドキュメントレコードを返す
- [x] `db.DeleteDocument(id)` — ドキュメントと全チャンクを削除；存在しない場合はエラー
- [x] `db.DocumentChunks(id)` — コンテンツ再構築のためチャンクをインデックス順に返す
- [x] `cmd/lite-rag/docs.go` — `docs list [--json]`、`docs show <id>`、`docs delete <id>`
- [x] 3つの DB メソッドに対するユニットテスト（9 テストケース）
- [x] ドキュメント更新（README、architecture.md、structure.md、usage.md）

---

## スコープ外 (v0.1.x)

- Windows サポート（`go-duckdb` の CGo 制約によりドロップ）
- マルチユーザーまたは並行インデックス
- インクリメンタル埋め込み更新（変更時はファイル全体を再インデックス）
- Web UI からのドキュメントインジェスト（インデックス作成はローカル CLI 操作）
