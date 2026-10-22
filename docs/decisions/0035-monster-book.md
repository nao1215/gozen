# ADR 0035: The monster book

Status: Accepted (2026-10-22). Home: `internal/layer2/gamedata` (enemy lore and checks), `internal/event` and `internal/rpg` (saved discovery state and query), `internal/play` (screen), `cmd/gozen` (book output).

## Users and problem

A game author wants a player to recognize the creatures met in play and learn their battle details through exploration. A separate catalog requires duplicate enemy records and can get out of date.

## Decision

### Definitions and setting

`enemies.<id>.lore` is an optional short description. `ui.bestiary: true` offers Monsters in the title and field menu. Both are part of schema version 4. The enemy definition remains the single source of its name, picture, stats, resistances, AI skills and drops.

### Progress

The event runner writes `bestiary.<id>.seen` when it issues a battle request and increments `bestiary.<id>.defeated` by the troop's members after a win. These values live in the existing saved variables. Expressions read `bestiary.<id>.seen` (boolean) and `.defeated` (number); game commands remain the only authored way to change state. A seen page shows name, picture and lore. A defeated page also shows stats, resistances, skills, drops and the count. F4 opens all pages under `--dev`.

### Checks and author tools

When the setting is enabled, `gozen check` warns with GZN0115 for an enemy named by any troop with no lore. Unknown enemy IDs in `bestiary.<id>` expressions are errors. `open: bestiary` requires at least one enemy. `gozen book` includes one page per enemy.

## Alternatives

1. Keep discovery in `gallery.json`: the gallery records pictures and scenes, while enemy progress must be queryable and saved with the game state.
2. Add separate book records: this duplicates details already held by `enemies`.
3. Unlock only after a win: seeing a creature is useful information even when the party retreats.

## Consequences

- No public Go API grows. The `schema` inventory gains `Enemy.Lore` and `UIStyle.Bestiary`.
- Battle discovery applies to the screen, Session and headless event runner because the event runner owns the shared transition.
- `ui.bestiary` is opt-in outside development play; games without the setting keep their existing menu.
