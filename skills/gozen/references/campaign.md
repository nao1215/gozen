# Campaigns

A campaign (SLG, a war of regions) sets `schema_version: 5`, `regions` and `factions` in data, and `campaign` in game.yaml; its place is a map whose points are named like the regions (see atlas.md).

- Generals are actors: list them in `campaign.generals` with their troops. Give each order a `label`, and `general`, `target` and `march: true` for an attack. `count: orders` with `per_turn` lets each order be given once a turn.
- `fight: clash_view` shows a march as a clash of units with a sortie screen. Set `clash.units` (1 to 4), `clash.tactics` and `clash.skills`, and `defend` troops and `terrain` on the regions. Leave `fight` out to decide by the numbers of `clash`, or use `fight: battle` for a battle of the game.
- Give each general (`generals.<id>.army`) and enemy (`enemies.<id>.army`) an army kind: `men`, `women` or `monsters`. `clash.armies.<kind>` lists the pictures of one soldier, drawn facing right (the engine flips the foe side). `clash.layout` is `below` (default) or `front`. `clash.max_soldiers` (4) sets how many soldiers one unit shows; `clash.rope: false` leaves out the rope between the sides.
- The map can be one picture of the whole land. Give each region an `area` (a polygon in picture pixels) and each point an `at`; `landmark` (a cut-out city or fort) is optional. A region's roads and borders show only for the region chosen unless `region_roads` or `region_borders` on the map is `always`. `gozen check` reports areas that overlap or leave the picture.
- Factions move by their `ai` rules when the turn ends (draft, march with `where`, `pick`, `by`). Expressions read `region.<id>.owner`, `faction.<id>.regions`, `campaign.turn`, `generals.<id>.troops`; `set` changes them.

## Name the region values

Regions carry free numbers in `values` (`values: {wealth: 5, food: 3}`, read and set as `region.<id>.wealth`). `campaign.labels` gives each value a display name in the region window; a value without a label shows its own name.

<!-- verify: parse fragment of the campaign section of game.yaml -->
```yaml
campaign:
  labels: {wealth: Treasury, food: Granary}
```

## Arrange the campaign screen

`campaign.heading` replaces the turn's words in the band over the map with a text with `{expressions}`. The map's `safe_area: [top, right, bottom, left]` (fractions of the screen) keeps the picture out from under the band, the meters and the generals; zoomed in with a `camera`, the picture still covers the whole screen. The map's `talk: dim` (or `hide`) darkens it under the conversations between turns. An order's `group` (and a location action's `group`) lists it under a heading in the place's list; the entries of one group stand together. `campaign.pin_end_turn: true` keeps the end of the turn in the last row, below the rows that scroll. `campaign.order_view: icons` offers a region's orders as a row of icons (each order's `icon`, its label in a square without one) instead of a list, and `campaign.end_turn_button` stands a button that ends the turn on the map (a click or E).

<!-- verify: parse fragment of the campaign section of game.yaml -->
```yaml
campaign:
  heading: "Year {var.year}, month {var.month}"
  pin_end_turn: true
  orders:
    raid: {label: Raid, group: Abroad, target: border}
    farm: {label: Farm, group: Castle}
```

## Measure before tuning

- `gozen sim campaign GAME` plays 200 campaigns (`-n`, `--seed`) with `--as greedy|random|ai` and prints win rate, turns, regions held turn by turn and orders given. `--tactic ID` marches every general with one tactic.
- `--compare strategies` or `--compare tactics` runs every variant on the same seeds. `--format json` includes win rate, mean ending turn, mean player regions held and per-region ownership details.
- Play orders with `gozen run GAME --dev`; use `--script` and an isolated `--save-dir` for repeatable checks.

The recipe is "Make a campaign" in the cookbook, and "Mix a campaign with a dungeon" shows generals who are also party members. A complete one is [examples/campaign](https://github.com/nao1215/gozen/tree/main/examples/campaign).
