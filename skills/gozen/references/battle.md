# Battle rules and events

## Events before and after a battle

Name common event IDs in `battle.before` and `battle.after` of game.yaml (RPG, schema version 3 or later). `before` runs just before the fight; if it changes the location, the battle is cancelled. `after` runs once the result branch has reached the next choice or battle, or the event ended; if it changes the location, a pending choice is cancelled. A retry does not run `before` again. An ordinary defeat with no defeat branch goes to the wipe-out handling and `after` does not run. `gozen sim dive` includes these events when `--events` is on. Use `meters[].max_value` (an expression) to give a meter a variable limit.

## Defeat and casualties

A `battle` command raises its defeated participants to 1 HP before `on_lose`. Set `recover: false` to keep their final HP instead; it is independent of `retry: false`, which only removes the retry question. Choose who fights with `fighters`. After every battle the read-only object `last_battle` is available to expressions:

- `last_battle.result` is `win`, `lose` or `escape` (empty before any battle)
- `last_battle.participated.<id>`, `last_battle.hp.<id>` (final HP before recovery), `last_battle.defeated.<id>`
- `last_battle.casualties`, the number of participants at zero HP

Check `participated` when you must tell an absent actor from a defeated one. The record survives saves and the next battle replaces it.

<!-- verify: parse fragment of an event; the troop and actors are the author's -->
```yaml
- battle: expedition_guard
  fighters: [guide]
  retry: false
  recover: false
  on_lose:
    - if: "last_battle.defeated.guide"
      then:
        - set: {flag.guide_lost: true}
        - leave: guide
    - say: "The expedition did not return."
```

## Forms, pictures and motion (schema version 7)

`hp_triggers` transforms a foe below an HP threshold; a self-scoped skill's `merge` combines eligible foes. Enemy `images` lists picture files and actor `images` lists face names at descending HP thresholds. `puppet` on an enemy or actor gives it a moving portrait (puppet.md). Recipes: "Transform an enemy", "Merge enemies", "Change pictures by HP" in the cookbook.

## Recruiting monsters (schema version 6)

`recruit: {chance, level, admission: party_only}` makes a recruit join the active party only (the default `reserve` sends extras to the reserve). The `capture: SPECIES` command recruits a species directly, without the chance roll, and stores `var.capture_result` as `party`, `reserve`, `party_full` or `unavailable`; branch on it. See monsters.md.

## AI history panel

The battle AI history panel is on by default (button, B/PageUp, L1). `ui.battle_ai_history: false` hides it and its input, without changing decisions, messages, rewards or traces.

## Measure

`gozen sim battle` and `sim dive` (measure.md).
