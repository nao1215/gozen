# ADR 0010: Owner of the core shared model and rules

Status: Partly changed. Corresponds to: T2, D8. "No public dungeon package is added" was changed by [ADR 0014](0014-dungeon-host-adoption.md), which made `dungeon` public. "The YAML type aliases are a public contract", "the sealed `Effect` and `Host.Handle`", and "saving is `rpg.Parse` and `savefile`" are superseded by [ADR 0021](0021-query-and-commands.md) (the owner of the definition types does not change).

## Problem

If loading definitions and state share the definition types, the same `rpg → data` dependency in layer 2 tends to appear. Dungeon movement and arrival handling at a place also need references between rule layers or a shared callback unless their responsibilities are separated. A way of allowing sideways dependencies between public packages is not adopted.

## Decision and alternatives

`internal/layer2/gamedata` owns the definition types, their validation, and YAML decoding. `data` exposes type aliases for game authors, plus `Load(fsys, path)` and `Validate`. `rpg` uses the shared model. The alternative of adding the common types to another public package is not adopted because it increases the entry points users must learn and the public surface. The alternative of creating a huge DB interface just for state is also not adopted.

The shared model in layer 2 needs a YAML library. Because the decoder responsibility previously assigned to `data` moves to the shared model, the same external dependency is assigned to `private2`. No exception between layers is added. The YAML settings and reference collection for assets are also placed in `internal/layer2/assetpolicy`, and `assetfs` in the foundation layer handles only normalized approvals and FS operations.

Dungeon generation and movement are owned by `internal/layer3/dungeon`, and the shared rules for places by `internal/layer3/field`. The public `world` assembles arrival handling, and function registration through `init` is not used. No public dungeon package is added. A generated map is mutable data in the DB, so one run takes the DB exclusively. A process-wide generation cache is removed.

Immediate effects of events are notified as values of the sealed `Effect` through `Host.Handle(Effect)`. Conversations, choices, and battles that need to wait are returned as a `Request`. The alternative of making hosts implement many screen methods is avoided. Suppressing an arbitrary conversation insertion is a separate one-method contract.

## Public contract and checks

The YAML type aliases are a public contract. So that moving a declaration inside cannot hide it from the list, the API check follows the fields and methods of referenced local aliases. The generated list of YAML fields and the schema are published in the [schema reference](https://nao1215.github.io/gozen/schema/) and placed in `data/schema/`. Parser helper functions, the context table for expressions, and the decoder for asset production records are not public.

Saved JSON in memory is read with `rpg.Parse`, reading bytes from an FS is `savefile.Load(fsys, path)`, and preparing the save location is `savefile.Prepare(id, Config)`. Accepting old-format JSON is only by explicit setting. The save directories of other applications are not searched or migrated.

The dependency direction is checked by the existing `go-arch-lint`, the public surface by `api/public.txt`, and behavior by contract tests on a neutral game. For how to run them, see [Development and verification](../development/testing.md).
