# ADR 0011: 表示の所有権と起動の境界

採用: 2026-10-18（T3）。既存の層・単一モジュール・内容境界は変更しない。

## 判断

ゲーム作者の入口は `runner.Run` と `play.Run` にする。既存の Ebitengine アプリへ組み込む作者には `play.New` と `Game` の Update/Draw/Layout/Close を提供する。引数と出力先を明示し、ライブラリはプロセスを終了しない。終了コード、シグナル、標準出力の選択はアプリが所有する。

大量の画面・制作設定は `internal/play.Options` に留め、公開は小さな `play.Config` と任意の `play.Development` に絞る。書込み先を持つ制作機能は明示指定にする。通常の保存は `savefile` の Gozen 用ディレクトリ、スクリプト等の試走は明示指定がなければ一時保存先を使う。

表示の共有実装は `internal/layer5/gfx`・`internal/layer5/sound`、メモのファイル記録は `internal/layer1/memo` に置く。メモは標準ライブラリの image とバイト列を受け、画面・ゲーム規則を知らない。dungeon は T2 の `internal/layer3/dungeon` を使う。公開パッケージ間の横依存は追加しない。

`event.Runner.BlockAside` は挿入イベントを画面の状態に応じて抑止する callback とする。`event.HostFunc` と組み合わせ、上位の Game 型を下位のイベント層へ渡さずに、回想・画像表示中の抑止を維持する。既存の `AsideBlocker` は後方互換のため保持する。

## 所有権と代案

画面内部のフォントキャッシュ、翻訳、音声設定が共有状態を持つため、同時に開ける Game は 1 つに制限する。Close は音声を停止し、キャッシュと一時保存先を解放する。複数ゲームを同時実行できるとは約束しない。Ebitengine のループもプロセス内で一度だけ起動する。

gfx は独立した Font を返し、sound は独立した Clip と呼出し側所有の audio.Context を使う。描画ヘルパーやグローバル音声管理を全公開する代案は、ゲームの組込みに不要な互換契約を増やすため採用しない。play の音声は既存の 44,100 Hz の Context を再利用する。異なるレートはエラーにする。

ビルド情報は既定で記録しない。明示 opt-in 時のみ埋込みの revision を読む。Git の実行、ソースの探索、ホスト情報の採取はしない。再現データの互換性は独立した形式・動作の版で判定する。

## 確認方法

公開契約と利用者は [表示 API](../architecture/presentation-api.md)、実行条件は [画面の開発](../development/graphics.md) を参照する。依存ポリシーの除外は増やさず、深い依存検査を維持する。
