# Measuring balance

`gozen sim` fights battles on AUTO with the same rules as the game and prints win rates. Use numbers, not guesses.

<!-- verify: run fixture=cave -->
```shell
gozen sim battle game --troop cave_boss --level 8
gozen sim dive game --location cave --floors 5 --boss 5:cave_boss
```

For a campaign, `gozen sim campaign game` (below).

- `sim battle GAME --troop ID` fights a troop 200 times with the starting party. `--party IDS`, `--level N` and `--hp` change the party and print the HP left after a win.
- `sim dive GAME --location ID` walks a generated dungeon floor by floor and prints levels, HP and boss win rates.
- `sim campaign GAME` plays the campaign 200 times with a strategy for the player (`--as greedy|random|ai`) and prints the win rate, the turns, the regions each side held turn by turn and the orders given. `--tactic ID` marches every general with one tactic; `--compare strategies|tactics` runs every variant on the same seeds (see campaign.md).
- `sim routes GAME` explores event choices without a screen for a novel or any event-driven game: paths to endings, options never offered and flags unreachable. Bound it with `--paths`, `--depth` and `--timeout`; `--fail-unreachable` makes unreachable content exit 1.
- `sim breed GAME` shows monsters reachable from captures and their shortest recipes (monsters.md).
- Every `sim` command supports `--format json|jsonl`. Read the version-1 envelope's `data`; JSONL has ordered items followed by a summary. The format does not change the simulation or file creation. Fatal diagnostics go to stderr as JSONL with the same GZN codes and exit values.
- `sim design GAME` writes report files only with `--out DIR`. Repeat `--only ID` to select multiple personas. `--seed N` overrides their seeds in every output format.
- Runtime errors and timeouts of `sim routes` always exit 1; inspect the report even on failure.
- A standalone `sim battle` does not run `battle.before`/`after`; `sim dive` with `--events` does. `--state FILE` starts from a saved note and `--as-is` keeps its HP; `--streak N` fights again without recovery.
- As an initial design heuristic, try a boss AUTO win rate near 70 to 85 percent and normal fights that leave most HP. These numbers do not establish fairness for human players; confirm difficulty through playtesting.
