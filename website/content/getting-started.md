---
title: Getting started
description: Create and play your first game with YAML and assets. No Go source or compiler is needed.
weight: 20
---

[Install the Gozen binary](/install/) for your operating system. You need a text
editor and a terminal. Creating and playing a game does not require Go, Git or a
programming language. The desktop window needs your platform's graphics runtime.

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

Press Enter or Z to select an option, use the arrow keys to move through choices,
and press Escape or X to go back. Talk to the guide, visit the meadow and try a
practice battle. The complete keys are in [Playtesting](/edit-play-loop/).

To start with a different kind of game:

```shell
gozen new my-story --template novel
gozen new my-campaign --template campaign
```

A novel starts with dialogue, a choice and an ending. A campaign starts with three
regions, two factions, a marching order and turn progression. All templates use
original placeholder material that you can replace.

## 2. Play Go Zen

[Play the larger showcase games in your browser](/play/). They show RPG battles,
novel scenes and strategy maps using the same engine. You do not need to clone or
compile their source to try them.

## 3. Start your own game

Your generated directory contains:

| File or directory | What to edit |
| --- | --- |
| `game.yaml` | Title, screen settings, starting location and other game settings |
| `data/world.yaml` | Dialogue, places, story choices or campaign regions |
| `data/rules.yaml` | Characters, classes and basic battle rules |
| `assets/` | Your pictures and sounds |
| `README.md` | Instructions kept with your project |

Open `data/world.yaml` in your text editor. Change a line of dialogue, save the
file, then check it again:

```shell
gozen check my-game
```

YAML uses indentation to group values. Keep the spaces at the beginning of a line
when changing its text. If a check fails, Gozen reports the file, line and field to
fix. The [schema](/schema/) and [cookbook](/cookbook/) describe supported settings.

<!-- verify: skip opens a window; live reload is covered by the documentation image checks -->
```shell
gozen run my-game --dev --save-dir test-saves
```

Press F5 after an edit to reload the definitions. The explicit save directory
keeps this manual test separate from ordinary play. Relative output paths are
relative to the terminal's working directory. Close the window to stop.

Keep the `id` in `game.yaml` unchanged once players have saves. Each new template
gets its own ID, even when two folders have the same name. If you need a specific
identity, choose it when creating the game with `--id`.

The built-in font supports Latin text. For Japanese or other scripts, add an
appropriately licensed font under `assets/` and set `fonts.regular` in `game.yaml`.

## Next

- [Edit–play loop](/edit-play-loop/): notes, recordings, scripts and live checks.
- [Commands](/commands/): creation, validation, simulation and play.
- [Cookbook](/cookbook/): add shops, battles, dialogue and other game features.
- [Schema](/schema/): supported YAML fields and their meaning.

Use [export](/commands/#desktop) to distribute the game with a
matching prebuilt runtime, or [export --target web](/commands/#web) for a static website. The existing
[Go API migration](/go-api/) explains how existing hosts move to the CLI. No Go code is needed for
these steps.
