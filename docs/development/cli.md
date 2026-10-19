# 汎用 CLI の導入と使い方

CLI は `gozen` の 1 本です（[ADR 0016](../decisions/0016-single-cli.md)）。公開済みのリリースは未確認です。公開後のインストールは次の形です。

```sh
go install github.com/nao1215/gozen/cmd/gozen@latest
```

ソースからは `go run ./cmd/gozen --help` で使えます。`gozen help <command>` は `gozen <command> --help` と同じ出力です。`gozen version` は導入した版を表示します（ソースからの組み立てでは `devel` などになります）。制作支援の `gozen prompt` と `gozen mosaic` は [制作支援 CLI](production-tools.md) に記載します。ビルド済み CLI の終了値は 0 が成功・help、1 が検査・I/O・実行失敗、2 が引数形式の不正です（`gozen prompt` だけは引数の誤りも 1）。`go run` 自体の終了値は CLI の終了値と一致しない場合があるため、E2E はバイナリを直接実行します。

## Requirements

通常の検査・レイアウト・シミュレーション・文書化・配布木の生成・文面の組立てには Go 以外の外部ツール、cgo、C コンパイラ、ONNX Runtime、モデル、画面、音声デバイスは不要です。`gozen mosaic` の推論だけは cgo で組み立てた `gozen` と ONNX Runtime の共有ライブラリ、利用者のモデルが必要です（[導入と OS 別の要件](production-tools.md#導入と-os-別の要件)）。インストール後は Go 自体も実行時に不要です。フォントや画像はゲームの FS に利用者が用意します。

| OS | 通常コマンド | shots でゲームを起動するとき |
| --- | --- | --- |
| Linux | Go 1.26.4 以降でビルド | X11・OpenGL の実行環境。無人実行は Ubuntu で `sudo apt-get install xvfb xauth`。その他のライブラリは [画面の導入手順](graphics.md#requirements) |
| Windows | Go 1.26.4 以降。Make、Bash、C コンパイラは不要 | GPU ドライバーとログインしたデスクトップ |
| macOS | Go 1.26.4 以降。C コンパイラは不要 | デスクトップ環境 |

`shots -bin` には Gozen の runner の起動フラグを持つゲームを指定します。外部コマンドやモデルの自動インストールは行いません。起動失敗は撮影結果の reason と `_work/<番号>_<id>/game.stderr.log` に残り、README の Requirements への案内と終了値 1 を返します。help はこれらの前提を要求しません。

## コマンド

OS のパスは引用して渡せます。フラグは位置引数の前後に置けます。`--` より後は全て位置引数で、先頭が `-` のディレクトリ名も指定できます。FS 内の相対パスを受け取る `trial -config` と `release -prefix` は `/` 区切りです。上位への移動や Windows の予約名は拒否します。

| コマンド | 入力と結果 |
| --- | --- |
| `gozen check <game-dir>` | 定義・参照・式・素材の存在と文字レイアウトを検査。警告は stderr |
| `gozen check -no-layout <game-dir>` | フォントの解析と文字レイアウトを省き、構造だけを検査。レイアウトは未検査と stdout に明示 |
| `gozen layout <game-dir>` | ゲームのフォントと表示規則で会話・選択肢の文字の収まりを検査。警告だけなら 0 |
| `gozen sim <game-dir> <troop> -n 100 -seed 7` | AUTO 戦闘の勝敗・平均ターン。party・level・gear・turns 等は help。`-state <file>` は保存データ（F9 のメモの `.save`）の一行を全回復して始め、`-hp` は勝った後の残り HP の帯を出す。開発用のバランス表示（F7）と同じ数になる |
| `gozen curve <game-dir> <location> -floors 3 -battles 5` | 階別の成長と戦闘集計。boss・shops・rest・supplies 等を指定可能 |
| `gozen floors <game-dir> <location> -floor 1 -try 3` | 生成迷宮の地図・歩数・遭遇の目安とシード比較 |
| `gozen trace <trace.jsonl> -only say,battle` | JSONL を時系列表示。`-check` は立ち絵と音楽の記録を検査し、不備があれば 1 |
| `gozen book <game-dir>` | 定義とイベントの Markdown を stdout へ出力 |
| `gozen design <game-dir> -out <new-dir>` | `designeval.yaml` の遊び手・報酬・時間係数に従い、summary.json・timeline・band.png・report.md を生成 |
| `gozen memos <memo-dir> -out <new-dir>` | メモから HTML とサムネイルを生成。省略時は `<memo-dir>-html`。元のメモは変更しない |
| `gozen release <game-dir> <new-dir> -overlay <dir> -inspect <file>` | 承認記録と参照素材を検査して配布木を生成 |
| `gozen trial <game-dir> <new-dir>` | `trial.yaml` の範囲から到達可能な定義と素材だけを抽出。`-overlay` と `-inspect` は組で指定 |
| `gozen shots <game-dir> <list.yaml> -bin <game> -raw -out <new-dir>` | 一覧に従ってゲームを起動し PNG・JPEG・HTML・shots.json を生成 |

生成先は空ディレクトリも含め既存のものを拒否します。生成先を入力の内側・親にしないでください。出力ファイル `trial -list` も既存ファイルを拒否します。`trial -n` は `-list` を指定しても書き込みません。設定の中立な実行例は `e2e/atago/fixtures/game/{trial,designeval}.yaml` です。

`design` のメモ生成は `-memos <dir>` を指定した場合だけです。Git やソースの履歴を収集せず、summary の hash は空です。シミュレーション中のエラーはレポートを残して終了値 1 にします。

撮影一覧の最小例です。

```yaml
shots:
  - id: camp
    title: Camp overview
    start: camp
    wait: 30
```

`id` は一つの安全なファイル名要素です。人物 `who`、段階 `phase`、構図 `pose` は任意の一般的な注記です。`scene`・`event`、`picture`・`line`・`answers`、`script` も指定できます。`-raw` と `-overlay` は択一です。`-overlay` はゲーム側の `asset-policy.yaml` に従って承認を照合し、承認待ちも終了値 1 にします。検査なしの撮影結果にはその旨を HTML に明記します。保存先は撮影ごとの作業木に隔離します。

## E2E

atago は v0.25.2 を使います。

```sh
go install github.com/nao1215/atago@v0.25.2
make e2e
# 画面環境を用意した Linux だけ
xvfb-run --auto-servernum --error-file=/dev/stderr make e2e-graphics
```

atago 自体は Gozen より新しい Go toolchain を必要とする場合があり、導入時は Go の自動 toolchain 選択を有効にするか stable を使います。Gozen の下限は変更しません。

Windows PowerShell では次の手順で通常 E2E を実行できます。推論を含む cgo ビルドには [C コンパイラ](production-tools.md#導入と-os-別の要件) が必要です。推論なしの検査は `$env:CGO_ENABLED = '0'` を先に設定して実行できます。

```powershell
New-Item -ItemType Directory -Force bin, build/atago | Out-Null
go build -trimpath -buildvcs=false -ldflags=-compressdwarf=false -o bin/gozen.exe ./cmd/gozen
if ($LASTEXITCODE -ne 0) { throw "gozen build failed" }
$env:PATH = (Join-Path $PWD bin) + [IO.Path]::PathSeparator + $env:PATH
Push-Location build/atago
try {
    atago run --ci --parallel 4 --skip-tag graphics ../../e2e/atago
    if ($LASTEXITCODE -ne 0) { throw "CLI E2E failed" }
} finally { Pop-Location }
```

通常の spec はシェルに依存せず、`gozen` の全サブコマンドの 65 シナリオで終了値、stdout/stderr、決定性、書込みの有無、生成物と上書き防止を検査します。`graphics` tag の撮影 spec は別に実行します。単体テストの撮影子プロセスは PNG と trace を返す模擬実装で、実ゲームの描画検証には数えません。

macOS の直接手順は同じビルドコマンドから `.exe` を外し、`bin` を PATH に追加して `build/atago` から同じ atago コマンドを実行します。Windows・macOS の実行結果とリモート CI はローカルでは未確認です。公開前の点検は [リリース点検](release-checklist.md) に従ってください。
