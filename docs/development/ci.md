# CI の構成

定義は [CI workflow](../../.github/workflows/ci.yml) と [Security workflow](../../.github/workflows/security.yml) を中心に、下の「そのほかの workflow」の表にある workflow です。main への push、pull request、手動実行で検査します。security の govulncheck は毎日 04:00 UTC にも実行し、既存の依存に対する新しい脆弱性情報を検査します（非公開の間は定期実行を省きます）。以下は配線の説明であり、リモートの実行成功記録ではありません。

## 非公開の間は最小

非公開のリポジトリは Actions の時間に上限があります。そのため非公開の間は、push と pull request で `quick`（Linux の vet とテスト）、`lint`、`govulncheck` だけが走ります。ほかのジョブは `github.event.repository.private` で止め、公開後か手動実行（workflow_dispatch）のときだけ走ります。Scorecard と gitleaks は公開リポジトリが前提なので、手動実行でも非公開の間は走りません。

## ジョブと対象

| ジョブ | 環境・検査 | 実行結果として扱う範囲 |
| --- | --- | --- |
| lint | Linux、stable。golangci-lint v2.14.0、go-arch-lint v1.19.0、actionlint v1.7.12。`make lint` | import の方向、API 一覧、静的解析、整形、workflow 構文 |
| unit | Ubuntu・Windows・macOS × Go 1.26.9 / stable。cgo 有効の `go test ./...` | 非描画の契約。`gozen` と internal を含む |
| race | Linux、stable、cgo 有効の `go test -race ./...` | 非描画の契約に対する race 検査 |
| build | 3 OS の amd64、Linux・macOS の arm64。cgo 無効 | 全パッケージのコンパイル。arm64 はクロスビルド。ONNX 推論なし |
| graphics | 3 OS、stable。Linux は setup-ebitengine と Xvfb、Windows/macOS は native desktop | 6 画面の実ループと PNG 検査、動く立ち絵の画素検査、実ゲームを子プロセスで起動する撮影 E2E。音声はミュート |
| cli-e2e | 3 OS、stable、atago v0.25.2 | 実際の `gozen` の全サブコマンドの終了値、標準出力・診断、ファイル変更、生成物。graphics tag は別ジョブへ |
| inference-build | 3 OS、stable、cgo 有効 | ONNX アダプタと推論を含む `gozen` のビルド。モデル未指定の推論テストは理由付き skip |
| examples | 3 OS、stable | `./examples/...` のビルド、minimal と headless の実行。画面の実行は graphics へ |
| govulncheck | Linux、Go 1.26.9（`go.mod` の下限）と stable、cgo 有効、govulncheck v1.9.0 | `./...` から到達する既知の脆弱性。JSON で判定し、`ACCEPTED` に無い物があれば落とす（今は空）。ONNX binding も解析対象 |
| codeql | Linux、stable、cgo 有効 | `./...` を明示してビルドし、Go の静的解析を実施 |

Go の下限は `go.mod:3` です。unit と govulncheck の下限と、更新時に揃えてください。lint と security は解析ツールを実行できる stable を使います。通常テストは画面を初期化しません。graphics は Ebitengine の 1 プロセス 1 ループの制約を守り、シナリオを別プロセスで順番に実行します（`runner/graphics_test.go:18`）。

Windows/macOS の graphics も有効で、失敗を許容する設定や skip はありません。setup-ebitengine が Xvfb を準備するのは Linux だけです。Windows/macOS の hosted runner に必要なデスクトップがあるか、この作業環境からは未確認です。初回リモート実行で確認し、実行不能なら具体的なエラーと理由を記録して再検討します。クロスビルドの成功を代わりの検収にはしません。

## そのほかの workflow

