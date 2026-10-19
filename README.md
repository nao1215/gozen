[![CI](https://github.com/nao1215/gozen/actions/workflows/ci.yml/badge.svg)](https://github.com/nao1215/gozen/actions/workflows/ci.yml)
[![Security](https://github.com/nao1215/gozen/actions/workflows/security.yml/badge.svg)](https://github.com/nao1215/gozen/actions/workflows/security.yml)
[![tested with atago](https://img.shields.io/badge/tested%20with-atago-7c3aed?logo=data:image/svg%2Bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCI%2BPHBhdGggZmlsbD0iI2ZmZiIgZD0iTTMuNiA0LjIgMTEuOSAxMmwtOC4zIDcuOC0xLjktMi4yTDcuOSAxMiAxLjcgNi40eiIvPjxyZWN0IGZpbGw9IiNmZmYiIHg9IjEyLjYiIHk9IjE3LjIiIHdpZHRoPSI5LjciIGhlaWdodD0iMi44IiByeD0iMS40Ii8%2BPC9zdmc%2B&logoColor=white)](https://github.com/nao1215/atago)
[![Go Reference](https://pkg.go.dev/badge/github.com/nao1215/gozen.svg)](https://pkg.go.dev/github.com/nao1215/gozen)
![GitHub](https://img.shields.io/github/license/nao1215/gozen)

# Gozen

Gozen is a 2D RPG framework and toolkit for Go and Ebitengine. A game is YAML plus your own pictures. The same rules run in a window, in a headless Go driver, and in command-line tools that validate and simulate the game, so balance and story progress can be tested in CI without a display.

[日本語](README.ja.md)

![Wayfinder, the Gozen sample game: title, conversation, battle, saving and walking](docs/images/wayfinder.gif)

The animation above is Wayfinder, the sample in [examples/play](examples/play). It is the real game, recorded from its own Draw loop.

Gozen is under development and the v0 API is still being established. There is no published release yet.

## Try it in 30 seconds

If you have Go 1.26.4 or later, clone the repository and run the smallest game. It needs no display and no assets:

```sh
git clone https://github.com/nao1215/gozen
cd gozen
go run ./examples/minimal
```

```text
location=camp gold=2
```

The game is two YAML files and a 35-line Go host. `data.Load` reads an embedded `fs.FS`, `rpg.New` creates the state, and `headless.New(db, state).Start()` runs the arrival event of the first location.

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

Even a game without battles defines the default `attack` and `guard` skill IDs.

## Play the sample

With a display (Linux needs X11 and OpenGL libraries; see [OS setup](docs/development/graphics.md#requirements)):

```sh
go run ./examples/play --mute
```

Choose New journey, talk to the courier, win the practice battle (Auto lets the engine fight), light the marker, and walk to the ridge. Save journey writes a slot, and Continue on the title screen restores it. The village has an inn, the grotto has a wild Pebble to fight again, and the observation room is a first-person grid with a minimap. [Walkthrough and controls](docs/examples/wayfinder.md).

Screens from the same run:

| Title | Conversation | Battle |
|:--|:--|:--|
| ![title](docs/images/wayfinder-title.jpg) | ![conversation](docs/images/wayfinder-talk.jpg) | ![battle](docs/images/wayfinder-battle.jpg) |

| Dungeon | Party | Cut-in |
|:--|:--|:--|
| ![dungeon](docs/images/wayfinder-dungeon.jpg) | ![party battle](docs/images/wayfinder-party.jpg) | ![cut-in](docs/images/wayfinder-cutin.jpg) |

## Why Gozen?

Pick the tool that fits the job:

| You want | Use |
|:--|:--|
| A general 2D game library with no RPG rules | [Ebitengine](https://ebitengine.org/), which Gozen is built on |
| A graphical editor for RPG maps and events | [RPG Maker](https://www.rpgmakerweb.com/) |
| A full engine with a scene editor and many export targets | [Godot](https://godotengine.org/) |
| An RPG defined as data in a Go program, with a headless driver and CLIs that check, simulate and screenshot it | Gozen |

Gozen's emphasis is that the game is data you can test. The same definitions that draw the title screen are loaded by `gozen check` for references and expressions, by `gozen sim` for win rates, and by `headless` in a Go test, so a broken reference or a balance regression fails in CI instead of in front of a player.

## Install

Gozen is a single Go module, `github.com/nao1215/gozen`, and needs Go 1.26.4 or later. Published installation is not yet verified; until a release exists, install the `gozen` command from a source checkout:

```sh
mkdir -p bin
GOBIN="$PWD/bin" go install -trimpath -buildvcs=false ./cmd/gozen
```

In PowerShell, set `$env:GOBIN = Join-Path $PWD bin` first. After a release the form will be `go install github.com/nao1215/gozen/cmd/gozen@latest`. Without a C compiler Go builds it without cgo, and `gozen mosaic` then reports that ONNX inference is missing. As a library, `go get github.com/nao1215/gozen@latest` adds the module.

## Requirements

Go 1.26.4+ builds the examples and tools. The graphical sample needs a display and platform graphics libraries; use `--mute` to avoid audio-device initialization. Linux needs X11 and OpenGL libraries even with a pure Go build. See [OS setup](docs/development/graphics.md#requirements) for Ubuntu packages and Windows and macOS preparation.

| Component | Additional requirements |
| --- | --- |
| Core, headless examples, normal gozen subcommands | None; no display or C compiler |
| Graphical game, gozen shots | Graphics runtime; Xvfb and xauth for unattended Linux |
| gozen prompt | User catalog YAML; no model or image-generation service |
| gozen mosaic inference | cgo and a C compiler at build time; compatible ONNX Runtime library and user-supplied models at runtime |
| Recording the sample animation | Linux graphics runtime, Xvfb, xauth, Bash, Python 3, ffmpeg and ffprobe |

## Recipes

Run these from the repository root.

### Drive a game from Go

`headless` runs the rules without a window. [examples/headless](examples/headless/main.go) seeds randomness, moves, makes a choice, fights, saves and restores in memory:

```sh
go run ./examples/headless
```

```text
location=trail battles=1 wins=1 gold=3
```

### Run an embedded game in a window

[examples/play/main.go](examples/play/main.go) embeds a game directory and passes it to the runner:

```go
//go:embed game
var files embed.FS

fsys, err := fs.Sub(files, "game") // game.yaml, data/, assets/
if err != nil {
	log.Fatal(err)
}
err = runner.Run(ctx, fsys, ".", runner.Config{Args: os.Args[1:], Output: os.Stdout})
```

`runner` provides `--mute`, `--seed`, `--window` and `--save-dir`. Author tools such as `--start` and `--script` need `--dev`. Saves go under the OS config directory in `gozen/<game-id>`. A script run isolates saves unless a destination is explicit. The host owns the filesystem, output writers and process exit.

To compose the screen inside an existing Ebitengine loop, use `play.New` and call `Game.Update`, `Draw`, `Layout` and `Close`; see the [presentation contracts](docs/architecture/presentation-api.md). Run graphics on the main thread, with one active game per process.

### Record the animation above

`scripts/capture-example.sh` builds Wayfinder, plays it from a script of key presses, takes frames from the Draw loop, checks that the saves contain the finished journey, and encodes the GIF. It runs in a virtual display, so it does not open a window on your screen:

```sh
xvfb-run --auto-servernum bash scripts/capture-example.sh
```

It needs Xvfb, xauth, Python 3, ffmpeg and ffprobe. [Capture details](docs/examples/wayfinder.md#capture).

## CLI commands

Run these from the repository root. The validation and simulation fixture is a small neutral game, separate from the graphical sample. [All gozen subcommands](docs/development/cli.md#コマンド).

### gozen

Requirements: Go to build. Checks, simulations, reports and `prompt` need no display or C compiler; `shots` needs a game binary and a graphics runtime; `mosaic` inference needs a gozen built with cgo, a compatible ONNX Runtime library and models you supply. [OS requirements](docs/development/cli.md#requirements). `gozen help <command>` shows each subcommand's flags and requirements.

`gozen check` validates definitions, references, expressions, asset existence and text layout. `--no-layout` skips the font-based layout check:

```sh
go run ./cmd/gozen check --no-layout e2e/atago/fixtures/game
```

```text
ok: 2 locations, 1 events, 1 actors, 2 skills, 0 items, 1 enemies, 1 troops, 0 ai sets
Structure checked; layout was not checked. Use gozen layout <game-dir>.
```

`gozen sim` fights a troop many times with a fixed seed and reports win rate and turns:

```sh
go run ./cmd/gozen sim e2e/atago/fixtures/game lesson -n 3 --seed 7
```

```text
lesson: 3 battles  win 100%  lose 0%  fled 0%  draw 0%  avg turns 1.0  avg standing after a win 1.0
```

`go run ./cmd/gozen --help` lists the other subcommands: `layout`, `curve`, `floors`, `trace`, `book`, `design`, `memos`, `release`, `trial`, `shots`, `prompt`, `mosaic`, `version` and `help`.

`gozen prompt` needs a catalog YAML, but no cgo, models or remote service. It assembles image-generation prompts; it does not generate pictures or send jobs.

```sh
go run ./cmd/gozen prompt --config docs/examples/production/catalog.yaml --scene landscape --format json
```

Use `--scenes ids.txt --format json` to build multiple scene IDs (one per line) with a single catalog load. See [batch output and exit status](docs/development/production-tools.md#gozen-prompt).

`gozen mosaic` inference additionally needs a C compiler at build time, a compatible ONNX Runtime library and models you supply. Manual-only processing needs none of those. [Setup and model contracts](docs/development/production-tools.md).

```sh
go run ./cmd/gozen mosaic --help
mkdir -p build/mosaic-input/assets
cp docs/examples/production/pattern.png build/mosaic-input/assets/pattern.png
go run ./cmd/gozen mosaic auto --fix docs/examples/production/fixes.yaml --out build/mosaic-output build/mosaic-input pattern.png
```

This pixelates selected rectangles in an original synthetic pattern without inference. The input stays unchanged; the output and its report go under `build/mosaic-output`.

## Features

- YAML definitions with reference validation, expressions and conditional events.
- Turn-based battles with auto-battle, world navigation, first-person grid exploration, and versioned saves.
- Headless simulation that shares the rules of the graphical game.
- Ebitengine presentation, font rendering, audio and an embedded-game runner.
- Conversation portraits, standing figures on the title screen, location and battle backgrounds, and a field menu.
- One installable `gozen` command with subcommands for validation, simulation, authoring and image processing.

## Configuration

`game.yaml` holds the game ID, schema version, start party and location, window settings, fonts and translated UI text. YAML files under `data/`, including nested directories, hold definitions and events. Asset paths are relative to `assets/` in the supplied `fs.FS`. The [schema reference](docs/schema.md) lists every field.

## Supported OS

Linux, Windows and macOS are CI targets, and amd64 builds and Linux/macOS arm64 cross-builds are configured. Tests, builds, the CLI scenarios, five graphical screens and the capture pipeline were verified on Linux amd64. Windows and macOS runtime, audio playback, ONNX inference and successful remote CI runs remain unverified; cross-compilation alone is not runtime verification. See the [release checklist](docs/development/release-checklist.md).

## Controls

Arrow keys select menu entries; Enter or Z confirms; X or Esc goes back or opens the menu. In the observation room, Up walks, Left and Right turn, and Down turns around. M mutes and F11 toggles fullscreen.

## Tests

```sh
make test lint build
# Linux, in a virtual display:
xvfb-run --auto-servernum make graphics
```

Lint needs go-arch-lint v1.19.0, golangci-lint v2.14.0 and actionlint v1.7.12. Make is optional; see [testing](docs/development/testing.md) for the direct commands and the CLI E2E scenarios.

## Limitations

- No published installation yet, and Windows and macOS runtime checks are pending.
- Some optional sample UI still uses the engine's Japanese defaults, and Go Regular lacks a few indicator glyphs. The sample has no music.
- The `x/` directories are placeholders with no implemented Go packages and no compatibility promise.
- ONNX models and the runtime are never bundled.

See the [v0 scope](docs/decisions/0002-public-api-scope.md).

## Documentation

[Architecture](docs/architecture/overview.md) · [API rules](docs/architecture/api-guidelines.md) · [Core API](docs/architecture/core-api.md) · [Presentation API](docs/architecture/presentation-api.md) · [Decisions](docs/decisions/index.md) · [CI](docs/development/ci.md) · [CLI guide](docs/development/cli.md) · [Production tools](docs/development/production-tools.md) · [Release checklist](docs/development/release-checklist.md)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Report vulnerabilities as described in [SECURITY.md](SECURITY.md). Changes are listed in the [changelog](CHANGELOG.md).

## License

The code is [MIT](LICENSE). The sample pictures of Wayfinder are CC BY 4.0 ([examples/play/game/assets/LICENSE](examples/play/game/assets/LICENSE)). The Go Regular font notice is in [ASSETS.md](ASSETS.md).
