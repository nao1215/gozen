# 制作支援 CLI

`gozen prompt` と `gozen mosaic` は `gozen` の制作支援のサブコマンドです（[ADR 0016](../decisions/0016-single-cli.md)）。入口は `cmd/gozen/main.go` の振り分けだけで、引数・出力・終了値は `internal/promptgen/command`・`internal/mosaic/command` が持ちます。ライブラリの公開 API は増やしません。

## 導入と OS 別の要件

公開後の導入経路は次のとおりです。公開版に対する `go install …@latest` の成功はまだ未確認です。

```sh
go install github.com/nao1215/gozen/cmd/gozen@latest
```

`gozen prompt` は Go 1.26.4 以降だけでビルドできます。実行時には利用者の catalog YAML が必要です。C コンパイラ、推論モデル、画面、画像生成サービスは不要です。

`gozen mosaic` の推論には、`gozen` のビルド時の cgo と C コンパイラ、実行時の ONNX Runtime 共有ライブラリと利用者の ONNX モデルが必要です。Go binding は `onnxruntime_go v1.36.0` で、同梱 C ヘッダは ONNX Runtime 1.29.0 の API を使います。共有ライブラリは対応する版・OS・CPU の物を選びます。モデル、共有ライブラリ、学習データは Gozen に同梱しません。モデルの利用条件は利用者が確認します。

| OS | C コンパイラの準備例 | `--ort` に渡す物 |
| --- | --- | --- |
| Ubuntu / Debian | `sudo apt-get install build-essential` | 配布アーカイブの `lib/libonnxruntime.so.1.29.0` |
| macOS | `xcode-select --install` | 配布アーカイブの `lib/libonnxruntime.1.29.0.dylib` |
| Windows | MSYS2 の MinGW64 シェルで `pacman -S mingw-w64-x86_64-gcc`。同じコンパイラの bin を Go を実行するシェルの PATH に追加 | 配布アーカイブの `lib/onnxruntime.dll` と必要な付属 DLL |

