# CLI developer notes

The CLI is the single `gozen` binary ([ADR 0016](../decisions/0016-single-cli.md)). Installation, requirements, conventions (GNU options, exit status), and every command with its options are described for users on the site: [Commands](https://nao1215.github.io/gozen/commands/). The options follow [ADR 0018](../decisions/0018-gnu-options-and-help.md), and the command tree and the diagnostic codes [ADR 0024](../decisions/0024-command-tree-and-diagnostic-codes.md). The production-support commands are in [Production tools](production-tools.md).

## Commands

The tree lives in `cmd/gozen/main.go` (`tree`): a group holds commands, a leaf runs. The help pages are `groups` and `pages` in `cmd/gozen/help.go`, keyed by the command path. `run` shares its options and help groups with `play.Main` in `internal/play/entry.go`; it requires a directory operand instead of `--dir`. `art prompt` and `art mosaic` hand their arguments to `internal/promptgen/command` and `internal/mosaic/command`, and `art fetch` to `mosaiccmd.Fetch`.

| Group | Commands | What they share |
| --- | --- | --- |
| `new` | (a leaf) | create a game directory from a template |
| `run` | (a leaf) | play a game directory; `--dev` enables author tools |
| `check` | (a leaf) `--only structure\|layout`, `--format text\|json` | the game directory |
| `sim` | `battle`, `dive`, `floors`, `design`, `campaign`, `routes`, `breed` | play by the rules without a window |
| `trace`, `notes` | (leaves) | inspect traces and F9 notes |
| `capture` | (a leaf) | PNG/GIF jobs in one graphics loop |
| `book` | (a leaf) | |
| `export` | (a leaf) | create a desktop or web package with a prebuilt runtime |
| `release`, `trial` | (leaves) | create a new output tree |
| `art` | `prompt`, `mosaic`, `fetch`, `faces`, `puppet` | pictures; cgo, ONNX Runtime and models for mosaic inference and faces |
| `theme` | `add`, `list` | bundled dungeon themes |
| `version`, `help` | | `gozen help` takes the words of a path, or a code |

An old name (`curve`, `memos`, `mosaic`, ...) is an unknown command; no alias table is kept.

## Diagnostic codes

A diagnostic can end with a code, `[GZN1101]`; `gozen check --format json` and `gozen.Diagnostic` carry it as `code`. The first digit is the exit value, the next two the family:

| Family | Exit | What to fix |
| --- | --- | --- |
| `GZN01xx`, `GZN02xx` | 0 | warnings of check; warnings of a measurement or a record |
| `GZN11xx` to `GZN19xx` | 1 | the definitions, fonts, simulation, traces, output paths, capture, packaging, art tools, I/O |
| `GZN21xx` to `GZN25xx` | 2 | the command shape, a value, a combination, a named path or ID, help |

The registry is `internal/diag`: `register` is the only way to make a code and needs a summary, a detail and a fix. Rules:

- One code per fix, not per call site. Reword a message freely; never renumber a code.
- `cliio.Invalid(code, …)` is an argument error (exit 2); `code.Errorf(…)` or `code.Wrap(err)` marks a failure (exit 1); a definitions check passes the code to `errorf` or `warnf`.
- After adding or changing an entry, run `go run ./internal/dev/errgen` to rewrite `website/content/errors.md`. The tests of `internal/diag` fail when the page is stale, when a code is not raised from non-test code, when its first digit disagrees with its package, and when no scenario of `e2e/atago` asserts it with the matching `exit_code` (a code that needs cgo, a display or the network is listed in `untested` with the reason).
- `gozen help GZN1101` prints the entry.

When testing, run the built binary directly. The exit status of `go run` itself can differ from the CLI's exit status, so the E2E does not use it.

Every `--help` has the same shape: the name, a one-line description and the version on line 1; `[Usage]` and a description; `[Commands]` (for the top level, the groups, `art mosaic` and `art mosaic manual`); `[Examples]` that really work; `[Options]` grouped by kind; notes, where `Requirements:` is written once on the top page, on a group page for what its commands share, and on a leaf only when it differs; and finally the links Documentation, Report an issue and GitHub Sponsors. The examples are written with `e2e/atago/fixtures/game` (troop `lesson`, dungeon `grotto`) as `game`. Help does not require any of the prerequisites. A neutral example of the configuration is `e2e/atago/fixtures/game/{trial,designeval}.yaml`.

## E2E

atago v0.25.2 is used.

```sh
go install github.com/nao1215/atago@v0.25.2
make e2e
# Only on Linux with a display environment prepared
xvfb-run --auto-servernum --error-file=/dev/stderr make e2e-graphics
```

atago itself can need a newer Go toolchain than Gozen. When installing, enable Go's automatic toolchain selection or use stable. The lower bound of Gozen does not change.

On Windows PowerShell, the normal E2E runs with the following steps. The cgo build that includes inference needs a [C compiler](https://nao1215.github.io/gozen/production-tools/). Checks without inference can run after setting `$env:CGO_ENABLED = '0'` first.

```powershell
New-Item -ItemType Directory -Force bin, build/atago | Out-Null
go build -trimpath -buildvcs=false -ldflags=-compressdwarf=false -o bin/gozen.exe ./cmd/gozen
if ($LASTEXITCODE -ne 0) { throw "gozen build failed" }
$env:PATH = (Join-Path $PWD bin) + [IO.Path]::PathSeparator + $env:PATH
Push-Location build/atago
try {
    atago run --ci --parallel 4 --skip-tag graphics,web ../../e2e/atago
    if ($LASTEXITCODE -ne 0) { throw "CLI E2E failed" }
} finally { Pop-Location }
```

The normal specs do not depend on a shell. For every `gozen` subcommand they check the form of help, GNU-style options, exit status, stdout/stderr, determinism, whether anything is written, generated files and protection against overwriting. The capture specs with the `graphics` tag run separately. The capture specs run the actual CLI with multiple jobs, PNG/GIF outputs and failure recovery. Capture does not launch a child game process.

For macOS, the direct steps are the same build command without `.exe`, `bin` added to PATH, and the same atago command run from `build/atago`. The results on Windows and macOS and the remote CI are not confirmed locally. For the check before publication, follow the [release checklist](release-checklist.md).

`make e2e-web` builds the common WASM runtime and runs the browser export
scenario with Chrome, Python 3 and `go-licenses`. It checks a choice, a save,
a page reload and a second save under a subdirectory URL. The ordinary CLI
suite excludes `web`; the Website and Release Smoke workflows run it separately.

## Structured result migration

[ADR 0042](../decisions/0042-command-and-output-contract.md) defines the target
command and output contracts. The shared I/O and argument errors now live in
`internal/cliio`; protected output paths remain in `cmd/internal/cli`.

`new`, `export`, `capture` and `theme list` currently support `--format text|json|jsonl`. JSON emits one
version-1 envelope with `command`, `status` and `data`. JSONL theme lists emit
ordered `item` records and one terminal `summary`; creation emits one summary.
Both machine formats send fatal diagnostics to stderr as JSONL, preserving the
existing GZN codes and exit values. Other commands retain their documented
formats until their migration batch; do not assume the new envelope everywhere.

The real-binary contracts live in `e2e/atago/new.atago.yaml`,
`e2e/atago/theme.atago.yaml` and the existing `errors.atago.yaml`. They cover
argument order, a leading-dash directory, format framing, structured diagnostics,
creation, validation and overwrite refusal. `internal/diag/usage_test.go` checks
both textual assertions and JSON `$.code` matchers against each code's exit value.
The registry and existing code meanings are unchanged.
