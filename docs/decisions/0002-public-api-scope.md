# ADR 0002: v0 の公開契約

状態: 一部変更。日付: 2026-10-18。対応: D2。公開パッケージの一覧と実験の置き場所は、下の「後の ADR で変わった所」のとおり変わりました。今の公開パッケージの正本は `api/public.txt` です。

## 背景

内部の実装まで公開すると、調整用の処理も互換維持の対象になります。ゲーム作者が必要とする機能と実験を分ける必要があります。

## 決定

v0 の契約対象は `data`、`rpg`、`battle`、`event`、`expr`、`world`、`headless`、`savefile`、`text`、`assetfs`、`sound`、`gfx`、`play`、`runner` とします。公開する個々の型と関数はさらに絞り、godoc に規則を記述します。

埋め込んだゲームの FS を受け取る入口は `runner` と呼びます。ライブラリの起動 API と CLI 用のパッケージとの混同を避ける名前です。

`adv`、`village`、盤面の `tactics`、`designeval`、`balance` は `x/` の実験に置き、互換性を約束しません。v0 の契約を優先し、実験は具体的な利用者が必要とするものから検討します。開発情報の採取や限定配布の機構は汎用部分だけを扱います。

## 理由

安定に扱う責務を定め、利用しない API を増やさずに開発できます。実験の予約領域は実装の提供を約束するものではありません。

## 結果

`dungeon` は層の分類に含まれますが、現時点の公開契約には含めません。実装は internal を優先します。公開判断と配置の一覧は [境界](../architecture/boundaries.md) を参照してください。

## 後の ADR で変わった所

この ADR の決定は採用時の記録です。次の点は後の ADR で変わりました。

- `dungeon` は [ADR 0014](0014-dungeon-host-adoption.md) で第 3 層の公開入口として契約に加えました。上の「結果」の「現時点の公開契約には含めません」は採用時の状態です。
- `react`（第 3 層）と `chat`（第 1 層）は [ADR 0015](0015-reaction-and-chat.md) で加えました。
- `puppet`（第 5 層）は [ADR 0020](0020-moving-portraits.md) で加えました。
- 勝率の計算（`balance`）は `x/` ではなく `internal/headless/balance` に置き、`gozen sim` と開発用のバランス表示で共有します（[ADR 0017](0017-dev-balance-panel.md)）。設計評価（`designeval`）は `cmd/gozen/internal/designeval` にあります。公開するかは別の ADR で決めます。

公開する型と関数を絞る方針と、`x/` の実験に互換を約束しない方針は変わっていません。
