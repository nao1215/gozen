# Contributing to Gozen

Gozen is a YAML-and-assets game toolkit. Describe the behavior you need in an issue before proposing a large feature or a CLI/YAML contract change. For bugs, include the Go version, OS, command, expected result and a small reproducible example. Use original, neutral fixtures and remove private data, models and machine-specific paths.

Read [AGENTS.md](AGENTS.md), the [architecture](docs/architecture/overview.md), [API rules](docs/architecture/api-guidelines.md), and relevant [ADRs](docs/decisions/index.md). Application packages belong under internal with an assigned level (internal/dev/hygiene/levels.go). Keep dependencies directed downward and graphics outside the core. Explain the consumer and contract of each new export and run `make boundary`.

Before v1.0.0 the YAML and save formats may change between versions without migration tools. Do not add migration commands or YAML upgraders.

Use Go 1.26.11 or later. Install the pinned development tools listed in the [testing guide](docs/development/testing.md), then run:

```sh
make test lint build
make e2e
# Linux, with graphics dependencies installed:
xvfb-run --auto-servernum --error-file=/dev/stderr make graphics e2e-graphics
```

[Direct commands](docs/development/testing.md#make-and-direct-commands) are available for environments without Make. The `gozen` command and its internal implementations share the root module checks. Choose regression tests for observable behavior: errors, I/O failures, state transitions, deterministic inputs and save/load round trips. Isolate saves and do not initialize graphics in parallel. ONNX tests require user-supplied models and runtime; explain missing prerequisites when reporting a skip.

Keep explanatory documentation under `docs/`. In a pull request, explain the change, its API impact, the commands run and their results, and anything not verified. Cross-compilation and skipped tests do not establish runtime support. Follow the [release checklist](docs/development/release-checklist.md) before publication. See the [CI guide](docs/development/ci.md) for the checks and permissions.

Create original general-audience examples. Record third-party asset sources and licenses in [ASSETS.md](ASSETS.md); do not include models or private game material. Contributions are provided under the repository's [MIT license](LICENSE), with separate notices preserved for third-party assets.

Report vulnerabilities through the private process in [SECURITY.md](SECURITY.md).

Run `go tool dango lint` for maintainer decision checks; see [Dango](docs/development/dango.md).

Runtime changes must preserve the [speed budgets](docs/development/speed-budgets.md). Run the headless budget with `go test ./internal/dev/perf`; run the graphics cases on a desktop or with Xvfb. Report each named section and distinguish measured runtime results from compile-only checks.
