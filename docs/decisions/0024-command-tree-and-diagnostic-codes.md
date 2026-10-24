# ADR 0024: A command tree of six tools, and diagnostic codes

Status: Accepted. Date: 2026-10-20. Changes part of [ADR 0016](0016-single-cli.md) (the list of subcommands) and [ADR 0018](0018-gnu-options-and-help.md) (`check --no-layout`, `gozen help` taking one word, `Requirements:` on every page). Adds codes to the diagnostics of [ADR 0012](0012-cli-ownership-and-output.md) without changing its exit values.

## Users and problem

The users are authors who check, measure, document and package a game with `gozen`, and agents that run it for them. `gozen --help` listed 18 commands, and `mosaic` had four more below it. Several of them did the same kind of job with different names:

- `layout` was the second half of `check` (`check` = structure and layout, `check --no-layout` = structure only, `layout` = layout only): one axis spelled as two commands and a negative option.
- `sim`, `curve` and `floors` take the same `<game-dir> <location|troop>` and copied the text of `--seed`, `--turns` and `--format`. `design` also plays without a window.
- `trace`, `memos` and `shots` all handle the record of a real play.
- `release` and `trial` described `--overlay` and `--inspect` in the same words.
- `faces` told the reader to run `gozen mosaic fetch face`.
- Every page repeated the same `Requirements:` paragraph and the exit values.

Diagnostics had no stable name. A message such as `unknown troop "lessn"` could not be searched for or linked to, and rewording it broke the scripts that matched it. The author asked for codes "like atago's" if they are needed.

## Decision

### The tree

```
gozen check   <game-dir>               --only structure|layout, --format text|json
gozen sim     battle|dive|floors|design
gozen play    run|trace|notes|shots
gozen book    <game-dir>
gozen pack    release|trial
gozen art     prompt|mosaic|fetch|faces|puppet
gozen version
gozen help    [<command> [<sub> ...]] | <code>
```

| Old | New |
| --- | --- |
| `gozen check --no-layout G` | `gozen check --only structure G` |
| `gozen layout G` | `gozen check --only layout G` |
| `gozen sim G T` | `gozen sim battle G T` |
| `gozen curve G L` | `gozen sim dive G L` |
| `gozen floors G L` | `gozen sim floors G L` |
| `gozen design G` | `gozen sim design G` |
| `gozen trace F` | `gozen play trace F` |
| `gozen memos D` | `gozen play notes D` |
| `gozen shots G L` | `gozen play shots G L` |
| `gozen release G O` | `gozen pack release G O` |
| `gozen trial G O` | `gozen pack trial G O` |
| `gozen prompt …` | `gozen art prompt …` |
| `gozen mosaic auto\|inspect\|manual …` | `gozen art mosaic auto\|inspect\|manual …` |
| `gozen mosaic fetch …` | `gozen art fetch …` (used by both mosaic and faces) |
| `gozen faces G` | `gozen art faces G` |
| `gozen puppet P` | `gozen art puppet P` |

