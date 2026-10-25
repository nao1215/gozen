# Stacked-space occupancy measurements

The renderer previously rebuilt and uploaded a 64 × 64 × 8 RGBA occupancy atlas on every Draw. An unchanged scene now reuses that atlas. This removes redundant CPU work and a 131,072-byte `WritePixels` submission; the byte count describes the API payload, not measured physical GPU traffic.

## Reproduce

The fixture generator is `spaceScalingFixture` in `internal/play/space_test.go`. It pads the public `examples/space/game` maps to 48 × 40 and 64 × 64 without moving the original geometry or camera. It tests one, three and eight aligned levels. The one-level variant removes the stair flights; additional padded levels are open air. A separate dense variant fills the added area with floors and periodic walls, rather than presenting empty extra levels as a dense town. All fixtures pass the normal loader.

```sh
go test ./internal/play -run '^TestSpaceOccupancy' \
  -bench '^BenchmarkSpaceOccupancy' -benchtime=100ms -count=3
LIBGL_ALWAYS_SOFTWARE=1 xvfb-run -a go test -tags=graphics ./internal/play \
  -run '^$' -bench '^BenchmarkGraphicsSpaceScaling$' -benchtime=1x -count=2
LIBGL_ALWAYS_SOFTWARE=1 xvfb-run -a go test -tags=graphics ./internal/play \
  -run '^TestGraphicsSpace' -count=1 -v
```

The CPU benchmark compares the retained full rebuild with a cache lookup under the same fixture. The conditional benchmark also measures a changed state, where rebuilding is required. It reports allocations separately from GPU work.

The native graphics benchmark varies dimensions, levels, depth 8/16, and stationary/moving cameras. Set `GOZEN_SPACE_BENCH_DENSE=1` to fill the added area with geometry; leave it unset for open-air padding. It starts a fresh host per sample, warms up for 120 simulation ticks, and reports sampled CPU Update/Draw durations in milliseconds and FPS/TPS. The first logged Draw provides `startup-ms`, including process startup and initial scene loading. `ns/op` includes fixture preparation and the scripted run. Sampled p50/p95 values are not full-frame latency percentiles or GPU timings: Ebitengine can defer GPU execution beyond Draw. Do not interpret the fixed script waits as traversal latency.

For revision comparisons, build `internal/play/testdata/host` for each revision and set `GOZEN_SPACE_BENCH_HOST` to the desired binary when running the same harness. The reference host used the occupancy implementation at `f528405` with the same Update diagnostic instrumentation. For an older host without Update samples, apply only the `GOZEN_DEBUG` timing block at the start of `Game.Update` before building; leave its occupancy implementation unchanged. The benchmark is explicit and does not run during ordinary CI tests.

## Cache correctness

`dungeon.CellAt` reads the immutable dungeon definitions, location, `Put`, `Done`, and tile `when`/`solid` conditions through the state expression environment. Spaces do not support roamers. A static space compares location and mutation/once maps. A conditional space conservatively compares the serialized state and load-repair report, covering flags, variables, actors, inventory, party, campaign values, position, choices and event history. It excludes only frame count, explored-map bytes and conversation memory, which the field expression environment does not expose.

Definition, DB and state identity changes invalidate the cache, including reload and save restore. Definitions are immutable between reloads. Stair metadata is rebuilt with the rest of the occupancy. Expressions containing `rand` or `chance` retain the previous per-Draw evaluation, and a state serialization failure forces a rebuild. Camera interpolation alone does not change occupancy. Mutations are visible in the next Draw, with collision continuing to use `CellAt` directly.

Tests cover static reuse, flags, variables, gold, position, conditional fallback, once/after state, tile replacement, reload, stair definitions, save restore, random conditions and serialization failure. The rendered route covers both stair flights, bridge undersides and return to street level; a separate rendered test closes and reopens a conditional wall and reloads the scene.

## Measurement environment

Measurements recorded on 2026-10-25 used Linux amd64 (kernel 7.0.0-38-generic), AMD Ryzen AI Max+ 395, native Go, and Xvfb with Mesa 26.0.8 llvmpipe (LLVM 21.1.8, software rendering). The scene is 960 × 540; the ray-traced target stays 640 × 360. Window size, internal resolution, textures, camera and draw distance were held equal for each before/after pair. Browser performance and hardware GPU timing are separate measurements and were not inferred from these runs.

For maps without building surfaces or roofs, the atlas dimensions, shader level stride, map bounds and draw-distance limits remain unchanged. These measurements do not establish a need to expand map limits or identify GPU ray traversal as a bottleneck.

## CPU results

Medians of three 100 ms benchmark samples; times are microseconds per operation. These rows compare the unchanged padded fixtures, excluding GPU submission and rendering. Full rebuilds allocate 416 bytes in one allocation; unchanged static lookups allocate 80 bytes in two allocations. Dense variants were also executed by the same matrix.

