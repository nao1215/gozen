# ADR 0012: Ownership and output destinations of the generic CLI

Status: Partly changed. Corresponds to: T4, D2, D8, D10. Where win-rate calculation lives was changed by [ADR 0017](0017-dev-balance-panel.md), and the option syntax by [ADR 0018](0018-gnu-options-and-help.md). The entry point was changed by [ADR 0016](0016-single-cli.md). `cmd/rpgcheck` and `cmd/rpgtool` became the single `cmd/gozen`, and `cmd/rpgtool/internal` became `cmd/gozen/internal`. What follows is the record at the time of adoption, and the rules for output, exit codes, and overwrite protection carry over to all subcommands of `gozen`.

Partly changed by [ADR 0024](0024-command-tree-and-diagnostic-codes.md): diagnostics carry codes (`GZN` and four digits whose first digit is the exit value).

## Problem

The generic CLI assembles not only checks but also simulation, documentation, partial distribution, and capturing. Making all helper processing a public package adds APIs with no users, and following the way output destinations are recreated risks erasing existing work.

## Decision

`cmd/rpgcheck` and `cmd/rpgtool` own process termination. Common arguments, output, and validation of save destinations are in `cmd/internal/cli`, the structural check that does not draw is in `cmd/internal/checkdata`, and the analysis for partial distribution is in `cmd/internal/trial`. The battle curve and design evaluation are used only by the CLI this time, so they live in `cmd/rpgtool/internal/balance` and `designeval`, and no public `x/` package is added.

All of these belong to an existing entry-point component. The ownership of the shared `cmd/internal/**` is stated in `.go-arch-lint.yml`. No dependency from a lower library to a command and no layer exception is added. Fonts and display rules are used through the existing internal packages of the presentation layer.

The help of every subcommand succeeds without starting a screen, sound, or external commands. Results go to stdout, and warnings, progress, and diagnostics go to stderr. The exit code is 0 for success and help, 1 for failures of I/O, checks, simulation, and capturing, and 2 for malformed arguments. Failures of writing and Close are not turned into success either.

Generated directories are limited to new locations. Existing files, empty directories, and symbolic links are also not overwritten. Containment relations with the input are checked taking into account symbolic links in the existing part and file identity, and paths inside the filesystem are distinguished from OS paths. trial, release, and the HTML of memos are committed from a dedicated work tree to new output. The dry run of trial writes nothing even when `--list` is given.

Design evaluation does not run Git or collect information about the production environment. Memo output is only with explicit specification. The kinds of rewards come from settings, and the categories for capturing are general characters, stages, and compositions. Waiting for approval in capturing exits with 1, and failure logs are left in separate files for stdout and stderr.

## Alternatives and consequences

The alternative of publishing them as an experimental package is put off until a concrete user other than the CLI appears. The settings for generalization are kept, but settings of any work and existing fixtures are not brought in. There is no change to the list of the public API.

The safety of output names and overwrite protection are protection against user mistakes. They are not a transaction that guarantees against a hostile separate process swapping a parent directory at the same time. Running on Windows and macOS is wired as CI for each OS, and is distinguished from cross-builds.
