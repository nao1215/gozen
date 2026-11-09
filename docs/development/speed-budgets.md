# Speed budgets

Authors investigating a slowdown can delimit a run with script commands:

```text
wait 30
mark courtyard
wait 60
mark conversation
read 60
quit
```

```sh
gozen run game --dev --script input.txt --shots shots --trace run.jsonl
gozen trace run.jsonl --only section
gozen trace run.jsonl --only section --format json
```

`mark NAME` accepts exactly one nonempty word, consumes no tick, and has no
runtime effect without tracing. A mark closes the preceding section and begins
the named section. The initial section is named `run`; adjacent marks can produce
empty sections. Every run also ends with a `section` whose `scope` is `run`,
covering the whole loop, including autoplay. Without marks this is the only section. The other sections have `scope: mark`.
Names can repeat; `from_tick` and the record's terminal `tick` locate each interval.
A mark executed during Update assigns that update to the new section. Loading
before the first Update is outside these records; himorime includes that loading.

Each record contains `frames` (completed Draw calls), `ticks` (completed simulation
updates), `elapsed_seconds`, `fps`, `tps`, `heap_inuse_bytes`, and `allocations`.
With at least one frame it also includes `slow_frame_share` and
`allocations_per_frame`. FPS/TPS divide the counts by wall time, including waits;
they are not inverse CPU times. An empty section has no frame ratios. Headless
`sim script` keeps its deterministic traces and does not collect these measurements.

A slow frame exceeds 1/60 second of CPU-side wall time spent in Draw plus all
updates since the preceding Draw. Multiple updates before a draw are accumulated;
multiple draws without an update count independently. This is not GPU execution
time, presentation latency, or a deadline verdict. Section boundaries discard
unfinished frame work from the preceding section; the whole-run accumulator
retains it. Undrawn final updates contribute ticks but not a frame ratio.

The collector samples Go runtime metrics every 60 updates and at boundaries,
using a retained three-element sample array. Heap in use is the sampled maximum
of heap objects plus unused space in allocated spans. It excludes free/released
heap and GPU memory; brief peaks between samples may be missed. Allocation counts
come from `/gc/heap/allocs:objects`, so they are runtime estimates and process-wide,
including background goroutines and existing trace/layout collection. They are
not allocations attributed exclusively to one scene. The added collector measured about 290 ns per update/draw pair, including periodic
sampling, with zero bytes and zero allocations per pair in the local microbenchmark; section serialization allocates only at boundaries. With no
trace, the timer and metric reads are disabled. Existing trace features have their
own overhead, including layout screen readback.

Ebitengine v2.10.4's public `DebugInfo` exposes GPU image memory but no draw-call
counter. Draw calls per frame are therefore omitted, not approximated from images.
All records are facts. Neither `gozen trace` nor `trace --check` gives a performance
verdict or an exit failure for their values.

## Maintainer checks

`internal/dev/perf` owns generated workloads and budget integration tests. The
hygiene table's `speed` decision runs the headless budget and the allocation-free
collector contract through dango. Graphics budgets additionally run in CI with
an actual window. Their success is not implied by a dango pass.

| Case | Work exercised |
| --- | --- |
| huge-map | A map with 2,000 named points and a generated 1024x1024 picture |
| pictures-4k | Decode and show a generated 3840x2160 picture |
| standing-pictures | Keep 32 named standing pictures visible together |
| phone-video | Advance a clip with 24 generated 1280x720 frames at 12 FPS |
| long-conversation | Read all 200 authored lines and execute the final command |
| large-sim | Complete 5,000 deterministic battles and assert the victory count |

The graphics scripts use 30 warmup ticks and a 60-tick named section, except
long-conversation, which uses `read 1` to finish all 200 lines. `read N` waits
for each line to reach age N; it is not a total duration. A headless regression
checks all lines and the final command complete before the 6,000-tick deadline. Both startup
and the named section have allocation, sampled heap and time limits; a missing
section or a section with no frames fails. Failures name the section, metric,
observed number and limit. Fixtures and saves live in private temporary directories.
The font is copied from the existing CLI fixture, with its existing license.

