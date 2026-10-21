---
name: gozen
description: Make or fix an RPG or a visual novel written for Gozen, the Go game framework whose games are game.yaml and data/*.yaml. Use it when you write or change a Gozen game's YAML; it runs gozen check until the game has no errors.
---

# Making a Gozen game

A Gozen game is a directory: `game.yaml`, `data/*.yaml` and `assets/`. A small Go program embeds it and calls `play.Main`. You write YAML. The `gozen` command checks and measures it.

## The loop

1. Read the part of the schema you need: https://nao1215.github.io/gozen/schema/. Do not guess a field.
2. Edit the YAML.
3. Run `gozen check` on the game directory. Fix every error and run it again. Stop only when it prints `ok:` and exits 0.
4. When a fight or a dungeon changed, measure it with `gozen sim` (references/measure.md).

<!-- verify: run fixture=cave -->
```shell
gozen check game
```

<!-- verify: output -->
```text
ok: 2 locations, 0 events, 1 actors, 2 skills, 0 items, 2 enemies, 2 troops, 0 ai sets
```

If `gozen` is not installed: `go install github.com/nao1215/gozen/cmd/gozen@latest`.

## What every game needs

- `game.yaml` with `schema_version: 3`, an `id`, and `start` with a `location` and a `party`.
- A class for each party member, with all eight stats (`mhp mmp atk def mat mdf agi luk`) and `exp_for`.
- The two skills `attack` and `guard`, even in a game without battles.
- Pictures under `assets/` for every `image` and `background` you name. Enemies need an `image`.
- No `fonts` is needed: the built-in Go font has Latin letters only. For Japanese or other scripts, put a font under `assets/` and name it in `fonts.regular`.

A complete small game is in references/minimal.md.

## Reading the errors

`gozen check` prints one line for each problem: file, line, column, the path in the data, the message and a code.

```text
data/world.yaml:30:10: locations.village.exits[0].to: unknown location "feild" [GZN1101]
```

- `gozen help GZN1101` explains a code. All codes: https://nao1215.github.io/gozen/errors/.
- `gozen check --format json game` gives the same problems as JSON (`file`, `line`, `col`, `path`, `code`, `message`, `severity`).
- Warnings do not fail the check. Read them anyway.

YAML traps are in references/yaml.md.

## Recipes

Tested YAML for shops, inns, flags, companions, dungeons, chests, keys, cut-ins and moving portraits: https://nao1215.github.io/gozen/cookbook/. Start from a recipe and change the names.

## Novel games

A novel game (ADV) sets `schema_version: 4` and `ui.message: novel` in game.yaml; its places mostly run events. The player can roll back, skip read lines, save in a scene and change the language without any YAML of yours.

- Give a `choice` an `id` to read the option taken as `chose.<id>`. Keep other facts in flags with `set`. `gozen check` warns of a flag set but never read (GZN0112) and read but never set (GZN0113).
- Stage a scene with `stand` (who stands where, how they come and go), `effect` (shake one portrait, fade through a color), `music` with `fade` and `wait` with `skip: false`.
- For a second language, set `written_in` and `languages` and translate every text in `lang/<code>.yaml`. `gozen check --format json` lists each text not translated yet (GZN0111); fill them until the check is quiet.
- Test every branch in Go with `gozen.Explore` and read `chose.<id>` and the flags at the end of each path.
- Keep who each character is in `profiles` (data/): a title, fields with your own labels, an introduction and how they see the others. Check new lines against it; `--dev` opens it as Characters (F6), and `profile.<id>.unlocked` reads whether its `when` holds.

The recipes are under "Let the player roll back", "Stage portraits and effects" and "Offer the game in two languages" in the cookbook. A complete short one is [examples/novel](https://github.com/nao1215/gozen/tree/main/examples/novel).

## Campaigns

A campaign (SLG, a war of regions) sets `schema_version: 5`, `regions` and `factions` in data, and `campaign` in game.yaml; its place is a map whose points are named like the regions.

- Generals are actors: list them in `campaign.generals` with their troops. Give each order a `label`, and `general`, `target` and `march: true` for an attack.
- `fight: clash_view` shows a march as a clash of units with a sortie screen. Set `clash.units` (1 to 4), `clash.tactics` and `clash.skills`, and `defend` troops and `terrain` on the regions.
- Give each general (`generals.<id>.army`) and enemy (`enemies.<id>.army`) an army kind: `men`, `women` or `monsters`. `clash.armies.<kind>` lists the pictures of one soldier of that kind, drawn facing right (the engine flips the foe side). `clash.layout` is `below` (the default, cards in a row under the armies) or `front`.
- The map can be one picture of the whole land. Give each region an `area` (a polygon in picture pixels) and each point an `at`; `landmark` is optional. A region's roads and borders show only for the region chosen (`region_roads` and `region_borders` in the map take `selected` or `always`). `gozen check` reports areas that overlap or leave the picture.
- Measure with `gozen sim campaign game` (and `--tactic`) before tuning numbers. Test with `gozen.Session`: `Do("- order: {id: attack, general: corin, target: ford}")` and `Query("region.ford.owner")`.

The recipe is "Make a campaign" in the cookbook. A complete one is [examples/campaign](https://github.com/nao1215/gozen/tree/main/examples/campaign).

## Do not

- Do not import `github.com/nao1215/gozen/internal/...`. Use `play`, `gozen` (Session, Query, Do) and the YAML.
- Do not add pictures, music or fonts without the right to use them. Record where each came from.
- Do not edit generated files (`data/schema/*.json` in Gozen itself).
- Do not say the game is done while `gozen check` reports an error.
