# セットアップガイド

## 必要環境

| ツール | バージョン | 用途 |
|---|---|---|
| Go | 1.26+ | ビルドツールチェーン |
| make | 任意 | ビルド自動化 |
| cc（clang/gcc） | 任意 | CGo コンパイル（go-duckdb 必須） |
| podman または docker | 任意 | Linux クロスコンパイル（任意） |

> **注意:** ビルドはプロジェクトローカルの Go モジュールキャッシュ（`.go/`）を使用するため、
> `~/go` への書き込みは不要です。Makefile により自動設定されます。

## クイックスタート

```sh
git clone <repo-url> lite-rag
cd lite-rag

# 設定ファイルをコピーして編集
mkdir -p ~/.config/lite-rag
cp config.example.toml ~/.config/lite-rag/config.toml
$EDITOR ~/.config/lite-rag/config.toml

# Git フックとビルドツールをインストール
make setup

# ビルド
make build

# 動作確認
./dist/lite-rag --help
```

## LM Studio の設定

1. LM Studio を起動し、以下のモデルをロードします：
   - 埋め込み: `nomic-ai/nomic-embed-text-v1.5-GGUF`
   - チャット: `openai/gpt-oss-20b`
2. ローカルサーバーを有効化します（デフォルトポート: `1234`）。
3. API が疎通しているか確認します：
   ```sh
   curl http://localhost:1234/v1/models
   ```

## Git フックのインストール

Git フックは `scripts/hooks/` で管理しています。インストール手順：

```sh
make setup
```

`scripts/hooks/pre-commit` と `scripts/hooks/pre-push` を `.git/hooks/` にコピーし、
実行権限を付与します。両フックとも `make check`（lint + test + build）を実行し、
失敗した場合はコミット・プッシュを拒否します。

## 環境変数

設定ファイルの各項目は環境変数で上書きできます：

| 変数名 | 上書き対象 |
|---|---|
| `LITE_RAG_API_BASE_URL` | `api.base_url` |
| `LITE_RAG_API_KEY` | `api.api_key` |
| `LITE_RAG_EMBEDDING_MODEL` | `models.embedding` |
| `LITE_RAG_CHAT_MODEL` | `models.chat` |
| `LITE_RAG_DB_PATH` | `database.path` |

デフォルトの設定ファイルパスは XDG Base Directory 仕様に従います：
`$XDG_CONFIG_HOME/lite-rag/config.toml`（デフォルト: `~/.config/lite-rag/config.toml`）。
`--config` フラグで上書き可能: `lite-rag --config /path/to/config.toml`。

## クロスコンパイル

### darwin ターゲット（macOS ホストから）

macOS システムの `clang` が `-arch` によるマルチアーキテクチャをサポートしているため、
追加ツールは不要です。

```sh
make cross-build-darwin
# 生成物: dist/lite-rag-darwin-arm64  dist/lite-rag-darwin-amd64
```

### linux ターゲット（コンテナ経由）

`go-duckdb` は GCC/libstdc++ でビルドされた DuckDB 静的ライブラリをリンクします。
macOS から Linux 向けにクロスコンパイルするには、マッチする C++ ランタイムを持つ
Linux 環境が必要です。コンテナを使った方法が最も簡単です：

```sh
# podman（brew install podman）または docker が必要
make cross-build-linux
# 生成物: dist/lite-rag-linux-amd64  dist/lite-rag-linux-arm64
```

Makefile は `podman` を優先し、なければ `docker` を使用します。

### linux ターゲット（Linux ホスト上で直接）

Linux マシン上で直接ビルドする場合：

```sh
# arm64 クロスコンパイルにはクロスコンパイラが必要
sudo apt-get install gcc-aarch64-linux-gnu

make cross-build-linux-native
```


## プロジェクトローカルモジュールキャッシュ

Makefile が以下を設定します：

```
GOPATH=$(pwd)/.go
GOMODCACHE=$(pwd)/.go/pkg/mod
GOCACHE=$(pwd)/.go/cache
```

ダウンロードしたモジュールとビルド成果物をプロジェクトディレクトリ内に保持します。
ネットワーク制限のある環境で有用です。`.go/` ディレクトリは .gitignore に登録されています。
