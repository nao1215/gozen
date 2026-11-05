# Quality pass, 2026-11-06

This pass preserves CLI, YAML and save contracts and adds no exported Go API.
It follows the existing dependency policy; no architecture exception, dependency
version or asset was changed.

## Findings and changes

- Text interpolation stopped at a closing brace inside an expression string.
  A failing five-case regression preceded the fix. Validation and rendering now
  share the private scanner at `internal/layer2/gamedata/text.go:29`, following
  the expression lexer's quote and escape rules. `TestInterpolationQuotedBraces`
  and `FuzzRenderQuotedText` compare interpolation with direct expression values.
- A batch JSON Lines record could execute its first object despite trailing
  objects or garbage. `TestRunCaseRejectsTrailingJSON` failed before the fix;
  `cmd/gozen/simbatch.go:105` now requires EOF before running a case.
- The process-wide expression cache retained unlimited expressions and the
  complete backing strings of dialogue substrings. Its private owner is now
  `internal/layer2/gamedata/expression_cache.go:19`: at most 1,024 entries,
  each with at most 4,096 source bytes. Larger expressions still evaluate but
  bypass caching. Sources are cloned before parsing because AST identifiers
  also retain substrings. Eviction does not invalidate active consumers.
  Boundary and concurrent tests cover reuse, eviction, invalid input and
  oversized sources.
- Collection evaluation allocated a map and chain for every element.
  `internal/expr/collections.go:23` gives each evaluation its own budget and
  each collection its own reusable item scope. Nested scopes retain their
  lexical behavior. Partition, filter/count and concurrent evaluation tests
  supplement the existing nested-scope and visit-budget tests.
- Three existing clash tests succeeded without entering the clash screen:
  their target did not border player territory. A new entry assertion first
  failed. The fixture now supplies a valid border and asserts entry and exit
  (`internal/play/clash_integration_test.go:37`). The coverage-named test file
  was renamed to describe its integration responsibility.

## Measurements

Go 1.26.10, Linux amd64, AMD Ryzen AI Max+ 395. Microbenchmarks used
`-benchmem -count=3` and CPU/allocation profiles. Median results for 1,024 rows:

| Expression | Before ns/op | After ns/op | Before B/op | After B/op | Before allocations | After allocations |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| sum | 324763 | 21866 | 401818 | 64 | 4102 | 3 |
| count | 310011 | 24682 | 401817 | 64 | 4102 | 3 |
| filter | 337186 | 34955 | 420635 | 18880 | 4112 | 13 |

The original allocation profile attributed 99.89% of allocated bytes to
collection evaluation. CPU profiles showed allocation/GC work before the
change and evaluation/reflection work afterward.

`BenchmarkRenderLargeSource -benchtime=2048x` renders distinct 64 KiB lines.
The sampled retained heap fell from 149.26 MiB to 6.53 MiB; about 145.25 MiB
previously pointed back to the benchmark's source strings. These are Go heap
samples, not process RSS or GPU memory.

New loader/check benchmarks measured uncached batch definitions at 2.79 ms,
1,252,176 B/op and 12,125 allocations; cached definitions at 0.292 ms,
208,042 B/op and 819 allocations; checking the neutral game at 5.58 ms,
2,750,734 B/op and 25,625 allocations. These characterize existing paths;
they are not claimed improvements. CPU and heap profiles were collected.

Both `make bench` runs completed all seven himorime benchmarks (eight command
measurements). Whole-command timing varied substantially even for unchanged
startup paths while other verification ran; it is not evidence that these
changes improve every CLI command. No interleaved `bench-compare` run was made.

The workboard Update/Draw test also produced CPU and heap profiles. Its short
CPU-side trace includes image drawing, allocation and GC. This does not verify
GPU execution or establish a display-frame performance improvement.

## Architecture review

The authoritative configuration is `.go-arch-lint.yml`, not an arch-go file.
`make arch` and `make boundary` found no layering violations. No new package,
public identifier or policy exclusion was needed.

The concrete ownership changes are within existing layers: expression scopes
belong to the foundation evaluator; bounded parsed-text ownership and the
shared interpolation scanner belong to the data layer; record framing remains
in the command adapter. The removed duplicate scanning and obsolete unbounded
cache implementation are replaced, not retained as compatibility paths.
The installed linter includes staticcheck and unused checks.

Deferred proposals:

1. Measure byte retention and eviction churn in the presentation image/text
   caches (`internal/layer5/gfx/cache.go`). Their entry-count bounds are not
   byte budgets. A lifecycle change needs real graphics tests first.
2. Keep batch cache validation filesystem-neutral. Its full directory walk is
   visible in profiles, but weakening change detection to improve a benchmark
   could return stale definitions.
3. Consider per-game expression-cache ownership only with concrete consumers
   and lifetime evidence. The bounded shared cache fixes retention without
   introducing a new configuration/API surface.
4. Audit other scripted scene tests for entry as well as exit assertions.
   The corrected clash tests demonstrate why final-state-only checks can pass
   without exercising their advertised behavior.
5. Obtain an isolated, interleaved whole-command benchmark and longer rendered
   frame profiles before making broader performance changes.

## Verification and coverage limits

The atomic unit profiles use `-coverpkg=./...`. Blocks with identical file and
source coordinates are merged by summing counters, then statement counts are
weighted after applying the exclusions in `.octocov.yml`:

| Profile | Covered statements | Statements | Coverage |
| --- | ---: | ---: | ---: |
| Before | 52803 | 60829 | 86.80564% |
| After | 53454 | 60872 | 87.81377% |

These are **partial unit-profile measurements**, not successful `make coverage`
results: both full runs fail when the unchanged downloader test attempts a local
HTTP listener. Its package does not emit a completed profile. The denominator
still includes that package. The real capture-process coverage could not be
collected because the sandbox prevents Xvfb from opening its display.
Accordingly, 95% is not achieved or claimed. The comparable partial metric
increased by 1.00813 percentage points. The clash entry assertions account for
most of the additional exercised production code.

Remaining large gaps include the interrupted command package, author screens,
planning, campaign presentation, profile charts, ensemble rendering and capture.
About 4,375 additional currently unexecuted statements would be needed to reach
95% with this denominator. Resume the standard complete coverage target in an
environment supporting loopback HTTP and a working Xvfb before prioritizing
the remaining gaps.

Verification performed:

- Full `go test ./...` and instrumented full runs: the existing
  `TestFetchAndList` panics because socket creation is not permitted.
  Passing package results are retained; the overall command is a failure.
- `make lint` with the pinned version: blocked downloading the tool.
  `make lint GOLANGCI_LINT=golangci-lint` uses the installed v2.15.0 built
  with Go 1.27.2, including staticcheck and unused; no issues.
- `make arch`, `make boundary`, `make build` and documentation tests pass.
- Race checks pass for expr, chat, react and gamedata with `-short`, and
  for the batch command regressions. This is not a full-module race claim.
- Expression parsing fuzzing: 310,440 executions in approximately 16 seconds.
  A timed YAML fuzz run ended with a deadline error; a subsequent fixed-budget
  run completed 20,000 executions successfully. Quoted-text fuzzing completed
  10,000 executions successfully.
- `make e2e`: 393 scenarios pass, none skipped. Graphics and web scenarios
  are outside that target.
- Nested Go Zen, novel and campaign tests pass. The office module has no Go
  packages; its game passes `gozen check`, not a nonexistent unit suite.
- `make graphics` was attempted only through `xvfb-run`; child game loops
  fail to open the display. CPU-side Update/Draw tests pass but are not a
  substitute for graphics runtime verification.
- No images were regenerated; no push, pull request or publication was made.
