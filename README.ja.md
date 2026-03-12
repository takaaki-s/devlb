# devlb

マルチワークツリー開発向けのローカル TCP リバースプロキシ。

git worktree で同じマイクロサービスを複数起動すると、ポートが競合します。**devlb** はデフォルトポートを確保し、ptrace で各プロセスの `bind()` を書き換えてトラフィックを透過的にルーティングします。設定変更も環境変数の調整も不要です。

```
┌─────────── devlb daemon (デフォルトポートで Listen) ──────────┐
│  :3000  → :3001 (worktree-a/api)    ● active                 │
│           :3002 (worktree-b/api)    ○ standby                 │
│  :8995  → :8996 (worktree-a/auth)   ● active                 │
└───────────────────────────────────────────────────────────────┘
```

## 特徴

- **自動ポートインターセプト** — `devlb exec` が ptrace で `bind()` システムコールを書き換え、空きポートを自動割当（Linux 限定）
- **瞬時の切り替え** — `devlb switch worktree-b` でプロセス再起動なしにトラフィックを切り替え
- **ヘルスチェック** — TCP プローブによる定期チェック、異常時は自動フェイルオーバー
- **接続メトリクス** — バックエンドごとのアクティブ接続数・転送バイト数を記録
- **HTTP 503 エラーページ** — バックエンド不在時に 503 を返却（非 HTTP トラフィックはそのまま通過）
- **設定のホットリロード** — `devlb.yaml` を編集するだけでサービスの追加・削除が即座に反映
- **TUI ダッシュボード** — リアルタイムのインタラクティブターミナル UI

## インストール

```bash
go install github.com/takaaki-s/devlb/cmd/devlb@latest
```

ソースからビルドする場合:

```bash
make build          # → bin/devlb
make install        # → $GOPATH/bin/devlb
```

**Go 1.24+** が必要です。プロキシデーモンは全プラットフォームで動作しますが、`devlb exec`（ptrace ベースのポートインターセプト）は **Linux 限定** です。

## クイックスタート

```bash
# 1. 設定ファイルを生成
devlb init

# 2. ~/.devlb/devlb.yaml を編集
cat ~/.devlb/devlb.yaml
# services:
#   - name: api
#     port: 3000
#   - name: auth
#     port: 8995

# 3. デーモンを起動
devlb start

# 4. サービスを実行（ポート 3000 は自動的にインターセプトされる）
devlb exec 3000 -- go run ./cmd/api

# 5. 別の worktree で同じポートのサービスを実行
devlb exec 3000 -- go run ./cmd/api    # worktree-b から

# バックエンドポートを明示的に指定する場合
devlb exec 3000:3001 -- go run ./cmd/api

# 複数ポートを同時に指定
devlb exec 3000,8995 -- go run ./cmd/server

# 6. トラフィックを切り替え
devlb switch worktree-b

# 7. 状態を確認
devlb status
devlb status -v    # 詳細表示: メトリクス付き

# 8. インタラクティブダッシュボード
devlb tui
```

## コマンド一覧

| コマンド | 説明 |
|---------|------|
| `devlb init` | `~/.devlb/devlb.yaml` 設定テンプレートを生成 |
| `devlb start` | デーモンをバックグラウンドで起動 |
| `devlb stop` | デーモンを停止 |
| `devlb status [-v]` | ルーティングテーブルを表示（-v: メトリクス付き） |
| `devlb route <port> <backend-port> [--label NAME]` | バックエンドを手動登録 |
| `devlb unroute <port> <backend-port>` | バックエンドを削除 |
| `devlb switch [port] <label>` | ラベル指定でアクティブバックエンドを切り替え |
| `devlb exec <port>[:<backend-port>][,...] -- <cmd> [args]` | ポートインターセプト付きでコマンド実行（Linux 限定） |
| `devlb tui` | インタラクティブターミナルダッシュボード |
| `devlb logs [label] [-f] [-n N] [--port PORT]` | バックエンドのログを集約表示 |
| `devlb status --json` | JSON 形式でステータス出力 |

## 設定

`~/.devlb/devlb.yaml`:

```yaml
services:
  - name: api
    port: 3000
  - name: auth
    port: 8995

# オプション: ヘルスチェック
health_check:
  enabled: true
  interval: "1s"
  timeout: "500ms"
  unhealthy_after: 3
```

デーモン稼働中でもサービスの追加・削除が可能です。変更は自動的に検知されます。

## TUI ダッシュボード

`devlb tui` でリアルタイムのターミナルダッシュボードを表示:

```
 devlb dashboard                                    auto-refresh: 1s

  PORT    BACKEND   LABEL          STATUS           CONNS       IN      OUT
  :3000   :3001     worktree-a     ● active             5    1.2M    567K
          :3002     worktree-b     ○ standby             0      0B      0B
  :8995   :8996     main           ● active              2    823K    1.4M
          :8997     feature-x      ✗ unhealthy           0      0B      0B

  ↑↓ 選択  s 切替  r 更新  q 終了
```

## ポートインターセプトの仕組み

`devlb exec 3000 -- your-server` を実行すると:

1. ptrace 下で子プロセスを起動
2. ポート 3000 を対象とする `bind()` システムコールをインターセプト
3. ポート引数をエフェメラルポート（例: 3001）に書き換え
4. 新しいポートをバックエンドとしてデーモンに登録
5. デーモンが `:3000 → :3001` を透過的にプロキシ

言語やランタイムを問わず動作します（Go、Ruby、Node.js、Python 等）。`exec` は **Linux** 必須です（ptrace は Linux 固有の API）。他のプラットフォームでは `devlb route` でバックエンドを手動登録してください。

## アーキテクチャ

```
cmd/devlb/cmd/       CLI (cobra)
internal/daemon/      Unix ソケットサーバー、JSON プロトコル、クライアント
internal/proxy/       TCP リスナー、ヘルスチェック、メトリクス、HTTP 503
internal/portswap/    ptrace bind() インターセプト
internal/config/      YAML 設定、状態永続化、ファイルウォッチャー
internal/tui/         bubbletea + lipgloss ダッシュボード
internal/exec/        プロセス実行ヘルパー
internal/label/       Git ブランチラベル検出
internal/model/       共有データ型
```

## 開発

```bash
make test       # ユニットテスト
make e2e        # E2E テスト（33 シナリオ）
make lint       # golangci-lint
make fmt        # gofmt
```

## ライセンス

MIT