| Fixture | Full rebuild (µs) | Reuse check (µs) |
| --- | ---: | ---: |
| 48 × 40 × 1 | 158.894 | 0.286 |
| 48 × 40 × 3 | 466.357 | 0.280 |
| 48 × 40 × 8 | 1246.674 | 0.287 |
| 64 × 64 × 1 | 152.815 | 0.136 |
| 64 × 64 × 3 | 441.504 | 0.137 |
| 64 × 64 × 8 | 1303.117 | 0.147 |

CPU frequency and scheduling vary, so these short samples should not be used to infer a precise scaling ratio between dimensions. The useful within-fixture result is that reuse avoids the full cell traversal.

For a 64 × 64 × 8 fixture with a `gold > 0` tile condition, three 300 ms samples gave median times of 2468.290 µs for a full rebuild, 2.637 µs for an unchanged state, and 2705.120 µs for a state change requiring rebuild. The unchanged snapshot used 1072 bytes / 6 allocations; the changed path used about 1500 bytes / 8 allocations. Conservative state comparison adds work when geometry must change; it is intended to remove repeated work while it remains unchanged.

## Native graphics results

Each row reports the mean of the two run-level sampled percentiles, in milliseconds, for padded scenes. These are CPU Draw timings, not GPU or end-to-end frame latency. Both revisions generally sustained 60 FPS / 60 TPS; no FPS improvement is claimed.

| Fixture / depth / moving | Before Draw p50 | After Draw p50 | Before Draw p95 | After Draw p95 | Before FPS p50 | After FPS p50 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 48x40x1/depth=8/moving=false | 0.532 | 0.215 | 0.637 | 0.297 | 43.6 | 60.0 |
| 48x40x1/depth=8/moving=true | 0.490 | 0.262 | 0.576 | 0.300 | 60.0 | 48.2 |
| 48x40x1/depth=16/moving=false | 0.524 | 0.267 | 0.706 | 0.312 | 59.1 | 57.9 |
| 48x40x1/depth=16/moving=true | 0.530 | 0.322 | 0.595 | 0.409 | 59.9 | 60.0 |
| 48x40x3/depth=8/moving=false | 0.663 | 0.311 | 0.950 | 0.430 | 60.0 | 60.0 |
| 48x40x3/depth=8/moving=true | 0.919 | 0.326 | 0.963 | 0.469 | 43.3 | 60.0 |
| 48x40x3/depth=16/moving=false | 0.885 | 0.272 | 0.975 | 0.363 | 43.5 | 59.9 |
| 48x40x3/depth=16/moving=true | 0.646 | 0.303 | 0.901 | 0.391 | 59.3 | 59.9 |
| 48x40x8/depth=8/moving=false | 1.559 | 0.349 | 1.849 | 0.493 | 60.0 | 60.0 |
| 48x40x8/depth=8/moving=true | 1.474 | 0.356 | 1.565 | 0.259 | 60.0 | 43.4 |
| 48x40x8/depth=16/moving=false | 1.572 | 0.260 | 1.691 | 0.308 | 60.0 | 60.0 |
| 48x40x8/depth=16/moving=true | 1.517 | 0.337 | 1.631 | 0.463 | 60.0 | 60.0 |
| 64x64x1/depth=8/moving=false | 0.645 | 0.237 | 0.686 | 0.304 | 60.0 | 60.0 |
| 64x64x1/depth=8/moving=true | 0.594 | 0.304 | 0.659 | 0.447 | 60.0 | 60.0 |
| 64x64x1/depth=16/moving=false | 0.634 | 0.246 | 0.698 | 0.368 | 60.0 | 58.4 |
| 64x64x1/depth=16/moving=true | 0.623 | 0.266 | 0.656 | 0.345 | 60.0 | 60.0 |
| 64x64x3/depth=8/moving=false | 0.977 | 0.329 | 1.396 | 0.393 | 60.0 | 60.0 |
| 64x64x3/depth=8/moving=true | 1.010 | 0.291 | 1.400 | 0.345 | 60.0 | 60.0 |
| 64x64x3/depth=16/moving=false | 1.224 | 0.305 | 1.337 | 0.400 | 60.0 | 60.0 |
| 64x64x3/depth=16/moving=true | 1.335 | 0.263 | 1.458 | 0.357 | 60.0 | 60.0 |
| 64x64x8/depth=8/moving=false | 2.963 | 0.319 | 3.149 | 0.378 | 60.0 | 60.0 |
| 64x64x8/depth=8/moving=true | 3.048 | 0.244 | 3.187 | 0.328 | 60.0 | 60.0 |
| 64x64x8/depth=16/moving=false | 2.926 | 0.307 | 3.027 | 0.352 | 60.0 | 60.0 |
| 64x64x8/depth=16/moving=true | 2.957 | 0.262 | 3.066 | 0.377 | 60.0 | 60.0 |

