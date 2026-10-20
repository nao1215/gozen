# Pre-release checklist

Check the following for a release candidate. Record in the change description the commands that succeeded, the target OS, the external environment needed and the items not verified. Keep private check records outside the repository.

## Content and documents

- Read the README, ADRs, API descriptions, help, comments, settings, tests, fixtures, goldens and examples in context, and confirm the boundary between general-audience content and production support follows [ADR 0004](../decisions/0004-content-boundary.md).
- Run the check for leaked specific information separately, with a private list kept outside the repository. Do not put the check list or the details of its matches in public material.
- Confirm the source and license of assets and that no model is bundled. Cover metadata, embedded files, generated files and strings in distributed binaries, not only how images look.
- Confirm that no links to deleted documents, absolute paths of a personal environment, host names or private production information remain. `go test ./internal/dev/hygiene -count=1` also checks for leaks of common environment information.
- Compare the README requirements with the Requirements in each CLI `--help`. Review a public API change together with `api/public.txt`, godoc and examples.

## Checks to run

Run at the repository root. For dependencies and per-OS setup, see [Development and verification](testing.md), [Graphics development](graphics.md) and [Production tools](production-tools.md).

```sh
make test lint build
make e2e
go test -race ./...
# Graphics checks on Linux. On Windows/macOS with a desktop, run them directly.
xvfb-run --auto-servernum --error-file=/dev/stderr make graphics
xvfb-run --auto-servernum --error-file=/dev/stderr make e2e-graphics
```

Run ONNX inference separately, with a user-supplied model and shared library. Do not count skips caused by missing prerequisites, cross-compilation or checks that were not run as verification. Do not infer Windows/macOS behavior from Linux results; check the CI result of each OS and how graphics and audio ran. Treat a failed govulncheck DB download as not verified, not as "no vulnerabilities". For the CI targets and permissions, see [CI configuration](ci.md).

## Pictures and distribution

- On Linux, `make docs-images-check` confirms that the README and site pictures match the current sample, and `make docs-steps` and `make website-check` confirm the steps and the links of the pages ([README and site pictures and steps](docs.md)). Look at regenerated pictures.
- Make images and videos from the general-audience sample, and keep the [asset ledger](../../ASSETS.md) and descriptions consistent.
- Use `-trimpath -buildvcs=false` for distribution builds. Build `bin/gozen` with `make cli-build` and inspect its strings and build information.
- After the tag is published, check the README `go install github.com/nao1215/gozen/cmd/gozen@version` (with and without cgo) in a clean environment outside the repository. Before publication, state that it is not verified.

## How releases work

Pushing a tag `v*` runs the [Release workflow](../../.github/workflows/release.yml). Its configuration is [.goreleaser.yml](../../.goreleaser.yml).

- Artifacts: `gozen` archives for Linux, macOS and Windows (amd64, arm64), Linux `.deb`, `.rpm` and `.apk` packages, an SBOM per archive, `checksums.txt` signed with cosign (keyless), GitHub attestation and SLSA provenance (`multiple.intoto.jsonl`). The license texts of dependencies are bundled in `THIRD_PARTY_LICENSES/`.
- Destinations: the cask in `nao1215/homebrew-tap`.
- Artifacts are built with `CGO_ENABLED=0` and contain no ONNX inference ([ADR 0016](../decisions/0016-single-cli.md)).
- Before publication, the contents and checksums of all archives and packages are checked on Linux, and `gozen version` and `gozen check` are run on Linux, macOS and Windows (`scripts/smoke_artifacts.sh`).

Before the first tag, register a repository secret. Without it the release job stops before GoReleaser and publishes nothing.

| secret | writes to | kind and permission |
| --- | --- | --- |
| `TAP_GITHUB_TOKEN` | `nao1215/homebrew-tap` | fine-grained, Contents: Read and write |

Locally, check the configuration and artifacts as follows (go-licenses v2 and syft are required). `dist/` and `third_party_licenses/` are ignored by git.

```sh
goreleaser check
goreleaser release --snapshot --clean --skip=publish,sign
./scripts/smoke_artifacts.sh dist
```

The procedure: merge a pull request that turns Unreleased in the CHANGELOG into a version section, then put an annotated tag on the latest commit of main and push it. Confirm that all jobs of the Release workflow (smoke, release, provenance, verification) succeed and that the Homebrew cask was created.

The repository social preview cannot be set through the API: upload `docs/images/gozen-ogp.png` in the repository Settings (Social preview) by hand.
