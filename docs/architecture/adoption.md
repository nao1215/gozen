# Using Gozen from a game

The game owns its data, assets, save location and authoring settings, and uses the public packages and the CLI. `play.Main` takes an FS and the startup arguments and returns failure as an error (`play/main.go`). Ending the process is the job of the game's main. A minimal embedding example is at `examples/minimal/main.go:1`.

## Module and entry points

The using module requires a public version of Gozen. For a local connection before a release, the using module's own go.mod can hold a relative `replace github.com/nao1215/gozen => ../gozen`. Do not add a replace or another module to Gozen's own go.mod.

Use the CLI with `go run github.com/nao1215/gozen/cmd/gozen` or `go install github.com/nao1215/gozen/cmd/gozen@latest`. A using module that wants to keep the development CLI as a dependency can register `cmd/gozen` in a Go `tool` declaration, and `go mod tidy` then keeps the needed dependencies. An unpublished version cannot be installed with `@latest`, so during development use the using module's require and replace with a versionless `go run`.

## Which call does what

| Purpose | Public entry and responsibility |
| --- | --- |
| Read the definitions | `gozen.Load(fsys, dir)`. Name the relative directory inside the FS. Definition errors come as `Diagnostics` |
| Create or continue one play | `gozen.New`, `Begin`; from a save, `gozen.Restore` (a mismatch of version or ID is `ErrIncompatibleSave`) |
| Run commands without a screen | `Session.Do(ctx, yaml)`. Choices and battles are answered by `Config.Policy` |
| Read the state | `Session.Query(expr, &out)` |
| Receive what came out and the effects | `Config.OnRequest`, `Config.OnEffect` |
| Explore branches | `gozen.Explore` and `ExploreOptions` |
| Walk and look at a dungeon | the commands `step`, `turn`, `walk` and the expressions `map`, `cell(x, y)`, `pos` |
| Where saves go | `gozen.Store` (`DirStore`, `MemStore`, your own) |
| Measure a font's width | `Font.Measure` of what `gfx.Load` returns. Do not use a global font setting |
| Embed in your own loop | `play.New(session, play.Config)`. Call `Screen.Close` at the end |
| Start an ordinary game | `play.Main(ctx, fsys, dir, args, play.Config)`. Name the arguments and the output explicitly |

The contracts are defined in the root `game.go`, `session.go`, `query.go`, `explore.go`, `store.go`, in `play/main.go`, `play/play.go`, and in `gfx/font.go` ([ADR 0021](../decisions/0021-query-and-commands.md)). The rules are implemented in internal and not published.

## Saves and development startup

The default save location is `UserConfigDir/gozen/<game-id>`. The save locations of other applications are not searched. A game that continues the saves of an existing work decides the old location in its own startup code, does any needed migration, and passes `--save-dir` for normal play. The game manages the names and rules of the migration.

Scripts, choosing a start point, replay and the like need `--dev`. A script needs `--shots` stated explicitly. A development run that does not state a save location is isolated to a temporary directory. Do not attach the normal-play save location unconditionally to a checking run. When you use external input, state `--external-input` too.

## Checking the tools you brought in

For a trial version, run `gozen trial <input> --out <new output> --list <list>` and check the output's YAML, assets and list with the public `gozen.Load` and FS operations. Use new paths for the output and the list. For text generation, use the structured output of `gozen art prompt --format json`. Neither exposes internal types to the using module.

Generate the memo list with `gozen notes <input> --out <new output>` and read `<new output>/index.html`. `--out` is required. Do not write HTML inside the input. A using module that publishes the list to the same place repeatedly generates it into a fresh working directory each time and replaces the previous list only on success. If generation fails, keep the previous list and the original JSON, pictures and save data. The overwrite protection of [ADR 0012](../decisions/0012-cli-ownership-and-output.md) stays.

Build information of memos is recorded only with an explicit `--dev --build-info`. `trace` is only the base name of the recording file, and `version` is only the revision embedded in the executable, when there is one (`internal/play/memo.go:242`, `internal/layer1/memo/memo.go:224`). The using module's checks also separate not recording by default from recording when asked. Do not require a revision from an executable without build information. Check places, maps, saves and resume, and replay with a trace, apart from the presence of build information.

Work-specific evaluation policy goes in the game's own package and combines the public state, battle, event and progress. The model and settings for authoring support are also stated by the using module. If an existing default is needed, manage it as the using module's setting.

## Checks

Run the game's vet, tests and CLI data checks separately from Gozen's own `make test lint build`. The using module's `go test ./...` does not run the dependency modules' tests. Check the screen following [rendering checks](../development/testing.md#graphics-tests): confirm the real game loop, the end of the script and the generated images. Starting the Xvfb process alone is not a success.
