# Writing and upgrading game YAML

Start a game with `gozen new game --template novel`, `rpg` or `campaign`.
New games use schema 11. Keep the generated `id` stable: it identifies the
player's saves. `data_version` is your own content revision, not the engine's
schema version. Run `gozen check game` after editing.

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
mapping form remains valid for one item too. Migration retains explicit counts
because old command replay fingerprints distinguish them from omission.

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

## Upgrade a versioned game

```sh
gozen check game
gozen migrate game --out game-v11
gozen check game-v11
gozen run game-v11 --dev --save-dir test-saves
```

`game-v11` must be a new directory outside `game`. The command validates the
source and result, copies assets and other files, and publishes only a valid
result. It never modifies the original or overwrites an existing destination.
Keep the original for rollback and review the YAML diff before adopting the copy.
Do not change only the version header: that could enable old disabled tactics.

The migration supports schemas 1 through 11. It writes `enabled: false` when an
old row omitted the switch, shortens events without metadata and item-only
transfers, and preserves `id`, `data_version`, text and command order. Annotated
wrappers, tagged events and explicit counts can remain in their longer form. Anchors and
comments are retained where the transformation can preserve them; formatting
may change. Running migration again produces an equivalent copy without further
YAML changes.

Schema 0/unversioned files still load but cannot be automatically upgraded by
this command: their treatment of numeric zero needs a separate review. Invalid
games must pass `gozen check` before migration. Write `schema_version` as a direct
integer rather than an anchor before migrating. Old engines cannot load schema
11; update both the authoring CLI and exported runtime bundle together.

Replay fingerprints of named commands are preserved by these transformations.
Trace recordings also hash the source file bytes, so record those again after
migration. This distinction applies even to a whitespace-only YAML edit.
Use the same seeds for before/after simulation; for example:

```sh
gozen sim battle game --troop training -n 100 --seed 17 --format json
gozen sim battle game-v11 --troop training -n 100 --seed 17 --format json
```

Replace `training` with a troop in your game. For narrative and workspace games,
use `gozen sim script` with the same script and seed. The repository regression
checks every example, including actual gameplay snapshots, and compares loaded
rules and named replay fingerprints separately.

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
