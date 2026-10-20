# ADR 0006: Supported environments and CI

Status: Partly changed. Date: 2026-10-18. Corresponds to: D6, D10. Per-module checks were changed to the checks of a single module by [ADR 0009](0009-single-module.md). The current jobs and workflows are listed in [CI layout](../development/ci.md).

## Context

Tests of rules without a screen, running the drawing, the input and output of the CLI, and builds for the target OSes are different checks. A cross-build alone cannot show that something runs on an OS.

## Decision

GitHub Actions targets Ubuntu, Windows, and macOS, with separate jobs for lint, unit, graphics, cli-e2e, build, security, and building the examples. unit runs the minimum Go and stable on the 3 OSes. lint uses golangci-lint v2 and go-arch-lint v1.19.0, CLI E2E uses atago, and security uses govulncheck and CodeQL.

Linux graphics prepares dependencies and Xvfb with a SHA-pinned setup-ebitengine, then starts Xvfb and runs the checks. On Windows and macOS we check whether it can run, and record the reason when it cannot. For amd64 all 3 OSes are added, and for arm64 only cross-builds for Linux and macOS are added. Supported versions and run conditions are kept consistent in the documents.

Normal workflows have read-only permissions, external actions are pinned by SHA, and no checkout credentials are left behind. When writing is needed, as with sending CodeQL results, only the permissions needed are granted to that job. Dependabot covers gomod and Actions of the single module.

Tests that need ONNX for authoring support, such as `internal/mosaic`, are separated from normal tests and skipped with a reason when the environment is missing. Models and shared libraries are not required for the core tests.

## Reasons

It identifies the failed contract per job, and keeps the permissions of checks small, including for fork PRs. The cgo for race and the cgo settings for distribution builds are kept separate.

## Consequences

In T1 we wired lint, API, architecture, unit, and build. In T3 to T5 we wired drawing, CLI E2E (merged into the single `gozen` by [ADR 0016](0016-single-cli.md)), and the ONNX cgo build. In T7 we wired the independent examples, race, govulncheck, CodeQL, and workflow syntax checks. Success of the remote CI has not been confirmed. Job targets, OS conditions, permissions, and the handling of skipped inference tests are recorded in [CI layout](../development/ci.md).
