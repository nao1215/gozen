# ADR 0016: CLI を gozen の 1 本にする

状態: 一部変更。日付: 2026-10-19。オプションの書き方と help の形は [ADR 0018](0018-gnu-options-and-help.md) で変更しました（下の表の「flag はそのまま」は名前の対応で、書き方は GNU 形式です）。[ADR 0012](0012-cli-ownership-and-output.md) の入口、[ADR 0001](0001-modules-and-layout.md)・[ADR 0009](0009-single-module.md) の CLI の配置、[ADR 0004](0004-content-boundary.md) の制作支援の配置の名前を変更します。

## 利用者と問題

利用者は Gozen でゲームを作る作者です。定義の検査・戦闘のシミュレーション・文書化・部分配布・撮影を使い、必要に応じて制作支援の文面組立てと画像のモザイク処理も使います。

これまでは `rpgcheck`、`rpgtool`、`promptgen`、`mosaic` の 4 本の CLI でした。導入が 4 回必要で、名前から Gozen の道具だと分かりません。一般的な名前のため、他の道具と衝突するおそれもあります。`rpgcheck` と `rpgtool check` はどちらも定義を検査し、どちらを使うべきか迷わせます。help の Requirements、README の節、終了値の規約、Makefile・CI・E2E の手順も 4 か所に分かれていました。

## 決定

CLI は `cmd/gozen` の 1 本にします。導入は `go install github.com/nao1215/gozen/cmd/gozen@latest` です。

| サブコマンド | 元の CLI |
| --- | --- |
| `check`、`layout`、`sim`、`curve`、`floors`、`trace`、`book`、`design`、`memos`、`release`、`trial`、`shots` | `rpgtool` の同名のサブコマンド |
| `check --no-layout` | `rpgcheck` |
| `prompt` | `promptgen`（サブコマンド `lint` と flag はそのまま） |
| `mosaic` | `mosaic`（サブコマンドと flag はそのまま） |
| `version`、`help [command]` | 新規 |

`rpgcheck` は廃止します。`rpgcheck` だけが持っていた振る舞いは、フォントの解析と文字の収まりの検査をしない構造だけの検査と、`Structure checked; layout was not checked.` の行でした。これは `gozen check --no-layout` に移します。終了値（成功 0、検査・I/O の失敗 1、引数の誤り 2）は同じです。`gozen help <command>` は `gozen <command> --help` と同じ出力です。

`prompt` と `mosaic` の引数・flag・出力・終了値は、それぞれ `internal/promptgen/command` と `internal/mosaic/command` が持ちます。`cmd/gozen` はプロセスの終了と振り分けだけを持ちます。`prompt` の終了値もほかのサブコマンドに揃え、help と成功が 0、引数の誤りが 2、それ以外の失敗が 1 です（従来の `promptgen` は引数の誤りも 1 でした）。診断の文も英語に揃えます。

ONNX 推論は既存の build tag（`cgo` と `!cgo` のアダプタ）で分けたままにします。`CGO_ENABLED=0` で組み立てた `gozen` でも、help、手修正だけの画像処理、既存 report の検査は使えます。推論が必要な処理では、この組み立てに推論が無いことと入れ直し方を stderr に出し、終了値 1 で終わります。Go は C コンパイラが見つからない場合に既定で cgo を無効にするため、C コンパイラの無い環境でも `go install` は推論なしの版として成功します。

古い名前の互換は残しません。別名のバイナリや旧 `cmd/` のパッケージは作りません。まだリリースがなく、外部の利用者もいないためです。

README の CLI の節は `#gozen` の 1 つにします。help の Requirements は、サブコマンドごとに実際に必要な物だけを書き、同じ節へ案内します。

依存検査では `commands` component を `cmd/gozen/**` と `cmd/internal/**` にします。`commands` から `promptgen`・`mosaic` component への依存を許可します。制作支援の内部から `commands` への依存と、ライブラリ・サンプル・実験から制作支援の内部への依存は、これまでどおり許可しません。

## 代案

1. cgo が要る物だけを別のバイナリにする案。`gozen-mosaic` を別に置き、`gozen mosaic` は PATH 上の `gozen-mosaic` を実行します（git や kubectl のプラグインの形）。本体を cgo なしに保てます。しかし導入が 2 回になり、版の不一致や PATH の探索という失敗の経路が増えます。cgo なしで組み立てた `gozen` も推論以外は動き、推論が要る時だけ入れ直し方を案内するので、別のバイナリにする利点は小さいです。そのため 1 本にします。
2. 画像系（`prompt` と `mosaic`）を制作支援の別のバイナリにまとめる案。汎用の道具と制作支援を名前で分けられます。しかし利用者は同じ作者で、導入と help の規約が 2 本に分かれる問題が残ります。内容の境界は配置（internal のパッケージ）で守れるため採りません。
3. 全部を 1 本にし、推論のために cgo を必須にする案。C コンパイラの無い環境では `gozen` 自体を組み立てられなくなります。検査やシミュレーションだけを使う利用者にも C コンパイラを要求するため採りません。この決定は、cgo を任意にした 1 本です。

## 影響

- 4 本分の導入・ビルド・E2E の手順が 1 本になります。Makefile の `cli-build` は `bin/gozen` だけを作ります。CI の E2E と推論ビルドも `./cmd/gozen` を対象にします。
- cgo を有効にして組み立てた `gozen` は ONNX の binding を含みます。共有ライブラリは実行時に `--ort` か `ONNXRUNTIME_LIB` で読み込むため、推論を使わない処理には共有ライブラリもモデルも不要です。
- 汎用の CLI の入口から制作支援の実装へ依存が生じます。[ADR 0004](0004-content-boundary.md) の許可は `internal/promptgen`・`internal/mosaic`（それぞれの `command` を含む）に限ります。`cmd/gozen` は振り分けと、中立な一覧の help だけを持ちます。
- [ADR 0012](0012-cli-ownership-and-output.md) の出力先・終了値・上書き防止の規約は、`prompt` を含む `gozen` の全サブコマンドに引き継ぎます。
- リリースの成果物（goreleaser のアーカイブ、Linux のパッケージ、Homebrew、winget）は `CGO_ENABLED=0` で組み立て、推論を含みません。6 つの OS・CPU の組み合わせに cgo のクロスコンパイラを持たないためです。推論が要る作者は、C コンパイラのある環境で `CGO_ENABLED=1 go install github.com/nao1215/gozen/cmd/gozen@latest` で入れ直します。推論の要る処理の診断がこの入れ直し方を出します（`internal/mosaic/detect/inference_nocgo.go`）。
- 推論の Pure Go 化（ONNX Runtime を WebAssembly にして Pure Go の実行環境で動かす案）は 2026-10-19 に試し、精度は同等でしたが、タグ付けが 48 倍遅いなど速さが足りないため見送りました。推論は cgo の版のままとし、build tag による分岐と `CGO_ENABLED=0` の版の診断を保ちます。
