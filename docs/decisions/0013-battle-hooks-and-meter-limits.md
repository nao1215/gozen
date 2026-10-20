# ADR 0013: Common battle events and state-dependent meter limits

Status: Accepted. Corresponds to: SW1, D2, D8.

## Users and problem

A game author wants to consume exploration resources just before a battle, and decide on a return after the result is applied. Copying the same processing into each battle command causes mismatches with random encounters and double consumption on retry. Also, a resource whose limit changes with abilities or progress should be shown as a pair of current value and limit.

## Decision

Only the following 3 fields are added to the existing public definition types. The owner remains `internal/layer2/gamedata`, and the user-facing names are the type aliases in `data`.

| Field | Who uses it and for what | When omitted |
| --- | --- | --- |
| `data.BattleCfg.Before` | A game author specifies a common event, just before the battle request, for resource consumption or condition checks | Not run |
| `data.BattleCfg.After` | A game author specifies a common event, after the result is applied, for returning or cleanup | Not run |
| `data.Meter.MaxValue` | A game author shows a limit computed from state, together with the current value, on the screen | The fixed `Max` and the existing display are used |

The types and contract are in `BattleCfg` at `internal/layer2/gamedata/types.go:243` and `internal/layer2/gamedata/types.go:325`, and the source of truth for the public surface check is `api/public.txt`. No new public types, functions, or interfaces are created. Events run with the existing `event.Runner`, and runs without a screen or drawing use the existing upper layers, so no layer exception is added.

The order of execution, abort by movement, ordinary defeat, retry, and the lower bound of meters are described in [Battle events and meters](../architecture/battle-hooks.md). The CLI's trial and demo reachability analysis handle the same settings.

## Alternatives

Asking game authors to copy per battle cannot keep encounters and consecutive battles consistent. Adding arbitrary callbacks or many Options adds a new extension point for a use that common YAML events already cover. Adding a display-only callback to meters is also not adopted, because it can be expressed with the existing expressions and state.

## Verification

`headless/battle_hooks_test.go` checks victory, defeat, and escape, choices, consecutive battles, and ending, movement in the pre-processing, and ordinary defeat. `internal/play/battle_hooks_test.go` checks the state on retry, fixed and variable limits, and the input path. Validation of references is handled by `data/battle_hooks_test.go`, partial distribution by `cmd/internal/trial/battle_hooks_test.go`, and measurement by `internal/headless/balance/battle_hooks_test.go`. The schema and the API list are updated at the same time.
