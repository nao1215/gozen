# YAML authoring usability review, 2026-11-06

> Note (2026-11-06): the `gozen migrate` command and its migration code described here were removed afterwards. Before v1.0.0 gozen has no migration tools.

## Scope and measurement

This review covers the author-facing schema, loader diagnostics, migration,
examples, documentation and Gozen skill. Runtime game rules and rendering are
outside the change. Inputs were the four principal game examples, authoring
references, architecture rules, ADRs and the no-code migration record.

Counts below cover `game.yaml` and `data/**/*.yaml`, excluding translations,
assets and generated schema. Lines include comments and blank lines. A compactable
event has exactly one key, `do`. An item-only transfer omits its count.

| Example | YAML files | Lines before | Lines after | Events | `do`-only events shortened | Repeated true switches removed | Item-only transfers shortened |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Go Zen | 13 | 2672 | 2647 | 29 | 25 | 51 | 14 |
| Novel | 3 | 290 | 282 | 8 | 8 | 0 | 0 |
| Office | 2 | 193 | 188 | 5 | 5 | 0 | 0 |
| Campaign | 6 | 245 | 241 | 7 | 4 | 0 | 0 |
| Total | 24 | 3400 | 3358 | 49 | 42 | 51 | 14 |

The four examples lose 42 physical lines (1.24%) and 2,302 bytes (1.40% of
164,656). The more useful reduction is structural: 42 of 49 events (85.7%)
lose one metadata-only nesting level, all 51 affirmative tactic switches disappear,
and 14 transfers no longer require an item map. These are source measurements,
not a claim about execution speed or human task-completion time.

There are also 475 `say` commands, 106 containing only `who` and `text`.
The office repeats six people's names in 32 exact scalar values across actors,
planning entities, profiles and cards: 5–6 copies per name, or 26 repeated
values beyond one name per person. This excludes names embedded in prose. Its two source files total 193 lines before this
change, but several lines combine many nested maps. Counting only physical lines
would underestimate that editing burden.

The skill's previous "What every game needs" section required a party, a class
and attack/guard skills for every game. That contradicted the classless-speaker
implementation and the existing novel starter. The full novel showcase still
has its historical guest class and four classed actors; removing them would
change saved party data, so that cleanup is not disguised as a syntax migration.

## Changes

### 1. Metadata-free events are command lists

Before:

<!-- verify: parse before or after comparison; referenced items and tactics are supplied by the game -->
```yaml
events:
  greeting:
    do:
      - say: Welcome back.
      - give: {item: ticket}
```

After:

<!-- verify: parse before or after comparison; referenced items and tactics are supplied by the game -->
```yaml
events:
  greeting:
    - say: Welcome back.
    - give: ticket
```

The loader accepts the compact form only in schema 11. Full event mappings remain
necessary for triggers, conditions and typed arguments. Numeric-presence checks,
complexity warnings and source positions follow the commands in either form.
Canonical `.do` paths remain internal replay roots; diagnostics still point to
the actual authored line. Explicit wrapper comments and custom YAML tags may keep a full mapping.
Source-position aliases use the existing sorted index, avoiding a scan of the
whole file for each compact event.

### 2. Tactic rows are enabled by default in schema 11

Before:

<!-- verify: parse before or after comparison; referenced items and tactics are supplied by the game -->
```yaml
battle:
  auto_tactics:
    scout_ai:
      - {enabled: true, condition: any, action: attack}
```

After:

<!-- verify: parse before or after comparison; referenced items and tactics are supplied by the game -->
```yaml
battle:
  auto_tactics:
    scout_ai:
      - {condition: any, action: attack}
```

This is the behavioral breaking change. Old schemas retain their existing
omitted-is-false rule. Migration supplies `enabled: false` for old omitted
switches and preserves explicit false. It handles both automatic rows and named
presets. The Go zero value and the saved tactic-row representation stay unchanged.

### 3. Single-item commands accept an ID

`give: {item: ticket}` becomes `give: ticket`; `take` follows the same rule.
An explicit quantity retains the full form. In particular, migration leaves
`count: 1` and `count: 0` untouched: the command fingerprint used by saved replay
cursors distinguishes explicit counts from omission. This preserves an existing
contract instead of rewriting every superficially equivalent value.

### 4. Typo diagnostics suggest a repair

Unknown definition fields previously reported only an unknown key, while command
verbs already had a spelling hint. Fields such as `backgroud` now suggest
`background`; ordinary location references such as `capm` suggest `camp`.
The code and source position are retained. Equal-distance alternatives receive
no guess. Suggestions are scoped to the definition's fields or the referenced
namespace and are never applied automatically.

New hints and migration diagnostics use Japanese `lang.T` keys with English
catalog entries. Existing diagnostic prefixes are preserved.

## Upgrade and compatibility

```sh
gozen migrate game --out game-v11
gozen check game-v11
```

