# ADR 0034: Compare campaign strategies and tactics

Status: Accepted (2026-10-22).

## Users and problem

Campaign authors need to compare player strategies and declared clash tactics with repeatable inputs, and need numbers that point to regions whose ownership changes unusually often or whose capture correlates with a win.

## Decision

`gozen sim campaign --compare strategies|tactics` plays each strategy or each declared tactic once for every seed in the requested range. The configured `--as` strategy applies to tactic comparisons; `--tactic` applies to strategy comparisons. Each row reports win rate, mean ending turn and mean player regions held across turns. The region detail reports mean ownership changes per campaign, mean turn of the first ownership change, end holder counts, and player win rate by turn among runs where the player held that region at the start of that turn.

The same result is printed as a table or JSON (`--format json`). These are CLI output fields, not game YAML or public Go API fields. Without `--compare`, the existing output remains unchanged.

## Alternatives

- Require an author to invoke one run per strategy or tactic. This makes matching seed ranges and comparing results error-prone.
- Add region diagnostics to every ordinary campaign report. The author chose opt-in detail so existing output stays stable.

## Consequences

Comparisons cost one full simulation batch per variant. Region win rates are observational correlations, not causal measures of balance.
