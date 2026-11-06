# YAML traps

- In a flow map `{...}`, quote a value that holds `, ` or `: `, or starts with `[`, `{`, `*`, `&`, `!`, `%` or `@`: `{say: "Wait, what?"}`. Without quotes the line is cut, and `gozen check` reports GZN1104 or a parse error.
- Text with a `#` after a space starts a comment. Quote it.
- Start new games at `schema_version: 11`. Older minimum versions describe feature availability, not genre presets: schema 3 adds maps (`map:`), painted scenes (`stage:`), dolls and balloons need it, and an older version asks for it. A novel (`ui.message: novel`) needs 4, a campaign (`regions`, `factions`, `campaign`) needs 5, monster collection 6, battle forms and enemy puppets 7, and stacked spaces and `ui.stage_framing` 8; `gozen check` says when the version is too low.
- An ID is the key of a map (`locations: {village: ...}`). Every reference names that key: `start.location`, `exits[].to`, `troops.*.members`, `actors.*.class`, `battle: TROOP`. A typo is GZN1101 with the file and line.
- Expressions are strings: `mhp: "20 + level * 6"`, `when: flag.asked`. Quote one that starts with `!` or holds `: `.
- `data/` may hold any number of files; they are merged. The same ID in two files is an error.

## Compact authoring and migration (schema 11)

<!-- verify: parse fragment; ticket and station belong to the author -->
```yaml
events:
  greeting:
    - say: Welcome to the station.
    - give: ticket
  first_visit:
    trigger: enter
    location: station
    once: true
    do: [{call: greeting}]
```

`give: ticket` is the same as `give: {item: ticket}`. For quantities, use
`give: {item: ticket, count: 2}`; zero remains zero. Tactic rows omit
`enabled: true`; write `enabled: false` to disable a row.

For an existing game, run `gozen migrate game --out game-v11` and
`gozen check game-v11`. The output directory must be new and outside the source.
Play the same scenarios and simulations before replacing the original directory.
The ID and data version remain unchanged. Keep the original as the rollback copy.
Explicit counts and annotated wrappers can remain in the full form to preserve
replay fingerprints and comments. Earlier schemas still load with their old
defaults; simply changing the header can enable old disabled tactics. Schema 0
is not upgraded automatically. New schema-11 data needs the updated engine.