Before: update-p50 range 0.025–0.054; update-p95 range 0.032–0.099; fps-p50 range 27.200–60.000; tps-p50 range 59.200–60.100; startup-ms range 905.900–2870.000.

After: update-p50 range 0.021–0.050; update-p95 range 0.027–0.061; fps-p50 range 28.200–60.000; tps-p50 range 59.900–60.400; startup-ms range 893.900–3738.000.

The additional dense 64 × 64 × 8 / depth 16 comparison used two runs per camera mode. Mean run-level Draw p50 values were:

| Camera | Before (ms) | After (ms) |
| --- | ---: | ---: |
| stationary | 1.634 | 0.301 |
| moving | 3.134 | 0.283 |

The optimization removes a demonstrated CPU cost while leaving the geometry, draw distance and internal resolution intact. It does not establish a GPU bottleneck or promise higher FPS. Profile a representative game on its target device before adding larger bounds or a different renderer.

## Upright sprite counts

The billboard pass reuses the occupancy cache to collect tile pictures. It culls by full projected image bounds, sorts visible planes by camera depth, then submits one clipped quad per visible image. Each sprite shader checks the shared terrain intersections only for nontransparent image pixels. This avoids a second full-screen terrain pass, but overlapping billboards still repeat ray work and increase draw submissions.

Set `GOZEN_SPACE_BENCH_SPRITES` to 0, 16 or 64 with the same harness:

```sh
LIBGL_ALWAYS_SOFTWARE=1 GOZEN_SPACE_BENCH_SPRITES=16 xvfb-run -a \
  go test -tags=graphics ./internal/play -run '^$' \
  -bench '^BenchmarkGraphicsSpaceScaling/48x40x3/depth=16' -benchtime=1x -count=2
```

`addSpaceBenchmarkSprites` places the original transparent innkeeper image on existing floor cells, without changing geometry, movement or collision. Counts are total placed images, not guaranteed simultaneously visible images: the stationary and moving cameras cull different subsets. These runs used the same environment and resolution described above, with no other test suite running concurrently. Each row is the mean of two run-level sampled percentiles; timing columns are milliseconds.

| Images | Camera | Draw p50 | Draw p95 | Update p50 | Update p95 | FPS p50 | TPS p50 |
| ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 0 | stationary | 0.232 | 0.285 | 0.029 | 0.034 | 59.95 | 60.00 |
| 0 | moving | 0.330 | 0.400 | 0.035 | 0.044 | 60.00 | 60.00 |
| 16 | stationary | 0.285 | 0.319 | 0.029 | 0.035 | 57.50 | 60.00 |
| 16 | moving | 0.428 | 0.444 | 0.034 | 0.091 | 26.15 | 59.95 |
| 64 | stationary | 0.257 | 0.311 | 0.028 | 0.040 | 54.45 | 59.85 |
| 64 | moving | 0.279 | 0.330 | 0.028 | 0.042 | 60.00 | 60.00 |

The 16-image moving runs recorded lower FPS even though the 64-image moving runs sustained 60. These short software-rendered samples do not establish a monotonic sprite-count cost or a cause for that variability. They also do not establish hardware GPU latency or a 60 FPS guarantee. The separate Lantern Inn correctness route completed at a final sampled 60 FPS / 60 TPS; correctness assertions cover visibility, transparency, occlusion and level transitions independently of these measurements.

## Continuous controller

`BenchmarkFreeSpaceController` measures the fixed-tick controller on a neutral 40 × 36 × 4 dense fixture derived from the public plaza. Walking reverses every 60 updates and uses a speed stage; idle still validates/synchronizes the saved pose. Median results from three 300 ms runs on the environment above were 0.165 microseconds per idle update (zero allocations) and 2.925 microseconds per walking update (4102 bytes, 9 allocations). These are CPU controller costs, excluding rendering and fixture setup.

```sh
go test ./internal/play -run '^$' -bench '^BenchmarkFreeSpaceController$' \
  -benchtime=300ms -count=3
LIBGL_ALWAYS_SOFTWARE=1 GOZEN_SPACE_BENCH_FREE=1 GOZEN_SPACE_BENCH_DENSE=1 \
  xvfb-run -a go test -tags=graphics ./internal/play -run '^$' \
  -bench '^BenchmarkGraphicsSpaceScaling/40x36x4' -benchtime=1x -count=2
```

The native benchmark's free-movement mode selects 40 × 36 × 4 at depth 16. Its moving route holds forward for four cells, turns through 180 degrees, and returns. It uses constant speed 2 and turning 90 degrees/second; the CPU benchmark separately exercises acceleration. No other test suite ran concurrently with these measurements. The fixture contains real floors, walls and stairs; it is not a finished town or a claim about every author's geometry.

