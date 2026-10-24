# ADR 0021: Read with expressions, change with commands

Product direction changed by [ADR 0041](0041-yaml-authoring-and-distribution.md). The public APIs described below were retired under that decision; retained implementations and first-party tests now live under internal. The remaining text records the earlier design.

Status: Accepted (2026-10-19). Stage 0 (this ADR), stage 1 (reduce unused public identifiers), stage 2 (additions to YAML), stage 3 (the new root gozen) and stage 4 (play.Main and play.New(session)) are done; stage 5 onward is planned. It partly supersedes [ADR 0002](0002-public-api-scope.md), [0010](0010-core-ownership.md), [0014](0014-dungeon-host-adoption.md) and [0015](0015-reaction-and-chat.md) (see "Superseded decisions" below).

## Principles

### Principle 0: constrained, but easy

Gozen is not a tool that can do anything. It is a framework in which, if you follow a fixed shape (game content in YAML, battle, dungeon and progression in Gozen's shape, a small and few Go entrances), you can do everything from checking, simulation, fixing while playing, saving, to the web build without getting lost (convention over configuration).

The criterion: when unsure whether to add a feature or an API, choose protecting ease even if it adds constraints. Protect "what can be done in a fixed shape without getting lost" over increasing "what can be done". For a request that does not fit the shape, first consider whether it can be expressed with a YAML name and command, and accept only what cannot be through a small entrance (the four below).

### Principle 1: read with expressions, change with commands

All game content is in YAML, and the names of state already exist as expression names (`flag`, `var`, `gold`, `party`, `actor`, `location`, `pos` and so on). The same things are not held twice as Go types. Reading state from Go is done by one expression (`Query`), and changing it by one YAML command (`Do`). Missing reads and writes are added not as Go getters and setters but as expression names or commands in YAML. Loops and function definitions are not added to expressions (the line that keeps YAML from becoming a program).

## Background

There were 1,709 public identifiers (`api/public.txt`, excluding package lines). Counting users' code with types (2026-10-19), the body of the large downstream game uses 6 (start, load, game ID, text), and the remaining users were the game's tests (519), the measurement tools that mirror the CLI (199), the streamer game (94) and the samples (57). 146 were used only by the CLI, and 903 were used by nobody. The readers of the public API were in effect two kinds of people, "those who write tests" and "those who build their own screen", and both used the Go types that mirror the YAML structure (823 definition types, the public fields of state, 11 kinds of wait types, 158 for battle) only to read or slightly change things.

## Verbs the users need

| Purpose | Entrance in the smallest form |
| --- | --- |
| Read and check | `Load`, `Validate`, `Game.Check`, `Game.YAML`, `Game.IDs` |
| Create one playthrough | `New`, `Restore`, `Session.Begin` |
| Run commands | `Session.Do` (to the end), `Start`, `Next`, `Answer` (one step at a time) |
| Run all branches | `Explore` |
| Read state | `Session.Query(expr, &out)` |
| Change state | `Session.Do(ctx, yaml)` |
| See what came out | `Config.OnRequest`, `OnEffect`, `OnLog`, `Request`, `Effect` |
| Fight | The command `- battle:` and `act` (one move of the next battle) and the expression `result` |
| Buy and sell | The commands `buy` and `sell` |
| Walk and look in a dungeon | The commands `step`, `turn`, `walk` and the expressions `map`, `cell(x, y)`, `pos` |
| Save | `Session.Save`, `Store` |
| Show the screen | `play.Main`, `play.New` |
| For streamers | `react`, `chat`, `puppet` (the book is made opaque and narrowed) |

## Treatment

| Current group | Treatment |
| --- | --- |
| Loading and diagnostics (`data.Load` and others) | Keep. Becomes the root `Load`, `Validate`, `Game.Check`, `Diagnostic` |
| Definition types (823 type aliases in `data`) | Removed from the Go promise. Placed in `gozen/schema` and managed separately by `schema_version` (`api/schema.txt`). To read one, take it as YAML with `Game.YAML(path)` |
| Creating state | Keep (`New`, `Restore`) |
| Reading state (fields and getters of `rpg.State` and `Actor`) | Fold into expressions (`Query`) |
| Changing state (`AddItem`, `Join`, `GainExp` and others) | Fold into commands (`Do`). Widen the left side of `set` for what is missing |
| Progression (`headless.Driver`, `event.Runner`) | Fold into `Do` and `Start`, `Next`, `Answer` |
| Waits and immediate effects (17 types in `event`) | One struct each (`Request`, `Effect`). `Kind` uses the same words as the trace JSON |
| Tactics, stone sets, SP | Expressions and commands (no Go types) |
| Saving | Keep (`Save`, `Restore`, `Store`) |
| Battle (`battle`) and observation of battle AI | Not public. Goes to commands, expressions, records (`Kind:"step"`, `"decision"`) and the CLI |
| Branch exploration | Only `Explore` stays in Go |
| Extension hooks (`Host`, `HostFunc`, `AsideBlocker`, `expr.Env` and others) | Fold into four entrances (`Policy`, `Verb`, `Scene`, `Store`) |
| Place entries (`world`) and dungeon (`dungeon`) | Expressions (`menu`, `map`, `cell`) and commands (`pick`, `step`, `turn`, `walk`) |
| Expressions (`expr`) and text (`text`) | Not public (`Query` evaluates, and `game.yaml` decides the language) |
| Assets (`assetfs`) and sound (`sound`) | Not public (internal to the CLI and the screen) |
| Screen and start (`play`, `runner`) | Into `play.Main`, `play.New`, `Config`, `Screen`, `Scene`. `runner` is merged into `play.Main` |
| Drawing parts (`gfx`) | Keep (plus `Font.Wrap`) |
| Reaction (`react`), chat (`chat`), moving portrait (`puppet`) | Keep. The book and script types are made opaque and read with `YAML(path)` |

Estimate: the core (`gozen`, `play`) is around 80 by apigen's counting (34 if only functions and methods), the streamer part around 70, a total of 150 to 170.

## Rules for consistency

| Item | Rule |
| --- | --- |
| Entrance verbs | `Load(fsys, path)` = from an FS, `Validate` = check without changing, `New(x, Config)` = create, `Restore(x, bytes, Config)` = create from a save, `Do(ctx, yaml)` = to the end, `Start`, `Next`, `Answer` = one step at a time, `Query(expr, &out)` = read, `Save()` = a byte slice. No entrance is made with any other verb |
| Naming | The same name is not used with different meanings (`Config` is one per package, and `Request`, `Effect`, `Diagnostic` are only in the root). YAML vocabulary (command names, `Kind`, expression names) and Go vocabulary are aligned, and no Go-only aliases are made |
| Read with expressions, change with commands | State is read only by `Query` and changed only by `Do`. No getters or setters are added to `Session` |
| Errors | An entrance that can fail is `(T, error)`. There are three sentinels: `ErrNotFound`, `ErrInvalid`, `ErrIncompatibleSave`. A positioned error is obtained as `Diagnostics` with `errors.As`. Messages are fixed English sentences. No panic on invalid input |
| Ownership | `Game` is immutable after loading and shareable across goroutines. `Session`, `Mind`, `Log` and `Puppet` are for one goroutine only. `Query` returns a copy. `Request` and `Effect` are value types |
| Concurrency | `play` is for the game thread only. No shared state in package variables |
| context | The first argument of things that loop or can be long (`Begin`, `Do`, `Explore`, `play.Main`, `Policy.Decide`, `Verb`). Not attached to a one-step synchronous call |
| Config | Works with the zero value. Functional Options are not used. A required collaborator is an argument; replacements and notifications go in `Config` |
| String promise | The YAML of `Do` and the expressions of `Query` are exactly the vocabulary of the [schema](https://nao1215.github.io/gozen/schema/). The vocabulary version is managed by `schema_version`, and Go's semver promises only the shape "`Do` accepts YAML" |
| Counting | `api/public.txt` is only for public packages. `schema` is counted separately in `api/schema.txt` |

## The author's decisions

The 9 points of 2026-10-19 (all as recommended):

1. `Request` is one struct (not a sealed union).
2. `react`, `chat` and `puppet` are covered by the semver promise.
3. `Explore` lives in Gozen.
4. Win rate, curves and design evaluation move to the CLI (JSON output), and the downstream tests call the CLI. They do not return to the Go API.
5. Writing abilities and levels directly with `set` is only in `--dev`. With production data, `gozen check` warns.
6. `Scene` and `Verb` get only the declarations first (`screens:` and `verbs:` in `game.yaml`), and the entrances come after users appear.
7. `battle` is not public. If a user who needs their own battle screen appears, start with an experiment.
8. `cmd/gozen` may use internal. The public API is not increased for the CLI.
9. The YAML types are placed in `gozen/schema` and managed separately by `schema_version` (`api/schema.txt`).

The previous 8 points (still valid): the YAML types are a separate promise from Go's semver. `runner` is merged into `play.Main`. `Policy` is an interface. The `react` book is narrowed as well. Tactics, stone sets and SP are narrow windows. No compatibility layer is placed. Text (`text`) is not per `Session`. Ability names are strings (pointed at by name, like `Stat("mhp")`).

## Plan

| Stage | Content | `api/public.txt` |
| --- | --- | --- |
| 0 | This ADR | — |
| 1 | Only reduce. Move to internal or make private the public identifiers that users (the downstream game, the streamer game, `cmd/gozen`, `examples`) do not use. apigen does not count members of private types. Remove the empty `x/` | See "Result of stage 1" below |
| 2 | Add to YAML (the left side of `set`, `pick`, `turn`, `step`, `walk`, `use`, `equip`, the expressions `menu`, `map`, `cell`, `result`, `over`, `repairs`, `tactics`, `verbs:`, `screens:`). `schema_version` 2. Make `Diagnostic` a public type, and `gozen check --format json` | Increases a little |
| 3 | Create the root `gozen` (`Game`, `Session`, `Request`, `Effect`, `Policy`, `Store`, `Explore`). The content is the current core moved to internal and wrapped. The old packages coexist | +65 to 76 |
| 4 | `play.Main`, `play.New(session)`, `Store`. Remove `runner` | +15 |
| 5 | Rewrite the downstream. Remove the mirrors of the measurement tools, and tests go to the CLI or `Explore` | — |
| 6 | Delete the old packages. Make the `react` and `chat` books opaque. Create `api/schema.txt` | 150 to 170 |
| 7 | Extend the CLI's `--format json`, `sim --explain`, `gozen new`, English godoc | — |

No compatibility layer is placed. The coexistence in stages 3 to 5 is only inside Gozen, and the downstream moves all at once in stage 5.

## Result of stage 1

Stage 1 reduced only "what nobody uses". What users use stays until it is replaced in stages 3 to 6.

- Counting method: with `golang.org/x/tools/go/packages`, the downstream game, the streamer game and the whole of Gozen (including tests) were read with types, and for each public identifier the references from "outside users", "`cmd/gozen`", "other Gozen packages" and "its own package" were counted.
- `assetfs` was moved to `internal/assetfs` and `sound` to `internal/layer5/sound` (0 outside users).
- Among the type aliases of `data`, those whose name and members no outside user uses were removed. `cmd/gozen` uses `internal/layer2/gamedata` directly.
- In packages with implementations, functions, methods, types and constants used only within their own package were made private, and those used nowhere were deleted. Methods that satisfy an interface, fields of public types, and types that outside users use through values were kept.
- `internal/dev/apilist` does not count members (fields and methods) of private types. A private type cannot be used by name, and its members are not part of the public promise.
- The empty reserved directory `x/` was removed. Experiments will be placed in `x/` when a concrete user appears.

Counts (identifiers in `api/public.txt`, excluding package lines): 1,709 to 1,322. The breakdown is `data` 854 to 594 (43 type aliases and their members, `Validate`, `CurrentSchemaVersion`), `assetfs` 48 to 0, `sound` 4 to 0, `rpg` 194 to 164, `react` 106 to 90, `battle` 157 to 144, `headless` 73 to 67, `event` 70 to 65, `expr` 63 to 60, `chat` 38 to 37, `play` 22 to 21 (`Run`). The 33 lines for members of private types disappeared through the counting fix. None of the 666 that outside users use was reduced.

Things kept in stage 1 although outside users do not use them (replaced in stages 3 to 6):

- Fields of public types (406), and what other Gozen packages (such as `internal/play`) and `cmd/gozen` use. Moving them requires moving the contents to internal, which is the work of stage 3.
- Types that appear in public fields and function types (`battle.DecisionTiming`, `RuleReason`, `StepKind`, `rpg.StoneLoadout`, `chat.Source`, `Mask`, `react.Analyzer`).
- Sets of constants that do not hold together if only some values are removed (`react.TagEmpty`, `AcrosticPrefix`, `battle.DecisionAction`).
- Entrances kept even in the smallest form (`chat.Load`, `react.Load`).
- `rpg.State.SetHolds`, which tests of other packages use.

## Result of stage 2

YAML vocabulary was added and the spec version became 2. YAML of versions 0 and 1 is read as it is. The added vocabulary can be written only in version 2, and using it in version 1 or earlier makes `gozen check` report an error that "schema_version: 2 is required" (so that, instead of an old engine failing on an item it does not know, it shows as a version difference).

| Kind | Added | Execution |
| --- | --- | --- |
| Commands | `pick: <display name>` (an entry of the current place's menu; the same display name, or the first one that contains it), `turn: left/right/back`, `step: <n>`, `walk: <characters>`, `use: <item>` (the companion receiving it is given by `target`), `equip: {actor, item, slot}` | `event.Runner`. `step` and `walk` run the floor commands and battle at each step and stack the continuation. Arrival, auto events and roamer entry and exit while walking are handled by the screen or headless after the command list ends (the current `headless.Driver.Walk` does so at each step). `use` uses the battle rules, so the screen and the simulation pass it to `Runner.UseItem` |
| Left side of set | `gold`, `item.<ID>`, `party`, `location` (without arrival processing), `actor.<ID>.tactics` (auto, manual, or a preset name). The values of flag, var, gold, item and the number of companions are expressions; location, party and tactics take names as written (`location: camp`, `party: [scout, guide]`) and cannot be written as expressions (this takes the simplicity of needing no quotes), `actor.<ID>.hp/mp/exp/level/sp` | Abilities and levels (hp, mp, exp, level, sp) are written only when `Runner.Dev` is on (the author's decision 5). The screen is on with `--dev`, and headless always. Writing them into production data makes `gozen check` warn |
| Expression names | `menu` (kind, label, new, marked), `map` (list of rows), `cell(x, y)` (char, passable, solid, look, seen, spent), `result` (win, lose, escape of the last battle), `over`, `cleared`, `repairs` (repairs when a save was read), `actor.<ID>.tactics` (mode, preset) | Expressions in event commands (`repairs` and `tactics` anywhere). Not added to `when` of places and floors |
| game.yaml | `verbs: [name]` (own commands. Only declared names are read as own commands, and an undeclared unknown name is an error as before, with a close name attached: `unknown field "sya" (did you mean "say"?)`), `screens: [name]` (own screens opened with `open`) | Declarations only (the author's decision 6). A declared command is recorded at run time as "no implementation" and skipped. A screen is passed to the screen as an `Open` request |

`Diagnostic` (file, line, col, path, message, severity) and `Diagnostics` are made public in `data`, and `data.Load` returns definition errors as `Diagnostics` (`errors.As`; the original error can be followed from `errors.Is` through `Unwrap`). They move to the root `gozen` in stage 3. `gozen check --format json` prints errors and warnings as a JSON array of the same shape.

Public count: 1,322 to 1,343 (+21). 12 of them are the Go promise (`data.Diagnostic` and 6 fields, `Diagnostics` with `Error` and `Unwrap`, `event.Runner.Dev` and `UseItem`), and the other 9 are mirrors of YAML types (7 fields of `data.Command`, `data.Game.Verbs` and `Screens`), which move to `schema` (`api/schema.txt`) in stage 6.

## Result of stage 3

The root package `gozen` was added (`doc.go`, `game.go`, `session.go`, `request.go`, `query.go`, `store.go`, `explore.go`, `errors.go`). The current public packages coexist until stage 6, and users move in stage 5.

| Entrance | Content |
| --- | --- |
| `Load`, `Validate`, `Game` (`ID`, `IDs`, `YAML`, `FS`, `Check`) | `data.Load`. Errors are `ErrInvalid`, and `Diagnostics` can be taken with `errors.As`. `YAML` returns the definition as written by logical path (defaults are not filled in). `Check` checks a command list or one expression (it ignores the spec version limit, because what is run from Go is not a promise about data) |
| `New`, `Restore`, `Session` (`Begin`, `Do`, `Start`, `Next`, `Answer`, `Query`, `Save`, `Clone`) | `event.Runner`, plus the handling of arrival, auto events and roamer entry and exit (in the same order as `headless.Driver`). `Do` runs commands one at a time and settles each time (a note from stage 2: arrival after `pick`, `step` and `walk` runs before the next command). The battle's `Auto` is `headless.AutoBattle` |
| `Request`, `Effect`, `Auto`/`Won`/`Lost`/`Fled` | One struct each (the author's decision 1). The design's `Troop` became `ID` (because it is used for three things: battle group, shop and screen) |
| `Policy`, `DefaultPolicy` | One interface. Embedding `DefaultPolicy` lets one answer only part |
| `Store`, `DirStore`, `MemStore` | `savefile.Write` (replace from a temporary file) |
| `Explore`, `ExploreOptions`, `Path` | `headless.Explore`. The end state is read by `Query` on `Path.End` (a Session) |
| `ErrNotFound`, `ErrInvalid`, `ErrIncompatibleSave` | Three sentinels |

The Go entrances for `Verb` and `Scene` are not placed (the author's decision 6). `Session.Game` was omitted because it has no use. The public increase is 77, one more than the design estimate (65 to 76). The extra one is `Config.Dev` (to uphold the author's decision 5 below in `Session`; added in review).

Construction promises during the coexistence:

- However many `Session`s use one `Game`, what is rewritten as play proceeds (the generated dungeon map) is held in a per-`Session` copy (`gamedata.Fork`). Loading definitions is done once (it takes a few seconds for a large work).
- `gozen` sits on top of the layer 4 `headless` (the component `gozen` in `.go-arch-lint.yml`). In stage 4 `play` uses `gozen`. Physically moving the content to internal is done once in stage 6, when the old packages are removed (moving now would be double work of re-wrapping the old packages with type aliases).
- The author's decision 5 is upheld by `Session`: if `Config.Dev` (zero value is production) is false, a `set` of a companion's ability or level in a command list passed to `Do`, `Start` or `Explore` is `ErrInvalid` (with a Diagnostic), and one inside a work's event is recorded and skipped. `play.New(session)` only follows the session's setting (stage 4). However, while `Explore` runs a path, that `set` inside a work's event is also written (because it wraps `headless.Explore`; `gozen check` warns on the work's data).
- The `Path.Answers` of `Explore` uses the same words as `Answer` (choices are numbers, battles are `Won`, `Lost`, `Fled`). Two rules are not put on the same thing.
- A constraint: the language of the engine's text (language and text in `game.yaml`) is set once per process by `Load`, and a work loaded later affects all Sessions. By the author's decision 7 (text is not per Session) it stays this way for now, and it is written in the godoc of `Load` too.

## Result of stage 4

- `play.Main(ctx, fsys, dir, args, play.Config)` took over the game's main (the start options, help and `--dev` of the current `runner.Run` were moved as they are). `runner.Run` now only calls `play.Main` and is removed in stage 6, after users have moved (the main of the large work still uses it).
- `play.New(*gozen.Session, play.Config) (*Screen, error)`: shows a Session on screen in your own Ebitengine loop or in a browser. A freshly made Session continues from the start event, and a played Session continues from where it is, playing on the Session's state. `Config.Dev` follows the Session. The Session's contents are passed through an internal entrance (`internal/layer4/sessionlink`), without increasing the public API.
- `play.Config` has three fields: `Store` (opens a `gozen.Store` from the work's ID), `Mute` and `Output`. Random numbers, save location, window size, full screen and authoring settings are only in the start options of `play.Main` (`play.Development` was removed). The `Scene` entrance is not placed (the author's decision 6).
- All saving goes through `gozen.Store` (slots, auto, settings, recollections, read marks). The default is the `gozen/<ID>` in the OS user's configuration directory, and in the browser (GOOS=js) it is localStorage (`gozen/<ID>/<name>`). A trial play uses memory (no temporary directory is made). The saved time is shown if the storage place knows it (`ModTime`, not public).
- Wayfinder (`examples/play`) runs with `play.Main` on the desktop and `play.New(session)` on `GOOS=js`. It was started in headless Chrome, the screen appeared, and it was confirmed that the autosave goes into localStorage. js/wasm was added to the CI cross-build.
- Public count: `play` 23 to 12 (-11).

## Result of stage 5b

The tools that every work needs and that the user (rpg's tests) had held by itself were put into Gozen. What could be added by names and commands was added by names and commands, and the Go entrances were limited to three (an additional FS for `Load`, `DefaultPolicy.Prefer`, `ExploreOptions.NoArrive`) and two fields of `Effect`.

| Kind | Added | Execution |
| --- | --- | --- |
| Commands | `buy: {shop, item, count}` and `sell: {shop, item, count}` (the same prices as the shop screen; selling is half price, and a shop with `sell: false`, important items and items with no value cannot be sold), `act: {actor, skill or item, target}` (one move of the next battle fought with AUTO; in the order written, used on a turn when that user can act, and moves left unused are recorded), `equip: {actor}` (without item and slot, the menu's "best equipment" entry, whose Japanese key is さいきょうそうび) | `event.Runner`. `act` is passed to `Runner.Act` and the `Session`'s battle puts it in `StartTurn`. On the screen it is recorded and skipped |
| Left side of set | `pos.x`, `pos.y`, `pos.facing` (north, east, south, west), `steps`, `roamers: none` (remove the current floor's roamers), and the mapping of `actor.<ID>.tactics` (`{mode: manual, rows: [...]}`, the same shape as the saved tactics) | Position, step count and roamers only when `Config.Dev` is on (treated like abilities) |
| Expression names | `ran.<event>` (count is the number of passes, order is the order of the last pass; counted wherever it is run from), `cell(x, y).raw` (the map character without applying `when`), `.shown` (whether it is shown on the auto map), `completed` (the current floor has been walked through), `moved` (the last step advanced), `actor.<ID>.tactics.rows` | `ran` goes into the save (`ran`). `moved` is not saved |
| Go | `Load(fsys, dir, extra ...fs.FS)` (overlay test definitions or a replaced game.yaml), `DefaultPolicy{Prefer: []string}` (choose by display name), `ExploreOptions.NoArrive`, `Effect.Faces` (a picture does not hide the portrait), `Effect.At` (the number of `Requests` that came before it in a `Path`; restores the order of appearance), and the ID of the companion who is the speaker of a say in `Request.ID` | |

Public count: +4 in the root `gozen` (`DefaultPolicy.Prefer`, `Effect.At`, `Effect.Faces`, `ExploreOptions.NoArrive`). The others are mirrors in `data`, `event` and `rpg` that leave the public surface in stage 6 (`Command.Buy`, `Sell`, `Act`, `Ran`, `Runner.Act`, `State.Ran`, `Moved`, `MarkRan`, `Sell`).

## Result of stage 6

The old public packages were taken off the public surface. No compatibility layer was placed (the author's decision 6).

- `rpg`, `event`, `battle`, `headless`, `expr`, `savefile`, `text`, `world` and `dungeon` were moved to the same-named places under `internal/`. `runner` was removed (`play.Main`). The CLI (`cmd/gozen`) is in the same module and so uses internal.
- `data` was named `schema`. These are the Go types of the YAML definitions (`Load` and aliases), following `schema_version`. Identifiers are counted in `api/schema.txt`, not `api/public.txt`.
- The `react` book and the `chat` script became types whose contents are not visible. The contents are read as YAML with `Book.YAML` and `Script.YAML`. The ways of writing characters, rules, asks, dodges and memory, the word types, `Analyzer` and `Script.Validate` were taken off the public surface. `Mind.NewSession` was changed to `Reset`.
- The examples (the end-to-end tests of minimal, headless and play) were rewritten with the `gozen` Session.

Public count: 1,450 to 197 (and 619 more in `api/schema.txt`). The breakdown is `gozen` 84, `react` 44, `chat` 35, `puppet` 17, `play` 12 and `gfx` 5. The difference from the design estimate (about 160) is mainly the fields and constants of `react`'s `Stimulus`, `Reaction` and `Tag` (which users use) and the words added to the root in stage 5b.

## Superseded decisions

- [ADR 0002](0002-public-api-scope.md): the list of packages under the v0 contract and the reservation of `x/`. The public packages are reduced stage by stage according to "Treatment" above, and finally become `gozen`, `play`, `gfx`, `react`, `chat` and `puppet`. `assetfs` and `sound` were removed from the contract in stage 1.
- [ADR 0010](0010-core-ownership.md): "YAML type aliases are the public contract", "`Host.Handle(Effect)` and a sealed `Effect`", and "saving through `rpg.Parse` and `savefile`". The YAML types move to the separate promise of `schema`, effects and waits become one struct each, and saving becomes `Session.Save`, `Restore` and `Store`. That `internal/layer2/gamedata` owns the definition types does not change.
- [ADR 0014](0014-dungeon-host-adoption.md): the publication of `dungeon`. Step-by-step operation is folded into commands (`step`, `turn`, `walk`) and observation into expressions (`map`, `cell(x, y)`, `pos`), and `dungeon` leaves the public surface in stage 6.
- [ADR 0015](0015-reaction-and-chat.md): the publication of the `react` book types (characters, rules, asks, memory) and the `chat` script types. The book and the script become opaque types and are read as YAML with `YAML(path)`. `Analyzer` is not made public until a user appears.

## Alternatives

- The previous design (public 420 to 480, keeping `battle`, `dungeon` and `expr`, and placing `gozen/sim`): it does not answer the author's remark, "although the idea is that YAML decides everything, the API is large". There is no user who draws battle themselves, and none who builds expressions in Go.
- Make `Request` a sealed union: a type switch would show omissions at compile time, but the number of types triples and the vocabulary splits from the trace JSON.
- Put the streamer part in `x/` and take it out of the promise: the count falls to 90, but it does not fit the decision to make the streamer part a selling point of Gozen.

## Weaknesses and remedies

- Spelling and types are not known at compile time: `Query` is checked by the type it receives, and `Game.Check` statically checks expressions and commands. Tests pass the expressions they use through `Check` first.
- Compatibility of the string vocabulary is not caught by Go's semver: the vocabulary is managed by `schema_version`, and `Game.Check` warns on old words.
- Performance: `Session` keeps the parse result per string, and the hot paths (simulation, exploration) run on internal types. To be measured in stage 3 before deciding.
