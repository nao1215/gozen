# Development and verification

## Environment for normal tests

The lower bound of Go is 1.26.10, at `go.mod:3`. Normal tests check the contracts of layers 1 to 6, shared internals, the neutral test game and loading of the samples. They do not initialize a screen, audio or inference. Normal tests and builds need no Xvfb, ONNX shared library, model or game assets. The cgo build of mosaic uses a C compiler. A test also runs that detects the form of paths of common personal environments and private host names. It does not replace checks for content specific to a work.

lint uses golangci-lint v2.14.0 (the linters and formatters are in `.golangci.yml`), the dependency direction uses go-arch-lint v1.19.0 (config version 3), workflow syntax uses actionlint v1.7.12, and workflow security uses zizmor v1.31.1 (its policy is in `.github/zizmor.yml`). Go's work cache and temporary directory need write permission. In a restricted environment, point `GOCACHE`, `GOTMPDIR` and `GOLANGCI_LINT_CACHE` to writable places, and create the directories first.

```sh
go install github.com/fe3dback/go-arch-lint@v1.19.0
go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
pipx install zizmor==1.31.1
```

golangci-lint is not installed: `make lint` builds and runs it with `go run` under `GOTOOLCHAIN=go1.27.2` (`LINT_GO` in the Makefile), the same Go as the CI lint job. gofmt's output changes between Go releases (Go 1.27 aligns map literals with wide characters differently from 1.26), so a golangci-lint built with another Go reports formatting that CI does not, or misses what CI reports. The first run downloads that toolchain. Without Make, run `GOTOOLCHAIN=go1.27.2 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run ./...` and the same with `fmt --diff` (in PowerShell, set `$env:GOTOOLCHAIN` first).

The declared dependency versions are at `go.mod:5`. T3 actually uses go-yaml, Ebitengine and x/image. In an environment that needs to fetch dependencies, run `go mod download`.

## Make and direct commands

Windows is assumed to have neither Make nor Bash. At the repository root, run the direct commands below from PowerShell or the command prompt. Put the external tools `go-arch-lint`, `actionlint` and `zizmor` on PATH; golangci-lint runs through `go run` as above.

