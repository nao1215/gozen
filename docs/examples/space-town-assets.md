# Lantern Inn fixture assets

The sprites in `examples/space-town/game/assets/` are original procedural artwork created for Gozen. No external game assets, models or reference images were used. The asset files are dedicated to the public domain under CC0-1.0:

| File | Content | Generator |
| --- | --- | --- |
| `innkeeper.png` | Neutral clothed innkeeper with transparent margins | `examples/space-town/generate.py` |
| `bed.png` | Red guest bed with wooden legs and a pillow | `examples/space-town/generate.py` |

Run `python3 examples/space-town/generate.py` from the checkout to reproduce both 64 × 64 PNG files. The generator uses Python's standard library. The YAML fixture and generator use the repository's code license.

The fixture is a small structural test of a street, two-story inn, real stair flight, NPCs and furniture. It is not a finished town art kit. The bed is a camera-facing image with a cell-sized collider, not a 3D bed with distinct side and back views.
