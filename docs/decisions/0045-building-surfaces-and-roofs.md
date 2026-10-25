# ADR 0045: Building surfaces and bounded roofs

Status: Accepted (2026-10-25).

## Context

Continuous towns need distinguishable facades and roof silhouettes. Full-story boxes cannot represent a sloped roof; a picture on a wall cannot occlude an upper-floor view correctly. The renderer already shares terrain intersections with sprite occlusion.

## Decision

Keep grid floors, walls, events and stairs. Add optional named materials, disjoint wall-region assignments, and at most 32 rectangular gable, shed or flat roofs. Materials are opaque decoration; a painted window or door does not change collision or create a sightline. Actual openings remain floor or air cells. Partial-height wall apertures and arbitrary meshes are outside this increment.

Compile roofs into at most 64 inclined slabs. A roof can retain constant vertical thickness or fill down to a horizontal underside to close its gable ends. Each horizontal cell indexes at most four compiled parts, including overhangs. Presentation uploads the static index and planes once per definition change; the common terrain/sprite query intersects only the current cell's candidates. Dungeon collision and interaction consume the same compiled planes. Roofs never become walking support.

Cache face assignments separately from mutable occupancy. Batch named textures into one atlas, rebuilt when loaded image identities change. Existing maps keep their original material path. New textures use bounded 256-pixel tiles with explicit world-space dimensions and origins; the quality bound is documented rather than hidden.

Validate finite bounded dimensions, material references, footprint bounds, part density, roof-volume overlaps and walking/stair headroom. Wall/eave junctions and touching roof edges are allowed. Positive-volume roof/roof intersections are rejected. Runtime footprint collision also blocks entering roof volumes from adjacent elevated areas, including narrow overhangs during grid steps.

## Consequences

Authors can build timber inns, shops with canopies and stone buildings without a model pipeline. Static indexing bounds per-ray roof work and preserves off-anchor visibility. This is additional rendering work, not a performance-neutral feature; compare the same route with boxes, materials, roofs and sprites. It does not provide traversable pitched roofs, transparent windows, arbitrary openings or physics.
