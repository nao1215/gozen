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
- `gozen sim script` (`play.Simulate`, [ADR 0046](../decisions/0047-headless-scripted-runs.md)) runs the same `Game` without `Draw` or the Ebitengine loop. A pending script shot pauses `Update` only while `rendering` is set, and the pause comes before the tick advances, so the ticks are those of `run`. Pictures are not decoded ahead, and `pictures.load` gives an empty image of the size in the file header (`picdecode.Size`). Interactive lines (`--script -`) are appended by `script.more` when the commands run out; `Simulate` writes `ready` and flushes before each read. `TestGraphicsSimulateMatchesRun` compares both paths line by line, without the drawing lines and `music`.
- Notes are made with F9 under `--dev --memos notes`. They record the screen, the state and an annotation, and finalize the JSON last. Adding the build revision is enabled separately with `--build-info`, and external input on Linux with `--external-input`. No source search or Git execution is done.

## Balance display

The F7 balance panel is described for users on the site: [F7 balance panel](https://nao1215.github.io/gozen/edit-play-loop/#f7-balance-panel). It shows the same numbers as `gozen sim battle`, with the same function and the same random seed (battle i uses `PCG(1, i)`), so a note's `.save` can be checked in a terminal with `gozen sim battle … --state … --hp`.

For developers: the measurement is divided across the game's updates at up to 3 ms per frame, and no other goroutine reads the game state or definitions. It does not change the game state or random numbers, and does not compute while the panel is closed. Full-heal numbers are measured in the state when the place, data, or the level or equipment of the line change. The numbers for the current HP and the win streak are measured again after HP, MP, states or experience change and then stay unchanged for 45 ticks (about 0.75 seconds); they are not measured again during a battle, and 200 battles are not rerun while recovery or fighting continues before things settle. Starting with `--dir` and reloading with F5 (the DB changes), moving to another place, or changing the level, experience, equipment or skill tree of a line measures again. The strings on the screen go through `lang.T`, so `text` in `game.yaml` can reword them.

The picture `docs/images/go-zen-balance.jpg` is remade by `make docs-images` ([README and site pictures and steps](docs.md)). The run `balance` in the ledger `docs/images/manifest.yaml` starts Go Zen in the Cistern Hall with the input of `scripts/capture-go-zen-balance.txt`, presses F7, uses `wait-graphs` to wait until every measurement finishes, takes the picture and makes a 960×540 JPEG.

The `balance` scene of `make graphics` takes the same display and checks that the color of the win-rate bar appears at the top right.

The input command `wait-graphs` takes no arguments and requires an open development balance panel. It waits for all measurements rather than a fixed frame count, and a closed panel or measurement error fails the run. While this command waits, measurement uses a 12 ms budget per update instead of the normal interactive 3 ms budget; the trials, seeds and results remain the same. The capture runner's process timeout bounds this wait.
