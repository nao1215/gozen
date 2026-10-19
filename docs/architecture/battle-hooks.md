# 戦闘イベントとメーター

ゲーム作者は `game.yaml` の `battle.before`・`battle.after` に共通イベントの ID を指定できる。公開定義の追加理由は [ADR 0013](../decisions/0013-battle-hooks-and-meter-limits.md) を参照する。

## 実行順

- `before` は `event.Runner` が戦闘要求を返す直前に実行する。処理中に場所が変わると、戦闘と元イベントの残りを中止する。
- 画面の再試行は前処理後の状態を復元する。同じ戦闘のやり直しで前処理を再実行しない。
- 戦闘の結果分岐を進め、次の選択・戦闘に達したところ、または残りのイベントが終了したところで `after` を実行する。後処理で場所が変わった場合は保留中の選択・連戦を中止する。
- 敗北分岐を持たない通常敗北では、全滅処理を優先し `after` を実行しない。設定を省略したゲームの進行は従来どおり。
- `after_win` の予約方式は維持する。元イベントが終了すると予約されたイベントを先に実行し、その後で残っている `after` を処理する。次の選択・戦闘の境界では `after` が先に走るため、`after_win` と `after` の固定順序には依存しない。

根拠: `event/event.go:249` の `Answer`、`event/event.go:298` の `Next`、`internal/play/play.go:1337` の `begin`、`internal/play/battle.go:1107` の `result`。`headless/battle_hooks_test.go:15` と `internal/play/battle_hooks_test.go:12` が共通の進行と再試行を検査する。

## 画面の上限

`meters[].max_value` は通常のフィールド状態を参照する式。評価に成功した上限を1以上に丸め、値を0から上限の間へ収める。指定時は現在値/上限を表示し、満杯時の `full` 表示は引き続き優先する。式を省略すると固定の `max` を使い、`max` が0なら既存の100を使う。評価に失敗した場合も固定上限へ戻す。

読み込み時に式の名前・構文を検査する。根拠: `internal/layer2/gamedata/check.go:262`、`internal/play/meters.go:15`。中立な使用例は `data/battle_hooks_test.go:10` と `internal/play/battle_hooks_test.go:44` にある。

## 制作 CLI

`gozen curve` の戦闘曲線は、イベント実行を有効にした探索で前後処理を反映する。資源による帰還は敗北・道具不足と区別し、設定された休息と補給を経て同じ階へ戻る。単体の勝率測定では探索の前処理を実行しない。HP の消耗率は後処理による帰還・回復の前に採る。根拠: `cmd/gozen/internal/balance/balance.go:317`・`cmd/gozen/internal/balance/balance.go:335`・`cmd/gozen/internal/balance/balance.go:401`・`cmd/gozen/internal/balance/balance.go:431` の `rate`・`fight`・`end`・`returnAndResume`。

体験版の絞り込みでは `battle.before`・`battle.after` を根として呼び先・素材・状態変化をたどる。タイルの `after` は別のタイルを指すため、イベント参照として扱わない。根拠: `cmd/internal/trial/prune.go:428` の `reach`、`cmd/internal/trial/battle_hooks_test.go:11`。

メモの入力経路は `play.Development.ExternalInput` または runner の `-external-input` で選択する既存契約を維持する。通常は画面内入力。IME の確定と通常文字が同時に届いても二重に追加せず、入力を消費したフレームの Enter を保存に使わない。OS の実際の IME 接続は環境ごとの確認が必要で、単体テストの成功は実機の入力確認を意味しない。根拠: `play/play.go:46`、`internal/play/memo.go:354` の `readInput`。
