# Continuous stacked spaces

For streets, houses with upper floors, bridges and atriums, set `schema_version: 8` and use `dungeon.space`. Supply 1 to 8 aligned rectangular `levels` (3 to 64 cells per side); `_` is open air. Straight `stairs` have a one-based lower `level`, zero-based `x` and `y`, a `facing`, and `length` 3 to 8. Keep headroom empty and both endpoints unconditional. `pos.level` is zero-based; save/restore, exploration and tile records keep the level.

Use `wall`, `floor_texture`, `ceiling_texture` and optional `space.sky` for materials. A sky panorama wraps horizontally, zenith at the top to horizon at the bottom. Do not combine a space with `map`, `generate`, roamers, themes, door or tile pictures, conditional looks, or slide and spin effects. Ordinary dungeons work without `space`.

Opt-ins and tuning:

- `space.free_move` turns on continuous walking with footprint collision, held-input speed stages and distance-based encounters; Up/Down walk, Left/Right turn, Shift+Left/Right strafe. Saved poses may be off-center. Without it, movement stays on the grid.
- Step and turn durations are set per location, independently, and map and scene walking speeds can be set too. An instant-transition setting for ordinary dungeons avoids ghosting from old frames.
- Buildings: facade materials, closed gable and shed roofs and canopies share collision and sprite occlusion. NPCs and furniture are camera-facing billboards (`tiles.<char>.picture`, `size` in stories, `solid: true` to block the anchor cell).

Read the [complete example and limits](https://github.com/nao1215/gozen/blob/main/docs/examples/stacked-spaces.md), run `gozen check examples/space/game` in the Gozen checkout, and verify movement in both directions in a rendered run.
