# ADR 0030: Campaigns — regions held by factions over turns

Status: Accepted (2026-10-21). Stages 1 (the YAML words, their checks, the expression names and the generated state names), 2 (the rules, the random numbers in a save, the prototype rewritten) 3 (the screen), 4 (`gozen sim campaign`) and 5 (battles) and 7 (the rules in the manner of Kichikuou Rance) are done; the rest is under "Plan" below. Home: `internal/layer2/gamedata` (YAML types, checks and the expression names), `internal/rpg` (the names on a play, `set`), `internal/layer3/campaign` (the rules).

## Users and problem

An author wants a game of conquest in the manner of region-conquest strategy games: a land of regions, sides that hold them, a few orders a turn, and the other sides moving between the player's turns. The prototype `examples/campaign-proto` (now `examples/campaign`) wrote such a game (five regions, three sides, ten turns) with the words Gozen had, and its notes list what that cost:

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

### One game, a campaign and an RPG

A game may hold a campaign beside its dungeons, maps and towns, each its own place, and the party goes between them with `move`. They share one state: the generals are the game's actors (`campaign.generals` names actors; there is no other kind of character), with the same level, stats, equipment and skills in a battle and in a march (the clash reads them as `a.atk`, `a.level` and the rest), and the gold, the items, the flags and the variables are the same everywhere. What a campaign adds to an actor (troops, whether it marched) is in variables with generated names, not in the actor. A test (`TestCampaignWithDungeon`) wins a battle, goes into a dungeon and back, raises the general's level, and checks that the gold, the experience, the flags and the march's strength follow.

### The screen

The campaign's place is shown as a map (`map:`) whose points are named like the regions. On it each region stands a flag in its owner's colour (a ring on a capital) and a plate with its name and garrison; a band over the map tells the turn, the player's faction and the actions; the meters show at the top right; a window tells the region chosen (owner, garrison, values); and the generals who have joined stand in a row with their troops, dim once they have marched. Choosing a region (arrows and OK, a click) lists the orders it can be given and Cancel; an order taken asks for the general. The token is not drawn. The menu's list keeps every order and "End turn". `gozen check` warns of a region without its point and of roads that do not match the borders. A script writes `order ID [GENERAL|-] [TARGET]` and `end_turn`; the trace has `region`, `order` and `end_turn` lines, and a session's `OnEffect` sees the kinds `order` and `end_turn`.

### The rules

- The orders are entries of the campaign's place, before its own actions, each listed while its `when` holds and the turn has the actions it costs, and "End turn" after them. An entry runs the command `order: ID`. The command asks "Who leads?" (the generals that `general: ready|any` allows) and "Where to?" (the regions that `target: border|own|any` and `where` allow) as engine choices ending with Cancel, unless it names them (`order: {id, general, target}`, for scripts and tests). It then binds them (`campaign.general`, `campaign.target`), spends the actions, marches when `march: true`, shows how the march went, and runs the order's `do`. A choice is an ordinary request, so a `Policy`, `Explore` and a save in the middle of an order work as for any choice.
- A march is decided by numbers (`clash`): `power` against `defense`, then the troops left on each side. Its random numbers are the state's, so a seed decides a campaign. A general's march leaves `garrison` of the troops left in the region taken; a faction's leaves all of them.
- `end_turn` lets each other faction that holds a region follow its rules in order: `draft` adds troops to each of its regions, `march` marches once from a region of at least `keep` troops, sending `send` of them, on a border where `where` holds, picked by `pick` and `by`. Then the turn advances, the generals may march again, the actions are worked out, and `on_turn` runs.
- Between commands (where auto events are tried) the campaign ends once: `lose` (or the end of the last turn) runs `lost` and then the new command `game_over`, a defeat without a battle; `win` runs `won` and then `clear`.
- A save made between commands now holds the random generator's bytes too (the envelope's `rand`, as a save in the middle of an event already did), so a restored game draws the numbers the saved one would have. The format version stays 2; an older save without them draws from the session's seed as before.

### Battles

An order with `fight: battle` decides its march by the game's own battle, the one its dungeons and fields fight: the general alone (`fighters`) against the target region's `defend` troop, with the general's level, stats, equipment, skills and HP. A region without `defend` is decided by the numbers. In the battle `self.troops` is a general's troops and a foe's the garrison of the region marched on, so a skill's formula may read them. When it ends, the troops are written back: a win takes the region, the general keeping the share of the troops of the HP left (a fifth at least) and leaving the garrison share there; a loss or a flight leaves the clash's `lost` and the region holds. A lost battle is no game over and asks no retry. The other factions' marches stay numeric. The battle is an ordinary request, so a `Policy`, `Explore` and `gozen sim campaign` answer it as any battle.

### Two ways to count a turn

The author chose the manner of Kichikuou Rance for the sample (2026-10-21): the base (regions, factions, turns, generals and troops, the map) is shared, and what differs is chosen in YAML. The manner of Sengoku Rance stays as the default.

| | `count: actions` (default) | `count: orders` |
| --- | --- | --- |
| A turn has | `actions`, which each order spends by its `cost` | each order `per_turn` times (1 by default) |
| A march | once per general | with `until_taken`, again and again until a region is taken; each general still marches once |

