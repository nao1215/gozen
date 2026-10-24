# ADR 0007: Embedding a game through the public API

Product direction changed by [ADR 0041](0041-yaml-authoring-and-distribution.md). Existing public APIs remain during migration; retirement is the final stage.

Status: Accepted. Date: 2026-10-18. Corresponds to: D7.

## Context

We need to confirm that game authors can build a game with the public API alone. An example that depends on internal implementation does not verify the public boundary.

## Decision

It is a design condition that the game side can assemble launch, data validation, state transitions, saving and restoring, and the connection of drawing and sound using only the public API. Data, expression, and extensions specific to a work are owned by the game side, and Gozen provides small, generic extension points.

## Reasons

It lets us find gaps in the public boundary through real usage, while avoiding making the specification of one particular work the library's default.

## Consequences

`examples/` is used as a user of the public API to check the contract. When a gap is found, the responsibilities and a small extension point are reviewed before adding to the API. Running on each screen, sound device, and OS is checked separately from tests without drawing and from cross-builds.
