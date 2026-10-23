# Extreme usage and documentation audit

Scope: checkout `2271a50`, reviewed on 2026-10-23; Linux/amd64, Go 1.26.10. The findings below describe the original checkout. The follow-up section records the subsequent fixes; original line references refer to that checkout. Timing samples are local observations, not portable performance budgets. GPU rendering, browser execution and remote CI timings were not measured in this audit.

## Priority findings

### P1: road weights can make route search effectively hang

Evidence: `internal/layer3/atlas/atlas.go:105` adds two `int` values without checking overflow. The validator only rejects negative road steps (`internal/layer2/gamedata/atlas.go:249`). Roads are bidirectional, so an overflow can repeatedly lower the distance back to a previously visited node.

Reproduction: construct an Atlas with two points named `0` and `1`, one road between them with `Steps: math.MaxInt`, and call `atlas.Routes(&rpg.State{}, a, "0")`. A compiled probe was terminated by `timeout 3s` with exit 124. The corresponding small road with one step returns immediately. This reproduces the route algorithm directly; an end-to-end YAML load of this case was not run.

Fix direction: use checked addition and define how unrepresentable total distances are handled; reject unreasonable individual road steps during validation. Bound predecessor reconstruction defensively. Test both the largest accepted weight and overflow of a multi-road total. `atlas.Cross` also loops once per step when encounters are configured (`atlas.go:255`), so arithmetic protection alone does not establish a practical runtime bound.

### P1: normal pictures have no decoded-size guard

Evidence: `internal/play/assets.go:193` reads the full file and calls `picdecode.Decode`; `internal/layer5/picdecode/picdecode.go:16` calls `image.Decode` without inspecting dimensions first. `assets.go:190` then converts to an Ebitengine image. `internal/layer2/gamedata/check.go:132` and `:138` check picture/file existence, not image dimensions or decoding success. Map picture dimensions are read without an upper bound in `internal/layer2/gamedata/stage.go:370`.

A generated PNG of 65,536 by 1 pixels passed `picdecode.Decode` and returned those bounds with no error. This intentionally uses little memory. No huge square allocation or GPU crash was attempted. A 16,384-square RGBA buffer alone requires 1 GiB, before GPU copies and other allocations; exhaustion and backend texture failures remain risks inferred from this path, not reproduced crashes.

The 256 MiB picture cache is not a decoding limit: accounting and trimming occur after loading, and `trim` retains the last image even if it exceeds the budget (`assets.go:155`). Puppets do have an 8,192-side guard (`internal/layer5/puppet/rig.go:22`, `:237`); this does not cover ordinary backgrounds and enemy images.

Fix direction: check encoded input size, dimensions and total pixel count before decoding, and use overflow-safe size arithmetic. Align checker diagnostics with runtime rejection. Treat GPU texture limits separately from CPU pixel budgets. Consider the sum of simultaneously visible images: LRU eviction below that working set can cause repeated decoding on successive frames.

### P2: large enemy groups lose mouse hit targets

Evidence: `internal/play/battle.go:1311` lays all enemies out in one row, shrinks the row to fit, then converts each rectangle to integer coordinates at `:1378`. The troop validator only requires a nonempty list (`internal/layer2/gamedata/check.go:217`). Mouse selection tests these rectangles at `internal/play/battle.go:1069`.

A temporary same-package probe created a 1280 by 720 Game and repeated neutral 64 by 64 images, then counted empty rectangles returned by `foeBoxes`:

| Enemy count | Empty hit rectangles |
| --- | --- |
| 10 | 0 |
| 100 | 0 |
| 1,000 | 350 |
| 3,000 | 2,359 |

This verifies layout geometry, not rendered frames or an entire battle. Empty rectangles cannot contain a mouse position. Keyboard selection was not tested.

Fix direction: establish a supported visible group size, with validation and a documented limit, or use rows/pages and a minimum selectable target size. Do not merely clamp every width to one pixel, which can introduce overlap. During target selection, `unitRect` recomputes all enemy boxes for each enemy; compute the layout once per selection/update and reuse it.

