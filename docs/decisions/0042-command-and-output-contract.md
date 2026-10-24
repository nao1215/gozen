# ADR 0042: Direct commands and structured output

Status: Implemented. Date: 2026-10-24.
Changes the command layout and output contracts of ADR 0012, 0018 and 0024.
The table below records the old and replacement forms. Verification is recorded in [the migration plan](../development/no-code-migration.md).

## Problems found

- `play run` adds a grouping word to the primary task; screenshot capture needs
  a game binary and two ordered path operands (`cmd/gozen/shots.go`).
- Desktop/web/release/trial exports take the output directory as a second operand;
  other commands use `--out`.
- `book --format json` changes the subject from the whole game to monster lineage
  (`cmd/gozen/main.go`).
- `sim design --format json` suppresses file creation, so encoding changes the
  operation (`cmd/gozen/design.go`).
- Prompt batches with `--format json` write multiple JSON documents instead of
  one (`internal/promptgen/command/batch.go`).
- Main, prompt and mosaic commands classify and print errors separately.
- Prompt catalogs define CLI option names dynamically. An agent cannot learn the
  command grammar without first opening a catalog.

## Command names and operands

Frequently used operations have one verb. Specialized groups have at most two
meaningful words. Help may group commands visually without adding syntax.
`GAME` means a directory containing `game.yaml`; it is never a subcommand.

| Current form | Target form |
| --- | --- |
| `new DIR` | unchanged |
| `check GAME` | unchanged |
| `play run GAME` | `run GAME` |
| `play shots GAME LIST --bin BIN` | `capture GAME --config LIST --out DIR` |
| `play trace FILE` | `trace FILE` |
| `play notes DIR` | `notes DIR --out OUT` |
| `pack desktop GAME OUT` | `export GAME --target OS-ARCH --runtime DIR --out OUT` |
| `pack web GAME OUT` | `export GAME --target web --runtime DIR --out OUT` |
| `pack release GAME OUT` | `release GAME --out OUT` |
| `pack trial GAME OUT` | `trial GAME --out OUT` |
| `sim battle GAME TROOP` | `sim battle GAME --troop ID` |
| `sim dive GAME LOCATION` | `sim dive GAME --location ID` |
| `sim floors GAME LOCATION` | `sim floors GAME --location ID` |
| `sim campaign/routes/breed/design GAME` | keep each existing command |
| `book GAME` | keep; formats describe the same book, `--section lineage` selects lineage |
| `theme list` | unchanged |
| `theme add NAME GAME` | `theme add GAME --name NAME` |
| `art prompt [lint]` | `prompt [lint]` |
| `art fetch --list` | `models list` |
| `art fetch MODEL` | `models fetch MODEL` |
| `art faces GAME` | `faces GAME` |
| `art puppet FILE` | `puppet FILE` |
| `art mosaic auto/inspect` | `mosaic auto/inspect` |
| `art mosaic manual ACTION` | `mosaic ACTION` |
| `art mosaic manual check` | `mosaic mark --checked` (writes review state) |
| `version`, `help` | retain |

Retained mosaic actions are `list`, `show`, `view`, `add`, `drop`, `grow`, `skip`
and `clear`. Preserve their image operands: they select input files, not settings
or output destinations. Do not flatten `sim` into one command with a conditional
`--kind` flag and incompatible option sets.

```sh
gozen run ./my-rpg
gozen capture ./my-rpg --config captures.yaml --out promo
gozen export ./my-rpg --target web --runtime web-runtime --out site
gozen sim battle ./my-rpg --troop boss --format json
```

- Options accept `--name=value` or `--name value`, before or after operands.
  `--` ends option parsing. Keep the existing GNU-style parser.
- Reserve `-h` for help, `-o` for `--out`, `-q` for quiet and `-v` for verbose.
  Keep `--version`; retire the conflicting top-level `-v` shorthand.
- `--config` names configuration; `--out` names generated output. Require explicit
  output destinations for capture, exports, release/trial preparation and HTML
  reports. `new DIR` is the creation exception.
- CLI paths are relative to the working directory. Paths inside configuration
  are relative to that file. Game YAML asset references remain game-relative.
  Defaults such as `GAME/designeval.yaml` are stated in help, never found by
  searching parent directories. Explicit `trial --config` becomes working-directory
  relative; the omitted default remains `GAME/trial.yaml`.
- Accept `-` for stream inputs where implemented (trace, scene lists, configuration),
  not for directories. Configuration from stdin uses the working directory as its base.
- New multi-value selectors use repeatable options. Replace catalog-specific prompt
  flags with `--select LAYER=ITEM`; keep the catalog YAML format.
- Existing output refusal, source overlap checks and approval policies remain.
  No generic overwrite flag is added to exports.

## Results for people and programs

`--format text|json|jsonl` selects representation only. A command advertises only
formats it implements; `book` also supports Markdown. Changing format must not
change the selected content, simulation, file creation, or mutations.

