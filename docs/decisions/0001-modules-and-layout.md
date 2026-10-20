# ADR 0001: Modules and layout

Status: Partly changed. Date: 2026-10-18. Corresponds to: D1, D9, D10. The original decision to split into separate modules was superseded by [ADR 0009](0009-single-module.md). The CLI layout was changed by [ADR 0016](0016-single-cli.md). The Go minimum was raised to 1.26.9, which includes standard library vulnerability fixes (`go.mod`).

## Context

The core of the game becomes a library usable from several games. The inference runtime for authoring support must not be required of every user who embeds a game.

## Decision

There is one module, `github.com/nao1215/gozen`, starting on the Go 1.26 series with 1.26.4 as the minimum. CI checks both the exact minimum and stable. The dependencies are Ebitengine v2.10.4, goccy/go-yaml v1.19.2, and x/image v0.45.0 (`go.mod:1`).

The library sits directly under the module root without an `engine/` directory, and the implementation is hidden in `internal/`. The CLI is a single `cmd/gozen` (changed from 4 binaries by [ADR 0016](0016-single-cli.md)). The authoring-support implementation lives in `internal/promptgen`, `internal/mosaic`, and their subtrees, and is not public API. No nested modules and no `go.work` are created.

## Reasons

A short import path shows the public boundary. Go does not build the dependencies of packages that are not used, so a single module can be kept without requiring the authoring-support cgo and ONNX from library users. Each CLI function has its own name and help, so only the needed features need to be installed.

## Consequences

`go test ./...`, lint, and build at the root are the entry point for everything. Inference execution checks are a separate target with prerequisites. No model is bundled. The content boundary follows [ADR 0004](0004-content-boundary.md).
