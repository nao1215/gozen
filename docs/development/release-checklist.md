# 公開前の点検

リリース候補に対して以下を確認します。成功したコマンド、対象 OS、必要な外部環境、未確認の項目を変更説明に記録します。非公開の検査記録はリポジトリ外に保存します。

## 内容と文書

- README、ADR、API の説明、help、コメント、設定、テスト、fixture、golden、サンプルを文脈ごと読み、一般向けの内容と制作支援の境界を [ADR 0004](../decisions/0004-content-boundary.md) に沿って確認します。
- 固有情報の混入検査は、リポジトリ外の非公開の一覧で別に回します。検査用の一覧や照合結果の詳細を公開物に含めません。
- 素材の出所とライセンス、モデルを同梱していないことを確認します。画像の見た目だけでなく、メタデータ、埋込みファイル、生成物、配布バイナリの文字列も対象にします。
- 削除した文書へのリンク、個人環境の絶対パス、ホスト名、私的な制作情報が残っていないか確認します。一般的な環境情報の漏れは `go test ./internal/dev/hygiene -count=1` でも検査します。
- README の要件と各 CLI の `--help` の Requirements を照合します。公開 API の変更は `api/public.txt`、godoc、利用例を併せてレビューします。

## 実行する検査

リポジトリのルートで実行します。依存と OS 別の準備は [開発と検証](testing.md)、[画面の開発](graphics.md)、[制作支援 CLI](production-tools.md) を参照してください。

```sh
make test lint build
make e2e
go test -race ./...
# Linux の描画検査。デスクトップ環境の Windows/macOS は直接実行する。
xvfb-run --auto-servernum --error-file=/dev/stderr make graphics
xvfb-run --auto-servernum --error-file=/dev/stderr make e2e-graphics
```

ONNX 実推論は利用者指定のモデルと共有ライブラリを準備して別に実行します。前提不足による skip、クロスビルド、未実行の検査を動作確認に数えません。Linux の結果から Windows/macOS の動作を推定せず、各 OS の CI 結果と graphics・音声の実行状況を確認します。govulncheck の DB 取得失敗も「脆弱性なし」と扱いません。CI の対象と権限は [CI の構成](ci.md) を参照してください。

## 紹介画像と配布

- Linux では `xvfb-run --auto-servernum --error-file=/dev/stderr bash scripts/capture-example.sh` でサンプルを撮影します。保存・復元と最終位置、GIF の寸法・枚数・長さの検査が成功することを確認し、PNG と GIF の表示内容も目視します。
- 画像と動画は一般向けのサンプルから作り、[素材台帳](../../ASSETS.md) と説明を揃えます。
- 配布用ビルドは `-trimpath -buildvcs=false` を使います。`make cli-build` で `bin/gozen` を生成し、文字列とビルド情報を点検します。
- タグ公開後は、README の `go install github.com/nao1215/gozen/cmd/gozen@version`（cgo あり・なしの両方） をリポジトリ外のクリーンな環境で確認します。公開前には未確認であることを明記します。

## リリースの仕組み

タグ `v*` の push で [Release workflow](../../.github/workflows/release.yml) が走ります。設定は [.goreleaser.yml](../../.goreleaser.yml) です。

- 成果物: `gozen` の Linux・macOS・Windows（amd64・arm64）のアーカイブ、Linux の `.deb`・`.rpm`・`.apk`、アーカイブごとの SBOM、cosign（keyless）で署名した `checksums.txt`、GitHub attestation、SLSA provenance（`multiple.intoto.jsonl`）。依存のライセンス本文を `THIRD_PARTY_LICENSES/` に同梱します。
- 配布先: `nao1215/homebrew-tap` の cask と、`microsoft/winget-pkgs` への pull request。
- 成果物は `CGO_ENABLED=0` で組み立て、ONNX 推論を含みません（[ADR 0016](../decisions/0016-single-cli.md)）。
- 公開の前に、Linux で全アーカイブとパッケージの中身・checksum を確かめ、Linux・macOS・Windows で実際に `gozen version` と `gozen check` を実行します（`scripts/smoke_artifacts.sh`）。

初めてタグを打つ前に、リポジトリの secret を 2 つ登録します。無いと release ジョブは GoReleaser の前で止まり、何も公開しません。

| secret | 書き込み先 | 種類と権限 |
| --- | --- | --- |
| `TAP_GITHUB_TOKEN` | `nao1215/homebrew-tap` | fine-grained、Contents: Read and write |
| `WINGET_GITHUB_TOKEN` | `nao1215/winget-pkgs` への push と `microsoft/winget-pkgs` への pull request | classic、`public_repo` |

手元では次で設定と成果物を確かめます（go-licenses v2 と syft が必要です）。`dist/` と `third_party_licenses/` は git の対象外です。

```sh
goreleaser check
goreleaser release --snapshot --clean --skip=publish,sign
./scripts/smoke_artifacts.sh dist
```

手順は、CHANGELOG の Unreleased を版の節に変える pull request をマージし、main の最新のコミットに annotated tag を打って push します。Release workflow の全ジョブ（smoke、release、provenance、verification）が成功し、Homebrew の cask と winget の pull request ができたことを確かめます。
