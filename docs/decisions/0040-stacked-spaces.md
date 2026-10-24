# ADR 0040: Bounded stacked spaces

Status: Accepted for implementation (2026-10-23).

## Context

Authors need continuous streets, house interiors, stairs and atriums, while existing single-floor and generated dungeons must retain their behavior. A stacked space is not restricted to one building: upper floors may cover only part of a town.

## Decision

In schema version 8, add optional `dungeon.space` with aligned, equally sized level maps and explicit bidirectional stairs. The initial bounds are eight levels and 64 cells per horizontal dimension. A story is 1.5 cell units high. Spaces (blank cells) have no floor or walls, allowing sky and atriums. Solid cells provide walls. Floors have a 0.15-cell-thick slab below the walking
surface, with visible sides and undersides. Existing tile definitions and commands remain the authoring vocabulary. Generated floors and roaming enemies are not mixed with stacked spaces in this version; validation reports unsupported combinations rather than ignoring them.

Position gains a zero-based level, defaulting to zero in older saves. Each stair joins adjacent levels over a bounded straight run, preserving per-cell stops and reverse traversal. Rules remain in layer 3, definitions and validation in layer 2, and camera interpolation and depth-correct rendering in presentation. Logical steps commit once; rendering interpolates their positions without changing simulation state. Saving preserves the committed cell and level, not animation progress.

Space rendering is opt-in. The existing dungeon renderer, transitions and data continue unchanged without `space`. An open plaza with wall-mounted exterior stairs, an elevated bridge and a roof terrace is the integration demonstration. A captured video must be reviewed before merging. Automated checks cover validation limits, stairs in both directions, forbidden sideways entry, floor-separated records, save/restore and rendered checkpoints, including side views and sky leaks beneath stairs.

## Alternatives

A fade or two-image crossfade cannot represent simultaneous views of several levels. Replacing all dungeon rendering would unnecessarily risk existing games. An unrestricted 3D scene format would greatly expand collision, tooling and asset contracts. Bounded aligned grids retain the existing grid movement model and permit predictable work limits.

Wall, floor and ceiling pictures reuse the existing material fields. Stairs use the floor material on closed stepped volumes; their sides and undersides occlude the view. Direct sunlight is occluded by upper slabs and walls, with ambient fill for interiors. Distance clipping returns fog rather than sky when geometry remains beyond the view range.