```sh
go test ./internal/dev/perf -run '^TestSpeedBudget$' -count=1 -v
go test ./internal/screen/play -run '^$' -bench '^BenchmarkAtlasHugeMap$' -benchtime=120x -benchmem
go tool dango lint
# Compile/static verification only; this does not render anything:
go vet -tags=graphics ./...
# Linux runtime verification:
xvfb-run --auto-servernum --error-file=/dev/stderr go test -tags=graphics ./internal/dev/perf -run '^TestGraphicsSpeedBudget$' -count=1 -v
# To isolate a case, set GOZEN_PERF_CASE=phone-video before the command.
```

Count ceilings are fixed in the tests. A local headless calibration measured
about 331 allocations per battle and 0.15 seconds for 5,000 battles; the test
allows 600 allocations per battle, 64 MiB heap in use and 3 seconds. The time
margin accommodates concurrent unit-test jobs. Graphics allocation ceilings for
pictures-4k, standing-pictures and phone-video
are 350, 1,900 and 1,300 per steady frame, respectively. The picture startup
ceiling is 1,700 allocations per frame. Standing-picture startup uses a total
allocation ceiling of 57,000 (the original 1,900 times 30 nominal frames),
because loading the 32 figures is fixed work. In repeated Xvfb runs, the same
38 startup updates produced 30 frames with about 32,000 allocations or 11 frames
with about 24,000 allocations. Dividing fixed startup work by the variable frame
count incorrectly rejected the latter. The steady-frame, heap and time limits
remain unchanged. Failure logs include frames, ticks and total allocations.
An Xvfb run measured approximately
165 steady and 838 startup allocations per frame for pictures, 433–1,028 for
standing pictures and 267–600 for phone video. Their sampled heap ceiling
is 256 MiB (measured peaks up to 120 MB), and their section time ceiling is five
seconds. Long-conversation retains a 20,000 allocation ceiling and allows 120
seconds for reading the entire conversation (1,434 ticks in the headless check).

The large map retains its static point-and-label layer while the visible state
is unchanged. State, focus, camera, viewport and theme changes redraw it;
animated tokens remain live.
The fixed map ceilings are 13,000 startup and 600 steady allocations/frame,
512 MiB sampled heap and 15 seconds per section. A local CPU-side benchmark
measured 12,443 startup allocations/frame over 30 updates/draws, and 391 per
warm update/draw over 120 iterations, including periodic state refreshes.
A Linux Xvfb runtime run measured 9,924 startup and 404 steady allocations
per frame, with 280 MiB sampled heap. Windows and macOS budget calibration
has not been performed. Campaigns and authored marker images retain their live drawing path.
Do not raise ceilings merely to hide a regression.

## CI cost

The `changes` job emits a separate fail-open `perf` output for runtime layers,
CLI sources, budget definitions, module versions and the relevant workflows.
Documentation-only and unrelated tooling changes skip the graphics work. Push and
manual runs retain the existing full-run policy; private automatic runs retain
the existing limited Actions policy.

Five budget workers and five whole-CLI benchmark workers run independently, with
one scenario per worker. The four short graphics scripts request about 1.5 seconds
of simulation each; one warmup plus five benchmark samples of those short cases
request about 9 seconds per revision per worker (about 18 seconds for comparisons).
The headless budget adds about 0.15
seconds per invocation locally. Generating all six fixtures took 0.88 seconds
locally; a cached CLI build took
3.00 seconds. At those local costs, a budget worker adds roughly 5 seconds and a
comparison worker roughly 25 seconds before runner setup and software-rendering
costs (ten workers run in parallel). Setup, cold compilation and software
rendering add runner-dependent cost; the desktop runtime and actual Actions
critical-path impact remain unmeasured until the coordinator runs them. Because
workers run alongside existing test/lint jobs, these durations do not add serially
to CI wall time. Inspect job durations and named-section logs after the first run.

The complete conversation takes about 24 seconds of simulation per run. Its
benchmark worker therefore takes substantially longer than the four short cases;
the previous 1.5-second estimate does not apply to it.
