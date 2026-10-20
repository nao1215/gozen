# ADR 0018: GNU-style options and a uniform help format

Status: Partly changed. Date: 2026-10-19. Applies to the `gozen` of [ADR 0016](0016-single-cli.md) and to the game start options received by `runner.Run`. The output destination, exit values and overwrite protection of [ADR 0012](0012-cli-ownership-and-output.md) do not change.

Partly changed by [ADR 0024](0024-command-tree-and-diagnostic-codes.md): `check --no-layout` is `check --only structure`, `gozen help` takes the words of a command path, and requirements are written once per page level.

## Users and problem

The users are authors who make games with Gozen, and people who run an author's game with `--dev`. The `gozen` and authoring-support subcommands so far used Go's standard `flag`, which accepts both `-seed 7` and `--seed 7` and does not distinguish short names from long names. `-n`, `-v` and `-o` had different meanings per command, help followed the layout of `flag.PrintDefaults`, and examples and prerequisites were written differently for each command. It also did not match the author's other tools (GNU-style `--name`, help grouped by kind).

## Decision

- Options are GNU style. The implementation is `github.com/spf13/pflag` (v1.0.10, the same line as the author's other tools). Long names are `--kebab-case`, and only commonly used options get a one-character short name (`-h` help, `-v` version or verbose, `-n` count or dry run, `-o` output destination, `-c` the catalog of prompt). Short names are not duplicated within one command.
- Both `--name=value` and `--name value` are accepted. Short names accept `-n 3` and `-n3`. Options may be placed before or after positional arguments, and everything after `--` is positional.
- Long names with a single dash such as `-seed` are not accepted. pflag reads them as a run of short names (`-out DIR` as `-o ut DIR`), so they are detected before parsing, the message says "use `--seed`", and the exit value is 2. No compatibility with the old syntax is kept (unreleased).
- Names that changed: `sim -n` is `-n, --battles`, `sim -tree` (default true) is `--no-tree`, `floors -nomap` is `--no-map`, `trial -n` and `mosaic auto -dry` are `-n, --dry-run`, `-v` of `sim` and `design` is `-v, --verbose`, and `mosaic manual view -o` is `--view-out` (`-o` is `--out`).
- Help has the same shape for every command. The first line is "name - one-line description (version)", followed by `[Usage]` and a description, `[Commands]` only for commands that have a top level and subcommands, `[Examples]` that actually run (2 to 5 per command), `[Options]` grouped by kind (short name, long name, value name, a description wrapped at 80 columns, and a meaningful default), supplementary paragraphs (including `Requirements:` that states prerequisites), and last the links Documentation, Report an issue and GitHub Sponsors. An option forgotten from every group appears under `Other:`, which a test detects.
- The top level has `-h/--help` and `-v/--version`. `gozen help <command>` prints the same as `gozen <command> --help`.
- The assembly lives in `internal/clihelp` and is used by `cmd/gozen`, `internal/promptgen/command`, `internal/mosaic/command` and `runner`. The dependency check gains a `clihelp` component and a `pflag` vendor. `clihelp` depends on no other component.
- The standard `flag` is not used inside Gozen. The `-update-schema` of the schema update test became the environment variable `GOZEN_UPDATE_SCHEMA=1`.

## Alternatives

1. Keep the standard `flag` and only tidy help. Both `-seed` and `--seed` would pass and short and long names could not be separated, so it is not chosen.
2. Adopt a command framework such as cobra. `cmd/gozen` already owns subcommand dispatch and exit values and the help shape is unified on our own, so the benefit of adding a dependency is small and it is not chosen.
3. Leave only the game start options on the standard `flag`. A `runner` user's main only passes `Config.Args`, and the `flag` types are not in the public API. The syntax would disagree with `gozen`, so `runner` is aligned too.

## Consequences

- The public API of `runner` (`Config{Args, Output}`) does not change. Callers that started a game with a single dash (such as `-dev`) must change to `--dev`.
- Exit values (0 for success and help, 1 for a processing failure, 2 for a bad argument) do not change.
- `pflag` becomes a direct dependency. It is BSD-3-Clause and has no other dependencies.