- `calendar: week` makes a turn a week: `campaign.week` and `campaign.year` (52 weeks), and the band says "Year 1, week 3".
- `generals: N` on an order lets N generals march together: after the first the order asks "Who else marches?" (or takes `with:` written); their troops add up in the clash (the leader's might and stats), and what is left is shared in proportion.
- `defeat: {out: N}`: a general whose troops a march brings to 0 is away for N turns (`generals.<id>.out`), then serves again with no troops until drafted. No general is lost for good (the author's rule: no heavy penalties).
- A region's own numbers (economy, defense) are its `values`, and orders change them (`set: {target.defense: "target.defense - 10"}` for sabotage); the clash's `defense` may read `b.defense`.
- `orders.<id>.open` and `orders.<id>.used` tell what the count allows now; the menu lists an order while it is open and its `when` holds.

### A clash shown as units

`fight: clash_view` decides a march by a clash of units that the rules play and the screen plays back. The generals marching stand on the left and the members of the target's `defend` troop on the right, sharing its garrison; at most four a side, the first two in front. For `clash.rounds` rounds (5) every unit still standing acts, the one with the fewest troops first (as in Kichikuou Rance), and strikes a unit of the other side's front, taking `clash.hit` troops; a unit with none is routed (a foe is caught by `clash.capture`), and the one behind steps up. The march takes the region when the other side is routed; when the rounds run out, the region holds (a draw). The troops are written back as by the numbers, and `campaign.clash` tells how it ended.

The screen shows two armies on the terrain's picture (`terrain` of the region, `clash.backgrounds`): each leader's card (the face, a gauge of troops with a red band that shrinks after the loss, the HP) at the front of its ranks of soldiers (one for `clash.per_soldier` troops, `clash.soldiers` the pictures; a soldier picture is drawn facing right, and the engine turns the foe side to face left), a rope of the two sides' troops over the field, the rounds under it. A strike lunges the card and bursts a small puff of dust and sparks where the armies meet (0.3 s), shakes the one struck red, shows the troops lost and the soldiers who went; the soldiers lost fall from the back of the ranks; a routed unit turns gray under a stamp. A table ends it: troops sent and left, the loss and its rate, the foes caught, the region. OK plays faster; A plays on by itself. A session sees the request kind `clash` (it needs no answer); the trace writes `clash` and `clash_hit`.

A side has at most `clash.units` units (1 to 4; 1 makes it a duel, with one in front), which also caps the generals of an order. Each unit fights by a tactic, `clash.tactics.<id>`: among the foes in front (every foe with `all`) for whom `where` holds (`t` the foe, `a` the unit), the one with the least or most `by` (`pick`; random by default), dealing `deal` and taking `take` of the troops (1 by default). Without tactics the engine has `front` (one in front at random), `weakest` and `strongest` (the fewest or most troops; the first deals less, the second more), `leader` (the leader with the least HP, among all) and `guard` (deals and takes 0.6). A general's tactic is the order's (`tactics: {id: tactic}`), else their own (`generals.<id>.tactic`), else the first; a foe fights by the first. A unit's skills (`generals.<id>.skills`, or the enemies in a skill's `foes`) charge each round; one charged (`charge` rounds, 2 by default) is used instead of a strike: it takes `power` troops (twice the hit by default) from the foe the tactic picks, or from every foe in front with `all`, and gives `heal` back. The screen shows its name across the card (with `cutin: true`, first the user's picture across the screen with the name), and a skill's healing puts soldiers back. A faction with `generals` (enemies) marches as a clash too when the campaign has an order with `fight: clash_view`: its strongest generals (by `atk`) up to `clash.units` share the troops sent, against the target's defenders, by the same rules.

With `clash.armies` the soldiers are not one kind for a side but follow the leader: `clash.armies.<kind>` lists poses of one soldier (three or four look best), drawn facing right (the engine turns the foe side to face left), and `generals.<id>.army` and `enemies.<id>.army` name the kind (`men` for a man, `women` for a woman, `monsters` for monsters; an army never mixes them). The engine lines a unit's soldiers up in staggered rows, one soldier for `clash.per_soldier` troops (at most 50): each row behind is a little smaller and higher, every other row half a step aside, the front row is drawn last, and the poses are scattered. The soldiers lost fall from the back rows, as with the ranks. Under the army a gauge of the troops (the side's colour, a red band that shrinks after a loss) and their number. The strike keeps its burst at the front, its loss numbers and its red shake, and the soldiers step forward and back; a routed unit grays under its stamp. `clash.soldiers` is not read with armies. `gozen check` reports a missing picture, a general or an enemy leading a unit without a known kind, and an unknown `layout`.

`clash.layout` places the leaders against their armies: `below` (default) puts the armies above and, along the bottom of the screen, one large card for each unit (a bust of the leader from the standing picture, the name large, the HP a small number; no HP bar), so the named characters show; the only bar is the gauge of troops under each army, and the rope of the troops over the field is not drawn (`clash.rope: true` brings it back). `front` stands a small card in front of each army at the edge facing the foe, with the rope as before. The author chose `below` as the default (a deeper crowd of larger soldiers, rows up to five of ten).

The soldiers tell the troops: each rank holds one soldier for `clash.per_soldier` troops (the last one fainter for a part of one, at most 24), the troops written over it; the card is the leader's face, name and HP; `clash.rope: false` leaves out the rope.

The player's march with more than one general or a clash shown as units, when the order names no general, asks for the target and then shows the sortie screen (Runner.Sortie, set by the play screen; a session and a script keep the choices): the generals who have joined on the left (face, troops, might and attack; those who marched or are away grayed), the slots of the front and the back on the right, each with the general's tactic, the troops marching against the garrison at the bottom, March and Cancel. OK puts a general in or takes them out, left and right change the tactic, a click on a slot and another swaps them; the first slot leads. `gozen sim campaign --tactic` marches every general with one tactic, to compare them.

A region's `landmark` is a cut-out picture the map stands on its point (`landmark_scale` of the map's width, 0.12 by default), the owner's flag beside it: the map's picture stays terrain only, and the cities, forests and forts are their own pictures, so a region can be added without painting the map again. While a region was fought over this turn or the last (`region.<id>.fought`, the turn of its last march), its landmark darkens and smokes. The map settles each landmark onto its terrain (`landmark_ground`, 0 to 1, 1 by default, 0 draws the picture as it is): a soft shadow at its foot leaning to the lower right, away from the light at the upper left, a faded bottom edge, and colours eased toward the ground around its point. A region's `area` is its shape on the map's picture: the map fills it in the owner's colour, draws its border (lit when chosen), and a click inside it chooses the region; the flag is left out. `gozen check` reports an area of fewer than three points, out of the picture, or overlapping another (a shared border is fine), and warns of one crossing itself.

