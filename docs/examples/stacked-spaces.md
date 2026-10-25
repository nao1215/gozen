# Continuous stacked spaces

`dungeon.space` lets a location contain a street, interiors and upper floors in
one volume. Ordinary dungeons without `space` keep their existing renderer and
floor transitions. The standalone demonstration starts with:

```sh
gozen run examples/space/game --dev --mute
```

Use Up to walk, Left and Right to turn, and Down to turn around. The demonstration
has an open plaza, two wall-mounted exterior stair runs, an elevated bridge,
a roof terrace, and ground-floor space beneath the upper staircase. [Short video](../media/stacked-space-short.mp4) and
[mobile GIF](../media/stacked-space-short.gif) show an 11.5-second continuous
excerpt (18.5–30 seconds) of the [full recorded route](../media/stacked-space-preview.mp4).
Playback speed is unchanged.
This is a technical demonstration of bounded geometry, not a finished town.

## Authoring

Set `schema_version: 8` in game.yaml. See the complete [example world](../../examples/space/game/data/world.yaml).
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
An optional `space.sky` picture provides a horizontally wrapping panorama.
Its top is the zenith and its bottom is the horizon; tint and brightness apply.
Without a sky picture, the renderer uses its gradient background.
See [material provenance](stacked-space-assets.md).

## Compatibility and current limits

`pos.level` and saved position levels are zero-based. Old saves default to level
zero. Exploration, tile changes and once-only records are separated by level.
An out-of-range saved level is normalized to zero. An invalid saved cell resets
to the lowest level's start.

Validation rejects mixing a space with `map`, `generate`, roaming enemies,
themes, door pictures, conditional dungeon looks, slide or spin
effects. These combinations are not partially rendered. Full-height walls and
solid floor slabs are the available building geometry; this does not yet provide
windows, railings, arbitrary meshes or free vertical looking.

The [E2E route](../../scripts/capture-space.txt) walks under the upper flight,
inspects the lower flight from the side, ascends twice, crosses the bridge, looks over the plaza from the roof and
returns. The native graphics test checks positions, levels and visible geometry,
and rejects sky-colored holes in ground-level views. A separate colored platform
checks that the renderer distinguishes its solid edge, underside, and open sky. Save/restore and
blocked sideways movement also have non-graphics tests. The short capture was reviewed before merging this feature.

## NPC and furniture billboards

The [Lantern Inn fixture](../../examples/space-town/game/data/world.yaml) puts a street, innkeeper, stairs and an upstairs guest bed in one location:

```sh
gozen check examples/space-town/game
gozen run examples/space-town/game --dev --mute --start inn
```

Walk to the innkeeper and press Confirm, then choose the interaction. The demonstration removes each innkeeper after greeting them; the two floors have independent once records despite using the same tile character and coordinates. Upstairs, inspect the bed with the same tile interaction. The [scripted route](../../scripts/capture-space-town.txt) ascends, turns back mid-flight, reaches the room, visits the upper NPC and returns without changing the location ID.

Use `tiles.<character>.picture` on a floor tile for an upright, camera-facing image. `size` is its height in stories: the default 0.6 occupies 0.9 cell units, because a story is 1.5 units. Size must be finite and greater than zero, up to 16; omitted or zero size retains the existing 0.6 default. Image aspect ratio, alpha transparency, tint and brightness are retained. Large artwork can extend outside its anchor cell; its collider remains the anchor cell. Camera-facing art does not have physically distinct side/back views.

`solid: true` blocks walking through the anchor cell without turning its image into a full-height wall. `when`, `else`, `once`, `after` and commands retain their tile semantics. Pictures are anchored to the actual level and its floor/stair height, and use the interpolated camera. Walls, slab sides/undersides and closed stair volumes occlude images using the same ray intersections as the terrain. Transparent pixels reveal more distant sprites and scenery.

Flat/floor decals, wall/door-mounted tile pictures and `away` pictures are not supported in stacked spaces and receive validation errors. Ordinary dungeon pictures retain their behavior. See the [original asset provenance](space-town-assets.md) and [performance measurements](../development/space-performance.md).

## Continuous walking

Opt in per space; omitting `free_move` keeps the existing grid controls:

```yaml
space:
  free_move:
    speed: 1.6
    turn_speed: 90
    acceleration: 0.12
    stages:
      - {after: 0.4, speed: 2.4}
      - {after: 1.2, speed: 3.2}
  # Keep the existing levels and stairs here.
```

Up/Down walk forward/backward, Left/Right turn continuously, and Shift+Left/Right strafe. Confirm uses the continuous viewing direction, with a reach of 1.25 cells and solid-cell occlusion on the active support level. Walking and turning remain separate from the game clock. Movement uses fixed 60 Hz updates; Draw never advances it. The saved camera can stop within a cell and at an arbitrary angle.

Speed is in cells per second, up to 12; omitted or zero selects 1.6. Turning is in degrees per second, up to 360; omitted or zero selects 90. Stages select absolute speeds after successful movement for the specified simulation seconds. Their positive thresholds must increase, with at most 16 stages and a maximum threshold of 3600 seconds. Omit stages for constant speed. Acceleration is the time to interpolate between targets: omitted means 0.12 seconds, explicit zero means immediate, and the maximum is 5 seconds. Release stops promptly. Reversing input, complete blockage, blocking UI, focus loss, placement and restore reset the stage; successful wall sliding continues it. Smooth turns do not reset walking just because the cardinal projection changes.

The player has a 0.18-cell radius and a 0.9-cell body height. Nearby solids, bounds, floor support and overhead surfaces constrain motion; bounded segments prevent tunneling at maximum speed. A staircase retains its authored foot and landing connections, including a bridge joining its foot from the side. Side entry halfway up a flight remains blocked. There is no jumping, falling or flying.

`pos.x` and `pos.y` are the floor of the continuous coordinates. `pos.level` identifies the connected support surface, not a rounded eye height. Cardinal facing changes only after passing a quadrant boundary by 0.04 radians, preventing boundary chatter. Actual motion and interaction use the full yaw. Every entered tile dispatches its commands before movement continues, with geometry checked again after synchronous commands. A simultaneous corner crossing enters the diagonal cell; a tile touched at only a point receives no entry. Blocking dialogue, battles and placement discard remaining displacement. Contact commands fire once while held; release or separation rearms them.

One legacy step is counted per cell-length actually travelled, independently of tile entries. Turning, idle time and wall pushing add none. Fractional distance is saved. Random-encounter distance accumulates only where the current rate is positive; leaving such an area clears its fractional encounter budget. A blocking entry event takes priority over an encounter due at that entry, as with grid steps.

Old saves initialize at the saved cell center/cardinal yaw. New saves retain the off-center coordinates, yaw, support level and fractional budgets; velocity is not saved. Invalid or obstructed restored poses fall back to a valid grid placement. `move at`, scripted grid steps/walks and explicit relocation reset continuous motion; scripts retain their existing tile-based semantics. F5 reload preserves a valid pose. Traces include `space_pose`, and capture scripts accept `hold up+right 60` or `hold up+shift+right 60` for exact simulated durations.

To reproduce the [continuous inn route](../../scripts/capture-space-free.txt), copy the Lantern Inn fixture and add `free_move: {speed: 2, turn_speed: 90, acceleration: 0}` under its existing `space`. The native graphics test also drives the original plaza route in this mode. Both routes retain one location while traversing real stairs. See [controller and rendering measurements](../development/space-performance.md) for timings and their scope.