Mean run-level sampled percentiles from two runs, in milliseconds:

| Camera | Draw p50 | Draw p95 | Update p50 | Update p95 | FPS p50 | TPS p50 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| stationary | 0.301 | 0.434 | 0.034 | 0.044 | 60.00 | 60.00 |
| continuous walking/turning | 0.294 | 0.482 | 0.040 | 0.069 | 60.00 | 60.00 |

Separate correctness tours took 20.194 seconds for the inn and 54.132 seconds for the plaza, with final sampled 60 FPS / 60 TPS. Those elapsed times include startup, scripted pauses, interaction and screenshots, so they are not pure walking-speed measurements. Tests assert stair support levels, return to the original location, off-center stopping, non-cardinal yaw and reload preservation. A native keyboard-driven inspection also exercised staged walking, turning, menu pause and Shift-strafing; recorded poses were identical on menu entry and exit. CPU timings, tour elapsed time and FPS/TPS describe different parts of the experience and should not be substituted for each other.

## Building surfaces and roofs

The building benchmark uses the same dense 40 × 36 × 4 layout and continuous route in four modes: boxes, named materials, roofs, and all features with 64 placed billboards. Resolution remains 960 × 540 with a 640 × 360 internal image and depth 16. The environment is the Linux/Mesa llvmpipe setup above. No other test suite ran concurrently. Each row averages two run-level sampled percentiles, in milliseconds.

| Mode | Camera | Draw p50 | Draw p95 | Update p50 | Update p95 | Frame interval p50 | Frame interval p95 | FPS p50 | TPS p50 |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| boxes | stationary | 0.3030 | 0.3589 | 0.0652 | 0.0732 | 16.735 | 16.815 | 60.00 | 60.00 |
| boxes | moving | 0.2888 | 0.3857 | 0.0646 | 0.0803 | 16.700 | 16.805 | 60.00 | 60.00 |
| materials | stationary | 0.3275 | 0.3858 | 0.0606 | 0.0809 | 16.725 | 17.245 | 60.00 | 60.00 |
| materials | moving | 0.2983 | 0.4303 | 0.0706 | 0.0837 | 16.695 | 16.840 | 59.95 | 60.00 |
| roofs | stationary | 0.2883 | 0.3145 | 0.0635 | 0.0732 | 16.700 | 16.790 | 59.95 | 60.00 |
| roofs | moving | 0.3032 | 0.3813 | 0.0665 | 0.1025 | 16.725 | 16.885 | 60.00 | 60.00 |
| combined | stationary | 0.4062 | 0.4413 | 0.0649 | 0.0847 | 16.810 | 16.885 | 60.00 | 60.00 |
| combined | moving | 0.3537 | 0.5972 | 0.0758 | 0.1058 | 16.765 | 17.030 | 60.00 | 60.00 |

Frame interval measures successive Draw start times, including pacing; it is not a presentation timestamp or a GPU timer. Draw and Update measure CPU callbacks. GPU timing is unavailable in this harness. Short-run differences do not show that roofs improve performance or guarantee 60 FPS on other devices. Placed billboard counts are not simultaneous visible counts.

```sh
for mode in boxes materials roofs combined; do
  GOZEN_SPACE_BENCH_BUILDINGS="$mode" LIBGL_ALWAYS_SOFTWARE=1 xvfb-run -a \
    go test -tags=graphics ./internal/play -run '^$' \
    -bench '^BenchmarkGraphicsSpaceScaling/40x36x4' -benchtime=1x -count=2
done
go test ./internal/play -run '^$' -bench '^BenchmarkBuildingSpaceUpdates$' \
  -benchtime=300ms -count=3
```

Median CPU controller/cache costs from three 300 ms runs were 2.352 microseconds for boxes, 2.376 for materials, 2.448 for roofs, and 2.484 for combined; each used 2547 bytes and seven allocations per update. These exclude rendering and fixture setup.

Unchanged occupancy does not call `WritePixels`. On invalidation, the baseline 64 × 512 RGBA atlas submits 131,072 bytes; building metadata extends it to 64 × 1664, or 425,984 bytes. Immutable roof/face metadata is reused during state changes. Named materials use a cached 2048 × 1280 RGBA atlas (10 MiB), rebuilt when the dungeon or source image identities change. These are API payload sizes and cache behavior, not measured GPU traffic. Roofs and wall materials remain in the terrain shader without per-roof draw submissions; visible billboards retain one submission each, followed by the final scaled composite.

Native correctness tests independently verify roof undersides, sprite occlusion and visible roof edges. The building route completed in 60.99 seconds including startup, pauses and captures; it visits the shop, climbs the stone terrace and inn stairs, interacts with the bed and innkeeper, then preserves an arbitrary yaw through reload. The public building fixture is a geometry regression example, not finished game artwork.
