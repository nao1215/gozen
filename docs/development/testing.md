# 開発と検証

## 通常テストの環境

Go の下限は `go.mod:3` の 1.26.4 です。通常テストは第 1〜6 層の契約、共有 internal、中立のテストゲーム、サンプルの読み込みを検査します。画面・音・推論は初期化しません。通常のテストとビルドに Xvfb、ONNX 共有ライブラリ、モデル、ゲーム素材は不要です。cgo 有効の mosaic ビルドには C コンパイラを使います。一般的な個人環境のパスと私的なホスト名の形式を検出するテストも実行します。これは作品固有の内容を調べる検査の代わりにはなりません。

lint は golangci-lint v2.14.0、依存方向は go-arch-lint v1.19.0（設定の版 3）、workflow の構文検査は actionlint v1.7.12 を使います。Go の作業キャッシュと一時ディレクトリには書込み権限が必要です。制限された環境では `GOCACHE`、`GOTMPDIR`、`GOLANGCI_LINT_CACHE` を書込み可能な場所に向け、先にディレクトリを作ってください。

```sh
go install github.com/fe3dback/go-arch-lint@v1.19.0
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0
go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
```

宣言した依存の版は `go.mod:5` にあります。T3 で go-yaml・Ebitengine・x/image をすべて実際に利用します。依存取得が必要な環境では `go mod download` を実行します。

## Make と直接のコマンド

Windows では Make や Bash がない前提です。リポジトリのルートで、次の直接のコマンドを PowerShell またはコマンドプロンプトから実行できます。外部ツールの `go-arch-lint`・`golangci-lint`・`actionlint` は PATH に入れてください。

