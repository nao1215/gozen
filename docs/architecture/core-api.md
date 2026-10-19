# 描画しない中核の使い方と公開契約

ゲーム作者が定義を読み、状態を進め、画面なしでイベント・戦闘・保存を検査するための API です。表示・音声・起動は [表示 API](presentation-api.md)、`gozen` CLI は [汎用 CLI](../development/cli.md) と [制作支援 CLI](../development/production-tools.md) を参照してください。実行可能な例は `examples/headless/main.go:1`、その結果を確かめるテストは `examples/headless/main_test.go:1` にあります。

## 利用者と公開する理由

`api/public.txt` は識別子単位の一覧です。以下の分類が各型・メソッド・フィールドを公開する目的で、詳細な値の意味は各 godoc と [スキーマ](../schema.md) に記載します。定義型のフィールドはゲームの設定・検査、状態型のフィールドはホストによる操作・表示のために公開しています。API 一覧の更新だけを公開承認の代わりにはしません。

| パッケージ | 利用者と目的 | 契約の入口 |
| --- | --- | --- |
| `text` | ホストが中核のメッセージを翻訳する | `Use` は map をコピー、`T` は未登録なら元のキー |
| `expr` | 作者の式を解析・実行し、制作道具が参照・型・範囲を調べる | `Parse`、`Expr`、`Env`・`Object`、`Func`、静的解析用の `Ref`・`ValueType`・`Range` |
| `data` | 作者が YAML 定義を読み、検査・編集・表示する | `Load`・`Validate`、`DB` と定義型のエイリアス、仕様版定数 |
| `rpg` | ホストが仲間・所持品・進行・装備を操作し、状態を保存する | `State`・`Actor`、戦術や装備の計画、式向けの view、`Parse` と `LoadReport` |
| `savefile` | ホストが保存先・バイト列・修復前の控えを管理する | `Prepare`・`Load`・`Write`・`WritePreserving` |
| `event` | フロントエンドが命令を実行し、待機要求に答える | `Runner`、`Request` の各値、即時効果の `Effect` と `Host.Handle` |
| `battle` | フロントエンドがターンを進め、行動を選び、結果を表示する | `Battle`・`Unit`・`Action`・`Step`、AI 判断を観測する `Decision`・`RuleEvaluation` |
| `world` | ホストが移動・操作候補を表示し、到着や自動イベントを処理する | `Entry`、`Entries`・`Entry.Commands`・`Arrive`・`Auto`・`PickTroop` |
| `dungeon` | 独自の探索操作を組み立てるホストと評価道具 | 一歩単位の操作と地図の観測。公開する各識別子の用途は [ADR 0014](../decisions/0014-dungeon-host-adoption.md) |
| `headless` | ゲーム作者と検査道具が画面なしで進行・分岐を確かめる | `Driver`、`Explore` と `Config`、観測結果の `Path`・`Step`、`AutoBattle` |
| `assetfs` | ホストが素材を差し替え、承認結果を確認し、配布用にコピーする | `New`・`Config`・`OverlayFS`、`Approval`・`Gate`・`Tree` |

定義の共有実装は `internal/layer2/gamedata` にあり、公開名は `data` にまとめます。エイリアス経由で使えるフィールドとメソッドも API 一覧で検査します。YAML の補完・診断、制作記録の解析、迷宮の生成と内部キャッシュは公開の入口を増やさず内部へ置きます。境界の理由は [ADR 0010](../decisions/0010-core-ownership.md) を参照してください。

## 読み込みから保存まで

1. `data.Load(fsys, ".")` で検証済みの DB を得ます。サブディレクトリも指定できます。エラー時の DB は nil です。仕様版 0 と 1 を受け入れ、1 は数値の省略と明示したゼロを区別します。仕様版 0 では空のゲーム ID を読み込めますが、`savefile.Prepare` は空の ID を拒否します。標準保存先を使う画面起動には有効な ID が必要です（`savefile/savefile.go:17`、`internal/play/play.go:363`）。
2. `rpg.New(db, rng)` で状態を作り、`headless.New(db, state)` と `Start()` で開始イベントを実行します。読み込み済みの保存を再開する場合は `Restore` を使い、開始・到着イベントを再実行しません。
3. `Run(ctx, commands)`、`GoTo`、`Do`、`Walk` で進めます。`Run` は開始前とイベント境界でキャンセルを確認します。戦闘は `MaxTurns`、イベントの収束と移動は回数で制限されます。
4. `State.Save()` は JSON を返します。ホストが `savefile.Write` または状態の `SaveFile` で保存先を選びます。読み込みは `savefile.Load(fsys, path)`、メモリ上の解釈は `rpg.Parse(db, bytes, rng)` です。
5. 保存形式版・ゲーム ID・データ版を照合します。定義の変更で修復した項目は `LoadReport` で通知できます。版なしの JSON は出所を確認した呼出し側だけが `ParseWithConfig` で明示的に許可します。

`savefile.Prepare` の空の Root は OS の設定ディレクトリ配下の `gozen` を使います。取得できなければエラーです。旧アプリケーションの保存先の探索、移行、作業ディレクトリへのフォールバックはしません。同じ保存先への書込みはホストが直列化してください。ファイルの同期・close・rename のエラーを返しますが、全 OS・全 FS での電源断耐性を保証するものではありません。

## 所有権と拡張

DB は定義を所有し、渡された FS を借ります。生成迷宮は DB の地図を書き換えるため、並行した別ゲームには DB・状態・乱数を個別に用意します。同じ状態や戦闘を複数 goroutine から変更することはできません。保存 JSON は乱数生成器の内部状態を保存しないため、保存再開後の乱数列の完全再現はホストの責務です。

`text.Use` と `T` は同期され、登録時に map をコピーします。ただし言語はプロセス全体で共有します。ゲームごとの異なる言語を並行して切り替える用途は未対応です。式は解析後に読み取り専用で使えますが、環境や拡張関数の副作用・並行安全性はホストが管理します。

イベントの即時効果は `Host.Handle(Effect)` で同期通知します。`HostFunc` で関数を渡すこともできます。移動効果を受けるホストは `State.Location` と `Steps` を更新します。会話・選択・戦闘などの待機は `Request` を受け取り、必要に応じて `Answer` します。直接 `Runner` を駆動するホストは `Log` に診断の受取先を設定してください。`headless` は診断を集めて error として返します。

`Explore` は状態の写しで各決定箇所を一度ずつ分岐させます。全組合せの網羅ではありません。ゼロ設定は最大 300 経路・経路あたり 20000 ステップ、負の制限値や空の戦闘結果候補は `Path.Errs` に記録します。写した状態は独立しますが、迷宮の再生成に借りた DB を使います。

## 素材と利用例の範囲

`assetfs.New` は基礎 FS に存在するファイルだけを上書き対象にします。任意の manifest が有効な場合に元ファイルの hash を照合します。manifest がない・不正な場合はこの任意フィルタを無効にする契約です。必須の承認は `Gate` で別途検査します。変更後は `Forget` でキャッシュを破棄し、FS 自身への書込みはホストが同期してください。

サンプルの定義・台詞と 8×8 の単色画像はこのリポジトリ用に新規作成し、画像は Go 標準ライブラリで生成します。第三者の素材は使っていません。リポジトリの MIT ライセンスが適用されます。テストゲームの画像 sentinel は存在確認専用で、画像デコードや画面の実行検証には使いません。