- `book` uses one complete model for JSON and Markdown.
- `sim design` writes artifacts only with `--out`, in every format.
- Prompt batches use one aggregate for JSON and item records for JSONL.
- Capture always writes its requested images; format controls the result report.
- `run` needs no artificial JSON summary. Explicit `--trace FILE` records gameplay.

JSON is exactly one document. The CLI output version is independent of YAML and
save versions. Each command documents its `data` shape and uses stable IDs as
well as human labels; empty collections are arrays, not null.

```json
{"version":1,"command":"capture","status":"ok","data":{"items":[],"artifacts":[]}}
```

JSONL uses complete UTF-8 records, each ending with a newline. Emit results in
input/config order and a final summary on ordinary completion, including partial
failure. Missing summaries indicate an incomplete stream.

```jsonl
{"version":1,"command":"capture","type":"item","id":"opening","status":"ok","data":{"png":"opening.png"}}
{"version":1,"command":"capture","type":"summary","status":"ok","data":{"succeeded":1,"failed":0}}
```

stdout holds results only. Progress and fatal diagnostics go to stderr; machine
modes use JSONL diagnostics without ANSI styling or prose summaries. Suppress
routine progress in machine mode. Do not introduce synonymous `--json` or
`--machine` switches.

```jsonl
{"version":1,"command":"capture","severity":"error","code":"GZN2102","message":"..."}
```

Exit values stay 0 (success/help), 1 (validation, I/O, processing or partial batch
failure), and 2 (invalid invocation or explicitly selected unknown ID). Invalid
game content is a check result on stdout with exit 1. Failure before a result
exists leaves stdout empty and reports to stderr. A reported failure must not
also print a duplicate text error. Short writes, broken pipes and close failures
must not return success. Treat IDs and diagnostic codes as stable; messages may
be reworded. Do not silently switch formats when stdout is redirected.

`gozen help --format json` will describe registered commands, operands, options,
types, defaults, accepted values, formats and prerequisites from the same metadata
as text help. It must not require a game, model, catalog or display.

## Internal execution and capture

Dispatch chooses a handler; handlers call typed internal operations. They must
not construct another command's argv, call its parser, or execute another `gozen`
subcommand. Production capabilities stay inside the existing content boundary.
Shared report serialization and error classification belong to CLI infrastructure,
not game rules. Move existing implementations when sharing them; do not add wrappers
or a second output framework.

Capture takes the game's filesystem, typed capture jobs and settings directly.
Run Ebitengine once for the batch; close each internal game before constructing
the next. Wait for pending Draw/screenshot writes and trace closure before judging
the result. Keep state, random seeds and saves isolated per job. PNG and animated
GIF use this same rendering path. A YAML job names its start/event/line, setup
operations, wait and animation interval. Define and test GIF size, frame-rate and
length bounds before exposing them in help.

Do not substitute `exec gozen run` for the old `--bin` subprocess. In-process
timeouts are cooperative, not a promise to terminate a stalled graphics driver.
Ordinary games need no `--raw`; configured approval policies still gate assets
actually shown. Failed jobs remain visible in the report and cause exit 1.

## Rollout and verification

Finish the sample-player migration first, then implement six reviewable batches:

1. Shared result/error contract, exercised by real commands such as `check`, `new`
   and `theme list`.
2. Primary command names, export dispatch and named input/output selectors.
3. Direct capture with PNG/GIF; remove `play shots` and its subprocess path.
4. Consistent analysis, book and trace results.
5. Production command flattening, prompt selectors, model listing and output.
6. Machine-readable help and a complete CLI/E2E/documentation audit.

Public Go API retirement follows these batches. Remove old dispatchers and update
callers, tests, help, README, site and skills in each batch; do not keep aliases or
unused compatibility wrappers. Publish the migration table with the CLI-breaking
release. YAML game definitions and saves retain their contracts.

Test option order, paths with spaces or leading dashes, missing inputs, unknown
options, stdin, invalid format, output overlap, partial batches and failed writes.
Parse actual JSON as one document and JSONL line by line; assert stdout/stderr
separation and exit values. Compare artifacts and domain results between formats.
Capture E2E must cover multiple jobs in one process, isolated saves, scene/line
selection, nonempty images, animated frames, approval failures and cancellation.

## References and alternatives

The [GNU command-line conventions](https://www.gnu.org/prep/standards/html_node/Command_002dLine-Interfaces.html)
support long options, help/version and named output destinations. Flexible option
placement is a GNU extension, not strict POSIX behavior. See also the
[POSIX utility conventions](https://pubs.opengroup.org/onlinepubs/9699919799/basedefs/V1_chap12.html),
[GitHub CLI structured output](https://cli.github.com/manual/gh_help_formatting)
and [JSON Lines](https://jsonlines.org/).

The command-depth limit and versioned result contract are Gozen design choices.
Keeping the old tree would preserve scripts but retain the reported ambiguity.
Renaming everything at once would make capture lifecycle changes harder to review.
The phased approach deliberately breaks CLI spellings once per migrated surface
while preserving game data and testing each replacement before API retirement.