## Game performance

`atlas.Routes` scans every road for every expanded point (`atlas.go:99`) and materializes a complete path for every destination (`:113`). On a chain, total path storage is quadratic even if road lookup is optimized. `atlasScene.update` refreshes the map every 60 updates (`internal/play/atlas.go:239`), so an idle map can incur repeated work.

Measured using a generated chain of points, unit road weights, an empty state, a GC before each measurement, wall time around `Routes`, and the difference in `runtime.MemStats.TotalAlloc`:

| Points | Elapsed | Allocated bytes |
| --- | --- | --- |
| 100 | 0.415 ms | 143,936 |
| 1,000 | 39.4–39.5 ms | about 12.1 million |
| 3,000 | 310.6–330.5 ms | about 124.35 million |

These are separate process samples, not a statistical benchmark or peak RSS. For 3,000 points, three samples were taken. No graphics was included.

Recommended order:

1. Build adjacency lists, evaluate road eligibility once per refresh, keep distances/predecessors, and reconstruct only paths actually needed by choices or the chosen destination. Preserve deterministic tie behavior and dynamic conditions.
2. Avoid repeated `PointIDs` construction in `Visited`: each call rebuilds ordered keys (`internal/layer3/atlas/atlas.go:327`, `internal/layer2/gamedata/stage.go:348`), and map refresh calls it per point.
3. Cache label placement until geometry/selection/labels change. `internal/play/atlas.go:687` compares each candidate label against all previously placed boxes on every draw. Consider a spatial index and an explicit dense-map display policy.
4. Reuse battle layout per frame/update; add geometry and CPU benchmarks at increasing actor counts. Measure actual frames before claiming a target frame rate.

Existing safeguards should be retained: dungeon side limits and their tests (`internal/layer3/dungeon/limits_test.go`), puppet side limits, and campaign `max_soldiers` validation at 12 (`internal/layer2/gamedata/campaign.go:656`). No claim is made that every extreme input is broken.

## Documentation and skill accuracy

The in-repository `skills/gozen/SKILL.md` and its references were inspected as documentation, not invoked as instructions to author a game. No external skills were edited.

- `docs/architecture/overview.md:52` says there are six tools and omits `theme`. The dispatch table contains seven tool groups (`cmd/gozen/main.go:37`). Update the overview.
- The direct-command table in `docs/development/testing.md` omits the campaign module from `tidy` and `lint`, although `Makefile:33` and `:40` include it. Its graphics command runs only `TestGraphicsScreens`; `Makefile:91` also includes Stage, Map, Wild, Novel and Campaign. `website-check` documents only `TestSiteLinks`, while `Makefile:190` runs both links and schema checks. These documented alternatives are not equivalent to Make.
- `skills/gozen/SKILL.md:104` promises that a trial cannot leak later content. The implementation is conservative abstract interpretation: unknown expressions become `expr.Top()` (`cmd/internal/trial/domain.go:65`), and branches are eliminated only when truth/falsity is ruled out (`cmd/internal/trial/prune.go:222`). This cannot guarantee exact reachability or recognize unintended content inside a retained asset. Replace the absolute guarantee with the actual pruning contract and require inspection of the resulting file listing and retained files. No actual private-content leak was exercised.
- The boss win-rate guidance in `skills/gozen/references/measure.md` is a design heuristic, not a verified fairness guarantee; AUTO simulation does not establish the experience of a human player. Label it as a starting point for playtesting.

## CI improvement candidates

These are repository-configuration observations. Savings have not been measured on GitHub Actions, and no checks were removed.

