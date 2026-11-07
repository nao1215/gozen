# Graphics development and verification

For ordinary dungeons, players can enable **Instant dungeon transitions** in Settings.
It switches the view immediately for steps, turns, blocked moves and arrivals, without
overlaying the previous frame. The default remains animated. The setting is saved with
the player's other preferences and survives live reload. It changes presentation only:
the existing input cooldown, held-key repeat, collision, encounters and tile events
retain their timing. It does not accelerate dialogue or gameplay, and stacked spaces
retain their interpolated camera. Independent author-controlled movement speeds are
tracked separately in issue #64.

Use Go 1.26.10 or later. The normal tests of T3 start no display or audio device. Real drawing is checked by separate tests with the `graphics` tag. For the items to check before publication, see the [release checklist](release-checklist.md).

## Original sample

Go Zen (`examples/go-zen`) is the original sample: a module of its own, so its pictures and music do not come with the gozen module. A normal run saves under the OS config directory in `gozen/go-zen`; a trial run with a start position uses a temporary save when the save destination is omitted. The sources of its assets are in `examples/go-zen/ASSETS.md`. `make go-zen` checks and plays it headless; `make go-zen-graphics` takes its screens.

The novel sample, After the Pass (`examples/novel`), is a module of its own in the same way: a short story in the novel window (`ui.message: novel`, schema version 4) with two choices, staging, and a Japanese translation in `lang/ja.yaml`. The README's novel pictures are taken from it. `make novel` checks it and reads every way through it headless; `make novel-graphics` takes its screens. Its assets are listed in `examples/novel/ASSETS.md`.

The startup options that `gozen run` accepts use the same GNU style as `gozen` ([ADR 0018](../decisions/0018-gnu-options-and-help.md)). `--seed 7` and `--seed=7` both work, and a single-dash long name such as `-dev` is an error. `--help` has the shape `[Usage]`, `[Examples]`, `[Options]` grouped by kind (game, trial runs, automatic play, scripts and records, notes and review), `Requirements:` and links; an option that needs `--dev` has `(--dev)` in the name of its group.

Run the samples with `gozen run examples/go-zen/game`, `gozen run examples/novel/game` or `gozen run examples/campaign/game`. Their graphics tests invoke the same CLI. Separate test modules keep the large assets outside the engine module download.

## Requirements

A normal build of Ebitengine v2.10.4 is pure Go. The race detector needs cgo and a C compiler. On Linux, even if cgo is disabled at compile time, the X11, OpenGL and audio libraries are needed at run time.

| OS | Setup needed |
| --- | --- |
| Linux (Ubuntu 24.04 or later) | `sudo apt-get install libx11-6 libgl1 libglx-mesa0 libxcursor1 libxi6 libxinerama1 libxrandr2 libxrender1 libxext6 libasound2t64`. For tests on an invisible display, `sudo apt-get install xvfb xauth`. For race, `sudo apt-get install build-essential` |
| Windows | Install Go, and provide a GPU driver that handles OpenGL and a logged-in desktop. A normal build needs no C compiler. For race, put a Go-compatible MinGW-w64 C compiler on PATH |
| macOS | Provide Go and a desktop environment. For the race C compiler, `xcode-select --install` |

Put ffmpeg on PATH only when recording (Ubuntu: `sudo apt-get install ffmpeg`, macOS/Homebrew: `brew install ffmpeg`, Windows: add the bin of an ffmpeg build to PATH). Only when using external input on Linux, `sudo apt-get install zenity`. When it is not specified or not installed, input is done inside the game screen. The sample needs no model or ONNX Runtime.

CI prepares the OS dependencies with setup-ebitengine pinned by SHA. On Linux Xvfb is started with the command below. Installing the action alone does not mean the screen tests ran.

## Graphics tests

```sh
# Linux
xvfb-run --auto-servernum --error-file=/dev/stderr make graphics
# Windows / macOS (in a desktop environment)
go test -tags=graphics ./internal/play ./internal/layer5/puppet -run TestGraphics -count=1
```

The pixel check of `./puppet` runs one Ebitengine loop during the test, and does the drawing and `ReadPixels` of each test inside the loop's Update. With small neutral shapes it checks the base picture, translucency, blinking, lip sync, switching of expressions (the blended colors in the middle and the final color), the bouncing reaction, that nothing is drawn after Close, and that the mesh does not come out away from the shape.

