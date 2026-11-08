# ADR 0061: Dango reports maintainer decision tests

Status: Implemented. Date: 2026-11-07.

## Decision

`go tool dango lint` is a maintainer-only tool, implemented by
`cmd/internal/dango` and registered by one local `tool` directive. It is not an
author command, is absent from gozen help, and is not built or packaged by CLI,
player or release artifact targets. ADR 0016 and ADR 0041 retain one author CLI.

The architecture assigns this executable its own development entry component,
with dependencies only on development metadata and the existing CLI reporting
and parsing infrastructure, plus the shared typed reproduction operation. Runtime commands and engine layers cannot import
it or development tooling. This classification grants no runtime exception.

Verdicts live only in Go tests. `internal/dev/hygiene` owns a self-contained
decision table with stable IDs, reasons, repairs and enforcing test names.
Dango invokes those tests through `go test -json` and formats their results;
it contains no policy predicates. Tests do not read ADR documents as an
authority and remain usable if those documents are removed. The meta test
checks test existence and lint/CI wiring; the runner rejects absent, skipped or
incomplete enforcing tests. `make lint` runs the table.

`lint --list` lists the same table without testing. `--format text|json` and
exit classes follow ADR 0042: 0 success, 1 violation or processing/I/O failure,
2 invalid invocation. JSON is one versioned result with stable decision IDs;
fatal diagnostics use the shared JSONL contract on stderr.

Existing visual input and image checks remain the authority for documentation
pictures. The local dango tool declaration cannot affect rendering; image input
hashing ignores only that declaration, retaining dependency changes. No image
baseline is updated by dango. The initial YAML coverage debt is an explicit,
size-capped allowlist that may only shrink, not a permission to exempt new words.

Following the ADR 0042 clarification, `dango repro` calls the same typed
`cmd/internal/repro.Generate` operation as `gozen trace --repro`. The operation
is command support, below both entry points, and has no dependency on the player or its rendering packages.
The record schema and exact-replay validator live in the foundation package
`internal/layer1/traceformat`, shared by the player and bundle generator. This
keeps replay acceptance identical without importing presentation or development
tooling into the other consumer. The support component depends only on this
foundation protocol and diagnostics.
It writes argv and shell scripts as output; it never executes them or invokes
CLI parsing. Sharing the existing implementation avoids duplicated bundle rules
and does not introduce an engine dependency on development tooling.
There is no `dango review`. Neither game rules nor author-facing tools gain judgments.
