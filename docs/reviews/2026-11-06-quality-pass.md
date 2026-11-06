# Quality pass, 2026-11-06

> Note (2026-11-06): the `gozen migrate` command and its migration code described here were removed afterwards. Before v1.0.0 gozen has no migration tools.

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

## Second pass

The combined branch was reviewed again against `origin/main`, including schema
11, migration, the phone and the merge resolutions. No package boundary, public
API or architecture exception was added. The review continued with migration
and phone-input fuzzing after the deterministic regressions.

### Reproduced defects

- The English catalog retained the replaced game-ID diagnostic.
  `TestEnglishHasEveryKey` failed on the combined branch; removing the unused
  key repairs the merge without losing the replacement translation.
- Loading or closing a game retained its phone. `TestPhoneReleasedOnResetAndClose`
  failed before `startState` and `Close` shared `closePhone`. Owned canvases,
  icons, media images and puppet references are released by
  `internal/play/phone.go:876`; `TestPhoneCloseReleasesOwnedReferences` creates
  those resources before checking cleanup and repeated close.
- Cursor replay and held Skip stopped at a phone. `TestPhoneFastForward` failed
  before the phone accepted both fast-forward paths. The schema-11
  `TestPhoneCursorReplayAndRollback` now saves after a phone, loads across it,
  rolls back across it and shows a fresh phone on the next advance.
- Saving during a phone could lose the event continuation.
  `TestPhoneSaveDoesNotLoseEvent` first reproduced the accepted unsafe save;
  it now receives the existing translated instruction to close the screen.
- Phone numbers accepted NaN/infinity, and excessively long clips could overflow
  ticks. `TestPhoneFiniteVideoParameters` covers rejection. The checker at
  `internal/layer2/gamedata/check.go:208` also validates puppet definitions and
  referenced pictures through the shared portrait checker;
  `TestPhoneChecksPuppetContents` first reproduced both omissions. Phone
  diagnostics now have Japanese keys and English translations.
- Long chat messages hid their beginning and advanced without showing it.
  `TestPhoneLongChatMustBeReadBeforeAdvancing` first failed; messages now start
  at their beginning and advance through overlapping pages. Their background
  images and submitted text are bounded by the viewport.
- Feed media canvases accumulated with every visited post.
  `TestPhoneVideoFramesStayBounded` first failed after the fourth post;
  `trimImages` releases frames outside the current drawing. Clip positions
  remain available when revisiting posts.
  `TestPhoneOffscreenPuppetsReleaseImagesAndKeepMotion` additionally reproduces
  retained offscreen puppet drawing resources; those are closed while preserving
  motion, and reopening restores the same pose and playback position.
- Feed drags moved opposite the pointer (`TestPhoneFeedFollowsDrag`); Auto and
  Skip keys did nothing (`TestPhoneModeShortcuts`); zero-second bounces never
  ran (`TestPhoneBounceAtStart`); positive subframe clips had zero ticks
  (`TestPhoneClipSubframeDuration`); large clock values displayed negative
  times (`TestPhoneClockLargeMinutes`). Each regression failed before its fix.
- Upright phones exceeded narrow viewports. `TestPhoneFitsNarrowScreen`
  first failed; both orientations now fit within the screen width.
- Minimum-size canvases were recreated on every request. Normalizing dimensions
  before the reuse check fixes `TestPhoneCanvasMinimumSizeIsReused`.

Additional contracts cover the first chat message being a video, one-post feeds,
missing pictures, auto reading, battle-event continuation, canvas reuse and
orientation changes. `TestMigratePhoneEvents` checks all three phone apps through
schema-10-to-11 migration, source immutability, compact events, replay fingerprints
and idempotence. `TestCompactPhoneDiagnosticLocation` checks authored line numbers
and diagnostic codes under event shorthand.

### Drawing and ownership

`BenchmarkPhoneDraw -benchtime=100x -benchmem -count=3` measures warm CPU-side
Draw calls with neutral text, a fixed viewport and no graphics loop. Median
results from the same environment as the first pass:

| Chat messages | Before ns/op | After ns/op | Before B/op | After B/op | Before allocations | After allocations |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 114048 | 105217 | 97714 | 95792 | 477 | 405 |
| 20 | 734038 | 330740 | 414370 | 367059 | 2844 | 1348 |
| 200 | 4461211 | 312045 | 780084 | 369937 | 16102 | 1363 |

Previously every visible-history message was wrapped and measured every frame,
including messages far above the viewport. Layouts now belong to the phone and
only currently drawn layouts are retained. Resize invalidates their width.
Media frames are reused at unchanged sizes and pruned when no longer drawn.
Oversized messages begin their line traversal at the viewport rather than
scanning every hidden line; the 10,000-line benchmark did not establish a
wall-time improvement under concurrent fuzzing.

Allocations for 200 messages fell by 91.5%, and bytes by 52.6%. Final timings were measured after fuzzing stopped. Earlier timings varied
while other checks ran, including a slower one-message case. These CPU-side
results do not establish a universal speedup, displayed frame rate or GPU-memory
measurement.
Actual GPU resource release still needs a display-backed lifecycle test.

### Coverage and verification

Using the same atomic, coordinate-merged partial-profile method and exclusions:

| Profile | Covered statements | Statements | Coverage |
| --- | ---: | ---: | ---: |
| Combined branch at start | 55553 | 61762 | 89.94689% |
| Second pass | 55835 | 61853 | 90.27048% |
| Second pass plus available command tests | 57188 | 61853 | 92.45792% |

