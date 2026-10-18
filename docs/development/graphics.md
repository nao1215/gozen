# 画面の開発と検証

Go 1.26.4 以降を使う。T3 の通常テストは表示・音声デバイスを起動しない。実描画は `graphics` tag の別テストで確認する。公開前の確認項目は [リリース点検](release-checklist.md) を参照する。

## オリジナルのサンプル

```sh
go run ./examples/play --mute
go run ./examples/play --help
go run ./examples/play --dev --mute --start camp --seed 7
```

Wayfinder は独自の小さなサンプル。キャンプで会話・練習戦闘を行い、道標を灯して尾根へ進み、保存・復元を試す。隣の部屋ではグリッド移動を試す。遊び方と Xvfb による PNG・GIF の再生成は [サンプルガイド](../examples/wayfinder.md) を参照する。通常起動の保存は OS の config ディレクトリ配下の `gozen/wayfinder-demo`。開始位置指定の試走は保存先を省略すると一時保存になる。素材とフォントの出典は [素材台帳](../examples/play-assets.md) に記載する。

埋込み FS の作り方と終了コードの扱いは `examples/play/main.go`、API の契約は [表示 API](../architecture/presentation-api.md) を参照する。ライブラリがプロセスを終了することはない。

## Requirements

Ebitengine v2.10.4 の通常ビルドは pure Go。race detector には cgo と C コンパイラが必要。Linux ではコンパイル時に cgo を無効化しても、実行時の X11・OpenGL・音声ライブラリは必要になる。

| OS | 必要な準備 |
| --- | --- |
| Linux (Ubuntu 24.04 以降) | `sudo apt-get install libx11-6 libgl1 libglx-mesa0 libxcursor1 libxi6 libxinerama1 libxrandr2 libxrender1 libxext6 libasound2t64`。見えない画面のテストには `sudo apt-get install xvfb xauth`。race には `sudo apt-get install build-essential` |
| Windows | Go をインストールし、OpenGL を扱える GPU ドライバーとログインしたデスクトップを用意する。通常ビルドは C コンパイラ不要。race は Go 対応の MinGW-w64 C コンパイラを PATH に置く |
| macOS | Go とデスクトップ環境を用意する。race 用の C コンパイラは `xcode-select --install` |

録画を使う場合だけ ffmpeg を PATH に置く（Ubuntu: `sudo apt-get install ffmpeg`、macOS/Homebrew: `brew install ffmpeg`、Windows: ffmpeg 配布物の bin を PATH に追加）。Linux の外部入力を使う場合だけ `sudo apt-get install zenity`。無指定時や未導入時の入力はゲーム画面内で行う。モデルや ONNX Runtime はこのサンプルに不要。

CI は SHA 固定の setup-ebitengine で OS の依存を準備する。Linux の Xvfb は次のコマンドで起動する。action のインストールだけでは画面テストを実行したことにはならない。

## 描画の検査

```sh
# Linux
xvfb-run --auto-servernum --error-file=/dev/stderr make graphics
# Windows / macOS（デスクトップ環境で）
go test -tags=graphics ./runner -run TestGraphicsScreens -count=1
```

タイトル・フィールド・会話・戦闘・dungeon の 5 シナリオを別プロセスで起動する。シードを固定し、テスト専用の保存先を使い、Draw 内で PNG を取得する。PNG の復号、960×540 の大きさ、単色でない内容、プロセスの正常終了を検査する。ピクセル完全一致の golden ではなく、配置や文字の見た目の品質は別に確認が必要。音声デバイスはミュートする。

Xvfb や描画環境がない場合は失敗する。skip や単なるコンパイルを描画の成功として扱わない。Xvfb 自体がソケットを作れる実行権限も必要になる。

## 入力スクリプトとトレース

例えば次を `script.txt` に保存する。

```text
wait 80
shot frame
quit
```

```sh
go run ./examples/play --dev --mute --seed 7 --start camp --script script.txt --shots captures
```

`wait N` は N tick 待機、`press <button>` は入力、`shot <basename>` は PNG、`quit` は通常終了。ボタン名は `ok`、`cancel`、`up`、`down`、`left`、`right` 等（正本は `internal/play/input.go` の scriptButtons）。名前にパス区切りを含む画像指定は拒否する。スクリプトには `--shots` の明示指定が必要。

`--trace trace.jsonl` で入力と進行の記録、`--replay trace.jsonl` で再現照合、`--record output.mp4` で録画する。録画はスクリプト・自動操作・再現のいずれかが必要。詳細なフラグは `--help` を参照する。長時間再現と ffmpeg の統合実行は T3 ローカルでは未確認。

メモは `--dev --memos notes` のとき F9 で作成する。画面、状態、注記を記録し、JSON を最後に確定する。ビルド revision の追加は `--build-info`、Linux の外部入力は `--external-input` で個別に有効化する。ソース探索や Git 実行は行わない。
