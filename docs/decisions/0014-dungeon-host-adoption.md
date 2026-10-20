# ADR 0014: Public surface for assembling custom dungeon actions

Status: Partly changed. Corresponds to: SW2, D7, D8. [ADR 0021](0021-query-and-commands.md) folds the public `dungeon` into commands (`step`, `turn`, `walk`) and expressions (`map`, `cell(x, y)`, `pos`), and it is removed from the public API in stage 6.

## Users and problem

A host game's exploration controller and route evaluation advance one step at a time and observe conditional terrain, moving enemies, one-time actions, and visit records. `headless.Driver.Walk` runs up to a landmark in one go, so the game side cannot make decisions along the way. If the internal implementation is copied, the screen and the game side's route decisions follow different rules.

## Decision

`dungeon` is added as the public entry point of layer 3. The owner of the implementation stays `internal/layer3/dungeon`, and the public surface is limited to the following set needed for the game side's operations. Generation algorithms, drawing, and internal caches for exploration are not public. It is implemented within the existing layer permissions, and no exception is added.

| Public identifier | Who uses it and for what |
| --- | --- |
| `Cell`, and its `X`, `Y`, `Char`, `Tile`, `Look`, `Solid`, `Passable` | A custom exploration controller decides the current position, conditional terrain, the target of an action, and whether a cell can be passed |
| `Of`, `CellAt`, `Front` | A host that uses the map observes the current floor and cells |
| `Dive`, `Place`, `Turn`, `Forward`, `Look` | The game side places on a floor and performs step-by-step actions |
| `Next` | A host that explores automatically chooses the next single action, and makes its own decisions between actions |
| `Seen`, `Shown`, `Spent`, `Completed` | An exploration policy distinguishes visited, shown, used, and floor-completed |
| `Walked`, `Hunters` | A host that assembles actions advances full-area reachability and the update of conditional moving enemies |
| `Trigger`, `Slides`, `SlideEnd` | Route evaluation decides how an action is done and the position and facing after a transfer |
| `BattleMusic` | The game side's regression check confirms the battle music actually chosen from the floor and enemy group |

`DB`, `State`, and the definitions are owned by the caller. A borrowed definition is not modified, and the same state is not operated on concurrently. For an operation that returns a command, the caller runs that command once with `event` or `headless`. The return value of `Next` is `here`, `left`, `right`, `look`, or `forward`, and the internal direction constants are not public.

The priority of battle music cannot be confirmed to match the actual choice by checking only the definition files, and it cannot be observed without a screen through the existing public entry points either. In this migration only the music choice result of `BattleMusic` is made public, and the audio backend and the internal settings of presentation are not.

## Alternatives not adopted

The internal structure of the demo version and the types for authoring support are not public. The game side checks the output and listing of `gozen trial`, and the output of `gozen prompt --format json`. Automatic play evaluation for each work is placed on the game side, using the public state, battle, events, and dungeon. Evaluation callbacks and private evaluation criteria are not added to the library.

For users who check many compositions, `gozen prompt --scenes FILE --format json` is provided. It avoids the cost of re-parsing settings for each composition, and provides batch output without making the internal catalog types public. Required words specific to a work are kept in the user's own catalog.

## Verification

The public surface is `dungeon/dungeon.go:1`, the check of the usage contract is `dungeon/dungeon_test.go:1`, and the identifier list is `api/public.txt`. Internal dungeon tests are compared against movement, save and restore, and event execution from the public entry point. Verification results at the original source and private materials are not put into the public repository.