- No option changes except `check --no-layout`, which becomes `--only structure` (`--only` already chooses a subset in `sim design`, `play shots` and `play trace`). Every other option keeps its name, short name, default and meaning.
- `check` takes only a game directory. Checking a trace (`play trace --check`) or a catalog (`art prompt lint`) stays with its own input.
- `sim` is play by the rules without a window; `play` is the record of the real game with a window. `pack` is not called `build`, which is easily confused with `go build` and `make build`. Mosaic stays under `art` (the author's choice); the game wraps its four-word calls in Makefile targets.
- A group without a command (`gozen sim`) exits 2 and lists its commands on stderr. `gozen sim --help` prints the group page; a leaf page is named by its path (`gozen sim battle`). `gozen help` takes the words of a path (`gozen help art mosaic manual`) and is the same as the path with `--help`.
- The top page holds the GNU option rules, the exit values, the code form and the general requirements once. A group page holds only what its leaves share (`play`: a display and Xvfb for shots; `art`: cgo, ONNX Runtime and models). A leaf page states requirements only when they differ from its parent's.
- No aliases are kept, not even with a warning (unreleased, one user who can update the same day, as in ADR 0016 and 0018). Until 2026-10-20 an old name exited 2 and said the new one (`gozen: unknown command "curve"; it is now "gozen sim dive" [GZN2101]`). That table was removed before the first public version (2026-10-20): no released name ever existed, so an old name is an unknown command like any other.

The `play run <game-dir>` addition (2026-10-24) runs a game directory through the same internal startup implementation as `play.Main`. It shares the runner options and help groups, takes a directory operand instead of `--dir`, and adds no public library API or dependency exception. The library remains available for embedding and custom hosts.

### Exit values

Unchanged: 0 for success and help, 1 when processing fails, 2 for invalid arguments. A name the command line gives that the game does not have (a troop, a location, a persona, a shot id) is an invalid argument and exits 2 in every command; `sim battle` with an unknown troop used to exit 1.

### Diagnostic codes

- A code is `GZN` and four digits. The thousands digit is the exit value (`GZN0xxx` warnings exit 0, `GZN1xxx` exit 1, `GZN2xxx` exit 2). The hundreds digit is the family, what the reader has to fix. Exit values are not widened to atago's 3 to 6: the author's Makefile, tools and tests rely on 0, 1 and 2.
- One code per fix, not per call site. The message may be reworded; the code does not change. Summary lines such as `check: 3 errors` carry no code; the diagnostics they count do.
- The code goes at the end of the line: `data/world.yaml:26:5: locations.grotto.encounters: unknown troop "lessn" [GZN1101]`. The position stays first so editors and existing `contains` assertions keep working. JSON output has `"code": "GZN1101"`, omitted when a diagnostic has none.
- Families: 01 check warnings, 02 measurement and record warnings, 11 game definitions, 12 fonts and layout, 13 simulation, 14 traces, 15 output paths, 16 capture, 17 packaging, 18 pictures and assets, 19 other I/O, 21 command shape, 22 values, 23 combinations, 24 names, 25 help.
- `internal/diag` owns the registry. `register` is the only way to make a `Code`, and it refuses an entry without a summary, detail, fix and version, so an undocumented code cannot exist. It depends on no other component and may be imported by the commands, the shared help, the definitions loader (layer 2), the headless player, and the production tools.
- `gozen help GZN1101` explains a code; no new command is added. An unregistered code exits 2 (`GZN2501`), names the codes of the same family and points to the site page.
- `website/content/errors.md` is generated by `go run ./internal/dev/errgen` and checked for drift. Tests also require that every registered code is raised from non-test code, that a code's thousands digit matches the exit value of the package that raises it, and that every code, except the ones listed with a reason (cgo, a display, the network), appears in an `e2e/atago` scenario next to its exit value.
- Codes are assigned family by family. A diagnostic without a code yet is printed as before.

### Public API

One field: `Diagnostic.Code string` with the JSON name `code` (`gozen.Diagnostic` is `schema.Diagnostic`). Its consumer is a tool or agent that reads `gozen check --format json` or the diagnostics of `gozen.Load` and branches on the problem instead of the wording. The `Code` type, the registry and the explanations are not exported; `gozen help <code>` serves people.

## Alternatives

1. Keep `mosaic` at the top. The top would have seven tools; the author chose `art`.
2. Keep the old names as aliases, with or without a warning. Help, E2E and documents would double, which is what this change removes.
3. Widen the exit values to one per family. The only user relies on 0, 1 and 2.
4. Add an `explain` command. `help` already explains commands, and Gozen has no specs for `explain` to share with.
5. Put the code first, as atago does. Gozen lines start with a position that editors read.

## Consequences

- Every E2E spec, the README, the site and the documents use the new names. The author's game changes its Makefile, tools and tests on the same day.
- `api/public.txt` and `api/schema.txt` gain `field Diagnostic.Code`.
- `.go-arch-lint.yml` gains the `diag` component.
