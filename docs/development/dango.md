# Dango maintainer checks

From the repository root, with its required Go toolchain installed:

```sh
go tool dango lint
go tool dango lint --list
go tool dango lint --format json
```

Dango is a development tool, not a gozen command or a release artifact. The
local tool directive adds no external tool dependency. `make lint` invokes it,
and CI invokes `make lint`. See [ADR 0061](../decisions/0061-dango-maintainer-tests.md).

The decision table is `internal/dev/hygiene/decisions.go`. Each entry supplies an
ID, name, one-line reason, repair and enforcing package/test names. The runner
executes precisely those tests, uncached, through `go test -json`; all verdicts
belong to tests. `--list` prints metadata without running them. Text failures
occupy one line per decision and include the test's evidence and repair.

Exit codes are 0 for success/help, 1 for violated decisions or I/O/processing
failures, and 2 for invalid invocation. A build failure, missing Go executable,
missing enforcing test or skipped test is a processing failure, never a pass.
There is no `--json` alias. JSON output has this shape (the real list is nonempty):

```json
{"version":1,"command":"dango lint","status":"ok","data":{"decisions":[]}}
```

Each decision has `id`, `name`, `reason`, `fix`, `tests` (objects with `package`
and `name`), `status` (`pass`, `fail`, or `not_run` for listing), and `details`
(an array of failed-test evidence). A violated decision is a result on stdout;
a failure before results exist leaves stdout empty and emits a diagnostic on
stderr. JSON diagnostics use the shared version-1 JSONL contract. Output failures
return 1. Changing representation changes neither tests nor verdicts.

## Mechanical coverage and limits

- Content and location tests check the published vocabulary boundary and generic
  private location patterns. They cannot prove the provenance of arbitrary data.
- Dependency tests inspect imports against the architecture policy. API checks
  reject functional Option types and check filesystem Load-family signatures.
  Existing specialized loaders are frozen by signature. These checks do not
  infer whether every constructor setting ought to be a Config field.
- Command checks reject root dispatch, direct parser entry and subprocess
  execution from handlers. Shared flag registration for an external input
  adapter is not another handler invocation. Typed operations remain reusable.
- The phone puppet clip test observes actual file reads while advancing headless
  playback. It runs without a window. Trace checks exercise extreme observation
  values and require no diagnostic or failure status for layout and state measurements.
- `TestDocImageInputs` and `TestImageManifest` retain the existing visual evidence
  checks. Pixel regeneration/comparison still requires `make docs-images-check`
  in a graphics environment; passing dango does not prove rendering.
- YAML checks collect schema property names and game-data YAML tags. Every new
  word needs schema presence, documentation outside ADRs, an atago YAML key and
  occurrences in at least two distinct examples/fixtures (files of one game count once). This is lexical coverage,
  not proof that two games use the word meaningfully. The fixed initial debt
  list and its maximum live beside the test; remove entries as coverage improves.
  Never grow or replace the allowlist to admit a new word.

The decision meta test finds unconditional enforcing test declarations and
checks the lint/CI route. The runner additionally requires their actual terminal
test and package events. No decision reads an ADR file to obtain its rule.

## Verification

```sh
go test ./cmd/internal/dango ./internal/dev/hygiene
go tool dango lint
make boundary arch complexity
make e2e
```

The E2E fixture preparer needs Python 3. It makes an isolated checkout with a
functional-option, loader and reconstructed-argument violations, removes ADR documents,
and runs the real tool there. The injected runtime source also exercises the
existing picture-input staleness check. It never modifies
linked source files or injects a test bypass into dango. The suite checks exit
codes 0/1/2, listing, text repair guidance and versioned JSON. Release builds
continue to target only gozen and its private player.

## Trace reproduction bundles

```sh
go tool dango repro run.jsonl --repro 60
go tool dango repro run.jsonl --repro 1.5s --out repro --format json
go tool dango repro run.jsonl --repro shots/frame.png --format text
```

The maintainer wrapper uses the same typed operation as `gozen trace --repro`.
It accepts a trace file and `--repro TICK|TIME|PNG`, optional `--out DIR`
(`-o DIR`), and `--format text|json`. It never executes gozen or its generated
script. Without `--out` it only prints the replay command or its JSON description.
With `--out` it requires a new directory and writes the cut trace, initial saves,
`repro.json` and `repro.sh`; no screenshot is rendered during bundle generation.
See [trace reproduction](graphics.md#reproduce-a-trace-screen) for target rounding,
replay boundaries and saved-state details.

Text prints the shell command; JSON uses command `dango repro` and the same
`data` as the author CLI. Format changes do not change the artifacts.
Exit codes are 0 for success/help, 1 for invalid trace/target, existing output,
or other processing/I/O failure, and 2 for invalid options or missing operands.
Failures leave stdout empty and use shared diagnostics on stderr.

The AST command check rejects `os/exec` imports (including aliases), dispatch
references such as `runWith`, and calls to the command parser. The current
`cmd/gozen` subprocess allowlist is empty and test-capped at zero. Artifact
strings are allowed. The same import guard covers the shared bundle operation.