Use a new destination outside the input directory. The CLI checks the original,
copies it to staging, rewrites YAML, checks the copy and publishes only on success.
It rejects existing destinations and leaves the original untouched. Game identity,
data version, assets and saves are retained. Original YAML is the rollback copy.
Run the same game scenarios before adopting the copy. Repeat migration to check
idempotence; already migrated YAML is not rewritten again.

Schemas 1–11 are supported by the migrator. Schema 0 still loads but requires a
separate numeric-default migration. Old engines cannot load schema 11; CLI and
export runtime bundles must be updated together. Do not change only the header.
Any source rewrite changes a trace's source-byte fingerprint, so recordings must
be made again. Named command replay fingerprints are checked separately and
remain identical for these transformations.

All nine `examples/**/game.yaml` games, the typed-argument documentation example,
and all three new-game templates were migrated. Historical ADR examples retain
their original version context. The current authoring guide and skill use schema
11 and explain classless narrative games. Images were not regenerated.

## Verification

The migration regression reconstructs equivalent schema-10 forms of current
samples and runs the actual command adapters before and after migration. Every
sample is checked and simulated with seeds 1 and 17, from a real starting
location and its opening event when present. Gameplay records and snapshots are
compared; the source-byte digest and asynchronous music decoder readiness are
excluded. The terminal record's line count is excluded because it includes music
readiness records. This is headless behavior verification, not graphics or audio
verification.

Separate checks compare loaded definitions and every non-nil named replay root.
Focused regressions cover zero/one counts, omitted versus explicit tactic flags,
anchors, comments, empty files, indentation, source positions, schema gating,
unsafe output directories and failed validation. Fuzzing mutates valid and
invalid author files and checks semantic equivalence, source immutability and
idempotence. Early fuzz failures exposed an empty-document panic, compact sequence
indentation errors, standalone CR line boundaries, and parser differences
around comments and custom tags. Each became
an explicit seed regression. Tags keep their full event wrapper; multiline
scalar indentation remains unchanged. When comment parsing is unavailable,
optional compaction is skipped while the version and old defaults are preserved.

A baseline taken before sample migration also compares 100 Go Zen battles and
100 campaigns at seed 17, and the novel's complete route exploration. Their
structured results match after migration. These checks supplement the source
and scripted-state comparisons rather than claiming complete visual equivalence.

Final command outcomes and environment limits are recorded in the external
handoff report. Full test runs include the repository's socket-dependent tests;
those are not disabled or counted as passing in a sandbox that forbids sockets.

## Implementation evidence

Line references below identify the implementation and executable contracts in
this working tree:

| Contract | Evidence |
| --- | --- |
| Compact events and single-item commands | `internal/model/gamedata/types.go:1600`, `internal/model/gamedata/command.go:288` |
| Version-aware tactic defaults and shorthand gating | `internal/model/gamedata/presence.go:237` |
| Unique close-name suggestions | `internal/model/gamedata/definitions.go:398` |
| Source-preserving migration and semantic comparison | `internal/model/gamedata/migrate.go:31` |
| Safe copy publication | `cmd/gozen/migrate.go:17` |
| Every-sample check and seeded simulation comparison | `cmd/gozen/migrate_test.go:20` |
| Loaded definitions and named replay fingerprints | `internal/model/gamedata/migrate_test.go:32` |
| Migration fuzz oracle | `internal/model/gamedata/migrate_test.go:243` |

## Proposals not implemented

- Bind workspace portraits, names and icons to entity data without repeating
  each value in cards and profiles. Review editing and layout needs before
  designing another template language.
- Explore a readable dialogue shorthand. A speaker-key map saves words but can
  silently interpret a misspelled field as a speaker; retain explicit `who` and
  `text` until a form avoids that ambiguity.
- Remove the novel showcase's guest class and dummy party with a separately
  tested save migration. New story templates already avoid this boilerplate.
- Unify author-facing names such as `picture`, `image`, `face` and `background`
  only where their contracts actually match. A mechanical rename would conceal
  different crop, ownership and presentation behavior.
- Add a deliberate schema-0 upgrader that materializes historical numeric
  defaults before adopting versioned zero semantics.

## Suggested commits

1. `feat(schema)!: add compact YAML authoring with schema 11` — version, Event
   and ItemCount decoding, numeric/default handling, schema generation, source
   and complexity handling, and their tests.
2. `feat(cli): migrate versioned games into validated copies` — migration
   transformation, `migrate` command/help registration and migration regressions.
3. `feat(check): suggest nearby YAML fields and references` — definition shape
   diagnostics, reference hints and targeted diagnostic tests.
4. `docs(authoring): migrate examples and document schema 11` — examples,
   starter templates, current docs and skill references, ADR 0056 and changelog.

The English catalog is shared by the first three groups. `definitions.go` is
shared by decoding and hints; stage those hunks with their respective change.
No commits were created in this checkout.
