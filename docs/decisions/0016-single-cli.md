# ADR 0016: One CLI named gozen

Status: Partly changed. Date: 2026-10-19. The option syntax and the shape of help were changed in [ADR 0018](0018-gnu-options-and-help.md) ("flags unchanged" in the table below is about names; the syntax is GNU style). This renames the entry point of [ADR 0012](0012-cli-ownership-and-output.md), the CLI placement of [ADR 0001](0001-modules-and-layout.md) and [ADR 0009](0009-single-module.md), and the authoring-support placement of [ADR 0004](0004-content-boundary.md).

Partly changed by [ADR 0024](0024-command-tree-and-diagnostic-codes.md): the subcommands are grouped into check, sim, play, book, pack and art.

## Users and problem

The users are authors who make games with Gozen. They use definition checking, battle simulation, documentation, partial distribution and screenshots, and as needed the prompt assembly for authoring support and the mosaic processing of images.

Until now there were four CLIs: `rpgcheck`, `rpgtool`, `promptgen` and `mosaic`. They had to be installed four times, and the names did not show that they were Gozen tools. Being generic names, they might also collide with other tools. `rpgcheck` and `rpgtool check` both check definitions, which left it unclear which to use. The Requirements in help, the README sections, the exit-code conventions, and the steps in the Makefile, CI and E2E were also split in four places.

## Decision

The CLI is the single `cmd/gozen`. Install it with `go install github.com/nao1215/gozen/cmd/gozen@latest`.

| Subcommand | Original CLI |
| --- | --- |
| `check`, `layout`, `sim`, `curve`, `floors`, `trace`, `book`, `design`, `memos`, `release`, `trial`, `shots` | The subcommand of the same name in `rpgtool` |
| `check --no-layout` | `rpgcheck` |
| `prompt` | `promptgen` (subcommand `lint` and flags unchanged) |
| `mosaic` | `mosaic` (subcommands and flags unchanged) |
| `version`, `help [command]` | New |

`rpgcheck` is removed. What only `rpgcheck` did was a structure-only check that skips font parsing and the text-fit check, and the line `Structure checked; layout was not checked.`. This moves to `gozen check --no-layout`. The exit values are the same (0 for success, 1 for a check or I/O failure, 2 for a bad argument). `gozen help <command>` prints the same as `gozen <command> --help`.

The arguments, flags, output and exit values of `prompt` and `mosaic` are owned by `internal/promptgen/command` and `internal/mosaic/command` respectively. `cmd/gozen` owns only process exit and dispatch. The exit values of `prompt` are aligned with the other subcommands: 0 for help and success, 2 for a bad argument, and 1 for any other failure (the old `promptgen` returned 1 for a bad argument too). Diagnostic messages are also aligned in English.

ONNX inference stays separated by the existing build tags (the `cgo` and `!cgo` adapters). A `gozen` built with `CGO_ENABLED=0` still offers help, hand-fixed image processing and checking of existing reports. For work that needs inference, it prints to stderr that this build has no inference and how to reinstall, and exits with 1. Go disables cgo by default when no C compiler is found, so even in an environment without a C compiler `go install` succeeds as a build without inference.

No compatibility with the old names is kept. No aliased binaries and no old `cmd/` packages are made, because there is no release yet and no external user.

The README CLI section becomes the single `#gozen`. The Requirements in help list only what each subcommand actually needs, and point to the same section.

In the dependency check, the `commands` component is `cmd/gozen/**` and `cmd/internal/**`. Dependencies from `commands` to the `promptgen` and `mosaic` components are allowed. Dependencies from the authoring-support internals to `commands`, and from libraries, samples and experiments to the authoring-support internals, remain disallowed as before.

## Alternatives

1. Make only what needs cgo a separate binary. Put `gozen-mosaic` apart, and `gozen mosaic` runs the `gozen-mosaic` found on PATH (the form of git and kubectl plugins). This keeps the main binary free of cgo. But installation takes two steps, and version mismatch and PATH lookup add failure paths. A `gozen` built without cgo also works except for inference, and guides reinstallation only when inference is needed, so the benefit of a separate binary is small. So it is one binary.
2. Group the image tools (`prompt` and `mosaic`) into a separate authoring-support binary. This separates general tools from authoring support by name. But the users are the same authors, and the problem of install and help conventions split in two remains. The content boundary can be kept by placement (internal packages), so it is not chosen.
3. Put everything in one binary and require cgo for inference. In an environment without a C compiler, `gozen` itself could not be built. It would demand a C compiler even from users who only use checking and simulation, so it is not chosen. This decision is a single binary with optional cgo.

## Consequences

- The install, build and E2E steps for four tools become one. The Makefile `cli-build` makes only `bin/gozen`. The CI E2E and inference build also target `./cmd/gozen`.
- A `gozen` built with cgo enabled contains the ONNX binding. The shared library is loaded at run time by `--ort` or `ONNXRUNTIME_LIB`, so work that does not use inference needs neither the shared library nor the model.
- The general CLI entry point now depends on authoring-support implementations. The permission in [ADR 0004](0004-content-boundary.md) is limited to `internal/promptgen` and `internal/mosaic` (including their `command` packages). `cmd/gozen` holds only dispatch and the neutral help listing.
- The conventions of [ADR 0012](0012-cli-ownership-and-output.md) for output destination, exit values and overwrite protection carry over to every `gozen` subcommand, including `prompt`.
- Release artifacts (goreleaser archives, Linux packages, Homebrew) are built with `CGO_ENABLED=0` and contain no inference, because there is no cgo cross-compiler for the six OS and CPU combinations. An author who needs inference reinstalls on a machine with a C compiler using `CGO_ENABLED=1 go install github.com/nao1215/gozen/cmd/gozen@latest`. The diagnostic of the work that needs inference shows this reinstall method (`internal/mosaic/detect/inference_nocgo.go`).
- winget was dropped later (2026-10-20); Windows users install the zip archive.
- Making inference pure Go (running ONNX Runtime as WebAssembly in a pure-Go runtime) was tried on 2026-10-19. Accuracy was equal but it was too slow (for example, tagging was 48 times slower), so it was dropped. Inference stays with the cgo build, keeping the build-tag split and the diagnostic of the `CGO_ENABLED=0` build.
