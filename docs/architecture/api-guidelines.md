# API naming and shape

These conventions apply to the v0 implementation. For the implemented contracts and usage of T2, see [the core API](core-api.md).

## Deciding what to publish

The default is `internal/`. A type, function, method or field is published only if you can explain who uses it and why. Helpers the game does not use, functions only tests use, and plain relay interfaces are not published. Review the real usage examples, the godoc contract and the diff of `api/public.txt` together.

## One shape for entry points

| Kind | Shape and meaning |
| --- | --- |
| Loading from an FS | `Load(fsys fs.FS, path string) (T, error)`. `path` is a relative path inside the FS. It accepts `.` when the target is a directory |
| Parsing from memory | Use `Parse`; it does no file search and no OS reads or writes |
| Validation | Use `Validate`; it does not change its input. Ordinary invalid input is returned as an error |
| Creating an instance | `New(..., cfg Config)`. When it can fail, `(T, error)`. Keep the return shape the same across entries of the same kind |
| Settings | Use a small `Config` struct. Do not add functional Options or a `With...` family alongside |
| Long-running, cancellable execution | Put the context first, as in `Run(ctx context.Context, ...) error` |
| Synchronous pure computation, FS loading | Do not add a context. Do not hide the network or background work in loading |

Write the meaning of the zero value of `Config`, its defaults and its allowed values in godoc. Settings are only for the users who need them; do not expose many engine internals or tuning fields. Tell a required collaborator from a setting value, and do not silently correct an invalid setting.

## Errors and side effects

A library does not use `os.Exit`, `log.Fatal` or a panic on invalid input. The error is the last return value, wrapped with `%w` when the cause must be kept. The other return values on error are in principle unusable. If several diagnostics are needed, design the contract as a dedicated validation result, and do not return a half-done state that cannot be told from success.

Exiting the process in a CLI is the job of `main`. stdout is for results and stderr for diagnostics. Do not swallow failures of output, saving or Close. Explain who owns a reader, writer or FS passed to the API, who closes it, and whether the input may be changed. Avoid implicit changes of the working directory and rewriting the process environment.

## CLI

There is a single CLI, `gozen`, installable with `go install github.com/nao1215/gozen/cmd/gozen@latest` ([ADR 0016](../decisions/0016-single-cli.md)). The general subcommands are in `cmd/gozen`; the authoring-support `prompt` and `mosaic` are implemented in the `command` package of each dedicated internal. The inference adapters are split into a cgo build and a build without inference; the `gozen` built without cgo diagnoses the lack when inference is needed and exits non-zero. The requirements are in [the authoring-support CLI](https://nao1215.github.io/gozen/production-tools/). Installing the published version is checked after release. Examples and prerequisites are in [CLI usage](https://nao1215.github.io/gozen/commands/).

Options are GNU style ([ADR 0018](../decisions/0018-gnu-options-and-help.md)). Long names are `--kebab-case`; short single letters exist only for common ones and do not stack within one command. Both `--name=value` and `--name value` are accepted, and a single-dash long name (`-seed`) is an argument error with guidance. Parsing and building help use `internal/clihelp`, not the standard `flag`.

The `--help` of `gozen`, of each subcommand and of the game's startup options (`runner`) has the same form. The first line is "name - one-line description (version)", then `[Usage]`, `[Commands]` only for those with sub-commands, `[Examples]` that really work, `[Options]` grouped by kind, supplementary paragraphs, and last the links to Documentation, Report an issue and GitHub Sponsors. When you add an option, decide which group it goes in (forgetting shows up under `Other:` and fails a test).

One supplementary paragraph starts with `Requirements:` and lists only the external things that subcommand needs. The cgo C compiler, the ONNX Runtime shared library, a user-specified model, Xvfb for screen checks and so on are shown only if the implementation is confirmed to need them. If there are no external prerequisites, say so. Help itself succeeds without inference, screen initialization or loading settings.

The same paragraph points to the matching README section (public URL `https://github.com/nao1215/gozen#gozen`). Give a public URL that can be followed also from an installed copy, and lead from the README section to the per-OS preparation steps. Distinguish the subcommands that need cgo (the inference of `mosaic`, and `faces`) from those that do not.

When an external thing needed to start is missing, print briefly to stderr what is missing and how to prepare it, point to the same README section, and exit with 1. An argument error prints the reason and a pointer to `--help` to stderr and exits with 2. Neither writes anything to stdout. Keep them distinct from a successful help display, and do not hide the real cause of a shared library search or a model read. When you add an implementation, check the form of help, a missing prerequisite and the exit codes with the CLI E2E.

## State and extension

Mutable state, randomness, save location, time and the like are passed as collaborators of the instance, independent from other games and tests. For processing with shared initialization or a main-thread constraint, state the constraint and whether concurrent use is allowed. Separating DB, state and randomness is a different contract from the goroutine safety of the same state.

For extensions on the game side, prefer small interfaces of one or two methods defined by the using side. Do not bundle many roles into one huge Host, and use the same name and argument order for the same role. Do not wrap a concrete type in an interface when it need not be swapped. Pass values the screen needs as lower-level neutral values or small contracts, and do not reference Ebitengine from below.

## Checking changes

`go test ./api` shows the increase or decrease of declared public identifiers independent of OS and build tags. Update it with `go run ./internal/dev/apigen`. Leave the reason for an addition in the change description and review. A list of names cannot guarantee compatibility of types, signatures and meaning, so contract tests and review are needed too. For details see [API checks](../development/api.md).
