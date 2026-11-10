# Dango maintainer checks

From the repository root, with its required Go toolchain, GNU Make, a POSIX shell (`sh`) and `grep` installed:

```sh
go tool dango lint
go tool dango lint --list
go tool dango lint --format json
```

Dango is a development tool, not a gozen command or a release artifact. `go tool` builds the runner from this checkout. The dead-code gate test executes Make and a POSIX shell to check tool failures and allowlist filtering. Ordinary Go tests skip that regression if its tools are absent; dango rejects a skipped enforcing test. `make lint` runs dango, and CI runs `make lint`. See [ADR 0061](../decisions/0061-dango-maintainer-tests.md).

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

Dango also checks generated schemas against the Go types and translation keys against their source messages and format arguments. It runs schema checks with `GOZEN_UPDATE_SCHEMA=0`, so lint never regenerates files even if that setting is enabled in the caller. The dead-code gate test injects tool and filter errors to check that lint fails instead of accepting empty output.


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
- Phone input tests cover picture taps followed by keyboard advancement, controls disabled during typing, input on timeout deadlines, shared close outcomes and replay of saved timed results. These tests do not compare pixels.
- Flag-based phone tests cover hidden result indices, hidden media reads, fixed replies, navigation and save replay. They also check that ordinary idle chat updates allocate nothing. They do not test a physical touch device.
- Embedded-game checks cover parent resource ownership, child failure and shutdown, approved archive validation, transactional installation, offline signed catalogs, installed-game replay and browser export limits. Graphics and browser E2E remain separate runtime checks. Browser scenarios cover archives larger than localStorage permits, aborted IndexedDB updates, offline reopening and legacy cache reads when IndexedDB is unavailable.
- Reproduction tests require tick and screenshot targets to leave room for the
  exclusive replay boundary, preserve draw cadence, and reject unsupported
  mutations. They validate generated bundles without launching a player.
- The dead-code gate test checks that tool failures, invalid filters, and blank
  or comment-only allowlists cannot silently suppress real findings.
- CI scope tests use isolated Git histories to keep runtime documentation checks
  enabled unless a pull request changes only the changelog. Missing history and
  other uncertain cases retain those checks.
- `TestCodeCitations` reads README.md, CONTRIBUTING.md, `docs/` and the site's
  pages (not the decision records and reviews, which are history) and requires
  every Go file named in backticks to exist, by a path from the root, a path's
  tail or a bare name, and no citation of a line: lines drift with every edit,
  so a document names the function or test instead. It does not check that the
  named function still does what the sentence says.
- `TestDocImageInputs` and `TestImageManifest` retain the existing visual evidence
  checks. Pixel regeneration/comparison still requires `make docs-images-check`
  in a graphics environment; passing dango does not prove rendering.
- YAML checks collect schema property names and game-data YAML tags. Every new
  word needs schema presence, documentation outside ADRs, an atago YAML key and
  occurrences in at least two distinct examples/fixtures (files of one game count once). This is lexical coverage,
  not proof that two games use the word meaningfully. The fixed initial debt
  list and its maximum live beside the test; remove entries as coverage improves.
  Never grow or replace the allowlist to admit a new word.

- Removed-feature tombstones reject names of deliberately removed features (the
  casual-game runner, its schema version and the migrate command, listed in
  `internal/dev/hygiene/hygiene_test.go`) anywhere outside the changelog, decision
  and review records, and their YAML keys in game-data tags and generated schemas.
  Returning a feature needs a new ADR that also deletes its tombstone.
- File organization checks reject new test files with vague or numbered coverage
  suffixes (`_more_test.go`, `_cov2_test.go`) and E2E fixtures, scripts or bench
  files that no other file names. The existing misnamed test files form a fixed
  baseline that may only shrink as they are consolidated. Naming is lexical: a
  reference proves a file is mentioned, not that it is used.
- `TestBoundedPictureDecode` reads every Go file outside tests and rejects a
  reference to `image.Decode`, `png.Decode`, `jpeg.Decode`, `gif.Decode`,
  `gif.DecodeAll` or a `webp` package's `Decode` that no `picturelimit.Header`
  or `picturelimit.Dimensions` precedes in the same function: a picture's header
  decides how much memory its decode takes. `picturelimit.Read` bounds only the
  encoded bytes and does not count. `internal/screen/picdecode` is the checked
  decoder itself. A function bounded another way is listed by file and name with
  its reason in `pictureDecodeAllowlist` of `decisions_test.go`, and an entry
  that excuses nothing fails. The check is lexical: it sees that a limit call
  comes first in the source, not that its error is obeyed, and it does not follow
  a decoder passed in from another package.

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