`on_win`, `on_lose` and `on_draw` on a march run after it by how it went (any `fight`), before `do`: a general's word, with the language, the backlog, the read lines and the trace of any line.

### Measuring

`gozen sim campaign` plays a campaign many times with the internal rules (as `sim battle` and `sim dive` do), campaign *i* from the seed `--seed` + *i*, and prints, or writes as JSON, the win rate, the turns the campaigns ended in, the mean of the regions each faction held at the start of each turn, the factions that fell and their mean turn, the orders given and how the marches went ([ADR 0021](0021-query-and-commands.md) decision 4: the numbers come from the CLI's JSON). The player's strategy is `greedy` (march when the clash's odds, its formulas with `rand()` at its middle, are 1.15 or better; else an order with a general, the strongest first; else another order), `random`, or `ai`: the player's faction follows its own `ai` rules, which a game may therefore write for its player's faction too (they are used only here).

### No new public API

`api/public.txt` does not change. `api/schema.txt` gains `Game.Campaign`, `DB.Regions`, `DB.Factions` and the commands `Command.Order`, `Command.EndTurn` and `Command.GameOver`. It also gains the fields `Clash.Armies`, `Clash.Layout`, `General.Army` and `Enemy.Army`. The new types have no alias in `schema`; the helpers that read a campaign's state are functions of `internal/layer2/gamedata`, not methods of `DB`.

## Plan

| Stage | Content | Status |
| --- | --- | --- |
| 1 | The words, `schema_version: 5`, the checks, the names, the generated state names, `set` | Done |
| 2 | The rules (`internal/layer3/campaign`): the commands `order`, `end_turn` and `game_over`, the actions of a turn, the numeric clash, the factions' rules, winning and losing, the random numbers in a save; the prototype rewritten | Done |
| 3 | The screen: the flags in the owners' colours, the band, the meters, the region's window, the generals, orders from a region, the script and the trace | Done |
| 4 | `gozen sim campaign` (`-n`, `--seed`, `--as greedy|random|ai`, `--format json`): the win rate, the turns, the regions by turn, the factions' falls, the orders; the CLI's E2E | Done |
| 5 | The bridge to battles: `fight: battle`, `defend`, `self.troops`, the troops written back | Done |
| 7 | Rules in the manner of Kichikuou Rance, chosen in YAML: a calendar of weeks, orders counted per kind (`count: orders`, `per_turn`), a march open until a region is taken (`until_taken`), generals marching together (`generals`), a beaten general away for some turns (`defeat.out`) | Done |
| 8 | The clash screen (`fight: clash_view`), `on_win`, `on_lose`, `on_draw` | Done |
| 8b | The sortie screen, `clash.units`, tactics and skills, `area` | Done |
| 6 | The remaining checks, the sample, the documents | |

## Alternatives

- Keep the prototype's way (variables and dispatch events). It works for five regions, but its cost grows with every region and nothing checks it.
- Go code in the game. There is no way from YAML into Go (declared verbs are skipped), and `check`, `Explore` and the panel would not see the rules ([ADR 0021](0021-query-and-commands.md)).
- A campaign as a way of showing a place (`war:` next to `map:`). The regions and the factions are data of the game, not of one place, and the author decided that a new kind of game may have its own words.
