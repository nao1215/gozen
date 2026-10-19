# ゲームから Gozen を利用する

ゲーム側は自分のデータ・素材・保存先・制作設定を所有し、公開パッケージと CLI を利用する。`runner.Run` は FS と起動引数を受け取り、失敗を error で返す（`runner/runner.go:27`）。プロセスの終了はゲームの main が担当する。埋込みの最小例は `examples/minimal/main.go:1` にある。

## モジュールと入口

利用側のモジュールに Gozen の公開版を require する。リリース前の手元での接続には、**利用側の** go.mod に `replace github.com/nao1215/gozen => ../gozen` のような相対指定を置ける。Gozen 自身の go.mod に replace や別モジュールは追加しない。

CLI は `go run github.com/nao1215/gozen/cmd/gozen` または `go install github.com/nao1215/gozen/cmd/gozen@latest` で使う。開発用の CLI も依存として維持したい利用側は、Go の `tool` 宣言に `cmd/gozen` を登録すると `go mod tidy` が必要な依存を保持する。未公開版の `@latest` 導入はできないため、開発中は利用側の require と replace を使う版なしの `go run` を使う。

## 呼び出しの対応

| 用途 | 公開入口と責任 |
| --- | --- |
| 定義を読む | `data.Load(fsys, path)`。FS 内の相対ディレクトリを明示する |
| 保存された JSON を読む | `rpg.Parse` または `rpg.ParseWithConfig`。バイト列からの解析と FS の読取りを分ける |
| イベントの効果を受け取る | `event.Host.Handle(event.Effect)` または `event.HostFunc`。必要な効果を型で選ぶ |
| 画面なしで命令列を実行する | `headless.Driver.Run(ctx, commands)`。進行中の中断に context を渡す |
| 分岐を探索する | `headless.Explore` と `headless.Config` |
| 一歩ずつ迷宮を操作する | `dungeon`。位置・訪問記録・条件付きマスを観測し、返された命令は呼出側で一度実行する |
| フォントの幅を測る | `gfx.Load` の返す `Font.Measure`。大域的なフォント設定を使わない |
| 独自のループへ組み込む | `play.New(fsys, play.Config)`。終了時に `Game.Close` を呼ぶ |
| 通常のゲーム起動 | `runner.Run(ctx, fsys, sub, runner.Config)`。引数と出力先を明示する |

契約の正本は `data/load.go:16`、`rpg/save.go:79`、`event/event.go:24`、`headless/headless.go` の `Run`、`play/play.go:70`、`gfx/font.go:15`。迷宮公開面の利用者と識別子ごとの目的は [ADR 0014](../decisions/0014-dungeon-host-adoption.md) に記録する。旧実装の公開関数を一律に公開する方針は採らない。

## 保存と開発用の起動

既定の保存先は `UserConfigDir/gozen/<game-id>`。別のアプリケーションの保存先は探索しない（`savefile/savefile.go:12`）。既存作品の保存を継続するゲームは、自分の起動コードで従来の場所を決め、必要な移行を行い、通常プレイに `--save-dir` を渡す。移行の名前や規則はゲーム側で管理する。

台本・開始地点の指定・リプレイなどには `--dev` が必要。台本では `--shots` を明示する。保存先を明示しない開発用の実行は、一時ディレクトリへ隔離される。通常プレイの保存先を、検査用の実行へ無条件に付加しない。外部入力を使う場合は `--external-input` も明示する。

## 内部化した道具を検査する

体験版は `gozen trial -list <一覧> <入力> <新しい出力>` を実行し、出力の YAML・素材・一覧を公開 `data.Load` と FS 操作で検証する。生成先と一覧は新しいパスにする。文面生成は `gozen prompt --format json` の構造化出力を使う。いずれも internal の型を利用側へ露出させない。

メモ一覧は `gozen memos <入力> -out <新しい出力>` で生成し、`<新しい出力>/index.html` を読む。`-out` を省略すると出力は `<入力>-html` になる（`cmd/gozen/memos.go:33`）。入力の中には HTML を書かない。繰り返し同じ場所へ一覧を公開する利用側は、毎回別の作業ディレクトリへ生成してから、成功時だけ前の一覧を入れ替える。生成が失敗した場合は前の一覧と元の JSON・画像・保存データを保持する。[ADR 0012](../decisions/0012-cli-ownership-and-output.md) の上書き防止は維持する。

メモの制作情報は明示した `--dev --build-info` でのみ記録する。`trace` は記録ファイルのベース名、`version` は実行ファイルに埋め込まれた revision がある場合だけになる（`internal/play/memo.go:242`、`internal/layer1/memo/memo.go:224`）。利用側の検査も、既定での非記録と明示指定時の記録を分ける。ビルド情報のない実行ファイルに revision の存在を要求しない。場所・地図・保存・再開と、トレースを指定した再生は制作情報の有無と別に検査する。

作品専用の評価方針はゲーム側のパッケージに置き、公開の状態・戦闘・イベント・進行を組み合わせる。制作支援のモデルと設定も利用側が明示する。既存の既定値が必要なら利用側の設定として管理する。

## 検査

ゲーム側の vet・テスト・CLI によるデータ検査と、Gozen 側の `make test lint build` を別々に実行する。利用側の `go test ./...` は依存モジュールのテストを実行しない。画面の検査は [描画の検証](../development/testing.md#描画の検査) に従い、実際のゲームループ・台本の終了・生成された画像を確認する。Xvfb のプロセス起動だけでは成功としない。
