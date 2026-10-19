# ADR 0006: 対応環境と CI

状態: 一部変更。日付: 2026-10-18。対応: D6、D10。モジュール別の検査は [ADR 0009](0009-single-module.md) で 1 つのモジュールの検査に変わりました。今のジョブと workflow の一覧は [CI の構成](../development/ci.md) です。

## 背景

画面を使わない規則のテスト、描画の実行、CLI の入出力、対象 OS 向けビルドは異なる検査です。クロスビルドだけで OS 上の実行成立を判断できません。

## 決定

GitHub Actions は Ubuntu・Windows・macOS を対象に、lint、unit、graphics、cli-e2e、build、security、examples のビルドを分けます。unit は下限の Go と stable を 3 OS で実行します。lint は golangci-lint v2 と go-arch-lint v1.19.0、CLI E2E は atago、security は govulncheck と CodeQL を使います。

Linux の graphics は SHA 固定の setup-ebitengine で依存と Xvfb を用意し、Xvfb を起動して検査します。Windows・macOS は実行可能か確認し、できない場合は理由を記録します。amd64 は 3 OS、arm64 は Linux・macOS のクロスビルドだけを追加します。対応版と実行条件は文書に揃えます。

通常 workflow は read-only 権限、外部 action は SHA 固定、checkout の認証情報を残しません。CodeQL の結果送信など書込みが必要な場合は、その job に必要な権限だけを付けます。Dependabot は単一モジュールの gomod と Actions を対象にします。

`internal/mosaic` など制作支援で ONNX を要するテストは通常のテストと分け、環境がなければ理由を付けて skip します。モデルや共有ライブラリは本体のテストに要求しません。

## 理由

失敗した契約を job 単位で特定し、fork PR を含む検査の権限を小さく保ちます。race 用の cgo と、配布ビルド用の cgo 設定は分けます。

## 結果

T1 で lint・API・architecture・unit・build、T3〜T5 で描画・CLI E2E（[ADR 0016](0016-single-cli.md) で `gozen` の 1 本に統合）・ONNX の cgo ビルド、T7 で独立した examples・race・govulncheck・CodeQL と workflow 構文検査を配線しました。リモート CI の成功は未確認です。ジョブの対象、OS 条件、権限、推論テストの skip の扱いは [CI の構成](../development/ci.md) に記録します。