| workflow | いつ | 何を |
| --- | --- | --- |
| [fuzz](../../.github/workflows/fuzz.yml) | 毎日 05:00 UTC と手動。非公開の間は手動だけ | `func Fuzz` を探して 1 つずつ `go test -fuzz` で 5 分探す。落ちた入力は artifact に残す。`go test` は普段から seed と `testdata/fuzz/` を流す |
| [licenses](../../.github/workflows/licenses.yml) | push・pull request・手動。非公開の間は手動だけ | go-licenses v2.0.1 で Linux・Windows・macOS ごとに、リンクする依存のライセンスを判定する。GPL・LGPL・AGPL・特定できない物で落ちる |
| [gitleaks](../../.github/workflows/gitleaks.yml) | pull request。非公開の間は走らない | pull request のコミットに秘密の文字列が無いか |
| [Scorecard](../../.github/workflows/scorecard.yml) | main への push、毎週土曜、手動。非公開の間は走らない | OpenSSF Scorecard の採点を公開し、README のバッジに出す |
| [Benchmark](../../.github/workflows/bench.yml) | pull request と手動。非公開の間は手動だけ | `bench/himorime.yaml` を himorime で測り、pull request では base と head を比べる（[bench/README.md](../../bench/README.md)） |
| [Release](../../.github/workflows/release.yml) | タグ `v*` の push | 成果物を作って 3 OS で実行してから公開する（[リリースの仕組み](release-checklist.md#リリースの仕組み)） |

## ONNX の扱い

通常 CI はモデルも共有ライブラリも取得しません。`inference-build` は `TestONNX` を verbose で選択し、cgo コンパイルの成否とモデル未指定の skip 理由をログに残します。推論を実行したという意味ではありません。ローカルの明示的な推論検査は [制作支援ツール](production-tools.md#検証) の環境変数を設定してください。設定済みで読めないモデルや不適合なモデルは失敗します（`internal/mosaic/command/inference_test.go:62`）。

## 文書サイト

[Website workflow](../../.github/workflows/website.yml) は `website/` を Hugo で組み立て、Pagefind で索引を作り、GitHub Pages（https://nao1215.github.io/gozen/）へ公開します。Hugo と Pagefind は版と SHA256 を固定し、action は SHA で固定します。非公開の間は Actions の予算のため、どのジョブも走りません。公開後の main への push・pull request か、手動実行（workflow_dispatch）のときだけ走ります。公開の書込み権限（`pages: write`、`id-token: write`）は deploy ジョブだけに付け、verify ジョブが公開された頁の commit と主な頁を確かめます。

手元では `make website`（hugo が必要）で `website/public/` に組み立てます。Schema の頁は `docs/schema.md` から組み立て、表の日本語は `website/data/schema_en.yaml` で英語にします。スキーマを変えて訳の無いセルができると、サイトの組み立てが失敗します。

## 権限と依存更新

どの workflow も既定は `contents: read`（Scorecard は `read-all`）、checkout は `persist-credentials: false` です。`pull_request_target` と bot による commit はありません。書込み権限は必要なジョブにだけ付けます。CodeQL の結果送信は codeql ジョブに `security-events: write`、Scorecard は結果の送信と公開に `security-events: write` と `id-token: write`、gitleaks はコミットの一覧に `pull-requests: read`、Release は公開・署名・provenance に `contents: write`・`id-token: write`・`attestations: write` を付けます。秘密を使うのは Release の Homebrew と winget だけです（[ADR 0006](../decisions/0006-platforms-and-ci.md)）。fork PR でも解析は実行し、アップロードは無効にします。private vulnerability reporting と CodeQL の表示には GitHub 側の設定も必要で、このチェックアウトでは設定状態は未確認です。

外部 action は 40 桁 SHA で固定しています。checkout、setup-go、setup-ebitengine は既存の CI の固定値を維持し、CodeQL は既存 OSS の固定値から採用しました。ここに記載するタグ注釈は参照の便宜で、実行対象は SHA です。リモートの到達性はこの段のローカル検査では確認していません。

[Dependabot](../../.github/dependabot.yml) は週次でルートの gomod と github-actions を監視します。提案を自動マージする処理はありません。更新時は action の変更内容、ツールと Go の互換性、モデルを持たない環境での挙動をレビューし、必須の検査を通します。

## ローカルでの確認

[開発と検証](testing.md) にツール導入と Make に対応する直接コマンドがあります。workflow だけの構文検査は `make workflows`、脆弱性検査は次のコマンドです。

```sh
go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
govulncheck ./...                              # go.mod の Go（1.26.9 以降）で
GOTOOLCHAIN=go1.26.9 govulncheck ./...         # 下限の標準ライブラリで
gitleaks git                                   # コミットの秘密の文字列
```

govulncheck は脆弱性 DB へのネットワーク接続が必要です。接続エラーを「脆弱性なし」と解釈しません。CodeQL の抽出・解析・アップロードは GitHub Actions 上で別途確認します。公開前の点検は [リリース点検](release-checklist.md) に従います。
