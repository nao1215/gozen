---
name: gozen
description: Make or fix an RPG, a visual novel or a strategy game (a campaign) written for Gozen, the no-code game toolkit whose games are game.yaml and data/*.yaml. Use it when you write or change a Gozen game's YAML; it runs gozen check until the game has no errors.
---

# Making a Gozen game

A Gozen game is a directory: `game.yaml`, `data/*.yaml` and `assets/`. Run it directly with `gozen run <game-dir>`, or embed it in a Go program calling `play.Main`. You write YAML. The `gozen` command checks, measures and runs it.

Start a new game with `gozen new <game-dir> --template rpg|novel|campaign`. The generated game contains YAML and assets, not Go source. Keep its generated ID stable for save compatibility; use `--id` only when choosing the identity deliberately. Existing destinations are never overwritten.

## The loop

1. Read the part of the schema you need: https://nao1215.github.io/gozen/schema/. Do not guess a field.
2. Edit the YAML.
3. Run `gozen check` on the game directory. Fix every error and run it again. Stop only when it prints `ok:` and exits 0.
4. When a fight or a dungeon changed, measure it with `gozen sim` (references/measure.md).
5. Play the game with `gozen run <game-dir> --dev`; it uses the same standard runner as `play.Main`, including live reload, scripts, traces and screenshots. Use `--save-dir` to isolate manual test saves. A display is required (Xvfb for unattended Linux); inspect `gozen run --help` before choosing options.
6. Capture promotional or review images with `gozen capture <game-dir> --config captures.yaml --out promo`. Jobs select a location, event or gallery line; `gif` records an animation. Saves are isolated automatically. Check the per-job results and exit status; partial failure exits 1. Use `--format json` or `jsonl` for machine-readable reports. See [capture](https://nao1215.github.io/gozen/commands/#capture) for limits and examples.
7. For a novel or event-driven game, run `gozen sim routes <game-dir>` to explore choices and check reachable endings and offered options; use `--format json --fail-unreachable` in CI.

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

For battle forms, HP pictures and enemy motion use schema version 7. `hp_triggers` transforms a foe below an HP threshold; a self-scoped skill's `merge` combines eligible foes. Enemy `images` lists picture files and actor `images` lists face names at descending HP thresholds. Set `puppet` on an enemy or actor for moving portraits; its YAML can use `soft_sway` with `rect` or `ellipse` and `follow: body` for gentle motion. Check it with `gozen art puppet` and see the [battle recipes](https://nao1215.github.io/gozen/cookbook/#transform-an-enemy).

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

A first-person dungeon can take a ready look: `gozen theme list` shows the themes (cave, ice, ruins, castle, crystal, temple, sewer, forest, tomb, keep, dream, mine, vine), `gozen theme add game --name cave` copies one into the game's assets, and `theme: cave` on `dungeon:` or a look fills the wall, door, floor and ceiling. A value written beside it wins. `gozen check` reports a theme named but not added (GZN1111).

## Continuous stacked spaces

For streets, houses with upper floors, bridges and atriums, set `schema_version: 8` and use `dungeon.space`. Supply 1–8 aligned rectangular `levels` (3–64 cells per side); `_` is open air. Straight `stairs` have a one-based lower `level`, zero-based `x` and `y`, a `facing`, and `length` 3–8. Keep headroom empty and both endpoints unconditional. `pos.level` is zero-based; save/restore, exploration and tile records retain the level.

Use `wall`, `floor_texture`, `ceiling_texture` and optional `space.sky` for materials. A sky panorama wraps horizontally from zenith at the top to horizon at the bottom. Do not combine a space with `map`, `generate`, roamers, themes, door/tile pictures, conditional looks, slide or spin effects. Ordinary dungeon recipes still work without `space`. Read the [complete example and limits](https://github.com/nao1215/gozen/blob/main/docs/examples/stacked-spaces.md), run `gozen check examples/space/game` in the Gozen checkout, and verify movement in both directions in a rendered run.

## Novel games

A novel game (ADV) sets `schema_version: 4` and `ui.message: novel` in game.yaml; its places mostly run events. The player can roll back, skip read lines, save in a scene and change the language without any YAML of yours.

- Give a `choice` an `id` to read the option taken as `chose.<id>`. Keep other facts in flags with `set`. `gozen check` warns of a flag set but never read (GZN0112) and read but never set (GZN0113).
- Stage a scene with `stand` (who stands where, how they come and go), `effect` (shake one portrait, fade through a color), `music` with `fade` and `wait` with `skip: false`.
- For a second language, set `written_in` and `languages` and translate every text in `lang/<code>.yaml`. `gozen check --format json` lists each text not translated yet (GZN0111); fill them until the check is quiet.
- Test every branch in Go with `gozen.Explore` and read `chose.<id>` and the flags at the end of each path.
- Keep who each character is in `profiles` (data/): a title, fields with your own labels, an introduction and how they see the others. Check new lines against it; `--dev` opens it as Characters (F6), and `profile.<id>.unlocked` reads whether its `when` holds.
- Press Tab in Characters for the relationship chart. Set optional `affiliation` on profiles and `kind` (`like`, `love`, `rival`, `enemy`, `family`, `friend`, `other`) on relations. The author decides what `love` means. Run `gozen art faces` for centred icons and `gozen check` for unknown kinds or targets.
- Add `enemies.<id>.lore` for creatures that appear in encounters and set `ui.bestiary: true` to offer the monster book. Encounters reveal name, picture and lore; victories reveal stats, resistances, skills and drops. `bestiary.<id>.seen` and `.defeated` are query expressions; F4 opens every page under `--dev`.

The recipes are under "Let the player roll back", "Stage portraits and effects" and "Offer the game in two languages" in the cookbook. A complete short one is [examples/novel](https://github.com/nao1215/gozen/tree/main/examples/novel).

## Campaigns

A campaign (SLG, a war of regions) sets `schema_version: 5`, `regions` and `factions` in data, and `campaign` in game.yaml; its place is a map whose points are named like the regions.

- Generals are actors: list them in `campaign.generals` with their troops. Give each order a `label`, and `general`, `target` and `march: true` for an attack.
- `fight: clash_view` shows a march as a clash of units with a sortie screen. Set `clash.units` (1 to 4), `clash.tactics` and `clash.skills`, and `defend` troops and `terrain` on the regions.
- Give each general (`generals.<id>.army`) and enemy (`enemies.<id>.army`) an army kind: `men`, `women` or `monsters`. `clash.armies.<kind>` lists the pictures of one soldier of that kind, drawn facing right (the engine flips the foe side). `clash.layout` is `below` (the default, cards in a row under the armies) or `front`.
- The map can be one picture of the whole land. Give each region an `area` (a polygon in picture pixels) and each point an `at`; `landmark` is optional. A region's roads and borders show only for the region chosen (`region_roads` and `region_borders` in the map take `selected` or `always`). `gozen check` reports areas that overlap or leave the picture.
- With armies, `clash.max_soldiers` (4 by default) sets how many soldiers one unit shows, and `clash.rope: false` leaves out the rope of the two sides' troops. `regions.<id>.landmark` (a cut-out picture of a city or fort, optional) stands on the map at the region's point.
- Measure with `gozen sim campaign game` (and `--tactic`) before tuning numbers. Use `--compare strategies` or `--compare tactics` to run every variant on the same seeds; `--format json` includes win rate, mean ending turn, mean player regions held and region ownership details. Play orders with `gozen run game --dev`; use `--script` and an isolated `--save-dir` for repeatable checks.

The recipe is "Make a campaign" in the cookbook. A complete one is [examples/campaign](https://github.com/nao1215/gozen/tree/main/examples/campaign).

## Monster collection

Use schema version 6. Define `monster_families`, then give each non-boss enemy a `family`, `rank` and `monster_class`; use `recruit: {chance, level}` for species that may ask to join after a won battle. The class supplies growth, experience and skills learned by level. Put `breeding: {min_level, skill_slots, pairs, families}` in data. Set `enemies.<id>.sex` to `any` (default), `male` or `female`; optional `sex_ratio` is the male probability for `any`. Captured and born instances keep their randomly assigned sex. Breeding requires one male and one female, so check that every pair and family rule can produce both roles. Specific species pairs win over family rules. The child starts at level one, inherits selected skills and a plus value; the parents leave. Add `open: ranch` to a safe town action for reserve swaps, release and breeding. Check with `gozen check game` and `gozen sim breed game --format json`; review every unreachable species and tune the party with `gozen sim battle` and `gozen sim dive`. `gozen book game` prints recruit and breeding details. Expressions read `reserve.size`, `monster.<instance>.family`, `.plus`, `.sex` and `bestiary.<species>.caught`; mutations use `reserve`, `release` and `breed` commands. The [collection recipe](https://nao1215.github.io/gozen/cookbook/#collect-raise-and-breed-monsters) has a compact YAML example.

For a player facing family tree, set `ui.bestiary: true` and give capturable leaves encounter troops in locations. The screen defaults to the shortest buildable recipe and shows up to seven pictures (species, parents and grandparents). Arrows choose a picture; Z makes it the top of a new two-generation view, X returns along the breadcrumb history, Tab switches the top species' recipe, and Q opens a chosen known species page. Capture cards show locations, family cards show families and an example pair, deeper parents have a `more` mark, and unseen species show `?` unless `--dev` is on. `gozen book game --format json` exports the nested tree for author tools. Place `open: ranch` at a safe location with a background if the breeding scene should use that location's picture.

The birth screen keeps the ranch keeper centered until they fade in place. Normal, Rare and Legendary births use progressively longer pauses before the child appears; white, gold, and gold plus blue-grey rings convey the tier without rarity text. The result shows `<Name> <sex> was born!` and one line in the message window; X opens the details.

## Browser builds

The sample browser builds keep game files outside WebAssembly. Their generated `game/files.json` manifest maps game paths to served files; the browser fetches each file when it is opened, so pictures used later do not delay the first screen. `make website` writes a precompressed `.wasm.gz` file for each sample, and the page decompresses it with the browser's `DecompressionStream`. Browser saves remain in `localStorage`.

## Ship a demo

`gozen trial game --out demo` prunes definitions and assets using the range in `trial.yaml`. Analysis is conservative: branches it cannot prove unreachable may remain, and retained files can contain later content. Use `--list FILE` and inspect the resulting definitions and assets before shipping.

## Do not

- Do not import `github.com/nao1215/gozen/internal/...`. Use `play`, `gozen` (Session, Query, Do) and the YAML.
- Do not add pictures, music or fonts without the right to use them. Record where each came from.
- Do not edit generated files (`data/schema/*.json` in Gozen itself).
- Do not say the game is done while `gozen check` reports an error.

For desktop delivery, use `gozen export GAME --out OUT --runtime RUNTIME
--target OS-ARCH` with a matching release runtime. Keep the output directory
new and outside both inputs. If the game has an asset policy, first use
`release` and export its approved output. Run the packaged player after
moving the directory, with isolated saves; check save/reload and the actual
native target. Cross-compilation alone is not runtime verification. For browser delivery, use `gozen export GAME --target web --out OUT --runtime RUNTIME` with
the matching js-wasm runtime bundle. Serve the output over HTTP(S), including
under a subdirectory, and verify choices plus save/reload in a real browser.
The exporter copies assets unchanged and the player prefetches the manifest;
saves remain origin-local. See the site's `export` command documentation.