Six scenarios (title, field, conversation, battle, dungeon and the balance panel (F7)) are started in separate processes. For the drawn scene (`stage:`, [maps and drawn scenes](../architecture/scenes.md)), `TestGraphicsStage` walks `internal/play/testdata/stage` (a small picture made by a script) and checks with the trace that the player's figure is visible, that clicking a figure walks to it and talks, that it disappears behind the fronts of a pillar (a pixel check), and that the range of an exit moves to another place. `TestGraphicsMap` checks that a piece walks the roads on a map, the one line of a closed point, and going from a point to a place. `TestGraphicsWild` checks a map inside a map (`at_point`), `token.picture` (pixels), the roads of `draw_roads` (pixels), that a point with `unknown: hidden` does not appear in the list, and that a battle on a road happens before arrival. `TestGraphicsNovel` plays `internal/play/testdata/novel` six times over one save directory and checks with the trace that skip mode passes the lines read in an earlier run and stops before a line not read and before a choice, that rolling back shows the line before and going on shows the same line again, that a portrait brought in by `stand` is drawn on the left and gone after it leaves (pixels), and that the F8 panel opens, draws its tabs and jumps to an event ([ADR 0026](../decisions/0026-read-lines-and-skipping.md)). `TestGraphicsCampaign` plays `internal/play/testdata/campaign` and checks that each region of a campaign's map stands a flag in its owner's colour (pixels), that OK on a region lists its orders, that an attack turns the region's flag to the player's colour, and that the trace has the `region`, `order` and `end_turn` lines ([ADR 0030](../decisions/0030-campaigns.md)). The seed is fixed, a test-only save destination is used, and PNGs are taken inside Draw. They check the PNG decoding, the size of 960×540, that the content is not a single color, and the normal exit of the process. This is not a pixel-exact golden, so the quality of layout and text still needs a separate check. The audio device is muted.

Without Xvfb or a drawing environment, the tests fail. A skip or a mere compile is not treated as a drawing success. The right to run is also needed for Xvfb itself to create its socket.

## Input scripts and traces

