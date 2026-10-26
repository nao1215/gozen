---
name: gozen
description: Make or fix an RPG, a visual novel or a strategy game (a campaign) written for Gozen, the no-code game toolkit whose games are game.yaml and data/*.yaml. Use it when you write or change a Gozen game's YAML; it runs gozen check until the game has no errors.
---

# Making a Gozen game

A Gozen game is a directory: `game.yaml`, `data/*.yaml` and `assets/`. Run it directly with `gozen run <game-dir>`. You write YAML. The `gozen` command checks, measures and runs it.

Start a new game with `gozen new <game-dir> --template rpg|novel|campaign`. The generated game contains YAML and assets, not Go source. Keep its generated ID stable for save compatibility; use `--id` only when choosing the identity deliberately. Existing destinations are never overwritten.

## The loop

1. Read the part of the schema you need: https://nao1215.github.io/gozen/schema/. Do not guess a field.
2. Edit the YAML.
3. Run `gozen check` on the game directory. Fix every error and run it again. Stop only when it prints `ok:` and exits 0.
4. When a fight or a dungeon changed, measure it with `gozen sim` (references/measure.md).
5. Play the game with `gozen run <game-dir> --dev`; it includes live reload, scripts, traces and screenshots. Use `--save-dir` to isolate manual test saves. A display is required (Xvfb for unattended Linux); inspect `gozen run --help` before choosing options.
6. Capture promotional or review images with `gozen capture` (references/release.md); a partial failure exits 1.
7. For a novel or event-driven game, run `gozen sim routes <game-dir> --format json --fail-unreachable` to check reachable endings and offered options (references/novel.md).

Use `gozen help --format json` to inspect registered commands and options without loading a game. `gozen book GAME --format json` describes the whole game; add `--section lineage` to select only monster family trees.

For every `sim` command, `--format json` returns a version-1 result with `command`, `status` and `data`; JSONL emits items and a terminal summary. Keep stdout and stderr separate and check the exit status. To inspect a recorded play, use `gozen trace run.jsonl --format json`, or `gozen trace - --format jsonl` for standard input. Repeat `--only KIND` to select kinds; `--check` rejects these display filters and preserves diagnostic codes in its issue records.

<!-- verify: run fixture=cave -->
```shell
gozen check game
```

<!-- verify: output -->
```text
ok: 2 locations, 0 events, 1 actors, 2 skills, 0 items, 2 enemies, 2 troops, 0 ai sets
```

If `gozen` is not installed: `go install github.com/nao1215/gozen/cmd/gozen@latest`.
In GitHub Actions, `nao1215/setup-gozen` installs the version in the game's `go.mod`; or run `go run github.com/nao1215/gozen/cmd/gozen check game`.

## What every game needs

- `game.yaml` with `schema_version: 3` (4 for a novel, 5 for a campaign, 6 for monster collection, 7 for battle forms, 8 for stacked spaces), an `id`, and `start` with a `location` and a `party`.
- A class for each party member, with all eight stats (`mhp mmp atk def mat mdf agi luk`) and `exp_for`.
- The two skills `attack` and `guard`, even in a game without battles.
- Pictures under `assets/` for every `image` and `background` you name. Enemies need an `image`.
- No `fonts` is needed: the built-in Go font has Latin letters only. For Japanese or other scripts, put a font under `assets/` and name it in `fonts.regular`.

A complete small game is in references/minimal.md.

Battle forms, HP pictures and moving enemies need schema version 7 (references/battle.md, references/puppet.md).

## Reading the errors

`gozen check` prints one line for each problem: file, line, column, the path in the data, the message and a code.

```text
data/world.yaml:30:10: locations.village.exits[0].to: unknown location "feild" [GZN1101]
```

- `gozen help GZN1101` explains a code. All codes: https://nao1215.github.io/gozen/errors/.
- `gozen check --format json game` returns a version-1 result with the diagnostic array in `data` (`file`, `line`, `col`, `path`, `code`, `message`, `severity`). Validation failure exits 1 with `status: failed`; do not expect a duplicate stderr error.
- Warnings do not fail the check. Read them anyway.

YAML traps are in references/yaml.md.

## Recipes

Tested YAML for shops, inns, flags, companions, dungeons, chests, keys, cut-ins and moving portraits: https://nao1215.github.io/gozen/cookbook/. Start from a recipe and change the names.

A first-person dungeon can take a ready look: `gozen theme list` shows the themes (cave, ice, ruins, castle, crystal, temple, sewer, forest, tomb, keep, dream, mine, vine), `gozen theme add game --name cave` copies one into the game's assets, and `theme: cave` on `dungeon:` or a look fills the wall, door, floor and ceiling. A value written beside it wins. `gozen check` reports a theme named but not added (GZN1111).

## Read a reference when the game needs it

- references/minimal.md: a complete two-file game to copy from.
- references/yaml.md: YAML traps and which feature needs which `schema_version`.
- references/measure.md: balance and route numbers with `gozen sim` (battle, dive, campaign, routes, breed).
- references/novel.md: a visual novel: `stand`, `choice` ids, profiles, `ui.stage_framing` for tall portraits, picture limits, `text:` wording, languages, `sim routes`.
- references/atlas.md: a map location: `hide: closed`, `selection_panel` advice, `activation: instant`, `camera` pan and zoom.
- references/campaign.md: a war of regions: orders, clash, armies, `labels`, `sim campaign`.
- references/battle.md: `battle.before`/`after`, `recover: false`, `last_battle` casualties, battle forms, recruit `admission`, `capture`, `battle_ai_history`.
- references/puppet.md: moving portraits: blink, mouth, sway, `soft_sway`, `idle`, `hit: soft_only`.
- references/monsters.md: monster collection, ranch, breeding and the monster book.
- references/space.md: stacked spaces (streets, upper floors, continuous walking).
- references/release.md: `gozen capture`, trial builds, desktop and browser export.

## Do not

- Do not import Gozen implementation packages. Author games with YAML and assets, and use the CLI to check, play, simulate, capture and export.
- Do not add pictures, music or fonts without the right to use them. Record where each came from.
- Do not edit generated files (`data/schema/*.json` in Gozen itself).
- Do not say the game is done while `gozen check` reports an error.

For shipping (capture, trial, desktop and browser export) read references/release.md.
