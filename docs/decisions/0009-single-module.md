# ADR 0009: Merge authoring support into a single module

Status: Accepted. Date: 2026-10-18. Corresponds to: D9, D10. It changes the multi-module layout of ADR 0001, the layout of ADR 0004, and the per-module checks of ADR 0006.

## Context

At first `tools/` was a separate module to keep heavy authoring dependencies and the purpose boundary apart. But Go does not build the dependencies of packages that are not used. There is no need to split a module only to avoid requiring cgo or an ONNX build from library users.

With separate modules, version tags, dependency updates, and checks are duplicated. Also `go install ...@version` does not work with a local `replace` in the target module. The `go.work` and core-first release plan of D9 would leave the same operational burden.

## Decision

Use the single module `github.com/nao1215/gozen`. `tools/` is abolished and no `go.work` is created. The entry points of authoring support are `gozen prompt` and `gozen mosaic` (merged into `cmd/gozen` by [ADR 0016](0016-single-cli.md)), and the implementation is `internal/promptgen` and `internal/mosaic` (including their subtrees for detection, splitting, tagging, and configuration). The public API is not increased.

After publication the CLI is installed with `go install github.com/nao1215/gozen/cmd/gozen@latest`. It is handled with a single version and tag, and no separate tools tag or core-first publication step is needed. The help of `gozen` and the diagnostics for missing prerequisites follow the [API guidelines](../architecture/api-guidelines.md).

The content standard is separated by location, not by module. The permission in [ADR 0004](0004-content-boundary.md) applies only to the authoring-support locations above, and private work data is included nowhere. Models are specified by the user and are not bundled.

## Reasons and alternatives

Compared with the alternative of keeping separate modules joined by a workspace, installation, updates, and releases are simpler. In exchange, the boundary of content and dependencies is stated explicitly. Using a single module is not a guarantee that dependency downloads or per-module vulnerability checks are also separated.

## Consequences

Make, CI, and Dependabot target the root module. The ONNX execution check is separated from normal tests, and missing prerequisites are skipped with a reason. A build with cgo disabled is not treated as a guarantee that ONNX inference works.

The dependency check classifies the entry points and implementation of authoring support as a component for each tool. Dependencies from the library, examples, and experiments into authoring-support internals are not allowed. Dependencies from the CLI entry point into authoring-support internals were allowed by [ADR 0016](0016-single-cli.md). New external dependencies are registered after their necessity is confirmed at implementation time.

Common development rules are consolidated in `AGENTS.md`. Separate files for a particular agent, such as `CLAUDE.md`, are not kept (author's decision on 2026-10-19). Tuning procedures for a particular agent are not published.
