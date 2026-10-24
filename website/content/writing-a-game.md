---
title: Writing a game
description: How a Gozen game directory is laid out, and how places, events, commands, battles, dungeons and expressions fit together in YAML.
weight: 30
toc: true
---

A Gozen game is data. The Go program only embeds the directory and starts the runner; everything a player sees comes from YAML and the pictures beside it. This page is the map. The [Schema](/schema/) has every field, and the [Cookbook](/cookbook/) has short recipes for common tasks, such as a shop, a boss fight or a locked door.

## The game directory

```text
game/
├── game.yaml        # the game: title, start, screen, fonts, battle rules, text
├── data/            # definitions; every .yaml file here is read and merged
│   ├── world.yaml
│   └── ...
└── assets/          # pictures, fonts, music and sounds, referred to by file name
```

`game.yaml` is a [Game](/schema/#game). Each file in `data/` is a [DataFile](/schema/#datafile) and may hold any of its sections, so split by area (`town.yaml`, `cave.yaml`) or by kind (`enemies.yaml`, `skills.yaml`) as you like. Every ID lives in one namespace per section, and a duplicate is an error.

Start every `game.yaml` with `schema_version: 2`. From version 1 on, a number you leave out and a `0` you write are different, and the game `id` (used for the save directory) is required. Version 2 adds the commands and `set` targets marked "version 2" below; a version 1 game reads the same as before.

## What goes where

| Section | Holds | Example |
|:--|:--|:--|
| `classes` | stats by level, skills learned by level, skill trees | `mhp: "20 + level * 6"` |
| `actors` | the party members: name, class, faces, AI | `hero: {name: Hero, class: fighter}` |
| `skills`, `items`, `states` | what can be used and what it does | `effects: [{damage: a.atk}]` |
| `enemies`, `troops` | foes and the groups they come in | `slimes: {members: [slime, slime]}` |
| `ai` | shared tactics, by ID | `[{use: heal, when: "self.hp_rate < 0.3"}]` |
| `locations` | places: people, actions, exits, encounters, dungeons | see below |
| `events` | named command lists, called or triggered | `trigger: enter` |
| `shops`, `music` | goods; songs and moods | |
| `profiles` | the character book: a page per character | `mira: {title: Courier, text: ...}` |
| `enemies` with `ui.bestiary` | the monster book: a page per enemy | `mote: {lore: "Lives near the river stones."}` |

Two skills are required in every game: `attack` and `guard`, the battle commands. `game.yaml`'s `battle:` can point them elsewhere.

## Places

The player is always at a location. A location shows its people, its actions and its exits as a menu:

<!-- verify: load -->
```yaml
locations:
  village:
    name: Village
    background: village.jpg
    people:
      - name: Elder
        image: elder.png
        talk:
          - say: {who: Elder, text: Slimes have been seen in the field.}
          - set: {flag.asked: true}
    actions:
      - label: Rest
        do: [{heal: all}, {say: You feel rested.}]
    exits:
      - {to: field, label: Go to the field, when: flag.asked}
```

Anything with `when` appears only while its expression holds. `on_enter` runs on arrival; `safe: true` lets the player save from the menu there; `encounters` sets their rate and troops.

### Maps and painted scenes

A location can also be shown as a map or as a painted scene (`schema_version: 3`). Its people, actions and exits stay the same; these only say where things are.

<!-- verify: load -->
```yaml
locations:
  town:
    name: Town
    map:
      picture: maps/town.jpg            # points are in the picture's pixels
      points:
        square: {at: [960, 700], to: plaza, start: true}
        shop:   {at: [600, 520], to: shop}
        gate:   {at: [1500, 900], to: gate, when: "item.key > 0", locked: The gate is shut.}
        sign:   {at: [1120, 460], label: Signpost, do: [{say: "WELL, this way."}], once: true}
      roads:
        - {between: [square, shop], via: [[800, 650]], steps: 2}
        - {between: [square, gate], encounters: {rate: "0.2", troops: [{troop: wolves}]}}
        - {between: [square, sign]}
  plaza:
    name: Plaza
    background: bg/plaza.jpg            # the painting, from a fixed camera
    stage:
      depth: bg/plaza_depth.png         # 16-bit gray, near is light (optional)
      scale: [{at: [960, 1060], height: 190}, {at: [1250, 420], height: 78}]
      walk: [[[1000, 330], [1480, 330], [1700, 760], [1500, 1080], [430, 1080], [960, 480]]]
      blocks: [[[1150, 360], [1370, 360], [1380, 410], [1150, 410]]]
      fronts: [{area: [[1222, 50], [1305, 50], [1385, 405], [1150, 405]], base: 392}]
      spots:
        start: {at: [960, 980], facing: north}
        alley: {at: [1040, 470], facing: south}
      exits: [{to: shop, area: [[1000, 330], [1100, 330], [1100, 360], [1000, 360]]}]
      people:
        - {id: ox, at: [1180, 520], facing: west, doll: {hair: bald, parts: [{kind: apron, color: "#2f6fb8"}]}}
      things:
        - {id: board, name: Notice board, area: [[700, 700], [760, 700], [760, 760], [700, 760]], do: [{say: WATER 5G.}], once: true}
    people:
      - {id: ox, name: Ox, image: ox.png, talk: [{say: Hold the line.}], bubbles: [Somebody has to.]}
    exits:
      - {to: shop, label: The shop}
      - {to: town, label: Back to town, at: square}   # at is the point of the map to stand on
```

On a map the party goes from point to point along the open roads (paint the roads in the picture: their lines in the YAML are what the token walks, drawn only with `draw_roads: true` (on a campaign's map `region_roads` and `region_borders`, `selected` by default, decide instead whether all roads and borders are drawn or only the region chosen's); each point stands a small `marker`, a pin unless you give a picture; the name plates keep clear of each other, their text sized by `label_size`, 0.03 of the screen's height by default; the token is the leader's round face, as in the balloons, sliding along the roads, unless `token` gives a picture or says `doll`); a point with `to` leads into that location, and coming back puts the party on the point that leads to where it came from. In a scene the party walks the painting as a small doll; the picture and its `fronts` hide whoever stands behind them, and walking into an exit's area leaves. The rules know only where the party stands, `spot` (a point of the map or one of the scene's `spots`): every point, person, thing and spot is an ordinary menu entry, so `pick`, `gozen sim design` and the tests walk them without a screen, and `exits[].at` names the spot or point to arrive at.

