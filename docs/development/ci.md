# CI の構成

定義は [CI workflow](../../.github/workflows/ci.yml) と [Security workflow](../../.github/workflows/security.yml) です。main への push、pull request、手動実行で検査します。security は毎週月曜 02:30 UTC にも実行し、既存の依存に対する新しい脆弱性情報を検査します。以下は配線の説明であり、リモートの実行成功記録ではありません。

## ジョブと対象

| ジョブ | 環境・検査 | 実行結果として扱う範囲 |
| --- | --- | --- |
| lint | Linux、stable。golangci-lint v2.14.0、go-arch-lint v1.19.0、actionlint v1.7.12。`make lint` | import の方向、API 一覧、静的解析、整形、workflow 構文 |
| unit | Ubuntu・Windows・macOS × Go 1.26.4 / stable。cgo 有効の `go test ./...` | 非描画の契約。`gozen` と internal を含む |
| race | Linux、stable、cgo 有効の `go test -race ./...` | 非描画の契約に対する race 検査 |
| build | 3 OS の amd64、Linux・macOS の arm64。cgo 無効 | 全パッケージのコンパイル。arm64 はクロスビルド。ONNX 推論なし |
| graphics | 3 OS、stable。Linux は setup-ebitengine と Xvfb、Windows/macOS は native desktop | 5 画面の実ループと PNG 検査、実ゲームを子プロセスで起動する撮影 E2E。音声はミュート |
| cli-e2e | 3 OS、stable、atago v0.25.2 | 実際の `gozen` の全サブコマンドの終了値、標準出力・診断、ファイル変更、生成物。graphics tag は別ジョブへ |
| inference-build | 3 OS、stable、cgo 有効 | ONNX アダプタと推論を含む `gozen` のビルド。モデル未指定の推論テストは理由付き skip |
| examples | 3 OS、stable | `./examples/...` のビルド、minimal と headless の実行。画面の実行は graphics へ |
| govulncheck | Linux、stable、cgo 有効、govulncheck v1.8.0 | `./...` の既知の脆弱性検査。ONNX binding も解析対象 |
| codeql | Linux、stable、cgo 有効 | `./...` を明示してビルドし、Go の静的解析を実施 |

Go の下限は `go.mod:3` です。unit の下限と更新時に揃えてください。lint と security は解析ツールを実行できる stable を使います。通常テストは画面を初期化しません。graphics は Ebitengine の 1 プロセス 1 ループの制約を守り、シナリオを別プロセスで順番に実行します（`runner/graphics_test.go:18`）。

Windows/macOS の graphics も有効で、失敗を許容する設定や skip はありません。setup-ebitengine が Xvfb を準備するのは Linux だけです。Windows/macOS の hosted runner に必要なデスクトップがあるか、この作業環境からは未確認です。初回リモート実行で確認し、実行不能なら具体的なエラーと理由を記録して再検討します。クロスビルドの成功を代わりの検収にはしません。

## ONNX の扱い

通常 CI はモデルも共有ライブラリも取得しません。`inference-build` は `TestONNX` を verbose で選択し、cgo コンパイルの成否とモデル未指定の skip 理由をログに残します。推論を実行したという意味ではありません。ローカルの明示的な推論検査は [制作支援ツール](production-tools.md#検証) の環境変数を設定してください。設定済みで読めないモデルや不適合なモデルは失敗します（`internal/mosaic/command/inference_test.go:62`）。

## 文書サイト

[Website workflow](../../.github/workflows/website.yml) は `website/` を Hugo で組み立て、Pagefind で索引を作り、GitHub Pages（https://nao1215.github.io/gozen/）へ公開します。Hugo と Pagefind は版と SHA256 を固定し、action は SHA で固定します。非公開の間は Actions の予算のため、どのジョブも走りません。公開後の main への push・pull request か、手動実行（workflow_dispatch）のときだけ走ります。公開の書込み権限（`pages: write`、`id-token: write`）は deploy ジョブだけに付け、verify ジョブが公開された頁の commit と主な頁を確かめます。

手元では `make website`（hugo が必要）で `website/public/` に組み立てます。Schema の頁は `docs/schema.md` から組み立て、表の日本語は `website/data/schema_en.yaml` で英語にします。スキーマを変えて訳の無いセルができると、サイトの組み立てが失敗します。

## 権限と依存更新

両 workflow の既定は `contents: read`、checkout は `persist-credentials: false` です。`pull_request_target`、秘密を必要とする実行、bot による commit、公開・デプロイの処理はありません。CodeQL の結果送信だけ、codeql ジョブに `security-events: write` を付けます（[ADR 0006](../decisions/0006-platforms-and-ci.md)）。fork PR でも解析は実行し、アップロードは無効にします。private vulnerability reporting と CodeQL の表示には GitHub 側の設定も必要で、このチェックアウトでは設定状態は未確認です。

外部 action は 40 桁 SHA で固定しています。checkout、setup-go、setup-ebitengine は既存の CI の固定値を維持し、CodeQL は既存 OSS の固定値から採用しました。ここに記載するタグ注釈は参照の便宜で、実行対象は SHA です。リモートの到達性はこの段のローカル検査では確認していません。

[Dependabot](../../.github/dependabot.yml) は週次でルートの gomod と github-actions を監視します。提案を自動マージする処理はありません。更新時は action の変更内容、ツールと Go の互換性、モデルを持たない環境での挙動をレビューし、必須の検査を通します。

## ローカルでの確認

[開発と検証](testing.md) にツール導入と Make に対応する直接コマンドがあります。workflow だけの構文検査は `make workflows`、脆弱性検査は次のコマンドです。

```sh
go install golang.org/x/vuln/cmd/govulncheck@v1.8.0
govulncheck ./...
```

govulncheck は脆弱性 DB へのネットワーク接続が必要です。接続エラーを「脆弱性なし」と解釈しません。CodeQL の抽出・解析・アップロードは GitHub Actions 上で別途確認します。公開前の点検は [リリース点検](release-checklist.md) に従います。
