# Gozen

[日本語](README.ja.md) · [MIT license](LICENSE) · [Asset licenses](ASSETS.md)

## Overview

Gozen is a Go and Ebitengine 2D RPG framework and toolkit. Define a game in YAML, embed your own filesystem, and run the same game rules with a window or a headless driver. It is under development; the v0 API is being established.

![Wayfinder: an original trail game built with Gozen](docs/images/wayfinder.gif)

The [Wayfinder sample](docs/examples/wayfinder.md) uses original code-drawn art and a short journey through conversation, battle, events, movement and saving. [Full-size screenshot](docs/images/wayfinder.png).

## Features

- YAML definitions, reference validation, expressions and conditional events.
- Turn-based battles, world navigation, grid exploration, and versioned saves.
- Headless simulations sharing the rules used by the graphical game.
- Ebitengine presentation, font rendering, audio and an embedded-game runner.
- Four installable CLIs for validation, simulation, authoring and image processing.

## Installation

Use Go 1.26.4 or later. From a source checkout, install all four commands locally:

```sh
mkdir -p bin
GOBIN="$PWD/bin" go install -trimpath -buildvcs=false ./cmd/rpgcheck ./cmd/rpgtool ./cmd/promptgen ./cmd/mosaic
```

Add `bin` to PATH, or run the commands with `go run` as below. In PowerShell, set `$env:GOBIN = Join-Path $PWD bin` before the same `go install` command. The module is `github.com/nao1215/gozen`; all commands share its version. Published installation is not yet verified. After a release, the form will be `go install github.com/nao1215/gozen/cmd/rpgtool@latest` (substitute any of the four command names).

## Requirements

