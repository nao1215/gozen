# ADR 0002: The v0 public contract

Status: Partly changed. Date: 2026-10-18. Corresponds to: D2. The list of public packages and where experiments live changed as described in "Changes made by later ADRs" below. [ADR 0021](0021-query-and-commands.md) supersedes the list of public packages and the reservation of `x/`. [ADR 0041](0041-yaml-authoring-and-distribution.md) subsequently retired all public application packages and their inventories. The text below records the historical contract.

## Context

If internal implementation is made public, tuning code also becomes subject to compatibility. Features that game authors need must be separated from experiments.

## Decision

The v0 contract covers `data`, `rpg`, `battle`, `event`, `expr`, `world`, `headless`, `savefile`, `text`, `assetfs`, `sound`, `gfx`, `play`, and `runner`. The individual types and functions made public are narrowed further, and the rules are described in godoc.

The entry point that receives the embedded game FS is called `runner`. The name avoids confusion between the library's launch API and the package for the CLI.

`adv`, `village`, the board-based `tactics`, `designeval`, and `balance` live in `x/` as experiments, with no compatibility promise. The v0 contract comes first, and experiments are considered starting from what a concrete user needs. The mechanisms for collecting development information and limited distribution handle only the generic part.

## Reasons

It fixes which responsibilities are treated as stable, and lets development proceed without adding APIs nobody uses. Reserving a place for experiments is not a promise to provide an implementation.

## Consequences

`dungeon` is part of the layer classification but is not included in the current public contract. Its implementation prefers internal. For the list of public decisions and layout, see [Boundaries](../architecture/boundaries.md).

## Changes made by later ADRs

The decision in this ADR is the record at the time of adoption. The following points were changed by later ADRs.

- `dungeon` was added to the contract by [ADR 0014](0014-dungeon-host-adoption.md) as the public entry point of layer 3. "is not included in the current public contract" in "Consequences" above is the state at the time of adoption.
- `react` (layer 3) and `chat` (layer 1) were added by [ADR 0015](0015-reaction-and-chat.md).
- `puppet` (layer 5) was added by [ADR 0020](0020-moving-portraits.md).
- Win-rate calculation (`balance`) lives in `internal/headless/balance` instead of `x/`, and is shared by `gozen sim` and the development balance display ([ADR 0017](0017-dev-balance-panel.md)). Design evaluation (`designeval`) is in `cmd/gozen/internal/designeval`. Whether to make it public will be decided in a separate ADR.

The policy of narrowing the public types and functions, and the policy of promising no compatibility for experiments in `x/`, have not changed.
