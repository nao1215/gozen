# Graphics development and verification

Use Go 1.26.9 or later. The normal tests of T3 start no display or audio device. Real drawing is checked by separate tests with the `graphics` tag. For the items to check before publication, see the [release checklist](release-checklist.md).

## Original sample

Wayfinder is an original small sample. How to play it and how to remake its PNGs and GIFs with Xvfb are in the [sample guide](../examples/wayfinder.md). A normal run saves under the OS config directory in `gozen/wayfinder-demo`; a trial run with a start position uses a temporary save when the save destination is omitted. The sources of the assets and fonts are in the [asset ledger](../examples/play-assets.md).

The startup options that `play.Main` accepts use the same GNU style as `gozen` ([ADR 0018](../decisions/0018-gnu-options-and-help.md)). `--seed 7` and `--seed=7` both work, and a single-dash long name such as `-dev` is an error. `--help` has the shape `[Usage]`, `[Examples]`, `[Options]` grouped by kind (game, trial runs, automatic play, scripts and records, notes and review), `Requirements:` and links; an option that needs `--dev` has `(--dev)` in the name of its group.

For how to make the embedded FS and handle exit codes, see `examples/play/main.go`; for the API contract, see the [presentation API](../architecture/presentation-api.md). The library never exits the process.

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
go test -tags=graphics ./play ./puppet -run TestGraphics -count=1
```

The pixel check of `./puppet` runs one Ebitengine loop during the test, and does the drawing and `ReadPixels` of each test inside the loop's Update. With small neutral shapes it checks the base picture, translucency, blinking, lip sync, switching of expressions (the blended colors in the middle and the final color), the bouncing reaction, that nothing is drawn after Close, and that the mesh does not come out away from the shape.

Six scenarios (title, field, conversation, battle, dungeon and the balance panel (F7)) are started in separate processes. For the drawn scene (`stage:`, [maps and drawn scenes](../architecture/scenes.md)), `TestGraphicsStage` walks `play/testdata/stage` (a small picture made by a script) and checks with the trace that the player's figure is visible, that clicking a figure walks to it and talks, that it disappears behind the fronts of a pillar (a pixel check), and that the range of an exit moves to another place. `TestGraphicsMap` checks that a piece walks the roads on a map, the one line of a closed point, and going from a point to a place. `TestGraphicsWild` checks a map inside a map (`at_point`), `token.picture` (pixels), the roads of `draw_roads` (pixels), that a point with `unknown: hidden` does not appear in the list, and that a battle on a road happens before arrival. The seed is fixed, a test-only save destination is used, and PNGs are taken inside Draw. They check the PNG decoding, the size of 960×540, that the content is not a single color, and the normal exit of the process. This is not a pixel-exact golden, so the quality of layout and text still needs a separate check. The audio device is muted.

Without Xvfb or a drawing environment, the tests fail. A skip or a mere compile is not treated as a drawing success. The right to run is also needed for Xvfb itself to create its socket.

## Input scripts and traces

Running the game with `--script`, `--trace`, `--replay` and `--record` is described for users on the site: [Scripts and screenshots](https://nao1215.github.io/gozen/edit-play-loop/#scripts-and-screenshots) and [Trace and replay](https://nao1215.github.io/gozen/edit-play-loop/#trace-and-replay). Developer notes follow.

- The canonical list of button names is `scriptButtons` in `internal/play/input.go`.
- A picture name that contains a path separator is rejected. A script needs an explicit `--shots`.
- Recording needs a script, automatic play or a replay. A long replay and the integrated run with ffmpeg are not confirmed in the local T3 environment.
- Notes are made with F9 under `--dev --memos notes`. They record the screen, the state and an annotation, and finalize the JSON last. Adding the build revision is enabled separately with `--build-info`, and external input on Linux with `--external-input`. No source search or Git execution is done.

## Balance display

The F7 balance panel is described for users on the site: [F7 balance panel](https://nao1215.github.io/gozen/edit-play-loop/#f7-balance-panel). It shows the same numbers as `gozen sim battle`, with the same function and the same random seed (battle i uses `PCG(1, i)`), so a note's `.save` can be checked in a terminal with `gozen sim battle … --state … --hp`.

For developers: the measurement is divided across the game's updates at up to 3 ms per frame, and no other goroutine reads the game state or definitions. It does not change the game state or random numbers, and does not compute while the panel is closed. Full-heal numbers are measured in the state when the place, data, or the level or equipment of the line change. The numbers for the current HP and the win streak are measured again after HP, MP, states or experience change and then stay unchanged for 45 ticks (about 0.75 seconds); they are not measured again during a battle, and 200 battles are not rerun while recovery or fighting continues before things settle. Starting with `--dir` and reloading with F5 (the DB changes), moving to another place, or changing the level, experience, equipment or skill tree of a line measures again. The strings on the screen go through `lang.T`, so `text` in `game.yaml` can reword them (Wayfinder replaces them with English).

The picture `docs/images/wayfinder-balance.jpg` is remade by `make docs-images` ([README and site pictures and steps](docs.md)). The run `balance` in the ledger `docs/images/manifest.yaml` starts Wayfinder in the grotto with the input of `scripts/capture-balance.txt`, presses F7, waits until the measurement finishes, takes the picture and makes a 960×540 JPEG.

The `balance` scene of `make graphics` takes the same display and checks that the color of the win-rate bar appears at the top right.