Go 1.26.4+ builds the examples and tools. The graphical sample needs a display and platform graphics libraries; use `--mute` to avoid audio-device initialization. Linux needs X11/OpenGL libraries even with a pure Go build. See [OS setup](docs/development/graphics.md#requirements) for Ubuntu packages and Windows/macOS preparation.

| Component | Additional requirements |
| --- | --- |
| Core, headless examples, rpgcheck, normal rpgtool commands | None; no display or C compiler |
| Graphical game / rpgtool shots | Graphics runtime; Xvfb + xauth for unattended Linux |
| promptgen | User catalog YAML; no model or image-generation service |
| mosaic inference | cgo + C compiler at build time; compatible ONNX Runtime library and user-supplied models at runtime |
| Showcase capture | Linux graphics runtime, Xvfb, xauth, Bash, Python 3, ffmpeg/ffprobe |

mosaic help and manual processing work without ONNX. A cgo-disabled build cannot perform inference. [OS-specific production-tool requirements](docs/development/production-tools.md#導入と-os-別の要件).

## Quick start

From the repository root:

```sh
go run ./examples/minimal
# location=camp gold=2
go run ./examples/headless
# location=trail battles=1 wins=1 gold=3
# With a display:
go run ./examples/play --mute
```

For unattended Linux use the capture command below, which runs and terminates the actual game. The interactive command keeps its window open until you close it.

## Minimal game

[examples/minimal/main.go](examples/minimal/main.go) is a small Go host: `data.Load(files, ".")`, `rpg.New(db, nil)` and `headless.New(db, state).Start()` load and run an embedded game. It uses these two YAML files:

```yaml
# game.yaml
schema_version: 1
id: tiny-trail
title: Tiny trail
start: {location: camp, party: [walker]}
```

```yaml
# data/world.yaml
classes:
  walker:
    name: Walker
    stats: {mhp: 20, mmp: 0, atk: 4, def: 2, mat: 0, mdf: 0, agi: 4, luk: 1}
    exp_for: level * 20
actors: {walker: {name: Walker, class: walker}}
locations:
  camp:
    name: Camp
    on_enter: [{say: A new trail begins here.}, {gold: 2}]
skills:
  attack: {name: Tap, effects: [{damage: 4}]}
  guard: {name: Wait, scope: self}
```

Even a game without battles defines the default `attack` and `guard` skill IDs. This example needs no assets and does not write saves.

## Run the sample

```sh
go run ./examples/play --help
go run ./examples/play --mute
```

Choose **New journey**, talk to Courier, win **Practice** (choose **Auto** or attack manually), **Light the marker**, then **Walk to the ridge**. **Save journey** writes a slot; **Continue** at the title restores it. **Explore chamber** opens a room with grid movement. [Walkthrough and capture details](docs/examples/wayfinder.md).

To regenerate the checked-in screenshot and nine-second GIF slideshow on Linux:

```sh
xvfb-run --auto-servernum --error-file=/dev/stderr bash scripts/capture-example.sh
```

## CLI commands

Run these from the repository root. The validation/simulation fixture is intentionally separate from the embedded graphical sample. [All rpgtool subcommands](docs/development/cli.md#コマンド).

### rpgcheck

Requirements: Go to build; no cgo, display or external runtime tools.

```sh
go run ./cmd/rpgcheck e2e/atago/fixtures/game
```

Validates definitions, references, expressions and asset existence. Text layout is a separate rpgtool check.

### rpgtool

Requirements: Go to build. Normal checks, simulations and reports need no display or C compiler; `shots` needs a game binary and graphics runtime. [OS requirements](docs/development/cli.md#requirements).

```sh
go run ./cmd/rpgtool --help
go run ./cmd/rpgtool sim e2e/atago/fixtures/game lesson -n 3 -seed 7
```

### promptgen

Requirements: Go and a catalog YAML. No cgo, models or remote service. Assembles prompts; does not generate images or send jobs.

```sh
go run ./cmd/promptgen --config docs/examples/production/catalog.yaml --scene landscape --format json
```

### mosaic

Requirements: Go; inference additionally needs a C compiler, compatible ONNX Runtime library and user models. Manual-only processing needs none of those runtime dependencies. [Setup and model contracts](docs/development/production-tools.md).

```sh
go run ./cmd/mosaic --help
mkdir -p build/mosaic-input/assets
cp docs/examples/production/pattern.png build/mosaic-input/assets/pattern.png
go run ./cmd/mosaic auto -fix docs/examples/production/fixes.yaml -out build/mosaic-output build/mosaic-input pattern.png
```

This example pixelates selected rectangles in an original synthetic pattern without inference. Input stays unchanged; output and its report go under `build/mosaic-output`.

## Configuration

`game.yaml` holds the game ID, schema version, start party/location, window settings, fonts and translated UI text. YAML files under `data/` (including nested directories and `.yml` files) hold definitions and events. Asset paths are relative to `assets/` in the supplied `fs.FS`. See the [schema reference](docs/schema.md).

`runner` exposes `--mute`, `--seed`, `--window` and `--save-dir`; author tools such as `--start` and `--script` require `--dev`. Normal saves go under the OS config directory in `gozen/<game-id>`. Scripted runs isolate saves unless a destination is explicit. [Startup and ownership contracts](docs/architecture/presentation-api.md).

## API examples

| Purpose | Entry point | Executable example |
| --- | --- | --- |
| Load embedded YAML and start rules | `data.Load`, `rpg.New`, `headless.New` | [minimal](examples/minimal/main.go) |
| Seed randomness, move, fight, serialize and restore | `Driver.Run`, `State.Save`, `rpg.Parse` | [headless](examples/headless/main.go) |
| Run an embedded game with flags and context | `runner.Run(ctx, fsys, ".", runner.Config{Args: args, Output: out})` | [play](examples/play/main.go) |
| Compose a screen in an existing Ebitengine loop | `play.New`, `Game.Update/Draw/Layout/Close` | [presentation contracts](docs/architecture/presentation-api.md) |

The host owns the filesystem, output writers and process exit. Run graphics on the main thread, with one active game per process, and close a composed `play.Game` after its loop stops.

## Tests and build

```sh
make test lint build
# Linux display checks:
xvfb-run --auto-servernum --error-file=/dev/stderr make graphics
```

Lint requires go-arch-lint v1.19.0, golangci-lint v2.14.0 and actionlint v1.7.12. Make is optional: [direct commands and CLI E2E](docs/development/testing.md). The API inventory is checked by tests; examples introduce no public identifiers.

## Supported operating systems

Linux, Windows and macOS are CI targets; amd64 builds and Linux/macOS arm64 cross-builds are configured. T8 verified tests, builds, 58 CLI scenarios, five graphical screens and a screenshot pipeline on Linux amd64. Windows/macOS runtime, audio playback, ONNX inference and successful remote CI runs remain unverified. Cross-compilation alone is not runtime verification. [Release checklist](docs/development/release-checklist.md).

## Controls

Arrow keys select menu entries; Enter/Z confirms; X/Esc goes back or opens the menu. In the observation room, Up walks, Left/Right turns and Down turns around. M mutes and F11 toggles fullscreen. [Save and load walkthrough](docs/examples/wayfinder.md#play-through).

## Limitations

The project is under development and has no verified published installation yet. Some optional sample UI still uses Japanese defaults, and Go Regular lacks a few indicator glyphs. The small sample has no music. ONNX models/runtime are never bundled; actual inference needs a separately prepared environment. The `x/` directories are placeholders with no implemented Go packages; future experiments have no compatibility promise. See [v0 scope](docs/decisions/0002-public-api-scope.md) and the [release checklist](docs/development/release-checklist.md).

## Developer links

[Release checklist](docs/development/release-checklist.md) · [Architecture](docs/architecture/overview.md) · [API rules](docs/architecture/api-guidelines.md) · [Core API](docs/architecture/core-api.md) · [Presentation API](docs/architecture/presentation-api.md) · [ADRs](docs/decisions/index.md) · [Testing](docs/development/testing.md) · [CI](docs/development/ci.md) · [Contributing](CONTRIBUTING.md) · [Security](SECURITY.md) · [Changelog](CHANGELOG.md) · [CLI guide](docs/development/cli.md) · [Production tools](docs/development/production-tools.md)

## License

[MIT](LICENSE). The Go font has a separate BSD notice in [ASSETS.md](ASSETS.md), which also records asset sources and reproduction steps.