## Commands

`talk`, `do`, `on_enter` and event bodies are lists of commands. Each command has exactly one verb:

| Verb | Does |
|:--|:--|
| `say` | a line; a string, or `{who, face, text}` |
| `choice` | a menu of `{label, do}` options |
| `set` | assign flags and variables: `{flag.met: true, var.trust: "var.trust + 1"}`; see [Changing the state](#changing-the-state) |
| `if` | `{if: EXPR, then: [...], else: [...]}` |
| `battle` | fight a troop, with `on_win`, `on_lose`, `on_escape` |
| `move`, `dive` | go to a location; go down a generated dungeon |
| `call` | run a named event |
| `gold`, `exp`, `give`, `take`, `heal`, `hurt` | change the party |
| `join`, `leave` | change the party members |
| `image`, `music`, `sfx`, `mood`, `wait` | show a picture, play music and sounds |
| `save`, `shop`, `end`, `clear` | open the save menu or a shop; end the event; finish the game |
| `pick` | take the entry of the current place's menu with this label, as the player would (version 2) |
| `turn`, `step`, `walk` | in a dungeon: `turn: left`, `right` or `back`; `step: 3` cells ahead; `walk: G` to the nearest `G` tile by the shortest way (version 2) |
| `use`, `equip` | use an item from the menu (`{use: tonic, target: hero}`); put on equipment (`{equip: {actor: hero, item: blade}}`, or `slot` alone to take it off, or `actor` alone for the menu's best equipment) (version 2) |
| `buy`, `sell` | trade at a shop as its screen does: `{buy: {shop: stall, item: tonic, count: 2}}`; selling pays half the price (version 2) |
| `act` | a move for the next battle fought on AUTO: `{act: {actor: hero, skill: slash, target: wolf}}` (or `item`); moves are taken in order, one per actor a turn (version 2) |

The full list, with the options each verb accepts, is [Command](/schema/#command).

### Changing the state

`set` writes flags and variables, and from version 2 also `gold`, `item.ID` (how many), `party`, `location` (without the arrival events) and `actor.ID.tactics` (`auto`, `manual` or a preset). The values of flags, variables, `gold` and `item.ID` are expressions; `location`, `party` and `actor.ID.tactics` take names as written: `{location: camp, party: [hero, sela], actor.hero.tactics: auto}`. `actor.ID.tactics` also takes the rows themselves: `{mode: manual, rows: [{enabled: true, condition: always, action: guard}]}`. In the development mode (`--dev`, `Config.Dev`) `set` also moves the party in a dungeon (`pos.x`, `pos.y`, `pos.facing`), sets `steps`, and sends the floor's strong foes away (`roamers: none`).

`actor.ID.hp`, `mp`, `exp`, `level` and `sp` can be set too, but only in the development mode (`--dev`) and in tests. A game played normally skips them, and `gozen check` warns where game data sets them. Grow a party in the game with `exp`, `heal` and `learn`.

### Your own commands and screens

A command or a screen that YAML cannot express is declared in `game.yaml` by name (version 2):

<!-- verify: skip needs Go code that registers the verb roll_dice and the screen album -->
```yaml
verbs: [roll_dice]     # - roll_dice: {sides: 6}
screens: [album]       # - open: album
```

`gozen check` then accepts `roll_dice` as a command and `album` as a screen for `open`. Any other unknown name is an error, with the nearest command when it looks like a typo (`unknown field "sya" (did you mean "say"?)`). The Go side that implements them comes in a later version; until then a declared command is logged and skipped.

## Events

An event is a named list of commands. Call it with `call`, or give it a trigger:

<!-- verify: load -->
```yaml
events:
  arrival:
    trigger: enter        # run when the party arrives at location
    location: ridge
    once: true
    do:
      - say: The ridge lantern answers your light.
      - gold: 4
      - join: sela
```

`trigger: auto` runs the event as soon as its `when` holds. `once: true` runs it a single time per save.

## Expressions

Conditions and formulas are strings in a small expression language:

<!-- verify: skip three separate expressions, not one file -->
```yaml
when: flag.trained && !flag.marker_lit
exp_for: level * 30
damage: a.atk * 2 - b.def
```

- Names in the field: `flag.NAME`, `var.NAME`, `gold`, `item.ID` (the count), `party`, `members`, `actor.ID`, `location`, `spot` (version 3: the point of a map or the spot of a scene the party stands on), `steps`, `pos`, `chose.ID` (version 4: the option last taken at the choice with that `id`, "" before).
- In the commands of an event (version 2 games and older): `menu` (the entries of the current place, each with `kind`, `label`, `new` and `marked`), `map` (the rows of the dungeon floor), `cell(x, y)` (`char`, `raw`, `passable`, `solid`, `look`, `seen`, `shown`, `spent`), `result` (the last battle: `win`, `lose` or `escape`), `over` and `cleared`, `completed` (the floor has been walked whole), `moved` (the last step went forward), `ran.ID` (an event's `count` of runs and the `order` of its last run), `repairs` (what loading a save had to fix), and `actor.ID.tactics` (`mode`, `preset`, `rows`).
- In battle formulas, `a` is the user and `b` the target; in stats, `level` is the actor's level.
- Operators: `+ - * /`, comparisons, `&&` (`and`), `||` (`or`), `!`, and `cond ? x : y`.
- Functions: `min`, `max`, `abs`, `floor`, `ceil`, `round`, `int`, `sqrt`, `pow`, `clamp`, `len`, `rand`, `chance`, `pages`.
- There are no loops or user functions: a list is read with `len(menu)` or `menu[0].label`. Going through every entry is the job of Go code, not of YAML.

A condition you leave out holds; a formula you leave out is 0. `gozen check` parses every expression and reports unknown names with file and line.

## Battles

Battles are turn-based, with a party of up to four (`max_party`). Each actor and enemy has eight stats: `mhp`, `mmp`, `atk`, `def`, `mat`, `mdf`, `agi`, `luk`. A skill's `effects` say what it does:

<!-- verify: load -->
```yaml
skills:
  shield_rush: {name: Shield Rush, mp: 2, scope: all_enemies, cutin: true, effects: [{damage: 9}]}
  mend: {name: Mend, mp: 3, scope: one_ally, anim: heal, effects: [{heal: 12}]}
```

Enemies, and party members on Auto, follow their `ai`: rules tried in priority order, each with an optional condition and target.

Battle forms, HP pictures and enemy motion use `schema_version: 7`. A trigger changes a foe when a hit crosses its threshold; a merge skill gathers eligible allies into a named form:

<!-- verify: parse These examples name forms and pictures supplied by the game. -->
```yaml
skills:
  gather: {name: Gather, scope: self, effects: [{merge: {into: king_slime, from: [drip_slime], count: 2, hp: sum}}]}
enemies:
  drip_slime:
    image: enemies/drip_slime.png
    images: [{below: 0.5, file: enemies/drip_slime_hurt.png}]
    ai: [{use: gather, when: "allies.kind_drip_slime >= 2"}]
  king_slime: {name: King Slime, image: enemies/king_slime.png}
  guardian:
    image: enemies/guardian.png
    hp_triggers: [{below: 0.5, transform: guardian_open, hp: ratio}]
  sprite:
    image: enemies/sprite.png
    puppet: sprite/puppet.yaml
```

An actor's `images` list uses `face` in place of `file`. A puppet file can add `soft_sway` with a `rect` or `ellipse` region, `follow: body`, and spring strength to move a part softly with breathing and battle reactions. Enemy and actor puppets share this YAML format. See [the battle recipes](/cookbook/#transform-an-enemy).

<!-- verify: load -->
```yaml
actors:
  sela:
    name: Sela
    class: mender
    ai:
      - {use: mend, when: "allies.hp_rate < 0.8", target: ally_lowest_hp_rate, priority: 1}
      - {use: attack}
```

`battle:` in `game.yaml` sets the global rules: hit variance, critical hits, escape chance, turn speed, the events before and after every battle. `cutin: true` on a skill shows the actor's standing picture across the screen.

Measure before you tune: `gozen sim battle game --troop TROOP` prints the win rate, and the F7 panel shows the same numbers in the game.

## Dungeons

A location with `dungeon` is a first-person grid. Draw the map by hand, or let Gozen generate floors:

<!-- verify: load under=locations -->
```yaml
  chamber:
    name: Observation halls
    dungeon:
      wall: wall.jpg
      floor: "#b4c2c6"
      map: |
        #######
        #S...T#
        #######
      tiles:
        S: {start: true, facing: east}
        T: {name: Old chest, picture: chest.png, solid: "true", once: true, do: [{gold: 5}]}
```

Tiles are characters with their own pictures, conditions and commands; `trigger: bump` runs them when the player walks into them. With `generate` instead of `map`, each floor is built from a seed, and `dive` goes down. `gozen sim floors` prints the generated maps; `gozen sim dive` plays a whole dive.

## Pictures, faces and text

Every file name is relative to `assets/`. A character's `faces` is a directory of expressions (`normal.png`, `smile.png`, ...), and `say` lines pick one with `face`. `gozen check` reports every picture that does not exist, and lays out every line with the game's font to catch text that overflows its window.

The engine's own words (menus, battle messages, settings, saves) come in English and Japanese. `language:` in `game.yaml` picks one: `en` (the default) or `ja`. `text:` lays the game's own wording over that language; a key is the engine's Japanese source text, a format with its `%d` and `%s` too, and the value may reorder them (`%[2]d`). [The English catalog](https://github.com/nao1215/gozen/blob/main/internal/text/catalog/english.go) lists every key. `terms:` names the currency and stats. `gozen check` warns when `language` is unset but the game is written in Japanese, and when the regular font lacks letters of the chosen words.

`profiles` keeps who each character is in the game itself: a page with the standing picture and its faces, your own fields (role, likes, anything), an introduction and how the character sees the others. Under `--dev` the title and the menu open it as Characters, so you can check a line against the character while you write; `ui.profiles: true` shows it to players, each page opening when its `when` holds ([the recipe](/cookbook/#keep-a-character-book)).

Press Tab in the book to see the relationship chart. Select a character and press Z to gather everyone connected to that character in either direction around them; press Z again for the page or X to return to the full chart. Each `relations` entry can set `kind` to `like`, `love`, `rival`, `enemy`, `family`, `friend`, or `other`; these colours carry only the meaning you give them. `affiliation` keeps members of a group together. Locked characters appear as `?` and hide their lines. The chart uses face crops from `assets/faces.json`, made by `gozen faces`.

`enemies.<id>.lore` adds a short habitat description to an enemy. Set `ui.bestiary: true` to add Monsters to the title and field menu. A battle encounter reveals its name, picture and lore; a victory reveals its stats, resistances, skills and drops, with a defeat count. Under `--dev`, F4 opens every page ([the recipe](/cookbook/#keep-a-monster-book)).

For collection, use schema version 6 and give each non-boss species a `family`, `rank` and `monster_class`. `recruit` lets it ask to join after a won battle; the Yes/No request also works in Session scripts and traces. Extra members go to the reserve. An inn or another safe place can run `open: ranch` to swap, release and breed monster instances. `enemies.<id>.sex` permits `any` (default), `male` or `female`, and optional `sex_ratio` gives the male probability for `any`. Each captured or born monster has a saved sex; breeding requires a male and a female. `breeding` defines specific species pairs and ranked family rules. `gozen check` validates the graph, `gozen sim breed game --format json` reports reachable and unreachable species, and `gozen book` prints the rules ([the collection recipe](/cookbook/#collect-raise-and-breed-monsters)).

The monster book also presents a buildable family tree: caught leaves give their encounter locations, specific recipes show both species, and family rules show both families and one concrete pair. The shortest recipe opens first. The screen shows up to seven monster pictures: the current species, two parents and four grandparents. Arrows select a picture, Z makes it the top of a two-generation view, X returns along a breadcrumb history, Tab switches the top species' recipe, and Q opens the selected known species' page. Parent cards with deeper recipes have a `more` mark; captures show their location and family recipes name the families and example parents. Unseen pictures appear as `?`; development play reveals every node. `gozen book --format json` exports the same structure.

## Languages

A game is written in one language, `written_in` (by default `language`), and can be offered in another: `lang/<code>.yaml` maps each text as written to its translation, and `languages` lets the player choose. The engine's own words come in English and Japanese, so the languages are `en` and `ja` ([the recipe](/cookbook/#offer-the-game-in-two-languages)).

## Novel games

A novel game is a game whose places mostly run events. `ui.message: novel` in `game.yaml` shows the lines in a band across the bottom of the screen, with the speakers' standing pictures above it:

<!-- verify: load as=game.yaml -->
```yaml
ui:
  message: novel
```

The band has buttons to roll back a line, for the backlog, auto, skip, saving and loading, hiding the window and the settings. Rolling back (Q, or the wheel) takes the state back with the line, and a save made in the middle of a scene goes on at its line ([the recipe](/cookbook/#let-the-player-roll-back-and-save-in-a-scene)). The player can also hold Ctrl to pass the lines already read, press N to turn skip mode on (it stays on until a line not read, a choice, a battle or the end of the events), and press Page Down, or the "Skip scene" button that shows when every line of the running event was read, to go at once to the next choice or line not read. The setting "Text to skip" makes skipping pass lines not read too.

Portraits come and go with the speakers unless an event places them with `stand` ([the recipe](/cookbook/#stage-portraits-and-effects)). A line can have a recorded voice ([Add voices](/cookbook/#add-voices)). [After the Pass](https://github.com/nao1215/gozen/tree/main/examples/novel), the novel sample, is a short one with choices, staging and a Japanese translation ([play it in the browser](/play/)).

A line counts as read by its source: the speaker and the text as written, before `{expressions}` are filled in. The read lines are kept in `read.json` beside the saves, so they last across saves and new games. Runs for testing (`--script`, `--start`, autoplay) do not touch them unless you pass `--save-dir`. In a trace, every `say` line says whether it was read, and `gozen trace --check` fails when skipping passed a line that was not.

## Campaigns

A campaign (`schema_version: 5`) is a war of regions over turns in the same game as its towns and dungeons. `regions` are the land (who borders whom, the owner, the garrison, the values such as wealth, the terrain, the troop that defends it), `factions` the sides with their colours, capitals and `ai` rules, and `campaign` in `game.yaml` the player's side, the turns, the generals (actors, with their troops), the orders and how a march is decided ([the recipe](/cookbook/#make-a-campaign)).

The campaign's place is a map whose points are named like the regions. Each region shows its owner's flag, or its `area` filled in the owner's colour; OK on it lists the orders. An order costs actions (or, with `count: orders`, can be given once a turn), may need a general and a target, and runs its `do` with `general` and `target` bound. A march (`march: true`) is decided by the numbers of `campaign.clash`, by a battle of the game (`fight: battle`), or by a clash of units shown on its own screen (`fight: clash_view`): the generals the sortie screen sends, each with a tactic and its skills, against the members of the region's `defend` troop, up to `clash.units` a side. `on_win`, `on_lose` and `on_draw` run a general's word after it.

When the player ends the turn, each other faction follows its first `ai` rule that holds (draft troops, march on a neighbour), the turn's `on_turn` event runs, and the campaign ends with `won` or `lost` when `win` or `lose` holds or the turns run out. Expressions read `region.<id>.owner`, `faction.<id>.regions`, `campaign.turn`, `generals.<id>.troops` and more ([Expressions](#expressions)); `set` changes a region's owner, troops and values. A generals' troops and HP are the same state as the rest of the game, so a dungeon can feed the war. `gozen sim campaign` measures the win rate before you tune the numbers.

A campaign can share a game with dungeons and towns. The generals are the same actors as the party, so their level, stats and HP carry over, and a clash's formulas read them (`a.atk`, `a.level`); `move` goes between the campaign's place and any other, both ways ([the recipe](/cookbook/#mix-a-campaign-with-a-dungeon)):

<!-- verify: load as=game.yaml group=mixed -->
```yaml
schema_version: 5
campaign:
  location: war
  player: crown
  generals: {walker: {troops: 400}}
  clash: {power: "a.troops * (1 + a.atk / 20)"}
  orders:
    attack: {label: Attack, general: ready, target: border, march: true}
    rest: {label: Rest at the inn, cost: 0, do: [{move: inn}]}
```

<!-- verify: load group=mixed -->
```yaml
factions:
  crown: {name: Crown, capital: home}
  raven: {name: Raven, capital: keep}
regions:
  home: {next: [keep], owner: crown, troops: 300}
  keep: {next: [home], owner: raven, troops: 600}
locations:
  war:
    name: The war
    map:
      picture: maps/land.jpg
      points: {home: {at: [400, 540], start: true}, keep: {at: [1500, 540]}}
      roads: [{between: [home, keep]}]
  inn:
    name: Inn
    exits: [{label: Back to the war, do: [{move: war}]}]
```

## Saves

Saves are JSON files in the OS config directory, under the game `id`. They record the schema and data versions; raise `data_version` in `game.yaml` when you change definitions in a way old saves should notice, and the loader reports what it repaired.

## A bigger example

[Go Zen's data](https://github.com/nao1215/gozen/tree/main/examples/go-zen/game/data) uses all of the above: a town on a map with a painted plaza, talk with moving portraits, flags, three companions, AI rules and presets, cut-ins, hand-drawn mazes with chests, keys and gates, a generated floor, and boss fights.