| Make | 同じ処理を直接実行 | 現在の対象 |
| --- | --- | --- |
| `make test` | `go test ./...` | 中核・`gozen` と internal・サンプル・API・開発道具の契約テスト |
| `make arch` | `go-arch-lint check` | runtime の import と vendor、深い依存検査 |
| `make api` | `go test ./api -count=1` | golden と現在の公開識別子の照合 |
| `make workflows` | `actionlint` | GitHub Actions workflow の構文・式・シェル検査 |
| `make api-update` | `go run ./internal/dev/apigen` | レビューする一覧の更新 |
| `make lint` | `go-arch-lint check`、`go test ./api -count=1`、`actionlint`、`golangci-lint run ./...`、`golangci-lint fmt --diff` を順に実行 | 依存・API・workflow・静的解析・整形 |
| `make build` | `go build -trimpath -buildvcs=false ./...` | 中核・`gozen` と internal・サンプル・開発道具のコンパイル |
| `make graphics` | `go test -tags=graphics ./runner -run TestGraphicsScreens -count=1` と `go test -tags=graphics ./puppet -run TestGraphics -count=1` | 6 画面の実ループ・PNG 検査と、動く立ち絵の画素検査（1 つのループの中で描いて読む）。Linux は Xvfb で包む |
| `make e2e` | [CLI の直接手順](cli.md#e2e) | `gozen` の全サブコマンドの atago v0.25.2 による契約検査 |
| `make e2e-graphics` | `atago run --ci --parallel 1 --tag graphics e2e/atago`（ビルドと PATH 設定は CLI の手順参照） | ゲーム子プロセスの撮影 E2E。Linux は Xvfb で包む |

`e2e` は T4 で実際のバイナリと atago の検証に置き換えました。`graphics` は T3 の画面テスト、`e2e-graphics` は T4 の CLI 撮影パイプラインです。前提不足による失敗を成功や skip として数えません。

`go build -trimpath -buildvcs=false ./...` はライブラリをコンパイルしますが、`bin/` にゲームや CLI を出すものではありません。`-trimpath` でソースの絶対パスを除き、`-buildvcs=false` で VCS 情報の自動採取を止めます。個別の CLI バイナリは `make cli-build` で `bin/` に生成します。

`make cli-build` と `make e2e-graphics`、CI の個別バイナリ生成では `-ldflags=-compressdwarf=false` も指定します。デバッグ情報を残したまま DWARF の圧縮を無効にし、圧縮データの偶然のバイト列を `strings` による内容照合で文字列として拾う問題を避けます。バイナリは圧縮時より大きくなります。内容検査の対象や判定条件は変えません。

モジュールはルートの 1 つです。`cmd/gozen` と `internal/promptgen`・`internal/mosaic` は同じ `./...` の検査対象です。`tools/` の個別検査や `go.work` は不要です。T5 で制作支援も実装し、通常検査と atago E2E に含めました。

## 描画の検査

描画を行うテストは `graphics` build tag に分けています。Linux は `xvfb-run --auto-servernum --error-file=/dev/stderr make graphics`、Windows・macOS はデスクトップで表の直接コマンドを実行します。6 シナリオを別プロセスで実行し、game loop 内でピクセルを読み、画像の内容と正常終了を検査します。音声はミュートします。

OS ごとの準備、サンプル、入力スクリプトは [画面の開発](graphics.md) にあります。通常テスト、クロスビルド、Xvfb の準備だけを描画の成功とは数えません。T3 では環境制限で実描画の検収が残っていました。T6 で Linux の 5 画面テストとサンプルの撮影を実行しました。Windows・macOS の実描画と実音声は未確認です。

## CLI と ONNX の検査

CLI は `e2e/atago/` に新規の中立 spec を作り、atago で終了コード・stdout/stderr・不正入力・ファイル変更・生成物を確認します。画面を開く spec と通常の spec は分けます。T4 で `e2e/atago/` に新規の中立 spec を追加し、atago v0.25.2 を Make と CI に揃えました。T5 で追加した制作支援も通常の対象に含め、graphics tag の実撮影を分けます。導入と Windows の直接手順は [CLI の検証](cli.md#e2e) を参照してください。

`internal/promptgen`・`internal/mosaic` では推論なしの処理と ONNX を使う検査を分けます。推論を使う検査は別 tag または明示した対象にし、共有ライブラリとモデルがなければ `t.Skip` で不足する前提を説明します。モデルの実パスは利用者が指定し、テストの失敗を無差別に skip へ置き換えません。T5 の ONNX テストはモデルと共有ライブラリの明示指定時だけ実推論します。[設定方法](production-tools.md#検証) を参照してください。モデルは同梱しません。

## CI

T7 の構成・OS・権限・固定したツール・依存更新は [CI の構成](ci.md) にまとめました。通常の lint・unit・build に加え、graphics、`gozen` の atago E2E、独立した examples、Linux の race、govulncheck、CodeQL を配線しています。ONNX の cgo コンパイルとモデル未指定時の skip 理由は独立した inference-build で確認します。定義したジョブと、実際のリモート実行結果を区別してください。公開前は [リリース点検](release-checklist.md) に従って実行結果と未確認事項を確認してください。

## T2 の追加検査

```sh
go test -race ./...
go test ./expr -run '^$' -fuzz '^FuzzParse$' -fuzztime=10s -parallel=2
go run ./examples/headless
```

race 検査には cgo と C コンパイラが必要です。サンプルはメモリ内のオリジナル定義と生成画像を使い、移動・会話・戦闘・保存・復元を実行します。成功時は `location=trail battles=1 wins=1 gold=3` を出します。保存先の OS ファイル操作は `savefile` と `rpg` のテストが `t.TempDir()` で隔離します。

スキーマを変えた場合は `GOZEN_UPDATE_SCHEMA=1 go test ./internal/layer2/gamedata -run '^TestSchemaCoverage$'` で `docs/schema.md` と `data/schema/` を更新し、通常の `go test` で一致を確認します。API 更新とは別の検査です。

これらは描画・実音声・CLI・ONNX の実行検査ではありません。
