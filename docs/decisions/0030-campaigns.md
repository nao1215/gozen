# ADR 0030: Campaigns — regions held by factions over turns

Status: Accepted (2026-10-21). Stage 1 (the YAML words, their checks, the expression names and the generated state names) is done. Stage 2 (the rules) is under "Plan" below. Home: `internal/layer2/gamedata` (YAML types, checks and the expression names), `internal/rpg` (the names on a play, `set`), `internal/layer3/campaign` (the rules).

## Users and problem

An author wants a game of conquest in the manner of region-conquest strategy games: a land of regions, sides that hold them, a few orders a turn, and the other sides moving between the player's turns. The prototype `examples/campaign-proto` wrote such a game (five regions, three sides, ten turns) with the words Gozen had, and its notes list what that cost:

- There are no regions, sides or turns, so every number is a declared variable (61 names for five regions), and the borders are written three times (the map's roads, the conditions of the orders, the lines of the other sides' rules). `gozen check` compares none of them.
- `set` names its target as written, so "the region chosen" is a dispatch event with one `if` per region, for reading too under `state.strict` (52 lines of 199).
- The other sides' rules are one `if` per border in a fixed order; "the weakest neighbour" cannot be written.
- There is no defeat without a battle, and a save between commands loses the random numbers, so a restored campaign ends differently.

The users are authors who write YAML and check it with `gozen check`, `gozen.Session` and `Explore`; none writes Go ([ADR 0021](0021-query-and-commands.md) principles 0 and 1). The author decided (2026-10-21) that a new kind of game may get new YAML words; it need not be squeezed into a way of showing a place.

## Decision

### New words at `schema_version: 5`

`data/` gains two sections and `game.yaml` one:

```yaml
# data/war.yaml
factions:
  crown: {name: Crown, color: "#3060c0", capital: vale}
  raven:
    name: Raven
    capital: well
    ai:
      - {draft: "170"}
      - {march: true, keep: 1000, where: "s.troops * 0.6 > t.troops * 1.25", pick: min, by: t.troops}
regions:
  vale: {name: Ashvale, next: [cross, ford], owner: crown, troops: 600, values: {wealth: 40}}
  ford: {name: Brookford, next: [cross, vale], troops: 350, values: {wealth: 30}}
# game.yaml
campaign:
  location: war              # the place whose menu lists the orders and "End turn"
  player: crown
  turns: 10
  actions: "2 + floor(faction.crown.regions / 2)"
  generals: {maren: {troops: 500, might: 3}}
  orders:
    attack: {label: Attack, general: ready, target: border, march: true}
    develop: {label: Develop, target: own, do: [{set: {target.wealth: "target.wealth + 15"}}]}
```

- A region has a name, its neighbours (`next`, written on both sides unless `one_way`), its first owner (a faction, or none), its garrison and its own numbers (`values`).
- A faction has a name, a colour, a capital and, unless it is the player's, its rules (`ai`).
- `campaign` names the place of the orders, the player's faction, the last turn, the actions of a turn, the generals (actors with their first troops and might), the orders, how a march is decided (`clash`), the conditions that win and lose (by default: hold every region; lose the capital) and the events run then.

The words are their own sections, not a way of showing a place ([ADR 0022](0022-maps-and-painted-scenes.md) keeps maps and scenes as looks). A campaign's place may still be shown as a map.

### The state is variables with generated names

As a dungeon keeps `var.floor`, a campaign keeps its state in `Vars` under generated names: `campaign_turn`, `campaign_actions`, `campaign_target`, `campaign_general`, `campaign_result`, `region_<id>_owner`, `region_<id>_troops`, `region_<id>_<value>`, `general_<id>_troops` and `general_<id>_acted`. An unset name reads the definition, so a new game has nothing written. Saves, rolling back, the F8 panel and the index of names see the campaign without a new field in the save, and `state.strict` knows the names.

### Read with names, change with `set`

The names below are known wherever field names are (conditions, labels, events), at version 5:

| Name | Value |
| --- | --- |
| `region.<id>` | `id`, `name`, `owner`, `troops`, `next`, `capital`, `border` (not the player's and next to the player's), and each value |
| `faction.<id>` | `id`, `name`, `color`, `capital`, `regions` (how many it holds), `troops`, `alive`, `player`, `sum_<value>` |
| `campaign` | `turn`, `turns`, `actions`, `actions_left`, `player`, `target`, `general`, `result` |
| `generals.<id>` | `id`, `name`, `troops`, `might`, `acted`, `joined`, `ready` |
| `target`, `general` | the region and the general bound by the running order (nil outside one) |
| `next_to(region, faction)` | the region borders a region the faction holds |

`set` takes `region.<id>.owner`, `region.<id>.troops`, `region.<id>.<value>`, the same through `target.`, `generals.<id>.troops` and `general.troops`. Binding the order's region and general to `target` and `general` is what removes the dispatch events: one `set` writes the region chosen.

### Checks

`gozen check` reports with file and line: a neighbour that does not exist, is the region itself, is written twice, or does not name the region back; an owner or a capital that is not a faction or a region; a capital not held by its faction at the start; the player's faction with `ai` rules; a rule that both drafts and marches, or has march words without `march`; `pick: min|max` without `by`; an unknown `player`, `location`, event or general; an order's unknown `general` or `target` word, a `where` without a target, a march without a general and a target; a value named like a region's field; an unknown region, faction, general, field or `sum_` value in an expression; a `set` of an unknown field or of an owner that is not a faction; `actions` reading names other than the campaign's; and the words at a version below 5. Warnings: regions in more than one part, a faction bordering no other side, a `win` or `lose` that holds at the start, and a first turn without actions.

### No new public API

`api/public.txt` does not change. `api/schema.txt` gains `Game.Campaign`, `DB.Regions` and `DB.Factions`. The new types have no alias in `schema`; the helpers that read a campaign's state are functions of `internal/layer2/gamedata`, not methods of `DB`.

## Plan

| Stage | Content | Status |
| --- | --- | --- |
| 1 | The words, `schema_version: 5`, the checks, the names, the generated state names, `set` | Done |
| 2 | The rules (`internal/layer3/campaign`): the commands `order`, `end_turn` and `game_over`, the actions of a turn, the numeric clash, the factions' rules, winning and losing, the random numbers in a save; the prototype rewritten | |
| Later | The screen (the factions' colours on the map, a band with the turn), `gozen sim campaign`, the bridge to battles | |

## Alternatives

- Keep the prototype's way (variables and dispatch events). It works for five regions, but its cost grows with every region and nothing checks it.
- Go code in the game. There is no way from YAML into Go (declared verbs are skipped), and `check`, `Explore` and the panel would not see the rules ([ADR 0021](0021-query-and-commands.md)).
- A campaign as a way of showing a place (`war:` next to `map:`). The regions and the factions are data of the game, not of one place, and the author decided that a new kind of game may have its own words.
