---
title: Writing a game
description: How a Gozen game directory is laid out, and how places, events, commands, battles, dungeons and expressions fit together in YAML.
weight: 30
toc: true
---

A Gozen game is a directory of YAML and pictures that `gozen run` plays, with no Go source or compiler. This page shows how the parts fit together. The [Schema](/schema/) has every field, and the [Cookbook](/cookbook/) has recipes such as a shop, a boss fight or a locked door.

## The game directory

```text
game/
├── game.yaml        # the game: title, start, screen, fonts, battle rules, text
├── data/            # definitions; every .yaml file here is read and merged
│   ├── world.yaml
│   └── ...
└── assets/          # pictures, fonts, music and sounds, referred to by file name
```

`game.yaml` is a [Game](/schema/#game). Each file in `data/` is a [DataFile](/schema/#datafile) and may hold any of its sections, so split by area (`town.yaml`, `cave.yaml`) or by kind (`enemies.yaml`, `skills.yaml`). IDs share one namespace per section, and a duplicate is an error.

New games start with `schema_version: 11`, as the templates do. To move an older game to 11, change the header and add `enabled: false` to every tactic row without `enabled`, since an omitted switch now means true. Before v1.0.0 the formats may change between versions, and there is no migration command.

From version 1 on, a number left out and a written `0` differ, and the game `id` (the save directory) is required. Version 2 adds the commands and `set` targets marked "version 2" below; a version 1 game reads as before.

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

Combat games need the battle commands `attack` and `guard` (`battle:` in `game.yaml` can point them elsewhere). A narrative game can use actors without classes, with no dummy party or combat skills.

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

A location can also be a map or a painted scene (`schema_version: 3`). These only say where things are; the people, actions and exits stay the same.

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

On a map the party walks the open roads from point to point. Paint the roads into the picture: the YAML lines are the path the token walks, drawn only with `draw_roads: true` (on a campaign's map, `region_roads` and `region_borders`, `selected` by default, decide whether all roads and borders show or only the chosen region's). Each point has a small `marker`, a pin unless you give a picture, and name plates keep clear of each other, with text sized by `label_size` (0.03 of the screen height by default). The token is the leader's round face, as in the balloons, unless `token` gives a picture or says `doll`. A point with `to` leads into that location, and coming back puts the party on the point that leads where it came from.

In a scene the party walks the painting as a small doll; the picture and its `fronts` hide whoever stands behind them, and walking into an exit's area leaves. The rules only track `spot`, where the party stands (a map point or one of the scene's `spots`). Every point, person, thing and spot is an ordinary menu entry, so `pick`, `gozen sim design` and the tests walk them without a screen. `exits[].at` names the spot or point to arrive at.

`hide: closed` on a map point removes the point, its marker, name plate and roads while `when` is false, and keyboard, pointer, selection panel and headless choices skip it. An event can open it without the party leaving the location. Without `hide` the point shows as the usual gray closed pin; `unknown` still controls unvisited points once they are open.

<!-- verify: parse fragment of a map point; the flag is set by a game event -->
```yaml
workshop:
  at: [640, 360]
  label: Workshop
  hide: closed
  when: flag.workshop_built
  do: [{say: Welcome.}]
```

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

`set` writes flags and variables, and from version 2 also `gold`, `item.ID` (how many), `party`, `location` (without the arrival events) and `actor.ID.tactics` (`auto`, `manual` or a preset). The values of flags, variables, `gold` and `item.ID` are expressions; `location`, `party` and `actor.ID.tactics` take names as written: `{location: camp, party: [hero, sela], actor.hero.tactics: auto}`. `actor.ID.tactics` also takes the rows themselves: `{mode: manual, rows: [{enabled: true, condition: always, action: guard}]}`. In the development mode (`--dev`) `set` also moves the party in a dungeon (`pos.x`, `pos.y`, `pos.facing`; `pos.x` and `pos.y` elsewhere are an error), sets `steps`, and sends the floor's strong foes away (`roamers: none`).

