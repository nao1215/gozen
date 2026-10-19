# 公開と internal の境界

公開するのは、ゲーム作者またはサンプルが実際に必要とする契約です。大文字の識別子は互換性の対象になるため、実装の補助は内部に置きます。

| 配置 | 利用者と責務 | 互換性 |
| --- | --- | --- |
| `expr`、`text`、`assetfs`、`savefile` | 式、翻訳、素材 FS、保存先を扱う作者 | v0 の契約対象 |
| `data`、`rpg` | 定義の読み込み・検証、状態・保存を扱う作者 | v0 の契約対象 |
| `battle`、`event`、`world`、`dungeon` | 規則を利用・拡張する作者 | v0 の契約対象 |
| `headless` | 画面なしで進行を検査する作者・CLI | v0 の契約対象 |
| `gfx`、`sound`、`play`、`runner` | 表示・再生・画面・起動を組み込む作者 | v0 の契約対象 |
| `internal/<package>/`、`<package>/internal/` | 所有パッケージの実装 | 非公開 |
| `internal/layerN/` | 第 N 層で共有する小さな実装 | 同じ層以上からだけ利用 |
| `internal/testgame/` | 新しく作る中立のテストデータの組立て | テスト専用、T2 で中立のゲームを新規作成 |
| `internal/dev/`、`api/` | 開発時の API 検査 | 実行時から利用しない |
| `cmd/gozen` | 唯一の CLI。汎用の制作・検査と、制作支援のサブコマンドへの振り分け | ライブラリから利用しない |
| `x/adv`、`x/village`、`x/tactics`、`x/designeval`、`x/balance` | 将来の実験の予約領域。現在は `.gitkeep` のみで利用できる Go パッケージはない | 互換性を約束しない |
| `internal/promptgen/`、`internal/mosaic/`（配下を含む） | `gozen prompt`・`gozen mosaic` 専用の実装。入口は各 `command` パッケージ | `cmd/gozen` と自身の実装だけから利用 |
| `examples/` | 新規の一般向けサンプル | 公開 API の利用例 |

T2 で第 1〜4 層、T3 で第 5・6 層の公開 API を提供します。[中核 API](core-api.md) と [表示 API](presentation-api.md) に用途と所有権をまとめます。`api/public.txt:1` が現在の宣言一覧です。将来の `x/` の識別子も一覧に入れますが、それによって安定契約にはなりません。

制作支援の専用 internal は 6 層の共有実装に含めず、対応する CLI が所有します。内容の扱いは [ADR 0004](../decisions/0004-content-boundary.md)、モジュール構成は [ADR 0009](../decisions/0009-single-module.md) に従います。

共有 internal は層の抜け道にしません。`internal/layerN/` 自身は下位の公開パッケージと、より下位の共有 internal だけを参照できます。同層の公開パッケージへの逆依存は作りません。所有パッケージ配下の internal は所有者と同じ制約を受けます。`internal/` 直下に未分類の汎用置き場を作ると検査で拒否します。

テスト用の組立ては本体に埋め込みません。未公開の機能は内部を既定とし、公開が必要なら利用者・目的・契約を示して ADR と API 一覧を変更します。`dungeon` は [ADR 0014](../decisions/0014-dungeon-host-adoption.md) の範囲だけ公開します。開発情報の採取や限定配布の仕組みを扱う際も、汎用に説明できる部分だけを検討し、私的な集計機構は含めません。
