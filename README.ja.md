# Gozen

[English](README.md) · [MIT ライセンス](LICENSE) · [素材の出典](ASSETS.md)

## 概要

Gozen は Go と Ebitengine による 2D RPG フレームワークと制作ツールです。YAML でゲームを定義し、自分のファイルシステムを埋め込んで、同じ規則を画面付きでも描画なしでも実行できます。開発中で、v0 の API を整備しています。

![Gozen のオリジナルサンプル Wayfinder](docs/images/wayfinder.gif)

[Wayfinder](docs/examples/wayfinder.md) は会話・戦闘・イベント・移動・保存を試せる小さなゲームです。絵は Go のコードで描き、文面と地図も新規に作りました。[大きなスクリーンショット](docs/images/wayfinder.png)。

## 機能

- YAML 定義、参照検証、式、条件付きイベント。
- ターン制戦闘、場所の移動、グリッド探索、版付きのセーブ。
- 画面と同じ規則を使う描画なしのシミュレーション。
- Ebitengine の画面、フォント描画、音声、埋込みゲームの起動口。
- 検証・シミュレーション・制作・画像処理を行う 4 つの CLI。

## インストール

Go 1.26.4 以降を使います。ソースのチェックアウトから 4 つの CLI を導入できます。

```sh
mkdir -p bin
GOBIN="$PWD/bin" go install -trimpath -buildvcs=false ./cmd/rpgcheck ./cmd/rpgtool ./cmd/promptgen ./cmd/mosaic
```

`bin` を PATH に追加するか、後述の `go run` で使います。PowerShell では `$env:GOBIN = Join-Path $PWD bin` を設定してから同じ `go install` を実行します。モジュールは `github.com/nao1215/gozen` の 1 つで、CLI も同じ版です。公開版からの導入は未確認です。リリース後は `go install github.com/nao1215/gozen/cmd/rpgtool@latest` の形で、名前を各 CLI に替えて導入します。

## 動作環境