| Make | Same thing run directly | Current target |
| --- | --- | --- |
| `make test` | `go test ./...` | contract tests of the core, `gozen` and internal, samples, API and development tools |
| `make arch` | `go-arch-lint check` | runtime imports and vendor, deep dependency check |
| `make api` | `go test ./api -count=1` | comparing the golden with the current public identifiers |
| `make tidy` | `go mod tidy -diff`, also in `examples/go-zen` and `examples/novel` | go.mod and go.sum hold what the imports need, no more |
| `make workflows` | `actionlint`, `zizmor --offline .github/workflows` | syntax, expressions and shell of GitHub Actions workflows; their pins, credentials, injection and caches |
| `make api-update` | `go run ./internal/dev/apigen` | updating the list to review |
| `make lint` | `go-arch-lint check`, `go test ./api -count=1`, `make tidy`, `make workflows`, `golangci-lint run ./...`, `golangci-lint fmt --diff`, then the same two in `examples/go-zen` and `examples/novel`, in order | dependencies, API, modules, workflows, static analysis, formatting |
| `make build` | `go build -trimpath -buildvcs=false ./...` | compiling the core, `gozen` and internal, samples and development tools |
| `make graphics` | `go test -tags=graphics ./play -run TestGraphicsScreens -count=1` and `go test -tags=graphics ./puppet -run TestGraphics -count=1` | real loop and PNG checks of 6 screens, and pixel checks of the moving standing pictures (drawn and read inside one loop). Wrapped in Xvfb on Linux |
| `make e2e` | [direct steps for the CLI](cli.md#e2e) | contract checks of every `gozen` subcommand with atago v0.25.2 |
| `make docs-images` / `make docs-images-check` | `go test ./internal/dev/docs -run '^TestDocImages$' -count=1` with `GOZEN_DOCS_IMAGES=update` (or `check`); inside xvfb-run on Linux | remake the README and site pictures from the ledger / fail if stale ([README and site pictures and steps](docs.md)) |
| `make docs-steps` | `GOZEN_DOCS_STEPS=1 go test ./internal/dev/docs -run '^TestDocSteps$' -count=1` | running the steps of the README and site |
| `make website-check` | `GOZEN_SITE_DIR=website/public go test ./internal/dev/docs -run '^TestSiteLinks$' -count=1` | link and image references of the built site |
| `make e2e-graphics` | `atago run --ci --parallel 1 --tag graphics e2e/atago` (see the CLI steps for the build and PATH setup) | capture E2E with a game child process. Wrapped in Xvfb on Linux |

`e2e` was replaced in T4 with checks of the real binary using atago. `graphics` is the screen test of T3, and `e2e-graphics` is the CLI capture pipeline of T4. A failure caused by a missing prerequisite is not counted as a success or a skip.

`go build -trimpath -buildvcs=false ./...` compiles the libraries; it does not put a game or CLI in `bin/`. `-trimpath` removes absolute source paths and `-buildvcs=false` stops automatic collection of VCS information. Build the individual CLI binary into `bin/` with `make cli-build`.

`make cli-build`, `make e2e-graphics` and the individual binary builds in CI also pass `-ldflags=-compressdwarf=false`. It keeps the debug information but disables DWARF compression, which avoids bytes of compressed data accidentally being picked up as strings by content checks with `strings`. The binary is larger than a compressed one. What the content checks look at and how they judge do not change.

The module is a single one at the root. `cmd/gozen`, `internal/promptgen` and `internal/mosaic` are checked by the same `./...`. No per-directory checks under `tools/` and no `go.work` are needed. T5 also implemented production support and included it in the normal checks and the atago E2E.

## Graphics tests

Tests that draw are separated with the `graphics` build tag. On Linux run `xvfb-run --auto-servernum --error-file=/dev/stderr make graphics`; on Windows and macOS run the direct commands of the table from a desktop. The 6 scenarios run in separate processes, read pixels inside the game loop, and check the image content and normal exit. Audio is muted.

Per-OS setup, the sample and input scripts are in [Graphics development](graphics.md). Normal tests, cross-builds and merely preparing Xvfb do not count as a graphics success. In T3 the real-drawing acceptance remained undone because of environment limits. During the sample implementation (T6), the Linux screen tests and the sample captures were run. What the screen tests cover now is in the `make graphics` row of the table above. Real drawing and real audio on Windows and macOS are not confirmed.

## CLI and ONNX tests

For the CLI, new neutral specs in `e2e/atago/` check exit status, stdout/stderr, invalid input, file changes and generated files with atago. Specs that open a screen are separate from normal specs. T4 added the neutral specs in `e2e/atago/` and aligned atago v0.25.2 in Make and CI. The production support added in T5 is also a normal target, and the real capture with the graphics tag is separate. For installation and the direct Windows steps, see [Verifying the CLI](cli.md#e2e).

In `internal/promptgen` and `internal/mosaic`, checks without inference are separate from checks that use ONNX. Checks that use inference live under another tag or an explicit target, and `t.Skip` explains the missing prerequisite when the shared library or model is absent. The real model path is given by the user, and test failures are not replaced indiscriminately with skips. The ONNX tests of T5 run real inference only when the model and shared library are given explicitly. See [how to set it](production-tools.md#verification). No model is bundled.

## CI

T7 collected the configuration, OSes, permissions, pinned tools and dependency updates in [CI configuration](ci.md). Besides the normal lint, unit and build, it wires graphics, the atago E2E of `gozen`, separate examples, Linux race, govulncheck and CodeQL. The cgo compile of ONNX and the skip reason when no model is given are checked in the separate inference-build. Distinguish the jobs that are defined from the actual remote results. Before publication, follow the [release checklist](release-checklist.md) to confirm the results and the unverified items.

## Additional checks of T2

```sh
go test -race ./...
go test ./internal/expr -run '^$' -fuzz '^FuzzParse$' -fuzztime=10s -parallel=2
go run ./examples/headless
```

The race check needs cgo and a C compiler. The sample uses original in-memory definitions and generated images, and runs moving, talking, battle, saving and restoring. On success it prints `location=trail battles=1 wins=1 gold=3`. OS file operations of the save destination are isolated by the tests of `savefile` and `rpg` with `t.TempDir()`.

After changing the schema, run `GOZEN_UPDATE_SCHEMA=1 go test ./internal/layer2/gamedata -run '^TestSchemaCoverage$'`. It regenerates `data/schema/` (the [Schema page](https://nao1215.github.io/gozen/schema/) of the site is built from it); confirm with a normal `go test` that they match. This is a separate check from the API update.

These are not execution checks of drawing, real audio, the CLI or ONNX.
