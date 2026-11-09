# ADR 0064: Named dependency levels

Status: Accepted; implemented in three steps. Date: 2026-11-09. Changes: [ADR 0008](0008-architecture-and-api.md) (the numbered layer directories) and the layer names of [ADR 0010](0010-core-ownership.md).

## Context

The engine's packages were grouped in `internal/layer1` to `internal/layer6`, and `.go-arch-lint.yml` called those groups `private1` to `private6`. A number said where a group stood but not what it held, and the order itself was written nowhere: every `mayDependOn` list repeated by hand the components below its owner, so the order could only be read back from those lists. The packages that had a name (`rpg`, `battle`, `play` and the others) sat outside the numbered directories, so a package's directory did not tell its rank, and `internal/layer6` was declared but empty.

The Linux kernel answers the same need with its initcall levels: each rank has a name (`core`, `subsys`, `device`, `late`), and the names are given their order in one place.

## Decision

The dependency order has six named levels, from primitive values to what the author runs:

| Order | Level | Holds |
| --- | --- | --- |
| 1 | `base` | values, expressions, texts, save bytes, chat and conversation rules, diagnostics, and the CLI and browser I/O they share |
| 2 | `model` | the game's definitions (YAML types, checks, loading) and its state |
| 3 | `rules` | battles, events, places, campaigns, dungeons, planning |
| 4 | `drive` | running the rules without a screen: progress, sessions, simulation |
| 5 | `screen` | drawing, sound, portraits and the player |
| 6 | `app` | commands, the player entry, authoring tools and maintainer tooling |

`internal/dev/hygiene/levels.go` is the only definition of the order and of the level each architecture component belongs to. `.go-arch-lint.yml` keeps the finer rules between components; `go-arch-lint` checks imports against it, and the `0064` decision of `go tool dango lint` checks it against the levels: every component has exactly one level, and no component may depend on a component of a higher level. Components of one level may depend on each other only as `.go-arch-lint.yml` allows.

The engine levels are directories: the packages of `base` are under `internal/base/`, those of `model` under `internal/model/`, and so on, named packages included (`internal/model/rpg`, `internal/screen/play`). The `app` level has no directory of its own: its commands and tools keep the places other decisions give them (`cmd/`, `internal/promptgen` and `internal/mosaic` of [ADR 0004](0004-content-boundary.md), `internal/dev`, `internal/testgame`). The numbered directories and the empty sixth layer are removed and must not return.

The change is made in three steps, each with the same behavior: (1) the level table, the named components and the check; (2) `internal/layerN` renamed to the level directories; (3) the named packages moved into their level's directory.

## Consequences

A new package is placed by choosing its level first; its directory then says the level, and a dependency on a higher level fails `make lint` with the names of both levels. Paths in earlier ADRs and the changelog keep the names they had when they were written.

Alternatives not adopted: keeping the numbers and documenting them (the names would still not say what a level holds, and a document is not checked); generating `.go-arch-lint.yml` from the table (a second tool for a list a test can check); a seventh level for the CLI I/O packages (`diag`, `clihelp` and `cliio` depend on nothing of a game, and `play` uses `clihelp`, so they belong to `base`).
