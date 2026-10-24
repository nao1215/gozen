# Stacked-space preview materials

The `examples/space/game/assets/dungeon/` images are original generated material
textures for this repository, distributed under its [MIT license](../../LICENSE).
They were created with the built-in image generation tool; no shared ComfyUI
queue was used. The earlier prototype's copies of Go Zen textures were removed
when these materials replaced them.

| File | Generation brief | Use |
| --- | --- | --- |
| `limestone.png` | Seamless warm ivory limestone ashlar, subtle chisel marks, worn beveled edges, thin mortar, evenly lit, no perspective or objects | Walls and ceilings |
| `paving.png` | Seamless blue-gray slate flagstones, fine grout, gently worn bevels, restrained mineral variation, orthographic and evenly lit | Floors, stair treads and risers |
| `sky.png` | Horizontally wrapping 2:1 blue-sky panorama, sunlit cumulus clouds, pale horizon, no land, buildings, sun disc or lettering | Upper-hemisphere background |

The final generated PNGs are stored in the game directory. The ceiling and sky
share a bounded GPU texture; it is rebuilt when their loaded images change.
The stair renderer maps materials onto actual geometry rather than painting a
perspective staircase illustration onto a wall.
