# 設計判断

Gozen の配置、公開契約、依存関係、検査についての設計判断を記録します。番号順に並べます。状態は次の 3 つです。

- 採用: 書かれた決定が今も有効です。
- 一部変更: 決定の一部を後の ADR が変えました。変えた ADR を「変更・置き換え」に書き、元の ADR の冒頭にも変わった所を書きます。
- 置き換え済み: 決定の全体を後の ADR が置き換えました。今の決定は置き換え先を読みます。

| ADR | 判断 | 状態 | 変更・置き換え |
| --- | --- | --- | --- |
| [0001](0001-modules-and-layout.md) | D1 モジュールと配置 | 一部変更 | 別モジュールの構成は [0009](0009-single-module.md)、CLI の配置は [0016](0016-single-cli.md) |
| [0002](0002-public-api-scope.md) | D2 v0 の公開範囲 | 一部変更 | 公開パッケージの追加は [0014](0014-dungeon-host-adoption.md)・[0015](0015-reaction-and-chat.md)・[0020](0020-moving-portraits.md)、実験の置き場所は [0017](0017-dev-balance-panel.md) |
| [0003](0003-publication-content.md) | D3 公開内容と検査データ | 採用 | |
| [0004](0004-content-boundary.md) | D4 本体と制作ツールの境界 | 一部変更 | 配置は [0009](0009-single-module.md)、CLI の入口は [0016](0016-single-cli.md) |
| [0005](0005-examples-and-assets.md) | D5 サンプルと素材 | 採用 | |
| [0006](0006-platforms-and-ci.md) | D6 対応環境と CI | 一部変更 | モジュール別の検査は [0009](0009-single-module.md)。今の CI の構成は [CI の構成](../development/ci.md) |
| [0007](0007-adoption.md) | D7 公開 API による組込み | 採用 | |
| [0008](0008-architecture-and-api.md) | D8 層と最小 API | 採用 | |
| [0009](0009-single-module.md) | D9・D10 導入経路と単一モジュールへの変更 | 採用 | |
| [0010](0010-core-ownership.md) | T2 共有モデルと規則の所有者 | 一部変更 | 迷宮の公開パッケージは [0014](0014-dungeon-host-adoption.md) |
| [0011](0011-presentation-ownership.md) | T3 表示の所有権と起動の境界 | 採用 | |
| [0012](0012-cli-ownership-and-output.md) | T4 CLI の所有権と出力 | 一部変更 | 入口は [0016](0016-single-cli.md)、勝率の計算の置き場所は [0017](0017-dev-balance-panel.md)、オプションの書き方は [0018](0018-gnu-options-and-help.md) |
| [0013](0013-battle-hooks-and-meter-limits.md) | SW1 戦闘イベントと可変上限 | 採用 | |
| [0014](0014-dungeon-host-adoption.md) | SW2 独自の迷宮操作と利用側への組込み | 採用 | |
| [0015](0015-reaction-and-chat.md) | 人物の反応とチャット | 採用 | |
| [0016](0016-single-cli.md) | CLI を gozen の 1 本にする | 一部変更 | オプションと help の形は [0018](0018-gnu-options-and-help.md) |
| [0017](0017-dev-balance-panel.md) | 開発用のバランス表示と勝率計算の共有 | 採用 | |
| [0018](0018-gnu-options-and-help.md) | CLI のオプションを GNU 形式にし、help の形を揃える | 採用 | |
| [0019](0019-engine-languages.md) | エンジンの文言を英語と日本語で持ち、language で選ぶ | 採用 | |
| [0020](0020-moving-portraits.md) | 一枚絵を差分と網目の変形で動かす（puppet） | 採用 | |

今の公開パッケージの正本は `api/public.txt` と [境界](../architecture/boundaries.md) です。ADR の一覧ではありません。

判断を変更するときは、現在の制約、具体的な利用者、代案、影響を記録します。設定の除外を増やして依存違反を隠す変更は採りません。変更した ADR は、元の ADR の状態の行と、この表の状態・変更の列を同じ変更で直します。

## 記号

ADR と設計文書に残る記号は、公開前の初期実装の段と設計判断の番号です。版や API の区切りではありません。新しい文書では使わず、「表示層の実装時に」のように中身で書きます。

| 記号 | 意味 |
| --- | --- |
| D1〜D10 | 初期実装の前に決めた 10 の設計判断。ADR 0001〜0009 に書き起こしました（上の表の「判断」の列） |
| T1 | 土台。モジュール、層の依存検査、公開 API の一覧、開発文書 |
| T2 | 描画しない中核（第 1〜4 層）と、テスト用の中立なゲーム |
| T3 | 描画・音・実行（第 5・6 層）と迷宮 |
| T4 | 汎用の CLI と atago の E2E |
| T5 | 制作支援（`gozen prompt`・`gozen mosaic`）と ONNX の cgo ビルド |
| T6 | サンプルのゲーム、README、紹介画像の撮影 |
| T7 | CI とリポジトリの方針（CONTRIBUTING・SECURITY・CHANGELOG） |
| SW1 | 利用側のゲームを Gozen に載せる段の 1。戦闘の共通イベントと可変のメーター上限（ADR 0013） |
| SW2 | 同じく段の 2。独自の迷宮操作と利用側への組込み（ADR 0014） |
