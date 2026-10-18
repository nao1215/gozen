# 設計判断

Gozen の配置、公開契約、依存関係、検査についての設計判断を記録します。

| 判断 | ADR |
| --- | --- |
| D1 モジュールと配置 | [0001](0001-modules-and-layout.md) |
| D2 v0 の公開範囲 | [0002-public-api-scope](0002-public-api-scope.md) |
| D3 公開内容と検査データ | [0003](0003-publication-content.md) |
| D4 本体と制作ツールの境界 | [0004-content-boundary](0004-content-boundary.md) |
| D5 サンプルと素材 | [0005-examples-and-assets](0005-examples-and-assets.md) |
| D6 対応環境と CI | [0006-platforms-and-ci](0006-platforms-and-ci.md) |
| D7 公開 API による組込み | [0007-adoption](0007-adoption.md) |
| D8 層と最小 API | [0008-architecture-and-api](0008-architecture-and-api.md) |
| T3 表示の所有権と起動の境界 | [0011-presentation-ownership](0011-presentation-ownership.md) |
| T2 共有モデルと規則の所有者 | [0010-core-ownership](0010-core-ownership.md) |
| T4 CLI の所有権と出力 | [0012](0012-cli-ownership-and-output.md) |
| D9・D10 導入経路と単一モジュールへの変更 | [0009-single-module](0009-single-module.md) |

判断を変更するときは、現在の制約、具体的な利用者、代案、影響を記録します。設定の除外を増やして依存違反を隠す変更は採りません。