Running the game with `--script`, `--trace`, `--replay` and `--record` is described for users on the site: [Scripts and screenshots](https://nao1215.github.io/gozen/edit-play-loop/#scripts-and-screenshots) and [Trace and replay](https://nao1215.github.io/gozen/edit-play-loop/#trace-and-replay). Developer notes follow.

- The canonical list of button names is `scriptButtons` in `internal/play/input.go`.
- `hold BUTTON[+BUTTON...] TICKS` holds controls for 1 to 36000 exact simulation ticks; `shift` is also accepted. For example, `hold up+shift+right 60` walks diagonally in an opt-in continuous space. The first tick also presses each button; the next script command releases them. Held controls and Shift are recorded in input traces.
- A picture name that contains a path separator is rejected. A script needs an explicit `--shots`.
- In a development capture, `battle-hp enemy|party INDEX HP` sets a living battle unit's HP by zero-based troop or party index. HP must be from 1 through that unit's maximum. The trace records each setup as `battle_setup`; it does not count as a damage event.
- `battle-log off` hides the appended tactic decision text and its on-screen button during a scripted development battle capture. `battle-log on` restores both; decision trace events remain available.
- Recording needs a script, automatic play or a replay. A long replay and the integrated run with ffmpeg are not confirmed in the local T3 environment.
- `snapshot` writes `{"t":"snapshot","state":...}` with the state of `end`, without taking a tick.
- `window W H` sets the window's inside to W×H device-independent pixels and waits 10 ticks, so a capture takes the stream screens' portrait or fitted landscape layout ([ADR 0050](../decisions/0050-stream-screens-fit-the-window.md)). Without a window (`gozen sim script`) it does nothing. In the native planning workshop, scripted captures use those dimensions: the 2560×1080 scenario must assert width 2560 for every view, just as the 1920×1080 and 3840×2160 scenarios assert their own widths. This checks the authored viewport.
- `gozen sim script` (`play.Simulate`, [ADR 0046](../decisions/0047-headless-scripted-runs.md)) runs the same `Game` without `Draw` or the Ebitengine loop. A pending script shot pauses `Update` only while `rendering` is set, and the pause comes before the tick advances, so the ticks are those of `run`. Pictures are not decoded ahead, and `pictures.load` gives an empty image of the size in the file header (`picdecode.Size`). Interactive lines (`--script -`) are appended by `script.more` when the commands run out; `Simulate` writes `ready` and flushes before each read. `TestGraphicsSimulateMatchesRun` compares both paths line by line, without the drawing lines and `music`.
- Notes are made with F9 under `--dev --memos notes`. They record the screen, the state and an annotation, and finalize the JSON last. Adding the build revision is enabled separately with `--build-info`, and external input on Linux with `--external-input`. No source search or Git execution is done.

### Reproduce a trace screen

```sh
gozen run game --dev --seed 7 --script input.txt --shots shots --trace run.jsonl --trace-shots
gozen trace run.jsonl --repro shots/frame.png
gozen trace run.jsonl --repro 750 --format json
gozen trace run.jsonl --repro 12.5s --out repro
sh repro/repro.sh
```

`--repro` prints a POSIX shell command that replays the recorded inputs and
writes a PNG. Targets are simulation ticks, durations (rounded to the nearest
60 Hz tick), or PNG paths from `shot`, `--shot-every`, or `--trace-shots` records.
A basename is accepted only when it identifies one tick. Tick N means the state
after update N; the generated command uses `--until N+1`, since `--until` stops
before its argument. JSON output uses the usual result envelope; its `data`
contains `game_dir`, the exact integer `seed`, `start_options`, `initial_saves`,
`initial_memo` when present, `tick`, `until`, `argv`, `command`, and `screenshot`.

New start records include `exact_version: 2`, `game_dir`, `scripted`, device locales,
book display options, the development mode, window scale and effective fullscreen setting, and, inside `setup`,
`save_data` (base64 bytes), `save_times`, `resources` (an asset/translation digest), and optional `memo` (the initial save,
resume cursor and randomness). Existing configuration, read marks, unlocks,
start/event/flag options, save digests and data fingerprint remain recorded.
Successful `save_write` records include `file`, `time`, and `time_known`. Replay
restores subsequent writes with their recorded timestamp and UTC offset, or with
no timestamp when the original store provided none. Replay restores initial saves
in memory, including their ordering for Continue;
it never overwrites the original slots. The trace may contain saved player data.

`--out` requires a new directory and writes `trace.jsonl` through the chosen tick,
initial slot copies under `saves/`, an `initial.save` for a memo start,
`repro.json`, and `repro.sh`. The script reads
the bundled trace, whose start record also contains the saves. The directory can
move; the game directory and unchanged game assets must still be available at
the recorded location. No game assets are copied. Without `--out`, the command
uses the original trace and writes to `<trace>.repro-<tick>/`.

The `viewport` records describe render width, height, UI scale and portrait layout
at the initial layout and each change. Reproduction rejects changed viewports
and script `window` actions through the target tick with
"cannot reproduce exactly: the recording changed the window size" (exit 1).
It does not restore viewport changes. The start record preserves the plain
`window` option and `fullscreen`; exact native-screen reproduction still requires
the same display scale. Injected in-process state is an unsupported start condition.

Script `do`, `jump`, `guides`, and `window` operations are recorded as direct
actions alongside input; their redundant `dev_set`, `dev_do`, and `jump` facts
are omitted because `act` already reproduces the command. Panel commands keep
those mutation records and are not reproducible. Campaign `order` and `end_turn` use the same `do` path.
Script `recruit`, `guarantee-recruit`, and `battle-log` emit `unsupported_script`
with an `operation` name; `battle-hp` emits `battle_setup`. Replay and reproduction
reject targets that include these mutations. Record older scripts again: older
traces did not identify the recruitment mutations.

Replays still default guides off (ADR 0055). Reproduction rejects any `guide`
record through the target tick with "cannot reproduce exactly: guide at tick N"
(exit 1), including explicit `guides on` scripts. Guide rings
and the prompt pulse use the recorded tick, but exact text pixels have not been
established across seeking and ordinary drawing. Ordinary replay still accepts
the explicit `guides on` action.

Exact reproduction uses the shared `ExactReplay` validator in
`internal/play/trace.go`. Both `trace --repro` and `run --replay ... --until N`
accept only explicitly listed record kinds through the target tick (`N - 1`
for `--until`). `exactReplayKinds` documents the complete allow-list: `start`
restores context, `save_write` restores save timestamps, `input`, receiver-bound `pick`, and supported `act` names drive replay,
`repro` marks the seek boundary, and the other listed records describe derived
outcomes or presentation. Viewports must remain unchanged. Unknown action names
are rejected too.

Every unlisted kind fails with `cannot reproduce exactly: <kind> at tick N`
and exit 1. This includes `panel` whenever F8 is opened (typed text and Frames-tab wall-clock
measurements are not recorded), `dev_set`, `dev_do`, `jump` from panel mutations,
`unsupported_script`, `battle_setup`, `live_reload`, guide and review records,
and future unknown kinds. A mutation after the target does not prevent an
earlier reproduction. The constructor audit in `trace_test.go` scans all runtime
`emit` calls and requires each kind to appear in exactly one of the allow-list
or the explicit non-reproducible list. Dynamic kind constructors fail the audit.
Older traces without `exact_version: 2` must be recorded again for exact
reproduction: their missing source markers, pointer movement and touch long-press signals cannot be reconstructed. Ordinary
replay without `--until` retains its older trace support.

#### Script command reproduction audit

The command dispatch switch in `input.go` is audited by
`TestScriptCommandsHaveReproductionClassification`, including parser aliases.
Every command needs a classification and a row here. Refusals apply through the
target tick; an earlier target remains reproducible.

Successful `pick` emits a `pick` record with `on`, `index`, and `label`. Replay
withholds its OK until the same control receiver selects that indexed entry;
a changed, disabled, or unreached target fails replay. Record older scripted picks
again: their traces contain only OK and cannot recover the selected control. This covers choices,
confirmation dialogs, title/save/system menus, authored lists and surface controls.
`TestTraceReproScriptPick` checks non-default choice and menu state, and
`TestGraphicsScreensRepro/script-pick` compares the resulting PNG.

| Script command | Reproduction handling |
| --- | --- |
| `press` | Recorded `input` buttons/digits. |
| `hold` | Recorded `input` held buttons and Shift, including release. |
| `click` | Recorded `input` pointer position and buttons. |
| `drag` | Recorded `input` pointer motion, hold and release. |
| `read` | Recorded `input` OK; waiting adds ticks. |
| `until` | Recorded `input` OK; waiting adds ticks. |
| `skip` | Recorded `input.fast`; optional screenshots and TPS changes affect capture pacing only. |
| `lines` | Recorded `input` OK; screenshots observe pixels without changing game state. |
| `play` | Recorded `input` OK and `act battle_auto` for direct battle progression. |
| `walk` | Dungeon movement uses recorded `input`; map/painted-scene goals use `act walk`. The dungeon walk goal is script-only bookkeeping; mid-event battles use `act battle_auto`. |
| `face` | Recorded directional `input` or `act walk` with `face`, using the same movement receiver and mid-event battle actions as `walk`. |
| `pick` | Recorded `pick` selection applied at the control receiver, followed by OK. |
| `do` | Recorded `act dev_do`; derived unsupported sources still refuse exact reproduction. |
| `jump` | Recorded `act dev_jump`. |
| `order` | Parser alias for recorded `act dev_do`. |
| `end_turn` | Parser alias for recorded `act dev_do`. |
| `guides` | Recorded `act guides`; displaying a guide emits refused `guide`. |
| `window` | Recorded `act window`; refused because viewport changes are unsupported. |
| `type` | Refused at `text_input` when text reaches the field. |
| `memo` | Refused at `unsupported_script` before queuing the capture, including before Draw or file writing; disabled memo output is a no-op. |
| `battle-hp` | Refused at `battle_setup` when HP changes. |
| `battle-log` | Refused at `unsupported_script` when log visibility changes. |
| `recruit` | Refused at `unsupported_script` before recruitment. |
| `guarantee-recruit` | Refused at `unsupported_script` when recruitment probability changes. |
| `wait-graphs` | Requires an open balance panel, already refused at `graphs`; its larger computation budget cannot enter an accepted prefix. |
| `wait` | Script scheduling only; elapsed simulation ticks are preserved. |
| `gap` | Script scheduling only; resulting input ticks are recorded. |
| `shot` | Pixel output only; no game-state mutation. |
| `snapshot` | Observation only (`snapshot`). |
| `look` | Observation only (`look`). |
| `quit` | Ends the recording; the trace end bounds ordinary replay. |

#### External-state audit

The table covers the runtime in `internal/play` and its lower-layer dependencies.
“Recorded” means captured in the trace; “restored” means reconstructed from that
record or deterministic immutable inputs; “refused” means the shared validator
rejects the source through the target tick, even if its UI has since closed.
This is a screen reproduction contract, not an audio recording contract. Use the
same engine build, renderer and display scale. Game files must remain immutable
through recording and replay; startup verifies YAML plus all files under `assets/`
and `lang/`, including fonts, picture metadata, sound and translations. A changed
resource fails replay before play starts. Live editing is refused as below.

| Source and runtime owner | Reproduction handling | Contract coverage |
| --- | --- | --- |
| Pointer coordinates, movement, click/right click and down/held/up (`input.go`) | **Recorded/restored** independently: `mouse` supplies coordinates; `moved` is an explicit boolean, including false for stationary wheel/held/release events. Coordinate changes without movement (including returning from touch to a parked mouse) also emit a record. Pointer-down without a click is preserved. Revision 1 and older traces are **refused** for exact reproduction; ordinary replay retains legacy behavior. | `TestTracePointerRoundTrip`, `TestTraceParkedPointerConfirmation` |
| Touch tap, two-finger right click, long press (`input.go`) | **Recorded/restored** as pointer signals plus `long_press`. Live `longUsed` affects later tap suppression, whose resulting click is recorded. Device IDs and duration counters are polling bookkeeping, not replay inputs. | `TestTracePointerRoundTrip` |
| Touch swipe, origin, began/touching/swiped (`streams.go`, `profiles.go`) | **Refused** at stream initialization (`now`) or profile entry (`profiles`), before these unrecorded gesture fields or direct device polling are consumed. | `TestTraceStreamClockRefused`, `TestTraceProfileDeviceInputRefused` |
| Wheel deltas (`input.go`) | **Recorded/restored** vertical floating-point deltas without rounding, including fractional and negative values. Horizontal wheel is not consumed by the runtime. | `TestTracePointerRoundTrip` |
| Keyboard buttons, digits and key repeat (`input.go`) | **Recorded/restored** resolved `press`, `held`, `digit` and `fast` each tick. Repeat pulses and long-held OK are recorded after duration evaluation; replay never polls or recomputes repeat counters. Script scheduling uses recorded ticks and actions. | `TestTraceButtonsRoundTrip`, `TestTracePointerRoundTrip`, `TestTraceReproScriptDirectActions` |
| Gamepad buttons, analog stick/trigger thresholds (`input.go`) | **Recorded/restored** resolved buttons, held state, repeat pulses and fast mode. Raw axes and device IDs are not consumed downstream. The separate `pad` provenance is only used in stream text fields, already **refused**. | `TestTraceButtonsRoundTrip`; stream text boundary tests |
| Modifier keys (`input.go`, `play.go`) | **Recorded/restored** Shift, Control-derived fast mode, Escape and Alt+Enter filtering of OK. Fullscreen hotkeys are **refused** at `config_write`; raw editing keys remain behind text/panel refusal boundaries. | `TestTracePointerRoundTrip`, `TestTraceSourceBoundaries/settings` |
| Script `typeMemo`, unsubmitted text, IME composition/commit, Backspace; stream name/comment/tip fields (`memo.go`, `textfield.go`, `streams_*`) | **Refused** at `text_input` when a field is connected or script text is delivered. `stream_comment` and `stream_tip` are also refused. No submission is required. | `TestTraceStreamTextRefusedBeforeSubmission`, `TestTraceSourceBoundaries/IME` |
| F8 command field and Frames tab (`devpanel.go`, `meter.go`) | **Refused** at `panel`, before raw typing or `time.Now`/`time.Since` measurements. | `TestReplayRejectsPanelThroughTarget` |
| F9 memo editor, external editor goroutine, memo timestamp/elapsed seconds, disk writes and filename toast (`memo.go`, `entry_linux.go`, `layer1/memo`) | **Refused** at `memo_open`, before typing or external input; `memo` is also refused. `Game.started` and memo clocks affect only this path. | `TestTraceSourceBoundaries/memo` |
| Initial memo save, cursor, random state and legacy-loading option (`memo_resume.go`) | **Recorded/restored** in `setup.memo`, without rereading the original memo. | `TestTraceReproRestoresSavesAndMemo` |
| Initial save bytes, modification times and Continue ordering (`store.go`, `savefile`, browser storage) | **Recorded/restored** in memory. A recording-only store observer **refuses** subsequent external byte, timestamp or read-error changes at `external_save`. | `TestTraceReproRestoresSavesAndMemo`, `TestTraceExternalSaveChangesRefused` |
| Manual saves and autosaves, including OS, memory and browser clocks | **Recorded/restored** by `save_write`, including timestamp availability and offset. Replay never uses `time.Now` for writes. Failed writes are **refused** at `persistence_error`; backup-dependent writes at `save_backup`. | `TestTraceSaveWriteRestoresDisplayedTime`, `TestTraceSourceBoundaries/save_error` |
| Carry values loaded outside slots (`carry.go`) | **Refused** at `carry_read` before the first read, including a missing file; `carry` is also refused. | `TestTraceSourceBoundaries/carry` |
| Read marks and gallery unlocks (`readlog.go`, `gallery.go`) | Initial values **recorded/restored**; later changes derived from recorded play. Read-progress write failures terminate the run; unlock write failures can show a retry banner and are **refused** at `persistence_error`. | Presentation/save replay tests; `TestTraceSourceBoundaries/unlock_error` |
| Settings file, device locales, language, theme, display options (`config.go`, `locale_*`, `trace.go`) | Initial effective configuration and locales **recorded/restored**. Settings writes, including fullscreen hotkeys and their I/O errors, are **refused** at `config_write`. | `TestTraceRestoresPresentationSettings`, `TestTraceSourceBoundaries/settings` |
| Fonts, translations, pictures, face metadata and sound files (`msgfit.go`, `assets.go`, `layer5/gfx`, `layer5/sound`) | **Restored** from immutable, fingerprint-verified resources. Bundled fallback fonts belong to the required matching engine build. No host-font discovery occurs. | `TestTraceResourcesRejectChangedInputs` |
| Injected state or browser `?fps` frame meter | **Refused** in `setup.unsupported`. The browser meter also uses wall-clock frame measurements. | `TestTraceRejectsInjectedState`, `TestTraceSourceBoundaries/frame_meter` |
| Stream clock (`streams.go`), away earnings, private crowd RNG, browser visibility callbacks | Although `now` records the base time and offset, exact reproduction is **refused** at the first `now`: later stream state also depends on unsupported input, speech and loading completion. | `TestTraceStreamClockRefused` |
| Voice decode goroutines, browser speech completion, stream unanswered-comment files | **Refused** by stream initialization and `voice`/`stream_error`, before their timing or I/O can affect a supported target. | Stream clock/text boundary tests; existing stream voice timeout tests |
| Dialogue voice duration/auto-wait (`voice.go`) | **Refused** at `voice`, including missing clips. This avoids accepting voice-dependent timing under different playback/seek conditions. | `TestTraceSourceBoundaries/voice` |
| Boss fanfare completion (`battle_drama.go`) | **Refused** at `audio_timing` before polling the audio player's state. | `TestTraceSourceBoundaries/audio_timing` |
| Sound-length waits (`sfx.wait`, `layer5/sound`) | **Restored**: muted startup reads custom stream lengths without PCM decoding or playback; built-in durations share the synthesis definitions. Mute does not shorten waits. | `TestEffectDurationsWithoutPlayback`, `TestTraceUnmutedSFXWaitMutedReproduction` |
| SFX variation clock and unseeded random pitch/volume (`layer5/sound/vary.go`), effect-player completion | **Restored screen state**: these affect audio samples/player cleanup only, with no feedback into game state or pixels. Exact audio is outside this contract. | Sound variation tests; muted graphics reproduction tests |
| Gameplay/collection RNG, legacy-save parsing, music-room background shuffle (`play.go`, `rpg`, `gallery_music.go`) | **Recorded/restored** seed; traced runs replace seed zero before any RNG is created. Music-room wall-clock fallback and RPG unseeded fallbacks are not used by a supported recording's gameplay. Save-validation-only RNG does not change the live state. | Same-seed trace tests; `TestTraceMusicRoomSeedRestored` |
| Puppet animation RNG and display previews (`puppets.go`, `meters.go`, `layer5/puppet`) | **Restored** from stable actor hashes or a clone of the game RNG. Simulation ticks advance animation; Draw does not advance it. | `TestEnemyPuppetMotionDoesNotDependOnDrawing`, graphics seek comparison |
| Picture preload workers (`assets.go`, `picdecode`, `webfs`) | **Restored**: requested ordinary pictures synchronously join their load; preloading changes latency only. Captured doll/face loads wait without advancing ticks; the `time.Now` deadline fails instead of accepting a placeholder. | `TestDollCaptureWaitsForRequestedPicture`, `TestDollCaptureTimeoutDoesNotSavePlaceholder` |
| Asynchronously appearing doll/face sheets during interactive play (`stage_dolls.go`) | **Refused** at `async_picture` when an asynchronous request starts. Even a later completed frame is outside the accepted prefix. | `TestTraceAsyncPictureRefused` |
| F7 balance computation budget (`graphs.go`, `time.Now`/`time.Since`) | **Refused** at `graphs` when opened; partial progress depends on machine speed. | `TestTraceSourceBoundaries/graphs` |
| Profile swipes/raw Tab and touch polling (`profiles.go`) | **Refused** at `profiles` on opening the book. | `TestTraceProfileDeviceInputRefused` |
| Window size/layout, focus loss, fullscreen, browser visibility | Viewport **recorded**, changed viewports/window actions **refused**. Focus pauses at tick 0 before the first update are presentation-only and accepted; replay skips focus handling. Later focus loss resets movement and is **refused** at `focus_pause`. Fullscreen uses `config_write`; stream browser visibility is refused with streams. | Viewport tests; `TestTraceSourceBoundaries/focus`, `TestTraceStartupFocusPause` |
| Live reload watcher and reload requests (`live.go`) | **Refused** at `live_reload`, including requests that only show an unavailable/retry message. | `TestTraceSourceBoundaries/reload`, existing live-reload tests |
| Review logs, review text, timestamped verdict writes (`scenecheck.go`, `cgreview.go`) | **Refused** at review entry before reading files; review writes remain refused too. | `TestTraceSourceBoundaries/scene_review`, `picture_review` |
| Screenshot/PNG workers and video output (`shots.go`, `record.go`) | **Restored screen state**: workers receive copied pixels; completion never mutates game state. I/O failure terminates the run. A target screenshot waits for requested pictures. | Screenshot failure tests; `TestGraphicsScreensRepro` |
| Debug logging, autoplay reports, seek-loop budgets, browser cooperative yields (`play.go`, `autopilot.go`, `layer1/pace`) | **Restored screen state**: `time.Now`/`time.Since` affect diagnostics, scheduling or report files only. Seek batches preserve simulation ticks; these clocks never enter accepted screen state. | `TestTraceReproBatchesPreserveTargetState`, `TestTraceReproBatchesIdleTicks` |

Source markers are emitted at operations rather than by a new per-frame scan.
The store observer and resource fingerprints exist only for tracing/replay;
ordinary play retains its input, clocks, storage and loading behavior.


Exact reproduction requires source audit revision 3. Autopilot traces are refused
because selection changes are not recorded. Recordings of replays are also
refused because replay provenance and source-only refusal records are not
preserved by re-recording. Record the original scripted or interactive run.

Initial slots use separate `initial_save` records (filename and base64 data),
with an 8 MiB decoded limit per save. Larger slots or memo saves mark the start
unsupported, so `--repro` reports the limit instead of producing an incomplete
bundle. All trace writers and readers share a 16 MiB line limit. Bundling checks
initial save digests and refuses missing or altered bytes.

A replay with `--until` batches ordinary updates, without intermediate drawing
within each batch, to seek quickly. Normal play, capture and video recording keep
their existing pacing. Seeking and screenshot recording (including
`--trace-shots` without a script) wait for the faces and doll sheets requested
by the current screen. The wait shares a ten-second deadline per draw and does
not advance simulation ticks; a timeout fails the run instead of saving a
placeholder. Ordinary interactive play keeps loading these pictures asynchronously.
Data edits, live reloads and unsupported review starts
are rejected. Old traces without a game directory or required save bytes report
what must be recorded again; ordinary replay of older traces is unchanged.

Run the PNG comparison with `go test -tags=graphics ./internal/play -run
'^TestGraphicsScreensRepro$' -count=1` from a desktop (wrap with
`xvfb-run --auto-servernum --error-file=/dev/stderr` on Linux). It records neutral
novel, campaign and battle-puppet fixtures and executes the printed reproduction
command. All reproduction PNG comparisons allow GPU/driver rounding of at most
1/255 per channel in at most 0.05% of pixels (rounded down to a whole pixel).
Image bounds must match; exceeding either limit fails with the pixel count,
changed bounds and maximum channel delta. Replay game-state trace comparisons
through the target tick remain exact; this tolerance applies only to PNG pixels.
Enemy puppet motion advances in simulation
updates, independent of drawing. The puppet's composed mesh texture is unmanaged:
automatic atlas promotion after earlier draws changes float32 source coordinates
used by linear filtering, so drawing history can affect channel rounding even at
the same pose. `TestGraphicsSeekMatchesPlayback` in `internal/layer5/puppet`
compares complete motion state (including phases), poses and pixels after 90
updates with and without intermediate draws. Run it with the `graphics` tag in
the same display environment.

Round doll face textures also stay off the automatic atlas: map tokens and
campaign portraits scale them with linear filtering. `TestGraphicsScreensRepro`
compares both the ordinary campaign and a longer warmed-up run against seeking,
with the same bounded PNG tolerance. Offscreen layout defaults to device scale 1;
only `StartRendering` installs the monitor query, and layout tests can inject a
scale without initializing GLFW.

## Balance display

The F7 balance panel is described for users on the site: [F7 balance panel](https://nao1215.github.io/gozen/edit-play-loop/#f7-balance-panel). It shows the same numbers as `gozen sim battle`, with the same function and the same random seed (battle i uses `PCG(1, i)`), so a note's `.save` can be checked in a terminal with `gozen sim battle … --state … --hp`.

For developers: the measurement is divided across the game's updates at up to 3 ms per frame, and no other goroutine reads the game state or definitions. It does not change the game state or random numbers, and does not compute while the panel is closed. Full-heal numbers are measured in the state when the place, data, or the level or equipment of the line change. The numbers for the current HP and the win streak are measured again after HP, MP, states or experience change and then stay unchanged for 45 ticks (about 0.75 seconds); they are not measured again during a battle, and 200 battles are not rerun while recovery or fighting continues before things settle. Starting with `--dir` and reloading with F5 (the DB changes), moving to another place, or changing the level, experience, equipment or skill tree of a line measures again. The strings on the screen go through `lang.T`, so `text` in `game.yaml` can reword them.

The picture `docs/images/go-zen-balance.jpg` is remade by `make docs-images` ([README and site pictures and steps](docs.md)). The run `balance` in the ledger `docs/images/manifest.yaml` starts Go Zen in the Cistern Hall with the input of `scripts/capture-go-zen-balance.txt`, presses F7, uses `wait-graphs` to wait until every measurement finishes, takes the picture and makes a 960×540 JPEG.

The `balance` scene of `make graphics` takes the same display and checks that the color of the win-rate bar appears at the top right.

The input command `wait-graphs` takes no arguments and requires an open development balance panel. It waits for all measurements rather than a fixed frame count, and a closed panel or measurement error fails the run. While this command waits, measurement uses a 12 ms budget per update instead of the normal interactive 3 ms budget; the trials, seeds and results remain the same. The capture runner's process timeout bounds this wait.