サンプルとツールのビルドには Go 1.26.4 以降が必要です。画面付きゲームはディスプレイと OS の描画ライブラリを使います。`--mute` で音声デバイスの初期化を省けます。Linux の X11・OpenGL は pure Go ビルドでも実行時に必要です。Ubuntu のパッケージと Windows/macOS の準備は [画面の導入手順](docs/development/graphics.md#requirements) にあります。

| 対象 | 追加の前提 |
| --- | --- |
| 中核・描画なしの例・rpgcheck・rpgtool の通常機能 | なし。画面・C コンパイラ不要 |
| 画面付きゲーム・rpgtool shots | 描画環境。Linux の無人実行は Xvfb と xauth |
| promptgen | 利用者の catalog YAML。モデル・画像生成サービス不要 |
| mosaic の推論 | ビルド時に cgo と C コンパイラ。実行時に対応する ONNX Runtime と利用者のモデル |
| 紹介画像の撮影 | Linux の描画環境、Xvfb、xauth、Bash、Python 3、ffmpeg/ffprobe |

mosaic の help と手修正だけの画像処理は ONNX 不要です。cgo 無効のビルドでは推論できません。[制作ツールの OS 別要件](docs/development/production-tools.md#導入と-os-別の要件)。

## クイックスタート

リポジトリのルートから実行します。

```sh
go run ./examples/minimal
# location=camp gold=2
go run ./examples/headless
# location=trail battles=1 wins=1 gold=3
# 画面環境を用意して実行:
go run ./examples/play --mute
```

Linux で画面を表示せずに実行・終了まで確かめるには、後述の撮影コマンドを使います。対話起動はウィンドウを閉じるまで動きます。

## 最小構成

[examples/minimal/main.go](examples/minimal/main.go) は小さな Go の起動コードです。`data.Load(files, ".")`、`rpg.New(db, nil)`、`headless.New(db, state).Start()` で埋込み定義を読み、最初の到着イベントを実行します。定義は次の 2 ファイルです。

```yaml
# game.yaml
schema_version: 1
id: tiny-trail
title: Tiny trail
start: {location: camp, party: [walker]}
```

```yaml
# data/world.yaml
classes:
  walker:
    name: Walker
    stats: {mhp: 20, mmp: 0, atk: 4, def: 2, mat: 0, mdf: 0, agi: 4, luk: 1}
    exp_for: level * 20
actors: {walker: {name: Walker, class: walker}}
locations:
  camp:
    name: Camp
    on_enter: [{say: A new trail begins here.}, {gold: 2}]
skills:
  attack: {name: Tap, effects: [{damage: 4}]}
  guard: {name: Wait, scope: self}
```

戦闘しないゲームでも既定の `attack`・`guard` スキル ID を定義します。この例は素材不要で、セーブも書き込みません。

## サンプルの起動

```sh
go run ./examples/play --help
go run ./examples/play --mute
```

**New journey** から **Talk: Courier** を選びます。**Practice** で **Auto** または手動の攻撃を使って勝ち、**Light the marker**、**Walk to the ridge** と進みます。**Save journey** で保存し、タイトルの **Continue** で復元できます。**Explore chamber** はグリッド移動の部屋です。[遊び方と撮影](docs/examples/wayfinder.md)。

Linux で PNG と 9 秒の GIF スライドショーを作り直せます。

```sh
xvfb-run --auto-servernum --error-file=/dev/stderr bash scripts/capture-example.sh
```

## CLI 一覧と例

以下はリポジトリのルートから実行します。検証・シミュレーションには、埋込みの画面サンプルとは別の中立な fixture を使います。[rpgtool の全サブコマンド](docs/development/cli.md#コマンド)。

### rpgcheck

Requirements: ビルドに Go。cgo・画面・外部の実行ツールは不要です。

```sh
go run ./cmd/rpgcheck e2e/atago/fixtures/game
```

定義・参照・式・素材の存在を検査します。文字の収まりは rpgtool で別に検査します。

### rpgtool

Requirements: ビルドに Go。通常の検査・シミュレーション・レポートは画面・C コンパイラ不要です。`shots` はゲームのバイナリと描画環境を使います。[OS 別要件](docs/development/cli.md#requirements)。

```sh
go run ./cmd/rpgtool --help
go run ./cmd/rpgtool sim e2e/atago/fixtures/game lesson -n 3 -seed 7
```

### promptgen

Requirements: Go と catalog YAML。cgo・モデル・外部サービス不要です。文面を組み立てます。画像の生成やジョブの送信は行いません。

```sh
go run ./cmd/promptgen --config docs/examples/production/catalog.yaml --scene landscape --format json
```

### mosaic

Requirements: Go。推論には C コンパイラ、対応する ONNX Runtime、利用者のモデルも必要です。手修正だけの処理では推論用の実行環境は不要です。[準備とモデルの契約](docs/development/production-tools.md)。

```sh
go run ./cmd/mosaic --help
mkdir -p build/mosaic-input/assets
cp docs/examples/production/pattern.png build/mosaic-input/assets/pattern.png
go run ./cmd/mosaic auto -fix docs/examples/production/fixes.yaml -out build/mosaic-output build/mosaic-input pattern.png
```

新規に作った合成画像の矩形を、推論なしでピクセル化します。入力を保持し、出力画像と report は `build/mosaic-output` に置きます。

## 設定

`game.yaml` にゲーム ID・スキーマ版・開始場所と仲間・画面・フォント・UI の翻訳を置きます。`data/` 以下の YAML（サブディレクトリと `.yml` も対象）に定義とイベントを書きます。素材のパスは、渡す `fs.FS` の `assets/` からの相対パスです。[スキーマ一覧](docs/schema.md)。

`runner` は `--mute`・`--seed`・`--window`・`--save-dir` を受け付けます。`--start`・`--script` などの制作機能は `--dev` が必要です。通常のセーブは OS の config ディレクトリの `gozen/<game-id>` 配下に置きます。スクリプト実行は保存先を明示しなければ一時保存を使います。[起動と所有権](docs/architecture/presentation-api.md)。

## API 例

| 目的 | 入口 | 実行できる例 |
| --- | --- | --- |
| 埋込み YAML の読込みと開始 | `data.Load`・`rpg.New`・`headless.New` | [minimal](examples/minimal/main.go) |
| 乱数固定・移動・戦闘・保存・復元 | `Driver.Run`・`State.Save`・`rpg.Parse` | [headless](examples/headless/main.go) |
| 引数と context を渡してゲームを起動 | `runner.Run(ctx, fsys, ".", runner.Config{Args: args, Output: out})` | [play](examples/play/main.go) |
| Ebitengine の既存ループへ画面を組込み | `play.New`・`Game.Update/Draw/Layout/Close` | [表示 API の契約](docs/architecture/presentation-api.md) |

FS・出力 Writer・プロセスの終了はゲーム側が所有します。描画はメインスレッドで行い、同時に動かせるゲームはプロセス内に 1 つです。組込み時はループの停止後に `play.Game.Close` を呼びます。

## テストとビルド

```sh
make test lint build
# Linux の画面検査:
xvfb-run --auto-servernum --error-file=/dev/stderr make graphics
```

lint は go-arch-lint v1.19.0 と golangci-lint v2.14.0・actionlint v1.7.12 を使います。Make は必須ではありません。[直接の実行手順と CLI E2E](docs/development/testing.md) もあります。API 一覧はテストで検査します。この段のサンプル追加による公開識別子の増加はありません。

## 対応 OS

CI の対象は Linux・Windows・macOS です。amd64 のビルドと Linux/macOS arm64 のクロスビルドを設定しています。T8 では Linux amd64 のテスト・ビルド、CLI の 58 シナリオ、5 画面の実描画、撮影パイプラインを確認しました。Windows/macOS の実行、実音声、ONNX 実推論、リモート CI の成功は未確認です。クロスビルドだけでは実行の確認に数えません。[公開前の点検手順](docs/development/release-checklist.md)。

## 操作方法

メニューは上下キーで選択、Enter/Z で決定、X/Esc で戻る・メニュー表示です。観測室は上で前進、左右で方向転換、下で振り向きます。M でミュート、F11 で全画面を切り替えます。[保存・復元を含む手順](docs/examples/wayfinder.md#play-through)。

## 制限

開発中で、公開版のインストールは未確認です。サンプルの一部の補助画面には既定の日本語が残り、Go Regular には一部の記号の字形がありません。サンプルに音楽はありません。ONNX のモデルと共有ライブラリは同梱せず、実推論には別の環境準備が必要です。`x/` は予約ディレクトリのみで、利用できる Go パッケージはまだありません。将来の実験にも互換性を約束しません。[v0 の範囲](docs/decisions/0002-public-api-scope.md)と[公開前の点検手順](docs/development/release-checklist.md)を参照してください。

## 開発者向けリンク

[公開前の点検手順](docs/development/release-checklist.md) · [全体設計](docs/architecture/overview.md) · [API 規約](docs/architecture/api-guidelines.md) · [中核 API](docs/architecture/core-api.md) · [表示 API](docs/architecture/presentation-api.md) · [ADR](docs/decisions/index.md) · [開発と検証](docs/development/testing.md) · [CI](docs/development/ci.md) · [貢献](CONTRIBUTING.md) · [セキュリティ](SECURITY.md) · [変更履歴](CHANGELOG.md) · [CLI](docs/development/cli.md) · [制作支援](docs/development/production-tools.md)

## ライセンス

[MIT](LICENSE) です。Go フォントには別の BSD 通知があり、[ASSETS.md](ASSETS.md) に出典・素材の生成方法とともに記載しています。
