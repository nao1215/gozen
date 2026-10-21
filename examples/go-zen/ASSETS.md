# Go Zen assets

Go Zen is the sample game of Gozen. Its Go code and YAML (`main.go`, `files.go`, the tests,
`game/game.yaml`, `game/data/`) are MIT like the rest of the repository. The files under
`game/assets/` are listed here.

| Files | What | License |
|:--|:--|:--|
| `maren/`, `ox/`, `tilly/`, `kes/`, `vance/` | Moving portraits (`puppet.yaml`, the base picture and its difference pictures) and the whole-figure picture of each face | CC BY 4.0 |
| `pomm/`, `barnaby/`, `ansel/` | Standing pictures of the townsfolk | CC BY 4.0 |
| `icons/` | Round face icons for the balloons and the map's token | CC BY 4.0 |
| `maps/town.jpg` | The town map of Pellmere, also behind the title | CC BY 4.0 |
| `cg/cg1_fountain.jpg` | The event picture (also in the gallery) | CC BY 4.0 |
| `bg/` | The plaza as a painted scene with its depth (`plaza_depth.png`), the plaza with the water back, the shop, the inn, the well, the gate, Hobb's Landing, the valve hall, and the battle backgrounds | CC BY 4.0 |
| `dungeon/`, `tiles/`, `props/` | The dungeon's walls, floors, ceilings, doors and stairs, chests, crates, levers, the key, the valve and the vent, and the townsfolk's things (the water cart, Hobb's desk, the notice) | CC BY 4.0 |
| `enemies/`, `<id>/cutin.png` | Enemies and the cut-in pictures | CC BY 4.0 |
| `bgm/title.ogg`, `town.ogg`, `dungeon.ogg`, `garden.ogg`, `castle.ogg`, `dream.ogg`, `gate.ogg`, `battle.ogg`, `boss.ogg`, `victory.ogg`, `fanfare.ogg`, `finale.ogg` | Music written by the author for the author's own RPG | CC BY 4.0 |
| `sfx/boss.ogg`, `break.ogg`, `buff.ogg`, `crit.ogg`, `cutin.ogg`, `debuff.ogg`, `encounter.ogg`, `escape.ogg`, `fall.ogg`, `heal.ogg`, `hit.ogg`, `levelup.ogg`, `magic.ogg`, `miss.ogg`, `punch.ogg`, `tink.ogg`, `treasure.ogg`, `win.ogg` | Sound effects by the author for the author's own RPG, used by battle, healing, treasure and encounter cues | CC BY 4.0 |
| `fonts/mplus-1p-regular.ttf` | M+ 1p regular 1.060, M+ FONTS PROJECT | M+ FONTS LICENSE (below) |
| `fonts/uncial-antiqua-regular.ttf` | Uncial Antiqua, the title's letters; Brian J. Bonislawsky (Astigmatic), from Google Fonts | SIL Open Font License 1.1 (`fonts/OFL-uncial-antiqua.txt`) |
| `fonts/cinzel.ttf` | Cinzel (variable), the subtitle's letters; the Cinzel Project Authors, from Google Fonts | SIL Open Font License 1.1 (`fonts/OFL-cinzel.txt`) |

The pictures, music and sound effects are Copyright (c) 2026 CHIKAMATSU Naohiro and licensed under the
Creative Commons Attribution 4.0 International license
(https://creativecommons.org/licenses/by/4.0/; the text is in `game/assets/LICENSE`). Credit
"Go Zen sample art and music, CHIKAMATSU Naohiro" when you reuse them. The pictures are
generated images made for this repository. The music was rendered with FluidSynth and the
GeneralUser GS v2.0.3 SoundFont by S. Christian Collins (http://www.schristiancollins.com),
whose license lets the music made with it be used without restriction; the SoundFont itself
is not included.

The pictures are kept at the size they are shown at on a 1920x1080 screen or larger: the
whole-figure pictures are 1144 pixels high, the backgrounds, the map and the event picture 1920x1080. The depth of
the plaza was estimated from its picture with Depth Anything V2 Small (Apache-2.0).

## M+ FONTS LICENSE

```text
These fonts are free software. Unlimited permission is granted to use, copy, and
distribute them, with or without modification, either commercially or noncommercially.
THESE FONTS ARE PROVIDED "AS IS" WITHOUT WARRANTY.
```
