# YAML traps

- In a flow map `{...}`, quote a value that holds `, ` or `: `, or starts with `[`, `{`, `*`, `&`, `!`, `%` or `@`: `{say: "Wait, what?"}`. Without quotes the line is cut, and `gozen check` reports GZN1104 or a parse error.
- Text with a `#` after a space starts a comment. Quote it.
- `schema_version: 3` is the version of an RPG: maps (`map:`), painted scenes (`stage:`), dolls and balloons need it, and an older version asks for it. A novel (`ui.message: novel`) needs 4, a campaign (`regions`, `factions`, `campaign`) needs 5, monster collection 6, battle forms and enemy puppets 7, and stacked spaces and `ui.stage_framing` 8; `gozen check` says when the version is too low.
- An ID is the key of a map (`locations: {village: ...}`). Every reference names that key: `start.location`, `exits[].to`, `troops.*.members`, `actors.*.class`, `battle: TROOP`. A typo is GZN1101 with the file and line.
- Expressions are strings: `mhp: "20 + level * 6"`, `when: flag.asked`. Quote one that starts with `!` or holds `: `.
- `data/` may hold any number of files; they are merged. The same ID in two files is an error.
