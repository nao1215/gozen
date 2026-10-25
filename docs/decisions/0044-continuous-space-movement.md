# ADR 0044: Continuous movement on stacked grids

Status: Accepted (2026-10-25).

## Context

Authors need first-person walking, continuous turning and held-input acceleration while retaining grid-authored rooms, stairs and tile events. Interpolating a precommitted grid step cannot stop within a cell. Checking only the final cell would skip collisions and events.

## Decision

Opt in with `dungeon.space.free_move`. Existing spaces and ordinary dungeons retain grid movement. Layer 3 owns a fixed 60 Hz controller and collision/event queries; presentation supplies input and draws its authoritative pose. State saves horizontal coordinates, yaw, support level and fractional distance. Integer position remains a projection for existing expressions. Support comes from connected floors and stairs, never rounded camera height. The player cannot jump, fall or fly.

Use a circular footprint, bounded movement segments and grid-boundary traversal. Check dynamic solids and headroom before committing each segment. Resolve blocked motion along a free axis for wall sliding. Simultaneous corner crossings enter the diagonal cell; cells touched at only a point do not receive entry events. Dispatch entries in order and stop immediately when commands block or replace the position. Recheck geometry after synchronous commands. Bump fires on contact onset; interaction uses continuous yaw, bounded reach and occlusion rather than cardinal rounding.

One legacy step consumes one cell-length of actual travel. Encounter distance accumulates only while the current encounter rate is positive, and leaving an eligible area clears that residual. Cell entry and distance accounting are independent. Turning, idle time and blocked pushing consume neither budget.

Walking speed and turn sensitivity are separate. Optional ordered hold-time stages select speed targets, with a configurable interpolation duration. Release, reversal, complete blockage, focus loss, blocking UI, map transfer and restore reset transient acceleration. Sliding accumulates hold time only while movement succeeds. Cardinal projection uses hysteresis; crossing a cardinal boundary does not reset walking acceleration.

Old saves initialize at the cell center and cardinal yaw. New saves preserve off-center and stair positions but never velocity. Explicit placement and scripted grid steps reset continuous motion; headless tile commands remain usable. Capture inputs and traces include held controls and continuous pose. Unsupported authoring combinations fail validation.

## Alternatives and consequences

A separate arbitrary-mesh world would replace the existing authoring model and require new collision, editing and event systems. Presentation-only interpolation would not make simulation continuous. The selected controller preserves the grid model but introduces bounded nearby-cell and stair queries on Update. Profile those costs independently from rendering, and retain the existing renderer and movement defaults.
