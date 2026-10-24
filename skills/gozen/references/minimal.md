# A minimal game

Two files make a game that loads and checks. Write them under `game/`.

<!-- verify: file game/game.yaml -->
```yaml
schema_version: 3
id: first-steps
title: First steps
start: {location: village, party: [hero]}
```

<!-- verify: file game/data/world.yaml -->
```yaml
classes:
  fighter:
    name: Fighter
    stats: {mhp: "20 + level * 6", mmp: 4, atk: "5 + level", def: 3, mat: 1, mdf: 2, agi: 6, luk: 1}
    exp_for: level * 20
actors:
  hero: {name: Hero, class: fighter}
skills:
  attack: {name: Attack, effects: [{damage: a.atk}]}
  guard: {name: Guard, scope: self}
locations:
  village:
    name: Village
    people:
      - name: Elder
        talk:
          - say: The field is quiet today.
          - set: {flag.asked: true}
    exits:
      - {to: field, label: Go to the field, when: flag.asked}
  field:
    name: Field
    exits: [{to: village, label: Back to the village}]
```

<!-- verify: run -->
```shell
gozen check game
```

<!-- verify: output -->
```text
ok: 2 locations, 0 events, 1 actors, 2 skills, 0 items, 0 enemies, 0 troops, 0 ai sets
```

To play it in a window, run `gozen run game`. No Go host is needed.
