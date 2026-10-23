# Stacked-space preview materials

The standalone `examples/space` demo reuses `brick_wall.jpg`, `brick_floor.jpg`
and `brick_ceiling.jpg` from the Go Zen sample. These copies let the embedded
standalone example and the filesystem runner use the same files without reaching
outside the game directory. No new wall or floor artwork is required.

Source: [Go Zen assets](../../examples/go-zen/ASSETS.md).
Credit: Go Zen sample art and music, CHIKAMATSU Naohiro.
License: CC BY 4.0; the license text accompanies the demo assets.

The stair renderer maps the floor material onto the actual treads and risers.
The existing `brick_stairs_up.jpg` is a perspective illustration of an entire
staircase, so it is not used as a surface texture for the traversable geometry.
