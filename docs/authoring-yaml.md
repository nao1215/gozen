# Writing and upgrading game YAML

Start a game with `gozen new game --template novel`, `rpg` or `campaign`.
New games use schema 11. Keep the generated `id` stable: it identifies the
player's saves. `data_version` is your own content revision, not the engine's
schema version. Run `gozen check game` after editing.

For a home screen, contacts, predefined replies and scrollable message history, see [phones unlocked by flags](examples/phone-flags.md).

## A small story without combat setup

Write `game/game.yaml`:

<!-- verify: file game/game.yaml -->
```yaml
schema_version: 11
id: station-story
title: The last train
start: {location: station, event: opening}
ui: {message: novel}
```

Write `game/data/story.yaml`:

<!-- verify: file game/data/story.yaml -->
```yaml
actors:
  guide: {name: Rowan}
locations:
  station: {name: Station}
items:
  ticket: {name: Ticket}
events:
  opening:
    - say: {who: guide, text: There is still time.}
    - give: ticket
    - choice:
        - label: Board the train
          do: [{say: The journey begins.}]
        - label: Stay in town
          do: [{say: You turn toward home.}]
    - clear: true
```

<!-- verify: run -->
```sh
gozen check game
```

A story needs no class, combat stats, attack/guard skills or dummy party.
A speaker can gain a combat class later. Combat games still require valid
party members, classes and their attack/guard skills.

## Choose the shortest form that states your intent

An event without metadata is a list of commands:

<!-- verify: parse fragment of event definitions -->
```yaml
events:
  greeting:
    - say: Welcome back.
    - give: ticket
```

Keep a mapping when the event needs a trigger, condition, one-time marker,
location or typed parameters:

<!-- verify: parse fragment of event definitions -->
```yaml
events:
  first_visit:
    trigger: enter
    location: station
    once: true
    do: [{call: greeting}]
```

`give: ticket` and `take: ticket` mean one item. For another quantity, use
`give: {item: ticket, count: 2}`. An explicit zero transfers nothing. The
mapping form remains valid for one item too.

Tactic rows under `battle.auto_tactics` and `battle.tactic_presets.*.rows` are
enabled by default in schema 11:

<!-- verify: parse fragment; the tactic catalog is defined by the author -->
```yaml
battle:
  auto_tactics:
    scout_ai:
      - {condition: any, action: attack}
      - {enabled: false, condition: low_hp, action: guard}
```

The IDs in this fragment must be defined in the game's tactic catalog. A false
row remains editable but does not run. Old schema versions retain the old
omitted-is-false rule.

## Moving a game to schema 11

Before v1.0.0 the YAML and save formats may change between versions, and gozen
has no migration command. Older schema versions still load with their old
defaults. To move a game to schema 11, change `schema_version` and add
`enabled: false` to every tactic row that omitted `enabled` (in schema 11 an
omitted switch is true), then run `gozen check game` and play it. Old engines
cannot load schema 11; update the authoring CLI and the exported runtime bundle
together.

## Fix errors at their source

The checker includes file, line, column, logical path and diagnostic code.
For a unique nearby spelling it adds a correction hint:

```text
data/world.yaml:3:14: locations.trail.exits[0].to: unknown location "capm" (did you mean "camp"?) [GZN1101]
```

The hint uses the referenced namespace, not every identifier in the game. Equal
candidates receive no guess. The checker never rewrites a typo automatically.
The logical command path retains `.do` even for a compact event, but its source
line points to the command as written. Use `gozen help GZN1101` for the code and
`gozen check game --format json` for structured diagnostics.
