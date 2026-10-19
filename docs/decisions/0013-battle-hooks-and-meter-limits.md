# ADR 0013: 戦闘の共通イベントと状態に応じたメーター上限

状態: 採用。対応: SW1、D2、D8。

## 利用者と問題

ゲーム作者が、戦闘の直前に探索資源を消費し、結果の反映後に帰還を判断できるようにする。個々の戦闘命令へ同じ処理を複製すると、ランダム遭遇との不一致や、再試行での二重消費を招く。また、能力や進行によって上限が変わる資源を、現在値と上限の組で表示したい。

## 決定

既存の公開定義型に次の3フィールドだけを追加する。所有者は引き続き `internal/layer2/gamedata`、利用者向けの名前は `data` の型エイリアスとする。

| フィールド | 誰が何のために使うか | 省略時 |
| --- | --- | --- |
| `data.BattleCfg.Before` | ゲーム作者が、戦闘要求の直前に資源消費や条件確認の共通イベントを指定する | 実行しない |
| `data.BattleCfg.After` | ゲーム作者が、結果反映後の帰還や後始末を共通イベントで指定する | 実行しない |
| `data.Meter.MaxValue` | ゲーム作者が、状態から計算した上限と現在値を画面に表示する | 固定 `Max` と既存の表示を使う |

型と契約は `internal/layer2/gamedata/types.go:243`・`internal/layer2/gamedata/types.go:325` の `BattleCfg`、公開面の検査は `api/public.txt` が正本。新しい公開型・関数・インターフェースは作らない。イベントの実行は既存の `event.Runner`、画面・描画なしの実行は既存の上位層を使い、層の例外を追加しない。

実行順、移動による中止、通常敗北、再試行、メーターの下限は [戦闘イベントとメーター](../architecture/battle-hooks.md) に記載する。CLI の試行・体験版の到達解析も同じ設定を扱う。

## 代案

ゲーム作者に戦闘ごとの複製を求める案は、遭遇と連戦の一貫性を保てない。任意の callback や多数の Option を追加する案は、YAML の共通イベントで足りる用途に新しい拡張口を増やす。メーターに表示専用 callback を足す案も、既存の式と状態で表せるため採らない。

## 検証

`headless/battle_hooks_test.go` は勝利・敗北・逃走、選択・連戦・終了、前処理の移動、通常敗北を検査する。`internal/play/battle_hooks_test.go` は再試行の状態、固定・可変上限と入力経路を検査する。参照の検証は `data/battle_hooks_test.go`、部分配布は `cmd/internal/trial/battle_hooks_test.go`、測定は `internal/headless/balance/battle_hooks_test.go` が担当する。スキーマと API 一覧も同時に更新する。