`actor.ID.hp`, `mp`, `exp`, `level` and `sp` can be set only in the development mode (`--dev`) and in tests; normal play skips them, and `gozen check` warns where game data sets them. In the game, grow a party with `exp`, `heal` and `learn`, and start someone over for a new game plus with `reset_actor` ([cookbook](/cookbook/#grow-by-level-and-learn-skills)).

### Your own commands and screens

Event aliases in `game.yaml` are reusable commands: `- greet: true` runs the target event like `call: greeting`. Unknown targets, cycles and unsupported arguments are validation errors.

<!-- verify: parse game.yaml fragment; the event target is defined in data/ -->
```yaml
verbs: {greet: greeting}
screens:
  journal:
    title: Journal
    entries:
      - label: The station
        text: You found the old station.
        when: flag.found_station
```

Define `greeting` in `data/` as an ordinary event, and open the journal with `- open: journal`. Entries take an optional `picture`, interpolated text and `when`. The arrows select an entry and Cancel returns. An optional `do` list runs event commands once when the entry is confirmed:

<!-- verify: parse game.yaml fragment -->
```yaml
screens:
  stock:
    title: Workshop stock
    entries:
      - label: Add one board
        text: 'Boards: {var.boards}'
        do: [{set: {var.boards: 'var.boards + 1'}}]
```

After an action, text and conditions refresh and the selection stays where it can. Hidden entries are checked again before they run, a removed selection moves to an available entry, and an empty list can still be closed. Entries without `do` are information pages.

An action suspends the event that opened the screen. Dialogue, other screens and saving use their normal interfaces and return to the list. A location change closes the list after the action ends, and clear, game over and loading replace the screen as usual. Closing the list resumes the opening event after its `open`.

A save made in an action's dialogue resumes that entry's commands at the saved line without applying earlier changes twice. After loading, the action finishes at the current location, but the list and the suspended opening event are not restored, so keep changes that must follow the dialogue in the entry's `do`. Entries are identified by screen name and position, so keep their order stable for old saves. Scripted `press ok` takes the same path, and traces record `screen_action` with the screen name and entry index. No Go registration is needed, and the former lists of names are no longer accepted.

### Stories without combat

Start with `gozen new my-story --template novel`. A speaker needs only a name, `actors: {rowan: {name: Rowan}}`, and `say: {who: rowan, text: Welcome.}` works as usual. Faces, voices and profiles need no class, stats, attack skills or starting party, and saves keep story state with an empty party. Giving a speaker a `class` lets the same ID fight; a game with combat still needs a valid starting party and battle rules.

### Conversation reactions

`reactions: {rowan: rowan.yaml}` in `game.yaml` maps a speaker to a reaction book under `assets/`. `respond: {who: rowan, text: Hello., from: player}` sends text to the book and shows its reply as a normal line; `from` defaults to `player`, and both fields take interpolation. The book must contain the matching character ID. Saves keep the reaction memory, and random replies use the game's seeded random stream. The book is a set of authored rules, with no model or network service ([a complete book](/cookbook/#a-speaker-who-remembers-a-greeting)).

## Events

An event is a named list of commands. In schema 11, write the list directly when it needs no metadata, and run it with `call: greeting`:

<!-- verify: parse schema-11 event fragment; ticket is defined by the game -->
```yaml
events:
  greeting:
    - say: Welcome back.
    - give: ticket
```

`give: ticket` and `take: ticket` mean one item; `give: {item: ticket, count: 2}` gives more. Use the mapping form when the event needs a trigger, a condition or parameters:

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

A choice with `muted` waits behind a muted microphone, as on a video call: only an Unmute button shows, and the answers appear when the player presses it (OK, a click, or on a call a click on the struck-through mic). If the player stays muted for `seconds`, the muted `do` runs instead of an answer, a named choice records `muted` in `chose.<id>`, and the story goes on. For a choice without a `prompt` right after a line of the call, the call stays on screen with its mic struck. A choice whose options are all hidden by `when` is not asked, and its muted `do` does not run. `gozen sim routes` explores staying quiet as one more branch.

<!-- verify: parse a fragment of an event -->
```yaml
- say: {who: ann, text: Any objections?, ensemble: studio}
- choice:
  - {label: Friday is too tight., do: [{set: {var.floor: 1}}]}
  - {label: Agreed., do: [{set: {var.floor: 2}}]}
  muted:
    seconds: 8
    do: [{set: {var.floor: 3}}]
```

## Expressions

Conditions and formulas are strings in a small expression language:

<!-- verify: skip three separate expressions, not one file -->
```yaml
when: flag.trained && !flag.marker_lit
exp_for: level * 30
damage: a.atk * 2 - b.def
```

- Names in the field: `flag.NAME`, `var.NAME`, `gold`, `item.ID` (the count), `party`, `members`, `actor.ID`, `location`, `spot` (version 3: the point of a map or the spot of a scene the party stands on), `steps`, `pos`, `chose.ID` (version 4: the option last taken at the choice with that `id`, "" before).
- In the commands of an event (version 2 and later): `menu` (the entries of the current place, each with `kind`, `label`, `new` and `marked`), `map` (the rows of the dungeon floor), `cell(x, y)` (`char`, `raw`, `passable`, `solid`, `look`, `seen`, `shown`, `spent`), `roamers` (the strong enemies of the current dungeon floor, each with `kind`, `name`, `troop`, `x` and `y`), `result` (the last battle: `win`, `lose` or `escape`), `over` and `cleared`, `completed` (the floor has been walked whole), `moved` (the last step went forward), `ran.ID` (an event's `count` of runs and the `order` of its last run), `repairs` (what loading a save had to fix), and `actor.ID.tactics` (`mode`, `preset`, `rows`).
- In battle formulas, `a` is the user and `b` the target; in stats, `level` is the actor's level.
- Operators: `+ - * /`, comparisons, `&&` (`and`), `||` (`or`), `!`, and `cond ? x : y`.
- Functions: `min`, `max`, `abs`, `floor`, `ceil`, `round`, `int`, `sqrt`, `pow`, `clamp`, `len`, `rand`, `chance`, `pages`, `range(a, b)` (the whole numbers from `a` up to but not `b`), `first(list, fallback)` (the first element, or the fallback for an empty list), `has(object, "field")` (the object has that field and it is true), `str(x)` (the value as text) and `grouped(n)` (the number as text with thousands separators).
- Use collection expressions such as `filter`, `count` and `sum` to select or summarize records. A bounded `call.each` applies one parameterized event to selected IDs; screen entries also accept `each`. Read [typed arguments and repeated calls](https://github.com/nao1215/gozen/blob/main/docs/development/event-arguments.md) for scope and replay limits.

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

An actor's `images` list uses `face` in place of `file`. A puppet file can add `soft_sway`, a `rect` or `ellipse` region with `follow: body` and a spring strength, so a part moves softly with breathing and battle reactions. Enemies and actors share the puppet format ([the battle recipes](/cookbook/#transform-an-enemy)).

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

`battle:` in `game.yaml` sets the global rules: hit variance, critical hits, escape chance, turn speed, and the events before and after every battle. `cutin: true` on a skill shows the actor's standing picture across the screen. `gozen sim battle game --troop TROOP` prints the win rate before you tune, and the F7 panel shows the same numbers in the game.

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

Every file name is relative to `assets/`. A character's `faces` is a directory of expressions (`normal.png`, `smile.png`, ...), and `say` lines pick one with `face`. `gozen check` reports every missing picture and lays out every line with the game's font to catch text that overflows its window.

The engine's own words (menus, battle messages, settings, saves) come in English and Japanese; `language:` in `game.yaml` picks `en` (the default) or `ja`, and `text:` and `terms:` reword them ([the recipe](/cookbook/#change-the-language-and-the-engines-wording)).

`profiles` keeps a page per character in the game: the standing picture and its faces, your own fields (role, likes, anything), an introduction and how the character sees the others, with a relationship chart on Tab. Under `--dev` the title and menu open it as Characters, so you can check a line against the character while writing; `ui.profiles: true` shows it to players, each page opening when its `when` holds ([the recipe](/cookbook/#keep-a-character-book)).

`enemies.<id>.lore` adds a short habitat description to an enemy, and `ui.bestiary: true` adds Monsters to the title and field menu. Meeting an enemy in battle reveals its name, picture and lore; beating it reveals its stats, resistances, skills and drops, with a defeat count. Under `--dev`, F4 opens every page ([the recipe](/cookbook/#keep-a-monster-book)).

To collect monsters (schema version 6), give each non-boss species a `family` and `rank`; `recruit` lets it ask to join after a won battle, `open: ranch` at a safe place swaps, releases and breeds the caught ones, and `breeding` defines species pairs and ranked family rules. The monster book then shows a family tree of recipes. [The collection recipe](/cookbook/#collect-raise-and-breed-monsters) has the rules and the commands that check them.

## Languages

A game is written in one language, `written_in` (by default `language`), and can be offered in another: `lang/<code>.yaml` maps each text as written to its translation, and `languages` lets the player choose. The engine's own words come in English and Japanese, so the languages are `en` and `ja` ([the recipe](/cookbook/#offer-the-game-in-two-languages)).

## Novel games

A novel game is a game whose places mostly run events. `ui.message: novel` in `game.yaml` shows the lines in a band across the bottom of the screen, with the speakers' standing pictures above it:

<!-- verify: load as=game.yaml -->
```yaml
ui:
  message: novel
```

The band has buttons to roll back a line, open the backlog, auto, skip, save, load, hide the window and open the settings; `ui.saves.quick` adds a quick save and load. Rolling back (Q, or the wheel) takes the state back with the line, and a save made mid-scene resumes at its line, with a picture of the scene on the save screen ([the recipe](/cookbook/#let-the-player-roll-back-and-save-in-a-scene)).

Holding Ctrl passes lines already read. N turns on skip mode, which stays on until an unread line, a choice, a phone that keeps a result, a battle or the end of the events. Page Down, or the "Skip scene" button shown once every line of the running event was read, jumps to the next choice or unread line. With the setting "Text to skip" at all, skip mode, Ctrl and scene skip pass unread lines too (and scene skip is offered in any event with lines), but they still stop at choices, phones that keep a result and battles. The setting "Text size" draws the message window's lines larger, two lines a page, for a small screen such as a phone.

A line counts as read by its source, the speaker and the text as written before `{expressions}` are filled. Read lines are kept in `read.json` beside the saves, across saves and new games. Test runs (`--script`, `--start`, autoplay) leave them alone unless you pass `--save-dir`. In a trace every `say` line says whether it was read, and `gozen trace --check` fails when skipping passed an unread line.

![the settings raise the text size twice, and the line behind them comes back larger, two lines a page, as does the next](/images/novel-text-size.mp4)

Portraits come and go with the speakers unless an event places them with `stand` ([the recipe](/cookbook/#stage-portraits-and-effects)). A line can have a recorded voice, played again from the backlog ([Add voices](/cookbook/#add-voices)), and readings over its words (`|base《reading》`, [the recipe](/cookbook/#offer-the-game-in-two-languages)). The gallery lists event pictures, scenes to play again and the endings reached ([the recipe](/cookbook/#show-event-pictures-and-fill-the-gallery)). [After the Pass](https://github.com/nao1215/gozen/tree/main/examples/novel), the novel sample, is a short one with choices, staging and a Japanese translation ([play it in the browser](/play/), on a phone too: [Phones](/commands/#phones)).

Pictures can be up to 8192 pixels a side, 33,554,432 pixels in all and 64 MiB encoded, so a 4096 by 6144 portrait works without extra settings. The largest decoded RGBA picture takes 128 MiB, and GPU limits and memory vary by device, so export pictures near their displayed size and test on the target devices. `gozen check` checks actor face PNGs even when no profile refers to them.

### Frame portraits by their heads

For static novel portraits with transparent padding, `ui.stage_framing` aligns the heads instead of the whole canvas. It is set per picture and needs `schema_version: 8`:

<!-- verify: parse requires the author's portrait image and its measured head coordinates -->
```yaml
ui:
  message: novel
  stage_framing:
    portraits/guide.png:
      head: [0.4, 0.1, 0.2]
      height: 0.18
      top: 0.08
```

`head` is the head's horizontal center, top and height in the source picture: the center as a fraction of its width, top and height as fractions of its height. Here the head starts 10% down, is 20% of the height and is centered 40% across. All three must be finite and inside the picture, and the height must be positive and fit below the top.

`height` is the head's displayed height as a fraction of the screen height (above 0, at most 1), and `top` places its top in the same units (0 to 1, default 0). The head's horizontal center follows `stand.at` or the automatic speaker placement, even when the scaled picture runs past the screen. What is off screen is clipped, so transparent padding never pushes both speakers to the center, and small pictures use the full novel stage.

Give each expression its own entry when its canvas or head differs. `stage_scale` still multiplies the size, now around the head, and `stage_drop` adds a screen-height offset to the head's top. Pictures without an entry keep the canvas-based layout. The setting applies to novel portraits (and, with `ui.stage_puppets`, to the moving portraits standing in their place), not to battle cut-ins or the `box` window. Saves restore the standing cast; a reload or a resize recomputes the framing from the current picture and settings.

### Move the standing portraits

An actor with a `puppet` ([the recipe](/cookbook/#give-a-character-faces-and-a-moving-portrait)) can stand on the novel stage as a moving portrait: `ui.stage_puppets: true` (schema version 8) draws the puppet in place of the face picture, in the frame the picture would have.

<!-- verify: parse requires the author's actor, puppet and face pictures -->
```yaml
ui:
  message: novel
  stage_puppets: true
```

Every standing figure breathes, blinks and sways its hair and swaying parts; only the speaker's mouth moves while the text types. A line's `face`, or a `stand` with `face`, fades the figure to that face over the puppet's `fade`, and `effect: nod` and the other reactions play on it. A face the puppet lacks, or a figure that is not an actor, shows the picture as before. Give the puppet's base the canvas of the face pictures so that `stage_framing`, `stage_scale` and `stage_drop` fit it. A large base is composed near its drawn size, so a 4096 by 6144 portrait does not cost its full size every frame.

### A phone on the screen

#### Games inside the phone

Since 0.23.0 the phone can open another Gozen game, in portrait or landscape. The host supplies a Back control, and closing the app resumes the host. Each game has its own icon, progress and saves.

![Counter collects four tokens inside the office phone](/images/phone-game-counter.mp4)

Authors bundle games or offer optional downloads from an approved catalog, and players choose when to install or update. The public catalog has Counter, a small example game. The [phone game recipe](https://github.com/nao1215/gozen/blob/main/docs/examples/phone-games.md) covers setup, package metadata, browser limits and replay.

To share your game, [submit it for registration](https://github.com/nao1215/gozen/issues/new?template=game-registration.yml) with its source, release package, custom icon and licenses. Submissions are reviewed before publication; submitting does not install or publish anything.

#### Chats, feeds and other apps

A home screen opens contacts, chats and feeds. YAML defines the history and replies, and flags reveal contacts, messages and posts. Players pick fixed replies without typing, and read-only apps need no replies. See the [flag-based phone recipe](https://github.com/nao1215/gozen/blob/main/docs/examples/phone-flags.md).

![Opening Contacts and a chat, then returning to the phone home](/images/phone-navigation.mp4)

`phone` shows a phone over the scene and waits until the player puts it down. It changes no state itself; only its buttons, calls and liked posts keep anything. Skip and Ctrl pass a phone that only shows and stop at one that keeps a result (a button, a call, a liked post), as at a choice. `app` names what is on it, each with its own list, and `hold: landscape` turns the phone on its side.

<!-- verify: parse a fragment of an event; the pictures and the puppet are the author's files -->
```yaml
- phone:
    app: chat
    title: Team
    messages:
    - {from: Ann, icon: icons/ann.png, text: Are you up?}
    - {mine: true, text: Now I am.}
    - {from: Ann, icon: icons/ann.png, video: {picture: bg/room.png, puppet: ann/puppet.yaml, text: Look, seconds: 8, bounce: [2, 5]}}
- phone:
    app: feed
    title: Photos
    posts:
    - {who: ann, icon: icons/ann.png, picture: photos/beach.png, likes: 120, text: Day off.}
    - {who: bea, icon: icons/bea.png, picture: photos/ex.png, likes: 4, text: Old times., set: {flag.liked_ex: true, var.trust: var.trust - 1}}
- phone:
    app: feed
    scroll: across
    browse: true
    posts:
    - {who: ann, icon: icons/ann.png, picture: photos/beach.png, text: Day off.}
    - {who: bea, icon: icons/bea.png, picture: photos/cafe.png, text: Latte.}
- phone:
    app: video
    hold: landscape
    video: {picture: bg/stage.png, puppet: ann/puppet.yaml, text: Live clip, seconds: 30, bounce: [4, 9.5]}
```

A chat shows its first message when the phone comes up, and each press brings the next, other people's after typing dots. A message is text, a picture or a video; a press on the newest video turns the phone sideways and plays it until Cancel turns it back.

A feed stacks its posts and scrolls with the wheel, the up and down keys or a drag. A click or tap on a post (its picture, a text card or the heart) likes it, a second one takes the like back, and OK at the bottom puts the phone down; a post's video plays in place while it is on screen. `scroll: across` shows one post a screen, swiped sideways like a carousel: a drag follows the finger and settles on the nearest post, or on the next when flicked, and left, right, the wheel and OK step a post, with dots underneath. A post's `set`, written as the `set` command, runs when the phone is put down with that post liked, so the story can answer a mistaken like; a like taken back keeps nothing. Only the pointer likes (keys and pad scroll), so a story never waits on a like. A feed has no `var`, at most 31 of its posts may have `set`, and `browse: true` makes a feed look-only: a click likes nothing and `gozen check` rejects `set` on its posts. A caption too long for its three lines is drawn smaller.

A video is a picture pushed in slowly with an optional puppet in front, `zoom` times the frame height (1.5, the upper body, by default), breathing and swaying as on the stage and jiggling at each second in `bounce`. For real footage, `frames` lists pictures under assets/ (or a sorted pattern such as `clips/battle/*.jpg`) played at `fps` (12 by default) over `picture`, which becomes the poster. A feed loops them; elsewhere the clip stops on its last frame, and `seconds` defaults to their length. The engine decodes no video files, so cut the video into pictures first (for example `ffmpeg -i clip.mp4 -vf fps=12,scale=640:-1 clips/battle/%03d.jpg`).

Lock screens, pages and banners share `timeout` (seconds after the phone appears), `timeout_result` (default `expired`) and `var`. A timeout needs `var`; without a timeout, `var` needs a page button or a lock-screen call, while banners can always keep an outcome. When the deadline passes with no outcome, the phone closes, stores the timeout result and the event goes on; input on the deadline tick wins, and scrolling does not extend the deadline. Page buttons keep their IDs. On a page with `var`, Cancel returns `dismiss` even without a timeout, and finishing without pressing a button returns `open`; timed plain locks and ordinary notices in a lock with calls also return `dismiss` or `open`. Declining a call moves to the next notice, and declining the last returns `decline`. Auto and Skip cannot answer a phone that keeps a result, and outcomes are saved for replay. Incoming-call `ring` and `missed` work as before: the timer starts when that notice arrives, and the assignments run when it rings out.

<!-- verify: load group=phone-outcomes -->
```yaml
events:
  phone_deadlines:
    - phone:
        app: lock
        time: "07:00"
        var: var.lock_result
        timeout: 8
        timeout_result: later
    - phone:
        app: page
        var: var.page_result
        timeout: 15
        timeout_result: unread
        rows: [{text: A note for today.}, {button: Continue, id: read}]
    - phone:
        app: banner
        var: var.notice
        timeout: 4
        notices: [{app: Mail, title: New message, text: Open the message?}]
    - if: var.notice == "open"
      then: [{phone: {app: page, title: Mail, rows: [{text: Welcome!}]}}]
```

A banner with `var` waits: Enter or a click on it returns `open`, Esc or Cancel returns `dismiss`, and the shared timeout returns `expired` (or `timeout_result`). It takes exactly one notice without `event`; branch after it instead. Without `timeout` it waits for input. A banner without `var` is an ordinary asynchronous notification.

Repetition is ordinary event YAML. This call rings again after a short wait while it is left unanswered, and answering or declining ends the loop. There is no alarm, snooze or vibration mode; `effect: shake` before a phone cue shakes the whole scene once.

<!-- verify: load group=phone-outcomes -->
```yaml
events:
  ring_again:
    - phone:
        app: lock
        var: var.call_result
        timeout: 8
        timeout_result: later
        notices: [{app: Phone, title: Studio, call: true}]
    - if: var.call_result == "later"
      then:
        - wait: 2
        - call: ring_again
```

Phone colors are the game-wide `ui.phone` in `game.yaml`, separate from dungeon themes. `background` fills page bodies, `text` and `muted` color text, and `accent` colors the owner's chat bubbles, the chat back mark and page controls; a page's own `color` still wins for its controls. The defaults are white (`#ffffff`), dark ink (`#1d1f24`), muted ink (`#808690`) and blue (`#3d8bf2`). Bubble and button labels stay white, so pick an accent with enough contrast.

<!-- verify: load group=phone-outcomes as=game.yaml -->
```yaml
schema_version: 11
ui:
  phone: {background: "#182430", text: "#ffffff", muted: "#bec8d2", accent: "#6750a4"}
```

A `page` is any app or web page: a browser bar with `url`, or an app bar with `title` and `color`, over `rows` that stack and scroll as a feed does. A row is `text` (`size: large` or `small`, `bold`), a `picture` shown whole with its `caption`, `pictures` in a grid of `columns` (3 by default), a list `item` (icon, title, text, a value on the right), a `qr` code of its text with a `caption`, or a `button`, which puts the phone down. An `item`, or a page's single `picture`, may carry a `button` (with `id` and `set`) so that the product row or the photo is what the player taps; the label names it for the keys and in traces. A grid of `pictures` is one row that is only looked at. `locked` (true or a condition, worked out when the phone comes up) darkens a picture or grid with a lock and its `label`. Left, right or Tab select a page button and OK presses it. A script's `pick` presses a page, home or contacts button by its label, and `look` lists them (`on: phone:page`).

A `lock` screen shows a large clock (`time`, the phone's clock by default) and `date` over `notices` that arrive one a press, or by themselves with `auto`. A notice with `call` fills the screen as an incoming call: OK or the green button answers, Cancel or the red one declines, and the next notice rings again if it is a call too. With `ring: 8` an unanswered call stops after eight seconds and puts the phone down with `missed`, running the call's `missed` (written as `set`) even if more notices follow; without `ring` it rings until answered, declined or the shared `timeout` expires. `var` keeps how the phone was put down (a button's `id` or label, `answer`, `decline`, `missed`, or a shared outcome above), and a button's or call's `set` is written as the `set` command, so the story can branch after it.

In a chat, `time` shows beside a bubble, `read: true` marks the owner's message read, and `day` puts a separator above it; a picture keeps its shape and opens over the screen on a press or click. A feed post with only `text` is a timeline card, with `replies` and `reposts` beside its likes. A post's `time` shows beside the name, and a poster without an `icon` gets a circle with the name's first letter. A clip with `frames` may `fade` each frame into the next and `pan` in on each, so a few pictures at a low `fps` play smoothly. `app: banner` without `var` does not wait: each notice slides down from the top for a few seconds like a push notification, over any scene, and a click on it runs its `event` after the running event.

<!-- verify: parse a fragment of an event; the pictures are the author's files -->
```yaml
- phone:
    app: page
    title: Harbor Bank
    color: '#1f5fbf'
    var: var.transfer
    rows:
    - {text: Confirm the transfer, size: large, bold: true}
    - {item: {icon: icons/bank.png, title: Blue Tide Ltd., text: Invoice 0420, value: '-3,000,000'}}
    - {pictures: [shop/1.png, shop/2.png, shop/3.png], locked: '!flag.supported', label: Supporters only}
    - {qr: 'https://github.com/nao1215/gozen', caption: Scan to add Mia}
    - {button: Confirm, set: {flag.paid: true}}
    - {button: Cancel, id: cancel}
- phone:
    app: lock
    date: Tuesday
    notices:
    - {app: Monitor, icon: icons/alert.png, title: API error rate 6%, text: Over the 5% threshold}
    - {app: On-call, icon: icons/alert.png, title: On-call · API error rate, text: Incoming call, call: true, set: {flag.woke: true}}
- phone:
    app: banner
    notices: [{app: Chat, icon: icons/ann.png, title: Ann, text: Are you coming?, event: ann_chat}]
```

## Campaigns

A campaign (`schema_version: 5`) is a war of regions over turns, in the same game as its towns and dungeons. `regions` are the land (neighbours, owner, garrison, values such as wealth, terrain, the defending troop), `factions` are the sides with their colours, capitals and `ai` rules, and `campaign` in `game.yaml` sets the player's side, the turns, the generals (actors with troops), the orders and how a march is decided ([the recipe](/cookbook/#make-a-campaign)).

The campaign's place is a map whose points are named like the regions. Each region shows its owner's flag, or its `area` filled in the owner's colour, and OK on it lists the orders. An order costs actions (or, with `count: orders`, can be given once a turn), may need a general and a target, and runs its `do` with `general` and `target` bound. A march (`march: true`) is decided by the numbers of `campaign.clash`, by a battle of the game (`fight: battle`), or by a clash of units on its own screen (`fight: clash_view`): the generals sent from the sortie screen, each with a tactic and skills, against the members of the region's `defend` troop, up to `clash.units` a side. `on_win`, `on_lose` and `on_draw` run a general's word after it.

When the player ends the turn, each other faction runs every `ai` rule that holds, in order (draft troops, march on a neighbour), the `on_turn` event runs, and the campaign ends `won` or `lost` when `win` or `lose` holds or the turns run out. Expressions read `region.<id>.owner`, `faction.<id>.regions`, `campaign.turn`, `generals.<id>.troops` and more ([Expressions](#expressions)), and `set` changes a region's owner, troops and values. `gozen sim campaign` measures the win rate before you tune the numbers.

The generals are the same actors as the party, so their level, stats and HP carry over, a clash's formulas can read them (`a.atk`, `a.level`), and a dungeon can feed the war. `move` goes between the campaign's place and any other, both ways ([the recipe](/cookbook/#mix-a-campaign-with-a-dungeon)):

<!-- verify: load as=game.yaml group=mixed -->
```yaml
schema_version: 11
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

Saves are JSON files in the OS config directory, under the game `id`, each beside a small JPEG of the scene kept as base64 text so a browser can store it too (`save1.json` and `save1.jpg.b64`). `ui.saves.slots` sets the number of slots and `ui.saves.quick` adds a quick slot (`quick.json`). Saves record the schema and data versions; raise `data_version` in `game.yaml` when old saves should notice a change in the definitions, and the loader reports what it repaired.

## A bigger example

[Go Zen's data](https://github.com/nao1215/gozen/tree/main/examples/go-zen/game/data) uses all of the above: a town on a map with a painted plaza, talk with moving portraits, flags, three companions, AI rules and presets, cut-ins, hand-drawn mazes with chests, keys and gates, a generated floor, and boss fights.

## Battle AI history visibility

The battle AI history panel lists recorded AI decisions and rule matches. It opens by default from its button, B/PageUp or controller L1. A game that has not introduced AI rules can hide it in `game.yaml`:

<!-- verify: parse game.yaml presentation settings -->
```yaml
ui:
  battle_ai_history: false
```

This turns off the button and the keys; battle decisions, messages, rewards, traces and replay checks stay the same. Omit the setting or write `true` to show the panel. The development `battle-log` switch can hide tactic explanations in captures but cannot bring back a panel the author turned off.
