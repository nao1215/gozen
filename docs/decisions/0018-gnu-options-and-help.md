# ADR 0018: オプションを GNU 形式にし、help の形を揃える

状態: 採用。日付: 2026-10-19。[ADR 0016](0016-single-cli.md) の `gozen` と、`runner.Run` が受け取るゲームの起動オプションに適用します。[ADR 0012](0012-cli-ownership-and-output.md) の出力先・終了値・上書き防止は変えません。

## 利用者と問題

利用者は Gozen でゲームを作る作者と、作者のゲームを `--dev` で動かす人です。これまでの `gozen` と制作支援のサブコマンドは Go 標準の `flag` で、`-seed 7` と `--seed 7` の両方を受け、短い名前と長い名前の区別がありませんでした。`-n`・`-v`・`-o` が命令ごとに別の意味を持ち、help は `flag.PrintDefaults` の並びで、例も前提も命令ごとに書き方が違いました。作者の他の道具（GNU 形式の `--name`、種類ごとにまとめた help）とも揃っていませんでした。

## 決定

- オプションは GNU 形式にします。実装は `github.com/spf13/pflag`（v1.0.10。作者の他の道具と同じ系列）です。長い名前は `--kebab-case`、よく使う物だけに 1 文字の短い名前を付けます（`-h` help、`-v` version または verbose、`-n` 回数または dry run、`-o` 出力先、`-c` prompt の catalog）。同じ命令の中で短い名前を重ねません。
- `--name=value` と `--name value` の両方を受けます。短い名前は `-n 3` と `-n3` を受けます。オプションは位置引数の前後に置け、`--` より後は全て位置引数です。
- `-seed` のような単一ダッシュの長い名前は受けません。pflag はこれを短い名前の並び（`-out DIR` が `-o ut DIR`）と読むため、解析の前に検出して「`--seed` を使う」と案内し、終了値 2 にします。古い書き方の互換は残しません（未リリース）。
- 名前を変えた物: `sim -n` は `-n, --battles`、`sim -tree`（既定 true）は `--no-tree`、`floors -nomap` は `--no-map`、`trial -n` と `mosaic auto -dry` は `-n, --dry-run`、`sim`・`design` の `-v` は `-v, --verbose`、`mosaic manual view -o` は `--view-out`（`-o` は `--out`）。
- help は全ての命令で同じ形にします。1 行目は「名前 - 一言の説明 (版)」、続いて `[Usage]` と説明、トップと下位の命令を持つ物だけ `[Commands]`、実際に動く `[Examples]`（命令ごとに 2〜5 個）、種類ごとにまとめた `[Options]`（短い名前・長い名前・値の名前・80 桁で折り返した説明・意味のある既定値）、補足の段落（前提を書く `Requirements:` を含む）、最後に Documentation・Report an issue・GitHub Sponsors のリンクです。どのまとまりにも入れ忘れたオプションは `Other:` に出し、テストで検出します。
- トップは `-h/--help` と `-v/--version` を持ちます。`gozen help <command>` は `gozen <command> --help` と同じ出力です。
- 組み立ては `internal/clihelp` に置き、`cmd/gozen`・`internal/promptgen/command`・`internal/mosaic/command`・`runner` が使います。依存検査に `clihelp` component と `pflag` vendor を足します。`clihelp` は他の component に依存しません。
- Gozen の中で標準の `flag` は使いません。スキーマ更新のテストの `-update-schema` は環境変数 `GOZEN_UPDATE_SCHEMA=1` にしました。

## 代案

1. 標準の `flag` のまま help だけを整える案。`-seed` と `--seed` の両方が通り、短い名前と長い名前を分けられないため採りません。
2. cobra などの命令の枠組みを入れる案。サブコマンドの振り分けと終了値は既に `cmd/gozen` が持っており、help の形も独自に揃えるため、依存を増やす利点が小さく採りません。
3. ゲームの起動オプションだけ標準の `flag` に残す案。`runner` の利用者の main からは `Config.Args` を渡すだけで、`flag` の型は公開 API に出ていません。書き方が `gozen` と食い違うため、`runner` も揃えます。

## 影響

- `runner` の公開 API（`Config{Args, Output}`）は変わりません。単一ダッシュでゲームを起動していた利用側（`-dev` など）は `--dev` に直す必要があります。
- 終了値（成功と help 0、処理の失敗 1、引数の誤り 2）は変わりません。
- `pflag` が直接依存に加わります。BSD-3-Clause で、他の依存はありません。
