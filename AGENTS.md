# Development rules

Gozen is a public Go and Ebitengine 2D RPG framework and toolkit. Read [the architecture](docs/architecture/overview.md), [the API rules](docs/architecture/api-guidelines.md), and the relevant [ADRs](docs/decisions/index.md) before changing package boundaries.

## Scope and public content

- Work only in the assigned checkout. Treat external reference material as read-only.
- Do not include private game data, production notes, private generation text, private defaults, or machine-specific paths. Review meaning and context, including comments, fixtures, metadata, generated output, and binary strings.
- Keep the library and general-purpose commands suitable for general audiences. Only `internal/promptgen` and `internal/mosaic` (including their `command` packages, which `gozen prompt` and `gozen mosaic` dispatch to) may provide the production-specific capabilities permitted by [the content boundary](docs/decisions/0004-content-boundary.md). Models are user-supplied, never bundled.
- Create original neutral fixtures and examples. Record the source and license of any third-party asset. Keep project-specific inspection lists outside this repository.
- Do not modify `.git` or commit or push changes. The task coordinator handles version control.
- In Bash, do not merge stderr into stdout or discard stderr. Do not use `pkill` or `pgrep -f`.

## Design and implementation

- Public packages live at the module root; experimental packages belong in `x/`. Default implementation details to `internal/` and assign them a layer before adding imports.
- Follow the downward dependency policy. Ebitengine belongs only to presentation and startup. Stable packages must not import experiments, commands, or development tooling.
- Use a small `Config` struct consistently rather than functional options. Use `Load(fsys fs.FS, path string) (T, error)` for filesystem loading. See the API rules for context, errors, ownership, and extension interfaces.
- Explain the consumer and purpose of every new export. Document its contract, add meaningful usage coverage, and review the `api/public.txt` diff. Inventory updates do not approve API growth.
- Do not add architecture exceptions to silence a violation. Restructure the code; a necessary policy change needs a reasoned ADR first.

## Verification and reporting

- Read actual code before making claims. Cite `file:line` evidence; mark unverified behavior explicitly. Keep documentation under `docs/`, apart from repository entry documents.
- Use `rg` for searches. Run `make test lint build` or the equivalent commands in [development](docs/development/testing.md). The `gozen` command and its internal implementations belong to the root module and its checks.
- Choose tests for observable contracts: boundaries, failed I/O, state transitions, save/load round trips, and deterministic inputs. Isolate save paths and avoid parallel graphics initialization.
- Never report skipped, placeholder, cross-compiled, or unexecuted tests as runtime verification.
- Report completed work, commands and outcomes, limitations, and remaining work in the change description. Follow [the release checklist](docs/development/release-checklist.md) before publication. Keep private inspection records outside this repository.
