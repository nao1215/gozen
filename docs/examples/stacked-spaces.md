# Continuous stacked spaces

`dungeon.space` lets a location contain a street, interiors and upper floors in
one volume. Ordinary dungeons without `space` keep their existing renderer and
floor transitions. The standalone demonstration starts with:

```sh
go run ./examples/space --dev --mute
```

Use Up to walk, Left and Right to turn, and Down to turn around. The demonstration
has an open plaza, two wall-mounted exterior stair runs, an elevated bridge,
a roof terrace, and ground-floor space beneath the upper staircase. [Preview video](../media/stacked-space-preview.mp4) and
[mobile GIF](../media/stacked-space-preview.gif) are captured from the actual game.
The visual design is still under review; these are not README showcase images.

## Authoring

See the complete [example world](../../examples/space/game/data/world.yaml).
`space.levels` holds equally sized rectangular maps from bottom to top. A period
is floor, `#` is a full-height wall, and `_` or a blank cell is open air. Prefer
underscores because YAML block indentation can make leading blank rows ambiguous.
Tile definitions supply the start position and ordinary tile behavior. Leave
upper cells empty for atriums and the headroom above stair runs.

`space.stairs` describes straight runs with `level`, `x`, `y`, `facing` and
`length`. `level` is the one-based lower level; coordinates are zero-based.
The upper landing is `length` cells along the direction from `(x,y)`.
Both directions work, including stopping and turning back partway up. Side entry
into an intermediate stair cell is blocked. Logical movement commits once per
cell; the camera interpolates between positions without a scene transition.

Limits are 1–8 levels, 3–64 cells in each horizontal dimension, 128 stair runs,
3–8 cells per run, and view depth at most 16. A story is 1.5 cell units high.
A run must consist of unconditional walkable cells with no commands, with open
headroom and a walkable upper landing. Overlapping runs are rejected.

Wall, floor and ceiling textures use `wall`, `floor_texture` and
`ceiling_texture`, including their tint and brightness. Stair treads, risers and
closed sides reuse the floor material. Floors extend 0.15 cell units below their
walking surface (one tenth of a story), exposing solid edges and undersides. A flight occupies the volume between its
base story and its treads: the lower story beneath an upper flight remains
available, but this is not a thin suspended staircase within the same story.
See [material provenance](stacked-space-assets.md).

## Compatibility and current limits

`pos.level` and saved position levels are zero-based. Old saves default to level
zero. Exploration, tile changes and once-only records are separated by level.
An out-of-range saved level is normalized to zero. An invalid saved cell resets
to the lowest level's start.

Validation rejects mixing a space with `map`, `generate`, roaming enemies,
themes, door pictures, conditional dungeon looks, tile pictures, slide or spin
effects. These combinations are not partially rendered. Full-height walls and
solid floor slabs are the available building geometry; this does not yet provide
windows, furniture sprites, railings, arbitrary meshes or free vertical looking.

The [E2E route](../../scripts/capture-space.txt) walks under the upper flight,
inspects the lower flight from the side, ascends twice, crosses the bridge, looks over the plaza from the roof and
returns. The native graphics test checks positions, levels and visible geometry,
and rejects sky-colored holes in ground-level views. A separate colored platform
checks that the renderer distinguishes its solid edge, underside, and open sky. Save/restore and
blocked sideways movement also have non-graphics tests. Video review is required
before merging this feature.
