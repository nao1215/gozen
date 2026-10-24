# Development rules

Gozen is a no-code game creation toolkit for 2D RPGs, visual novels and strategy games, authored with YAML and assets and implemented in Go and Ebitengine. Public Go APIs remain during the phased migration; their retirement is the final stage (ADR 0041). Read [the architecture](docs/architecture/overview.md), [the API rules](docs/architecture/api-guidelines.md), and the relevant [ADRs](docs/decisions/index.md) before changing package boundaries.

## Scope and public content

- Work only in the assigned checkout. Treat external reference material as read-only.
- Do not include private game data, production notes, private generation text, private defaults, or machine-specific paths. Review meaning and context, including comments, fixtures, metadata, generated output, and binary strings.
- Keep the library and general-purpose commands suitable for general audiences. Only `internal/promptgen` and `internal/mosaic` (including their `command` packages, which `gozen prompt` and `gozen art mosaic` dispatch to) may provide the production-specific capabilities permitted by [the content boundary](docs/decisions/0004-content-boundary.md). Models are user-supplied, never bundled.
- Create original neutral fixtures and examples. Record the source and license of any third-party asset. Keep project-specific inspection lists outside this repository.
- In Bash, do not merge stderr into stdout or discard stderr. Do not use `pkill` or `pgrep -f`.

## Design and implementation

- Public packages live at the module root; experimental packages belong in `x/`. Default implementation details to `internal/` and assign them a layer before adding imports.
- Follow the downward dependency policy. Ebitengine belongs only to presentation and startup. Stable packages must not import experiments, commands, or development tooling.
- Use a small `Config` struct consistently rather than functional options. Use `Load(fsys fs.FS, path string) (T, error)` for filesystem loading. See the API rules for context, errors, ownership, and extension interfaces.
- Explain the consumer and purpose of every new export. Document its contract, add meaningful usage coverage, and review the `api/public.txt` diff. Inventory updates do not approve API growth.
- Do not add architecture exceptions to silence a violation. Restructure the code; a necessary policy change needs a reasoned ADR first.

## File organization and maintenance

- Do not add files merely to split a small change or append more coverage. Extend the existing file that owns the behavior unless a separate responsibility justifies a new file.
- Name unit test files after their implementation: `game.go` is tested in `game_test.go`. Do not use vague or coverage-driven suffixes such as `_more_test.go`, `_extra_test.go`, `_cov_test.go`, or numbered variants.
- Group tests spanning multiple implementation files by the behavior they integrate, in clearly named `*_integration_test.go` files or an established integration-test directory. Keep executable end-to-end scenarios in the existing E2E suite. Do not mislabel unit tests as integration tests to avoid the naming rule.
- Reuse existing helpers and implementations. When moving or replacing behavior, remove superseded code, duplicate logic and unused helpers in the same change; do not leave dead code or compatibility wrappers without an explicit consumer.
- Apply these rules to new and modified test organization, and consolidate nearby legacy test fragments when working on them. Preserve build tags, platform boundaries and meaningful test coverage during consolidation.

## Verification and reporting

- Read actual code before making claims. Cite `file:line` evidence; mark unverified behavior explicitly. Keep documentation under `docs/`, apart from repository entry documents.
- Use `rg` for searches. Run `make test lint build` or the equivalent commands in [development](docs/development/testing.md). The `gozen` command and its internal implementations belong to the root module and its checks.
- Choose tests for observable contracts: boundaries, failed I/O, state transitions, save/load round trips, and deterministic inputs. Isolate save paths and avoid parallel graphics initialization.
- Write every document (README.md, website/, docs/, CONTRIBUTING.md and other Markdown) in English only; `go test ./internal/dev/docs` fails on Japanese in a Markdown file. Keep the vocabulary of adult content inside the mosaic tool (internal/mosaic and the mosaic sections of the production-tools pages); the same test fails elsewhere.
- When you change README.md, the website or a picture they show, run `make docs-images` and the docs checks (`go test ./internal/dev/docs`, `make docs-steps`, `make website website-check`); see [README and site pictures and steps](docs/development/docs.md).
- Never report skipped, placeholder, cross-compiled, or unexecuted tests as runtime verification.
- Report completed work, commands and outcomes, limitations, and remaining work in the change description. Follow [the release checklist](docs/development/release-checklist.md) before publication. Keep private inspection records outside this repository.
