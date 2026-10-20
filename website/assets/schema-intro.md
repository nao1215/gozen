Every type and field Gozen reads from `game.yaml` and the files under `data/`, grouped by topic. For a short, copyable example of a common task, see [the cookbook](/cookbook/). `game.yaml` is a [Game](#game); each file in `data/` is a [DataFile](#datafile). For how the pieces fit together, start with [Writing a game](/writing-a-game/).

How to read the columns:

- Description says what the field is for.

- Default is the value used when the field is left out. Where `schema_version: 0` and `1` differ, both are given (v0 and v1). Version 2 reads like 1.
- When 0 says what an explicit `0` means for a number; `-` marks fields that are not numbers. With `schema_version: 1`, leaving a number out and writing `0` are different things; v0 reads old files and treats them alike for many fields.
- Required lists when the field must be present, and the fields it cannot be combined with.
- Refers to names the IDs the value must match, such as `troops` or an image in `assets/`. `gozen check` resolves every one of them.
- An expression is a string such as `level * 30` or `flag.trained && !flag.marker_lit`. Left out, a condition holds and a formula is 0.

Every field below is accepted by schema versions 0, 1 and 2, except the ones version 2 added: the commands `pick`, `turn`, `step`, `walk`, `use` (with `target`), `equip`, `buy`, `sell` and `act`, and `verbs` and `screens` in `game.yaml`. They need `schema_version: 2`. Likewise the words of maps and painted scenes (`map`, `stage`, `doll`, `bubbles`, `spot`) need version 3, and the words of novel games (starting with `ui.rollback`) need version 4. The same contract is published as JSON Schema in [data/schema/](https://github.com/nao1215/gozen/tree/main/data/schema), which editors can use for completion; `gozen.Load` and `gozen check` remain the authority on references and expressions.
