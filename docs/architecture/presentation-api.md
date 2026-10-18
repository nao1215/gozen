# 表示・音声・起動 API

ゲーム作者向けの第 5・6 層の契約を記載する。実行条件は [画面の開発](../development/graphics.md)、公開前の点検は [リリース点検](../development/release-checklist.md) を参照する。

| 公開宣言 | 誰が何に使うか・契約 |
| --- | --- |
| `gfx.Font`、`gfx.Load`、`Font.Measure`、`Font.Draw` | 独自の Ebitengine 画面を描く作者が FS のフォントを独立して読み、幅・高さの測定と描画に使う。サイズはピクセル、非正数・非有限サイズは測定 0 / 描画なし。Draw はオプションを変更しない |
| `sound.SampleRate`、`sound.Clip`、`sound.Load`、`Clip.Play` | 独自画面の作者が WAV/Ogg を読み、44,100 Hz の Context で効果音を再生する。返された Player の停止は所有者が行う |
| `play.Config` と各フィールド | 画面を埋め込む作者が乱数、保存先、ミュート、ウィンドウ倍率、全画面、制作機能を指定する |
| `play.Development` と各フィールド | 制作者が開始位置、入力スクリプト、画像出力先、トレース Writer、メモ先、ビルド情報採取、外部入力を明示する |
| `play.Run` | FS 内の game.yaml と data/ を使い、画面の読込みから終了・後始末まで実行する |
| `play.Game`、`play.New`、`Game.Update/Draw/Layout/Close` | ホスト側の ebiten.Game と合成する作者がループを所有する。Close 必須、呼出しはゲームスレッドに限定 |
| `runner.Config`、`Args`、`Output`、`runner.Run` | 埋込みゲームの作者が fs.FS とサブディレクトリを渡し、同じフラグ・help・制作モードを利用する。引数と出力先はアプリが渡す |
| `event.Runner.BlockAside` | 画面を持つイベント利用者が、回想やオーバーレイ中の挿入イベントを同期 callback で抑止する。nil では既存の Host / 画像状態による判定を使う |

`api/public.txt` の T3 差分は 35 宣言（型・フィールド・メソッド・定数をそれぞれ数える）。上表以外の画面、メモ、音声ミキサー、dungeon の API は増やさない。

## 起動と終了

`runner.Run(ctx, fsys, sub, runner.Config{Args: args, Output: out})` は相対 sub を fs.Sub で開く。`--dir` があればそのディレクトリを使う。nil Args は引数なし、nil Output は出力なし。`--help` はゲームを読み込まずに成功し、書込みに失敗すればそのエラーを返す。

Run はメインスレッドで呼ぶ。ウィンドウを閉じる通常終了は nil、キャンセルは context のエラー、保存やトレースの失敗はエラーを返す。Game.Update を直接使うホストにはスクリプト完了を ebiten.Termination で通知する。ライブラリは os.Exit や log.Fatal を呼ばない。Ebitengine は一つのプロセス内でループを再起動できない。

FS は借用し、変更・Close しない。素材と定義は利用中に安定している必要がある。Writer も呼出し側が所有し、同期的な書込みが返る必要がある。Game は非並行、同時に開けるインスタンスは 1 つ。New 失敗時も初期化途中の資源を後始末する。Close はループ停止後に呼び、二度目以降は何もしない。

画像の非同期先読みは CPU 上のデコードまでとし、描画用画像は利用する側のゲームスレッドで作る。画面の初期化失敗やループ終了後に worker が描画 API を呼ばない。先読み結果も画像キャッシュの予算内に収め、Close は読み終わるまで待って解放する（`internal/play/assets.go:180`・`194`・`402`、`internal/play/preload_test.go:12`）。

play の音声はプロセス共通の audio.Context を使う。既存 Context があれば 44,100 Hz に限り再利用し、他のレートは拒否する。Mute はデバイスを初期化しない。独立した Clip の Player は Game.Close の対象外なので、利用者が PauseAndStopReading を呼ぶ。

## 保存と制作情報

通常保存は `os.UserConfigDir()/gozen/<game-id>`。既存作品の保存先を探さない。SaveDir は完全な出力先であり、ID を追加しない。開始位置の指定、スクリプト、リプレイ等の試走では、SaveDir を省略すると一時ディレクトリを使って Close で消す。保存先の書込み失敗を成功扱いにしない。

制作フラグは `--dev` 必須。メモ、トレース、スクリーンショット、レビューの出力先に制作環境由来の既定値はない。メモは Notes / --memos 指定時だけ有効。CollectBuildInfo / --build-info は既定 false で、許可時に埋込みビルド情報の revision だけを追加する。外部コマンドによる Git やソース探索はない。

Linux の外部入力は --external-input で明示的に許可する。zenity が見つからなければ画面内入力を使う。Windows・macOS は画面内入力を使う。利用可能な zenity で入力をキャンセルした場合は、その入力を取り消す。

実装の配置と判断は [ADR 0011](../decisions/0011-presentation-ownership.md) を参照する。
