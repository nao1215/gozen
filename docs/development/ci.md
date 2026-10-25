# CI configuration

The workflows are the [CI workflow](../../.github/workflows/ci.yml), the [Security workflow](../../.github/workflows/security.yml) and the ones in the table under "Other workflows" below. They run on pushes to main, pull requests and manual runs. The govulncheck of the security workflow also runs every day at 04:00 UTC, to check existing dependencies against new vulnerability reports (the scheduled run is skipped while the repository is private). This page describes the wiring; it is not a record of successful remote runs.

## Minimal while private

A private repository has a limit on Actions time. While it is private, only `quick` (Linux vet and tests), `lint` and `govulncheck` run on pushes and pull requests. The other jobs are stopped by `github.event.repository.private` and run only after the repository is public or on a manual run (workflow_dispatch). Scorecard and gitleaks require a public repository, so they do not run even manually while it is private.

## Jobs and targets

| Job | Environment and check | What counts as a result |
| --- | --- | --- |
| lint | Linux, Go 1.27.2 (`LINT_GO` of the Makefile, not stable: gofmt's output changes between Go releases). golangci-lint v2.14.0 (built by `make lint` with that Go), go-arch-lint v1.19.0, actionlint v1.7.12, zizmor v1.31.1. `make lint` | import direction, API list, tidy modules, static analysis, formatting, workflow syntax and security |
| unit | Ubuntu, Windows and macOS × Go 1.26.10 / stable. `go test ./...` with cgo enabled | non-graphics contracts, including `gozen` and internal |
| race | Linux, stable, `go test -race -short ./...` with cgo enabled | race check of the non-graphics contracts (the heavy tests, skipped by `-short`, run without `-race` in the other jobs) |
| build | amd64 on 3 OSes, arm64 on Linux and macOS, js/wasm. cgo disabled | all packages on desktop targets; the player and its dependencies for browsers. arm64 and wasm are cross-builds. No ONNX inference |
| graphics | 3 OSes, stable. Linux uses setup-ebitengine and Xvfb; Windows and macOS use the native desktop | real loop of 6 screens with PNG checks, pixel checks of the moving standing pictures, and the capture E2E that starts a real game as a child process. Audio is muted |
| cli-e2e | 3 OSes, stable, atago v0.25.2 | exit status, stdout and diagnostics, file changes and generated files of every subcommand of the real `gozen`. The graphics tag is a separate job |
| inference-build | 3 OSes, stable, cgo enabled | building `gozen` with the ONNX adapter and inference. Inference tests without a model skip with a reason |
| examples | 3 OSes, stable | building `./examples/...` and running minimal and headless. Running the screen is left to graphics |
| govulncheck | Linux, Go 1.26.10 (the lower bound in `go.mod`) and stable, cgo enabled, govulncheck v1.9.0 | known vulnerabilities reachable from `./...`. Judged from JSON; fails on anything not in `ACCEPTED` (empty now). The ONNX binding is analyzed too |
| codeql | Linux, stable, cgo enabled | builds `./...` explicitly and runs Go static analysis |

The lower bound of Go is `go.mod:3`. Keep it in step with the lower bounds of unit and govulncheck when updating. security uses stable, which can run the analysis tools. lint uses one pinned Go, the same one `make lint` uses locally, so formatting is judged by the same gofmt everywhere; bump `LINT_GO` and the lint job's `go-version` together. Normal tests do not initialize a screen. graphics keeps Ebitengine's limit of one loop per process: `TestMain` of `internal/play/graphics_test.go` builds `internal/play/testdata/host` once, and the scenarios run side by side, each in a process of its own.

graphics is also enabled on Windows and macOS, with no setting that tolerates failure and no skip. setup-ebitengine prepares Xvfb only on Linux. This working environment has not confirmed whether the Windows and macOS hosted runners have the desktop that is needed. Check it in the first remote run; if it cannot run, record the concrete error and reason and reconsider. A successful cross-build is not accepted in its place.

## Pull requests that change only documentation

The heavy jobs of the CI, Coverage, Benchmark, licenses, Release Smoke and Security workflows are skipped on a pull request that changes only documentation. Each of these workflows starts with a `changes` job that runs [scripts/ci_changes.sh](../../scripts/ci_changes.sh), which compares the merge commit of the pull request with its base. Documentation only means every changed file is a Markdown file, under `website/` or `docs/` (but not `docs/examples/`, which tests read), a `LICENSE` file or a GitHub issue template. Any other file, including `.github/workflows/` and the script itself, gives `code=true` and runs everything. Pushes, tags, schedules and manual runs are not judged and always run everything.

`on.paths` is not used: a workflow that never starts leaves its required status checks pending, and the pull request cannot be merged. Instead the jobs wait for `changes` and are skipped with `if`; GitHub counts a skipped job as passed for a required check. A skipped job with a matrix would be reported under the name without its matrix values and would not match a required check such as `unit (ubuntu-latest, stable)`. So the matrix jobs that are required (unit, cli-e2e, govulncheck) still start and skip their steps, and the other jobs are skipped as a whole. Only the answer `false` skips; if `changes` fails, everything runs.

The `docs-checks` job of the CI workflow runs on these pull requests only, with the tests that read the documentation (`internal/dev/docs`, `internal/diag`, `internal/layer2/gamedata`). The Docs and Website workflows are not skipped.

## Time

CI, Docs, Coverage, Benchmark and Release Smoke cancel superseded runs for the same pull request. Pushes to main and manual runs keep distinct groups and finish independently. The sample job saves its build cache after all three sample modules, including campaign, have passed.

The longest jobs are the pictures of the Docs workflow and graphics; most of their time is the samples playing in real time and writing 1920x1080 frames. The game writes a screenshot off its loop (PNG at BestSpeed, by background workers), the docs pictures' runs play side by side, and graphics runs its scenarios side by side.

- The graphics matrix runs screen tests while a separate capture matrix runs the CLI screenshot pipeline on all three operating systems. Both remain mandatory executions for code changes; neither waits for the other.
- Release Smoke keeps a dedicated snapshot build cache for pull requests and main pushes, with main as its only writer. Cache keys include the Go version, module sums and revision. Tag releases and manual runs use clean caches. The first main run populates the cache for subsequent pull requests.
- capture and cli-e2e install atago from its release binary with setup-atago (the checksum is checked) instead of building it with `go install`.
- race, go-zen, the Docs jobs and Coverage keep Go caches of their own (`actions/cache`, keyed by the job, Go version, `go.sum` files and revision, saved from main; a dependency-specific prefix restores the newest compatible cache so source changes can refresh it). setup-go's cache is one per Go version and `go.sum`, shared by every job and saved by whichever job ends first, often a short cross-build, so it held little of what these jobs build. The Website workflow, which deploys the site, is left with setup-go's cache.
- On a pull request, the images job of the Docs workflow remakes the pictures only when the pull request changes a file that can change them. It skips them when every changed file is a Markdown file, under `website/`, `.github/` (other than `docs.yml`), `e2e/`, `bench/` or `skills/`, `LICENSE`, or a `_test.go` file outside `internal/dev/docs`. Any other file, `docs/images/` and `internal/dev/docs/` run them, and so does every push to main and manual run.

## Other workflows

| workflow | When | What |
| --- | --- | --- |
| [fuzz](../../.github/workflows/fuzz.yml) | every day at 05:00 UTC and manually. Manual only while private | finds each `func Fuzz` and runs `go test -fuzz` on it for 5 minutes. Failing inputs are kept as an artifact. `go test` normally runs the seeds and `testdata/fuzz/` |
| [licenses](../../.github/workflows/licenses.yml) | push, pull request and manual. Manual only while private | judges the licenses of linked dependencies with go-licenses v2.0.1 on Linux, Windows and macOS. Fails on GPL, LGPL, AGPL and licenses that cannot be identified |
| [gitleaks](../../.github/workflows/gitleaks.yml) | pull request. Does not run while private | checks that the commits of a pull request contain no secret strings |
| [Scorecard](../../.github/workflows/scorecard.yml) | push to main, every Saturday and manually. Does not run while private | publishes the OpenSSF Scorecard score and shows it in the README badge |
| [Benchmark](../../.github/workflows/bench.yml) | pull request and manual. Manual only while private | measures `bench/himorime.yaml` with himorime and, on pull requests with unchanged benchmark definitions, compares base and head ([bench/README.md](../../bench/README.md)) |
| [Docs](../../.github/workflows/docs.yml) | push, pull request and manual. Manual only while private | images retakes every image of `docs/images/manifest.yaml` in Xvfb and compares them with the committed ones (`make docs-images-check`; if stale, the generated images are kept as an artifact). steps runs the steps of the README and site (`make docs-steps`). See [README and site pictures and steps](docs.md) |
| [Release Smoke](../../.github/workflows/release-smoke.yml) | pull request, push to main, manual, and called by Release | builds the release artifacts with GoReleaser in snapshot mode (nothing published), checks their contents and checksums on Linux, and runs `gozen --version`, `gozen help`, `gozen version` and `gozen check` on Linux, macOS and Windows |
| [Release](../../.github/workflows/release.yml) | push of a tag `v*` | runs Release Smoke, then publishes ([How releases work](release-checklist.md#how-releases-work)) |

## ONNX

Normal CI fetches neither a model nor a shared library. `inference-build` runs `TestONNX` verbosely and logs whether the cgo compile succeeded and why tests without a model skipped. This does not mean inference ran. For an explicit local inference check, set the environment variables in [Production tools](production-tools.md#verification). A model that is set but cannot be read, or does not fit, fails (`internal/mosaic/command/inference_test.go:62`).

## Documentation site

The [Website workflow](../../.github/workflows/website.yml) builds `website/` with Hugo, indexes it with Pagefind and publishes it to GitHub Pages (https://nao1215.github.io/gozen/). Hugo and Pagefind are pinned by version and SHA256, and actions are pinned by SHA. While the repository is private, no job runs, to save the Actions budget. After it is public, jobs run on pushes to main, pull requests and manual runs (workflow_dispatch). The publishing write permissions (`pages: write`, `id-token: write`) are given only to the deploy job, and the verify job checks the commit and the main pages of the published site.

After building, the build job runs `make website-check`, which checks the links, images and `#` targets inside the pages and the links to files of this repository, and `make website-play-check`, which opens the built Go Zen, novel and campaign pages (`/play/go-zen/`, `/play/novel/`, `/play/campaign/`) in headless Chrome and fails when a game draws nothing within two minutes or the page logs an exception or a failed request (scripts/web_play_check.mjs; its screenshots are kept as the `web-play` artifact). It reports startup/input/screenshot peaks separately, then measures five seconds of idle rendering against the 250 ms Update/Draw limit. A missing frame-meter sample fails the check. Because the site carries the samples built from the engine, the workflow runs on every push and pull request, not only on changes under `website/`. Locally, `make website` (needs hugo) builds into `website/public/`. The Schema page is built from `data/schema/types.schema.json`, with the English wording of its contract cells in `website/data/schema_en.yaml` and a description of every type and field in `website/data/schema_fields/`. A new field, or a cell without English, fails the site build and `go test ./internal/dev/docs` until it is described.

## Permissions and dependency updates

Every workflow defaults to `contents: read` (Scorecard uses `read-all`), and checkout uses `persist-credentials: false`. There is no `pull_request_target` and no commit by a bot. Write permissions go only to the jobs that need them. CodeQL uploads results with `security-events: write` on the codeql job; Scorecard uses `security-events: write` and `id-token: write` to upload and publish results; gitleaks uses `pull-requests: read` to list commits; Release uses `contents: write`, `id-token: write` and `attestations: write` for publishing, signing and provenance. Only the Homebrew step of Release uses a secret ([ADR 0006](../decisions/0006-platforms-and-ci.md)). Analysis also runs on fork PRs, with uploads disabled. Private vulnerability reporting and the CodeQL display also need settings on the GitHub side, whose state is not confirmed in this checkout.

External actions are pinned by 40-character SHA. checkout, setup-go and setup-ebitengine keep the pins of the existing CI, and CodeQL uses a pin adopted from existing OSS. The tag comments noted here are only for reference; what runs is the SHA. Reachability of the remotes was not confirmed by the local checks of this stage.

[Dependabot](../../.github/dependabot.yml) watches the gomod of the root, of `examples/go-zen`, of `examples/novel` and of `examples/campaign`, and github-actions, weekly. It does not update `LINT_GO` or the lint job's Go. Nothing merges its proposals automatically. When updating, review the change in the action, the compatibility of tools with Go, and the behavior in an environment without a model, and pass the required checks.

## Local checks

[Development and verification](testing.md) has the tool installation and the direct commands that correspond to Make. Syntax-only checks of workflows are `make workflows`; the vulnerability checks are the following commands.

```sh
go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
govulncheck ./...                              # with the Go of go.mod (1.26.10 or later)
GOTOOLCHAIN=go1.26.10 govulncheck ./...         # with the standard library of the lower bound
gitleaks git                                   # secret strings in commits
```

govulncheck needs network access to the vulnerability DB. Do not read a connection error as "no vulnerabilities". CodeQL extraction, analysis and upload are checked separately on GitHub Actions. For the check before publication, follow the [release checklist](release-checklist.md).

The Docs image check uses four independent shards with an aggregate `images` status. Shared capture dependencies stay on one shard, and all image comparisons remain mandatory. See [parallel image checks](docs.md#parallel-image-checks-in-ci) for selection, timing and cache behavior.
