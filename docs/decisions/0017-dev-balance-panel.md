# ADR 0017: Balance display in dev mode and sharing the win-rate calculation

Status: Accepted. Date: 2026-10-19. This changes, for the win-rate calculation, the statement in [ADR 0012](0012-cli-ownership-and-output.md) that "the battle curve and design evaluation are used only by the CLI, so they live in the CLI's internal".

## Users and problem

The users are authors who make games with Gozen, and the AI that writes YAML on their behalf. In the loop of fixing a definition, playing with `--dev`, leaving a note with F9 and reloading with F5, tuning enemy strength meant reading the text output of `gozen sim` and `gozen curve` in another terminal. The request is to check, on the playing screen, how often the current party wins against the enemies of the current place and at what level it would win, and to have the numbers change as soon as the YAML is fixed.

If the screen and the CLI disagree on the numbers, nobody knows which to trust. They must be counted with the same function and the same random seed. But the win-rate calculation was in `cmd/gozen/internal/balance`, which `internal/play` could not use. The one-battle loop of `gozen sim` was written directly in `cmd/gozen/sim.go`.

## Decision

- Move `cmd/gozen/internal/balance` to `internal/headless/balance`. The `headless` component (layer 4) owns it, and `cmd/gozen` (`sim`, `curve`, `design`) and `internal/play` (layer 5) use the same implementation. No new public API is added. `api/public.txt` does not change either.
- Split one battle of `gozen sim` into `balance.Try`, the aggregation into `balance.Odds.Add`, and the starting state into `balance.Start` (`FreshStart` for a new game, `SavedStart` for save data). The random seed for battle i stays `PCG(seed, i)`. A unit test checks that the output is the same as before the sharing.
- Add to `gozen sim` the options `--state <file>` (such as the `.save` of an F9 note; starts with everyone fully healed) and `--hp` (the 10%, 50% and 90% points of the remaining HP after a win). The default output does not change.
- Addendum 2026-10-19 (the author: "I also want to know the conditions when HP is down"): add `--as-is` (start with the HP, MP and status of `--state` as they are) and `--streak N` (fight consecutively without healing and count the wins; between battles it is returned to one line by `battle.End`). The shared implementation is `balance.Prep.AsIs`, `StreakOne`, `Streak` and `Percentile`.
- F7 in `--dev` toggles the balance display. For each enemy group met in the current place (weighted by encounter, the `battle` of the place's actions, and the group in battle), it counts the win rate, the band of remaining HP and the average turns, equivalent to `-n 200 --seed 1 --turns 60`, starting with the current party fully healed, and draws as a line the win rate (fully healed) from the current level up to 5 levels above. If anyone's HP or MP is down or there is a status ailment, it lines up in a lower row the win rate and remaining HP when fighting as is. It also shows the median number of consecutive wins from the current HP without healing (capped at 9).
- The calculation is split inside `Update` into at most 3 ms per frame. The game's DB and state are not shared between goroutines (because `data.DB` is rewritten by dungeon generation and is not promised for concurrent use). The state uses a copy of the save data and does not change the game's random numbers or state. Everything is measured again on a DB swap (hot reload) and on changes of place, party level, equipment or skill tree. On changes of HP, MP, status or experience, only the part for the current HP is measured again once the values have stayed unchanged for 45 ticks (waiting during battle), so that 200 battles are not rerun at every heal or consecutive battle.

## Alternatives

1. Compute in a separate goroutine. It would not use frame time, but the DB is shared including during reloads, so a copy or a lock would be needed. A deep copy of the DB is large, and a lock stops the game update. It does not pay off for a display used only in development, so it is not chosen.
2. Call `gozen sim` as a subprocess from the screen. The numbers match for sure, but the game's entry point would depend on the presence and location of the CLI, and could not pass the embedded FS or the pre-reload data.
3. Expose it as a public package (such as `x/balance`). There is no use from game code yet, and it does not meet the criteria for going public ([API guidelines](../architecture/api-guidelines.md)). If it becomes necessary, decide in another ADR.

## Consequences

- A calculation shared by the CLI and the screen lives in the `headless` component's internal. The dependency check configuration does not change.
- A frame during calculation becomes up to 3 ms longer. Nothing is calculated while the display is closed.
- Two groups of different strength were added to the Wayfinder sample, and Walker's abilities were made a function of level (the Lv1 values are the same as before).