共有ライブラリは [ONNX Runtime のリリース](https://github.com/microsoft/onnxruntime/releases)、binding の版の契約は [v1.36.0 の README](https://github.com/yalue/onnxruntime_go/tree/v1.36.0#readme) を参照してください。これらは手順の案内で、3 OS でのローカル導入検証の記録ではありません。

```sh
# 推論を含むネイティブビルド。先に C コンパイラを PATH に追加する。
CGO_ENABLED=1 go install github.com/nao1215/gozen/cmd/gozen@latest
CGO_ENABLED=1 go build -trimpath -buildvcs=false -o bin/gozen ./cmd/gozen
# 推論なしでの利用・クロスビルドも可能。
CGO_ENABLED=0 go build -trimpath -buildvcs=false -o bin/gozen ./cmd/gozen
```

PowerShell では先に `$env:CGO_ENABLED = '1'` または `'0'` を設定して同じ `go build` を実行します。Go は C コンパイラが見つからないと既定で cgo を無効にするため、C コンパイラの無い環境の `go install` は推論なしの版になります。cgo 無効版でも help、手修正だけの画像処理、既存 report の検査は使えます。推論が必要になれば、この組み立てに推論が無いことと入れ直し方を stderr に出して終了値 1 にします。Xvfb はどちらのサブコマンドにも不要です。

## gozen prompt

```sh
go run ./cmd/gozen prompt --help
go run ./cmd/gozen prompt --config docs/examples/production/catalog.yaml --scene landscape --format json
go run ./cmd/gozen prompt --config docs/examples/production/catalog.yaml --plan docs/examples/production/plan.yaml
go run ./cmd/gozen prompt lint --config docs/examples/production/catalog.yaml --format json
```

設定は版 1 の catalog です。層、項目 ID、既定の選択、依存・排他・置換、優先度、強調、負語数の上限、独立ブロック、構図、検証規則を定義できます。`--<layer> ID` は設定の層ごとの選択を上書きし、繰り返して複数項目を選べます。`--scene` と `--plan` は同時に指定できません。自由文の位置引数は受け付けません。

多数の構図は `--scenes ids.txt --format json` でまとめて組み立てます。入力は1行1 ID（空行は無視）で、設定は一度だけ読み込みます。出力は入力順の JSON Lines で、各行に `id`・`result` と失敗時の `error` を持ちます。個別の失敗後も続行し、1件でも失敗すれば終了値は1です。書込み失敗は直ちに返します。`lint`・`--scene`・`--plan`・層の上書きとは併用できません（`internal/promptgen/command/batch.go:15`）。

SDXL の強調は弱い順に 0.9、1.0、1.1、1.2、1.5 の 5 段階で、生成と lint の重みの上限は 1.5 です。SDXL の整形を提供し、語の重複統合・正負の衝突解消・上限による脱落を JSON に出所付きで記録します。未知の設定欄、複数 YAML 文書、不正な参照、制御構文を拒否します。語数上限とトークン近似は別です。近似はモデルの tokenizer そのものではありません。人物層を選ぶ場合の年齢制約はツールが所有し、設定による置換や負語数の上限では外せません。

`lint` は未選択項目・依存先・構図も検査します。error は失敗、warning のみは成功です。help は設定が存在しなくても成功し、モデルや外部サービスへ接続しません。終了値は正常・help が 0、引数の誤りが 2、それ以外が 1 です。`--config` には短い名前 `-c` があります。層の名前が固定のオプション（`config`・`scene`・`plan`・`scenes`・`format`・`help`）と同じ catalog は、層の上書きを作れないため失敗にします。

`gozen prompt` は入力文面の組立てに責務を限定します。画像生成サービスへの送信は行わず、利用者が出力 JSON を任意の生成系へ渡せます。

## gozen mosaic

```sh
go run ./cmd/gozen mosaic --help
go run ./cmd/gozen mosaic auto --help
go run ./cmd/gozen mosaic manual help
# model.onnx と共有ライブラリの場所は利用者が用意する。
go run ./cmd/gozen mosaic auto --model model.onnx --ort runtime-library --out processed game image.png
```

引数の形は `auto [flags] <game-dir> [asset-relative-images...]`、`inspect [flags] <game-dir> [images...]`、`manual <sub> [flags] <game-dir> [image]` です。オプションは GNU 形式で、位置引数の前後どちらにも置けます。画像は `<game-dir>/assets/` から読み、指定省略時は YAML の画像参照を列挙します。手修正は `--fix` と `--manual`、タグキャッシュは `--tags` で明示します。対応する `MOSAIC_*` 環境変数も help に表示します。

検出モデルは YOLO の `images` 入力、`output0` 出力 `[1, 4+クラス数, N]` を使います。クラス順は `internal/mosaic/settings/settings.go` に定義します。`--face` は任意の顔検出モデル、`--shape mask --seg model.onnx` は任意の分割モデル、`--tagger dir` は `model.onnx` と `selected_tags.csv` を持つ WD 形式のディレクトリです。明示した補助モデルが読めない場合は失敗にし、別の処理へ黙って切り替えません。モデルの自動探索、ダウンロード、制作環境のキャッシュ探索は行いません。

既定の検出・補助判定のしきい値は汎用の 0.5、形状は矩形です。用途に応じて利用者が調整します。信頼度は `--threshold`・`--sure`、形状は `--shape`、余白は `--margin`・`--pad`・`--min`、マスク拡張は `--dilate` で選べます。モデルの推論や自動判定は人による出力確認の代わりにはなりません。

画像処理は元の画素のセル平均によるピクセル化です。強度はブロックの画素数で、0 は `max(4, ceil(長辺/100))`、1 は画素値を変えない指定です。JPEG quality は圧縮品質で、強度とは別です。矩形・多角形・楕円・マスクの複数領域に対応し、同じセルは一度だけ元画像から平均します。境界をまたぐセル全体を塗り、端は画像範囲で切ります。低水準のセル列挙でも block 0 は同じ自動値です。

`--rect x,y,w,h` は正の幅高、`--box x0,y0,x1,y1` は昇順端点を要求します。部分的な画像外領域は切り詰め、完全に画像外・ゼロサイズ・逆順は失敗にします。`--norm` は 0〜1、数値は有限である必要があります。NaN/Inf、未知の YAML 欄、負の強度、過大な座標を拒否します。CLI の block は 0〜256、quality は 1〜100 です。

入力は PNG/JPEG/WebP。出力は PNG/JPEG です。WebP の入力は `<元の名前>.png` として PNG を出し、report の `output` に実際の名前を記録します。`inspect` と `manual view --src masked` はこの欄を使います。元の WebP 名に JPEG の中身を書き込む経路はありません。

手修正のサブコマンドは `list/show/view/add/drop/grow/skip/check/clear` です。`grow --rect` と `--region-id` が使えます。変更後は画像・report・確認図・YAML を一時領域で完成させてから反映します。推論・画像処理失敗時は以前の YAML と出力を維持します。これはプロセス内での反映手順であり、停電や外部プロセスによる同時書換えをまたぐトランザクションではありません。

`auto --dry-run` と `inspect` は画像・report・タグキャッシュを書きません。タグキャッシュは読むだけで、欠けたタグを新たに推論しません。inspect の `none/todo/done/review` は JSON 内の状態です。未処理・要確認を返しただけでは終了値を失敗にしません。画像内の既存格子の検査は厳密な等色セルの補助的な検出で、JPEG 等の非可逆圧縮では見つからない場合があります。

出力は assets と重ならない場所に限定し、シンボリックリンク経由の重なりやパストラバーサルも拒否します。出力木内のシンボリックリンクを拒否し、出力ファイルは一時ファイル経由で置換します。追跡した出力だけを片付け、無関係な画像を掃除対象にしません。バッチは途中失敗までの出力を残します。終了値は正常・help が 0、不正なサブコマンドやフラグ構文が 2、処理・前提不足・I/O 失敗が 1 です。

### 合成画像での実行

以下の入力・catalog・plan はこの段で新規作成しました。画像は Python 標準ライブラリで生成する 24×24 の RGB 勾配で、第三者素材はありません。WebP の回帰用 fixture も同じ勾配から生成し、任意の `--webp` 引数で Pillow を使って lossless WebP に変換できます（再生成時だけ `python3 -m pip install Pillow==12.1.1` が必要）。生成元と画像にはリポジトリの MIT ライセンスを適用します。

```sh
python3 docs/examples/production/generate_fixture.py
# 作業用 game/assets/ に pattern.png をコピーし、fixes.yaml を指定する。
go run ./cmd/gozen mosaic auto --fix docs/examples/production/fixes.yaml --out processed game pattern.png
go run ./cmd/gozen mosaic inspect --fix docs/examples/production/fixes.yaml --out processed game pattern.png
```

`fixes.yaml` は自動検出を使わず複数の矩形だけを処理するため、モデルや共有ライブラリなしで動きます。原画像のコピー方法を含む実行可能な検査は `e2e/atago/production.atago.yaml` にあります。

## 検証

通常の `make test lint build` と `make e2e` に `gozen` の全サブコマンドと内部実装を含めます。cgo 無効の全体ビルドは推論の実行検証には数えません。CI の `inference-build` は 3 OS の cgo ビルドを分け、モデル未指定時は推論テストの理由を表示します。

```sh
go test -v ./internal/mosaic/command -run TestONNX -count=1
```

実推論を検査する場合は `ONNXRUNTIME_LIB` に共有ライブラリを指定し、検出は `GOZEN_MOSAIC_MODEL`、分割は `GOZEN_MOSAIC_SEG_MODEL`、タグは `GOZEN_MOSAIC_TAGGER_DIR` を設定します。各テストはモデルとライブラリの未指定時だけ理由付きで skip します。指定済みで読めない・形が不適合な場合は失敗です。入力は常に新規の合成画素です。実推論の結果は、モデル・共有ライブラリ・OS を含めて記録し、前提不足による skip と区別してください。