1. Add pull-request concurrency cancellation to `.github/workflows/bench.yml`. CI, coverage and docs have cancellation groups; Benchmark currently has none. Superseded PR revisions can continue expensive base/head comparisons. Keep manual runs independent.
2. Refresh job-specific build caches after source changes. Coverage (`.github/workflows/coverage.yml:73`) and race/sample jobs (`ci.yml:326`, `:393`) key caches by job, Go version and dependency sums, then skip saving on an exact hit. Their build caches consequently do not learn new source builds while dependency sums remain unchanged. Evaluate revision-specific primary keys with a dependency-prefix restore key, retaining main-only writes and controlling cache growth.
3. Measure coverage instrumentation cost before changing its scope. `Makefile:26` instruments `./...` across packages with atomic coverage, separate from ordinary tests and race runs. Benchmark narrower instrumentation or a cheaper coverage mode only after checking concurrency and cross-package coverage requirements; preserve the coverage denominator and race checks.
4. Profile cold and warm runs separately, and record per-step time and cache hits before reducing the OS/Go matrix. The existing documentation-change filter, separate race cache, and explicit slow-test omission in race already address some repeated work.

## Validation and remaining work

- `make test lint build`: normal tests succeeded using cached results; architecture, API inventory, module tidy and actionlint passed. Lint stopped because `zizmor` is unavailable. The remaining golangci-lint stages did not execute in that command.
- `make build`: succeeded separately after allowing access to Go's build cache.
- Fresh `go test -count=1` runs for atlas, dungeon and picdecode passed.
- The temporary crowd probe executed successfully and reported the invalid geometry above; its passing exit status means the diagnostic ran, not that the layout is correct. Temporary diagnostic test code was removed from the source package.
- The route overflow probe timed out intentionally; ordinary routes and the thin-image probe completed.
- Fresh `go test -count=1 ./internal/dev/docs` passed after allowing build-cache access; the initial sandbox attempt failed on cache permissions. Opt-in documentation image/step/site tests were not run.
- No production fixes, commits, remote issues, PRs or CI edits were made. Follow-up work should start with checked route arithmetic and image-load limits, then battle selection geometry and repeatable performance benchmarks.

## Follow-up implementation

The subsequent fix shares image input/dimension checks in layer 1, validates road steps and troop counts, checks path-distance arithmetic, and replaces eager all-destination paths with a shortest-path tree. Map refresh reuses its point order for visited flags, and mouse targeting computes the enemy layout once. The old production `Routes` entry and the separate puppet dimension guard were removed rather than retained as unused wrappers.

Road steps are limited to 10,000, troop members to 32, and images to 8,192 per side, 16,777,216 pixels and 64 MiB encoded input. These intentionally reject previously accepted excessive definitions. The checker preserves its existing behavior for unrecognized image headers; runtime decoding remains authoritative. See [resource limits](../architecture/scenes.md#resource-limits).

Benchmark uses the same `Choices` workload for both implementations: a chain of 3,000 points with one actionable destination. A temporary test-only copy of `Choices` and `Routes` from the original revision supplied the baseline and was removed after measurement. Three local samples measured a median of about 220 ms (range 163–388 ms) / 124.55 MB allocated before, versus 1.46–1.59 ms / 1.14 MB after. Timing varied with other local work, so these samples do not establish a stable speedup ratio. This is a CPU microbenchmark, not a frame-rate claim. The permanent `BenchmarkLargeMapChoices` covers 100, 1,000 and 3,000 points. Maps with an action at every point still require many complete paths; label-placement cost remains a profiling candidate.

Documentation command mismatches and the trial/fairness overclaims were corrected. Benchmark cancels superseded PR runs. Job-specific caches now save revisions with dependency-prefix restoration. Coverage scope and the OS matrix were preserved; no measured CI speedup is claimed.

The new atago boundary scenarios check both rejection and repair for road steps, enemy counts and image dimensions. The full normal CLI suite passed 244 scenarios with no skips. Existing Linux graphics tests for play and puppet passed under Xvfb (the driver emitted initialization warnings; tests completed successfully). Public API inventories remain unchanged. Final `make test lint build` passed, including architecture, API inventories, tidy for all modules, actionlint, zizmor 1.31.1 and golangci-lint for the root and three sample modules. Zizmor ran through `uv tool run` because it was not on PATH. A WebAssembly cross-build of picdecode and play passed; this is compilation evidence only, not browser runtime verification.
