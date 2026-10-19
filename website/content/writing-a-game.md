---
title: Writing a game
description: How a Gozen game directory is laid out, and how places, events, commands, battles, dungeons and expressions fit together in YAML.
weight: 30
toc: true
---

A Gozen game is data. The Go program only embeds the directory and starts the runner; everything a player sees comes from YAML and the pictures beside it. This page is the map. The [Schema](/schema/) has every field.

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

Start every `game.yaml` with `schema_version: 1`. In version 1 a number you leave out and a `0` you write are different, and the game `id` (used for the save directory) is required.

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

Two skills are required in every game: `attack` and `guard`, the battle commands. `game.yaml`'s `battle:` can point them elsewhere.

## Places

The player is always at a location. A location shows its people, its actions and its exits as a menu:

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

Anything with `when` appears only while its expression holds. `on_enter` runs on arrival; `safe: true` turns off random encounters; `encounters` sets their rate and troops.

## Commands

`talk`, `do`, `on_enter` and event bodies are lists of commands. Each command has exactly one verb:

| Verb | Does |
|:--|:--|
| `say` | a line; a string, or `{who, face, text}` |
| `choice` | a menu of `{label, do}` options |
| `set` | assign flags and variables: `{flag.met: true, var.trust: "var.trust + 1"}` |
| `if` | `{if: EXPR, then: [...], else: [...]}` |
| `battle` | fight a troop, with `on_win`, `on_lose`, `on_escape` |
| `move`, `dive` | go to a location; go down a generated dungeon |
| `call` | run a named event |
| `gold`, `exp`, `give`, `take`, `heal`, `hurt` | change the party |
| `join`, `leave` | change the party members |
| `image`, `music`, `sfx`, `mood`, `wait` | show a picture, play music and sounds |
| `save`, `shop`, `end`, `clear` | open the save menu or a shop; end the event; finish the game |

The full list, with the options each verb accepts, is [Command](/schema/#command).

## Events

An event is a named list of commands. Call it with `call`, or give it a trigger:

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

```yaml
when: flag.trained && !flag.marker_lit
exp_for: level * 30
damage: a.atk * 2 - b.def
```

- Names in the field: `flag.NAME`, `var.NAME`, `gold`, `item.ID` (the count), `party`, `members`, `actor.ID`, `location`, `steps`, `pos`.
- In battle formulas, `a` is the user and `b` the target; in stats, `level` is the actor's level.
- Operators: `+ - * /`, comparisons, `&&` (`and`), `||` (`or`), `!`, and `cond ? x : y`.
- Functions: `min`, `max`, `abs`, `floor`, `ceil`, `round`, `int`, `sqrt`, `pow`, `clamp`, `len`, `rand`, `chance`, `pages`.

A condition you leave out holds; a formula you leave out is 0. `gozen check` parses every expression and reports unknown names with file and line.

## Battles

Battles are turn-based, with a party of up to four (`max_party`). Each actor and enemy has eight stats: `mhp`, `mmp`, `atk`, `def`, `mat`, `mdf`, `agi`, `luk`. A skill's `effects` say what it does:

```yaml
skills:
  shield_rush: {name: Shield Rush, mp: 2, scope: all_enemies, cutin: true, effects: [{damage: 9}]}
  mend: {name: Mend, mp: 3, scope: one_ally, anim: heal, effects: [{heal: 12}]}
```

Enemies, and party members on Auto, follow their `ai`: rules tried in priority order, each with an optional condition and target.

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

Measure before you tune: `gozen sim game TROOP` prints the win rate, and the F7 panel shows the same numbers in the game.

## Dungeons

A location with `dungeon` is a first-person grid. Draw the map by hand, or let Gozen generate floors:

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

Tiles are characters with their own pictures, conditions and commands; `trigger: bump` runs them when the player walks into them. With `generate` instead of `map`, each floor is built from a seed, and `dive` goes down. `gozen floors` prints the generated maps; `gozen curve` plays a whole dive.

## Pictures, faces and text

Every file name is relative to `assets/`. A character's `faces` is a directory of expressions (`normal.png`, `smile.png`, ...), and `say` lines pick one with `face`. `gozen check` reports every picture that does not exist, and lays out every line with the game's font to catch text that overflows its window.

The engine's own words (menus, battle messages, settings, saves) come in English and Japanese. `language:` in `game.yaml` picks one: `en` (the default) or `ja`. `text:` lays the game's own wording over that language; a key is the engine's Japanese source text, a format with its `%d` and `%s` too, and the value may reorder them (`%[2]d`). [The English catalog](https://github.com/nao1215/gozen/blob/main/internal/text/catalog/english.go) lists every key. `terms:` names the currency and stats. `gozen check` warns when `language` is unset but the game is written in Japanese, and when the regular font lacks letters of the chosen words. [Wayfinder's game.yaml](https://github.com/nao1215/gozen/blob/main/examples/play/game/game.yaml) keeps only its own wording in `text:`.

## Saves

Saves are JSON files in the OS config directory, under the game `id`. They record the schema and data versions; raise `data_version` in `game.yaml` when you change definitions in a way old saves should notice, and the loader reports what it repaired.

## A bigger example

[Wayfinder's world.yaml](https://github.com/nao1215/gozen/blob/main/examples/play/game/data/world.yaml) is 240 lines and uses all of the above: talk, flags, three companions, AI rules, a cut-in, a hand-drawn maze with a chest, a gate and a wheel, and a triggered event.
