# Internal session contracts

`internal/session` owns non-rendering sessions used by first-party integration tests. It sits above the rules and headless layers and below presentation. It is not an application API. Authors use [CLI commands](https://nao1215.github.io/gozen/commands/).

`Load` reads definitions; `New` creates state; `Restore` resumes saved state. `Begin`, `Do`, and `Start`/`Next`/`Answer` advance events. `Query` reads expressions, `Explore` visits branches, and `Save` serializes state. The integration tests exercise choices, battles, translations, campaigns and save round trips.

A loaded `Game` is shared read-only. Each `Session` owns its state, randomness and event cursor and is used from one goroutine. `Clone` copies state and randomness. Callbacks belong to the invocation; rules do not import presentation.

The directory loader is `internal/layer2/gamedata.LoadDir`. It validates the filesystem path, scopes reads to that directory, and returns structured diagnostics. `internal/savefile` owns directory and memory stores, independently of sessions and graphics.

Package relocation does not change YAML schema or save versions. The pre-retirement fixture and CLI restore scenario live in `e2e/atago/fixtures/pre-api-retirement.save` and `e2e/atago/graphics.atago.yaml`.