The phone itself increased from 251/431 (58.24%) to 491/498 (98.59%). The last
row additionally runs existing command tests after explicitly excluding the
socket-dependent downloader cases that otherwise abort their package. Only that
unchanged package's coverage blocks are added. This is additional measurement,
not coverage created by new tests, and is not a successful full coverage target.
Overall 95% remains unmet; the expanded partial denominator still needs about
1,573 additional covered statements.

`make build` succeeds. Related play, event, gamedata and CLI packages pass
`go test -race -short`; focused phone race tests also pass. `make e2e` passes
393 selected scenarios without skips; the target excludes graphics and web
tags. The installed-linter form
`make lint GOLANGCI_LINT=golangci-lint` passes, including architecture, boundary,
module, workflow, formatting and static checks. The merged face/campaign test
spacing, battle-test switches and unused phone-test return value were corrected.

Full tests still fail at the existing local HTTP-listener restriction and at
missing phone GIF/MP4 documentation assets referenced by the combined branch.
Documentation images were left as supplied. Missing media references remain
visible to the checks. CPU-only runs are not counted as graphics verification.

Feed swipe checks also reproduce a release-frame error: crossing the swipe
threshold only when releasing used the previous held position. Release now uses
the current pointer coordinate, including a swipe with no held frame between
press and release. `TestPhoneFeedFollowsDrag` covers both cases, and the input
fuzzer includes pointer press, hold and release sequences.

### Long-run migration findings

After 8,208,533 executions, `FuzzMigratePreservesDefinitions` found an accepted
source whose list marker follows spaces and a tab. Removing the event wrapper
and shifting its leading spaces made the output invalid YAML. The saved corpus
case `377e5b972a7cd1d5` failed before the fix and passes afterward. Such events
now retain their full mapping and original indentation while required schema
updates still run. Source immutability and migration idempotence remain checked
by the same fuzz contract. Phone chat, feed and video seeds were also added.

A subsequent run found `0<<` accepted as a merge key while the decoder omits
its prefix from the token's value and position. Inserting an explicit tactic
default then produced invalid output. Such ambiguous merge-key tokens now take
the existing `errUnrestorable` refusal path before any edits are prepared.
`TestMigrateRefusesUnrestorableMergeToken` failed before the guard and passes
afterward; corpus `5e270ef0afc447e3` retains the full-game reproducer. This is a
safe, actionable refusal, not support for migrating that unusual spelling.

Corpus `b10cf0a4e361b2de` then exposed a panic after an inline comment with
mixed LF/CRLF line endings. The parser's line numbers drifted from the source
and produced an inverted slice range. Migration now derives line ranges from
its already reconstructed byte offsets for both event and tactic edits. The
corpus failed with a panic before the fix and now completes the same definition
equality, source preservation and idempotence checks.

A later run found the loader looping while following a self-referential merge
alias, before migration began (`bdea1aa44efc2bbe`). Its isolated regression timed
out after 15 seconds in `mergeInto`. Alias resolution now tracks visited names
and leaves cycles to the decoder, which reports a bounded diagnostic.
`TestDefinitionCyclicMergeAliases` and the saved corpus cover the ordinary and
prefixed spellings. The final verification run was restarted after this fix.

After the last loader fix, `FuzzMigratePreservesDefinitions` passed 4,277,596
executions over 17m25s. The expanded keyboard/pointer `FuzzPhoneInputs` passed
51,530,949 executions over 2h7m43s; the earlier keyboard-only run passed
23,776,269 executions over one hour. These check state transitions and migration
contracts, not displayed graphics or GPU resource release.

### Consolidation and remaining work

Ordinary and battle phone waits now share `updateTap`. Thumbnail and playback
puppets share deterministic preparation. The stray picture-method comment is restored to its actual method.
Lifecycle, save/replay and battle contracts are grouped in
`phone_integration_test.go`. Existing route traversal, portrait framing and
campaign merge tests remain intact.

Remaining work includes real graphics tests for rotations, long-message pixels,
puppet/video frames and GPU release, generating the already referenced phone
media in a display-capable environment, completing the socket-dependent tests,
and continuing coverage work outside the phone. Offscreen phone puppets retain
motion but release their drawing resources.
The shared rig cache is still outside this phone-local lifetime policy.
No push or PR was made.

## Integration onto the newer phone

The review ran on a34fe0e. Before it was merged, the phone was split into
`phone_chat.go`, `phone_feed.go`, `phone_page.go`, `phone_lock.go` and
`phone_banner.go`, and gained frames, fps, fade, pan, results and banners. The
fixes were re-applied to that code by intent:

- Phone waits share `updateTapOrPhone`, which also answers the event with the
  phone's result; a battle's phone now answers its own runner.
- The schema also rejects non-finite `fps` and `fade` and footage whose frames
  at its rate run longer than a tick count holds.
- The feed scrolls up and down: its drag already followed the finger, and now
  also takes the movement of the tick it lets go. The horizontal swipe fix no
  longer applies.
- Long chat messages page from their top over the chat's own bubble layout,
  which is now kept for the visible messages only.
- A load also clears queued banners.
- Phone schema tests stay in `phone_test.go` of gamedata and event, which the
  newer code extended, instead of moving to `check_test.go` and `event_test.go`.
