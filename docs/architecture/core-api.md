# Using the non-rendering core and its public contract

This is the API for a game author to load definitions, advance a play, and check events, battles and saves without a screen. The only entry is the root package `gozen` ([ADR 0021](../decisions/0021-query-and-commands.md), "read with expressions, change with commands"). For display, sound and startup see [the presentation API](presentation-api.md); for the `gozen` CLI see [the commands](https://nao1215.github.io/gozen/commands/) and [the production tools](https://nao1215.github.io/gozen/production-tools/). A runnable example is `examples/headless/main.go`, and the test that checks it is `examples/headless/main_test.go`. The walkthrough for game authors is in [Go API](https://nao1215.github.io/gozen/go-api/#test-the-game-headless).

## What is published

`api/public.txt` lists identifiers. The implementation of the rules (state, events, battles, dungeons, expressions, the save format, wording) is in `internal/` and is not published as Go types. You read with an expression (`Session.Query`) and change with a YAML command (`Session.Do`).

| Package | Users and purpose | Entry of the contract |
| --- | --- | --- |
| `gozen` | Game authors, checking tools and custom screens load definitions, advance one play and try branches | `Load`, `Validate`, `Game`; `New`, `Restore`, `Session` (`Begin`, `Do`, `Start`, `Next`, `Answer`, `Query`, `Save`, `Clone`); `Request`, `Effect`; `Policy`, `DefaultPolicy`; `Store` (`DirStore`, `MemStore`); `Explore`; `Diagnostics`; `ErrNotFound`, `ErrInvalid`, `ErrIncompatibleSave` |
| `schema` | Authoring tools read definitions as Go types | `Load` and aliases of the definition types. It follows the YAML promise (`schema_version`) and is counted separately in `api/schema.txt` |

## From loading to saving

1. `gozen.Load(fsys, dir)` gives a checked work. A definition error is `ErrInvalid`, and `errors.As` gets `Diagnostics` (file, line, col, path, message, severity). Layering an `extra` FS adds test definitions or a replaced `game.yaml`. `Validate` returns warnings too, and `Game.Check` checks one command list or one expression.
2. `gozen.New(g, cfg)` makes a new play, and `Begin` runs the start events and the first arrival. To continue from a save use `gozen.Restore(g, save, cfg)`, which does not run the start and arrival events again. A save whose version or work ID does not match is `ErrIncompatibleSave`.
3. `Do(ctx, yaml)` runs commands one at a time, and after each one settles arrival, automatic events and the coming and going of strong enemies. Choices and battles are answered by `Config.Policy` (the default is the first choice and AUTO battles). A screen goes step by step with `Start`, `Next` and `Answer`.
4. `Query(expr, &out)` reads the state (`gold`, `actor.hero.level`, `ran.<event>.count` and so on; the names are in the [schema](https://nao1215.github.io/gozen/schema/)).
5. `Save()` returns JSON. The place is a `Store` (`DirStore` writes atomically to a directory, `MemStore` is memory).

## Ownership and extension

`Game` does not change after loading and can be shared between goroutines. `Session` holds a copy of the definitions, the state, the randomness and the commands being run, and is used from one goroutine. `Clone` is a copy of state and randomness. Immediate effects (movement, music, pictures, sound) reach `Config.OnEffect`, the record of what is shown on screen reaches `Config.OnRequest`, and non-stopping errors reach `Config.OnLog`. `set` of ability, level and position can write only when `Config.Dev` is on.

`Explore` is a copy of the play that branches each choice and each battle result once. It is not an exhaustive search of all combinations. The zero setting is at most 300 paths and 20000 steps per path. The end of each path is the Session of `Path.End`, read with `Query`.

The language of the engine's wording (`language` and `text` of `game.yaml`) is set once per process by `Load` ([ADR 0019](../decisions/0019-engine-languages.md)).

## Assets and the scope of examples

Swapping assets is not published and lives in `internal/layer1/assetfs` (used by the distribution, shooting and trial commands of `gozen`).

The sample's definitions, lines and 8x8 single-color images were newly created for this repository, and the images are generated with the Go standard library. No third-party assets are used. The repository's MIT license applies.
