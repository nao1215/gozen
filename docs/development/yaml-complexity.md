# YAML authoring budgets

`gozen check GAME` and `gozen check GAME --only structure` report `GZN0122`
when authored YAML becomes difficult to maintain. These warnings suggest a design
review; they do not reject a playable game or change simulation, saves or random
numbers. Text diagnostics contain the source file, line, column, measured value,
budget and a possible improvement. `--format json` returns the same warnings as
structured diagnostics. `--only layout` does not perform structural checks.

Put an optional `check-complexity.yaml` beside `game.yaml`. It is authoring
configuration, outside `data/`, and is not loaded by the runtime. Unknown keys,
wrong types and unreadable configuration fail the check. Missing configuration
uses these defaults:

```yaml
file_lines: 5000
block_lines: 500
repetitions: 8
min_block_nodes: 40
expression_length: 240
expression_depth: 8
screen_operations: 40
indexed_names: 8
unused_definitions: 8
```

Zero selects the default; negative values disable that metric. A negative
`min_block_nodes` disables duplicate detection. Size metrics warn **above** their
budget; repetition, indexed-name and unused-definition counts warn **at** the
budget. For example, a small gardening game can set `screen_operations: 12` to
review a menu as soon as it contains thirteen `set` or `plan` commands. A warning
might suggest moving the shared watering calculation into one event called by
several garden screens. Raising a budget should reflect an inspected authoring
need, rather than concealing generated copies.

| Metric | Measurement and interpretation |
| --- | --- |
| File lines | Physical lines, including comments and blank lines, in the source documents identified by the loader. A final newline does not add an empty line. |
| Block lines | Source span from the definition key to its last value token, including multiline scalar text, for events, screens and planning rules. Trailing comments and blank lines are excluded. |
| Repetitions | A structural fingerprint of each event, screen or planning rule, grouped across files. Mapping keys and scalar kinds remain; scalar values are erased. Numeric/declared-ID families in dotted variable and flag assignment keys are normalized as well. Mapping order is ignored and sequence order remains. Blocks with at least `min_block_nodes` value/container nodes participate. Smaller blocks also participate when their names share a numeric or declared entity/activity ID family; these are grouped by both family and shape. One warning represents a whole group. |
| Expression length/depth | Unicode character count and maximum parenthesis/bracket nesting in schema-typed expressions. Quoted delimiters do not add depth. This is syntactic nesting, not execution cost or binary-operator AST depth. Long prose is not an expression. |
| Screen operations | Number of authored `set` and `plan` command keys within a screen, including branches. Called event bodies are not expanded. |
| Indexed names | Sibling keys whose numeric runs become the same name, such as `plot_1` through `plot_8`. Dates and ID fragments can indicate a manually expanded collection. One warning represents each family. Expression reads and assignment targets also group `var`/`flag` keys by numeric runs and underscore-delimited declared entity/activity IDs. Literal keys count once across reads and writes. Concatenated indexes preserve literal fragments and replace unknown operands with `*`; each distinct source site counts once. Plain dynamic lookups without a literal fragment are ignored. |
| Unused definitions | Events, screens and planning rules without an exact scalar-value reference anywhere in loaded source. Events with triggers are excluded. One warning lists each section's candidates once it reaches the budget. |

The checker rereads source files after validation; keep the game unchanged during
a check. Parse trees remain temporary and are not retained by the runtime.

These are conservative authoring heuristics. Matching shapes are not necessarily
identical behavior: inspect differing values before merging. An alias is counted
where written, never recursively expanded; anchors retain their explicit wrapper.
Duplicate detection currently compares whole definitions, not arbitrary repeated
subtrees. A scalar reference does not prove reachability: self-references, cycles
and a text equal to an ID can hide unused definitions. Conversely, external
scripts may deliberately call otherwise unused events. This check is not a
safe-delete tool. It does not diagnose all unused attributes, variables or assets.
The engine's existing flag warnings still cover unread flag writes.

Review large groups for reusable rules/events, collection operations, templates,
draw tables or iteration. Some abstractions may require engine work; the warning
does not claim those features already exist. Moving copies to another file or
reformatting a long expression into one line does not resolve their structure.

## Local and CI checks

```sh
gozen check examples/workshop/game --only structure --format json
go test ./internal/layer2/gamedata -run TestComplexity -count=1 -v
```

`make test` runs the budget contracts and loads every `examples/**/game.yaml`,
including games in nested modules. The default budgets currently produce no
complexity warnings for those examples. The example test fails on a new budget
overrun, so CI detects growth without changing the warning-only CLI exit status.
The atago check suite verifies plain and JSON warnings, threshold changes,
configuration errors and unchanged source files. The workshop scenario checks
that simulation and month closing retain their existing results.

For example, `var.checked_oak` through `var.checked_willow`, with eight declared
residents, report one indexed-state family even if no variable declarations exist.
`var["booking_" + arg.resident + "_12"]` also contributes to a family. Prefer a
typed resident attribute or a dated reservation over encoding identity in a key.
Quoted prose that merely mentions these expressions is ignored. Dynamic index
patterns are syntactic hints, not an enumeration of runtime keys. Existing
`indexed_names` and `repetitions` budgets and negative disabling options apply.
