---
title: Getting started
description: Create and play your first game with YAML and assets. No Go source or compiler is needed.
weight: 20
---

[Install the Gozen binary](/install/). Besides it you need a text editor and a terminal, not Go, Git or a programming language; the window needs your platform's graphics runtime.

## 1. The smallest game

Create a playable RPG:

```shell
gozen new my-game --template rpg --title "My first game"
gozen check my-game
```

<!-- verify: skip opens a window; atago graphics E2E plays the generated templates -->
```shell
gozen run my-game
```

Enter or Z selects, the arrow keys move, and Escape or X goes back. Talk to the guide, visit the meadow and try a practice battle. [Playtesting](/edit-play-loop/) lists every key.

To start with a different kind of game:

```shell
gozen new my-story --template novel
gozen new my-campaign --template campaign
```

The novel has dialogue, a choice and an ending; the campaign has three regions, two factions, a marching order and turns. The templates' pictures are placeholders to replace.

## 2. Play Go Zen

[Play the larger showcase games in your browser](/play/): RPG battles, novel scenes and strategy maps on the same engine, without cloning anything.

## 3. Start your own game

Your generated directory contains:

| File or directory | What to edit |
| --- | --- |
| `game.yaml` | Title, screen settings, starting location and other game settings |
| `data/world.yaml` | Dialogue, places, story choices or campaign regions |
| `data/rules.yaml` | Characters, classes and basic battle rules |
| `assets/` | Your pictures and sounds |
| `README.md` | Instructions kept with your project |

Change a line of dialogue in `data/world.yaml`, save, and check again:

```shell
gozen check my-game
```

YAML groups values by indentation, so keep the spaces at the start of a line. A failed check names the file, line and field to fix. The [schema](/schema/) and [cookbook](/cookbook/) describe the settings.

<!-- verify: skip opens a window; live reload is covered by the documentation image checks -->
```shell
gozen run my-game --dev --save-dir test-saves
```

Press F5 after an edit to reload. `--save-dir` keeps this test's saves apart from ordinary play; relative paths are relative to the terminal's working directory. Close the window to stop.

Do not change the `id` in `game.yaml` once players have saves. Each new game gets its own ID, even when two folders share a name; `--id` sets one when you create the game.

The built-in font covers Latin text. For Japanese or other scripts, add a font you may redistribute under `assets/` and set `fonts.regular` in `game.yaml`.

## Next

- [Edit-play loop](/edit-play-loop/): notes, recordings, scripts and live checks.
- [Commands](/commands/): creation, validation, simulation and play.
- [Cookbook](/cookbook/): add shops, battles, dialogue and other game features.
- [Schema](/schema/): supported YAML fields and their meaning.

[export](/commands/#desktop) packages the game with a prebuilt runtime, and [export --target web](/commands/#web) makes a static website.
