# Novel sample assets

The novel sample is the visual novel of Gozen. Its Go code and YAML (`main.go`, `files.go`,
the tests, `web/`, `game/game.yaml`, `game/data/`, `game/lang/`) are MIT like the rest of
the repository. The files under `game/assets/` are listed here.

| Files | What | License |
|:--|:--|:--|
| `bg/hill.jpg` | The tavern on the snowy hill at dusk, also behind the title | CC BY 4.0 |
| `bg/tavern.jpg` | The tavern inside, with the fire lit | CC BY 4.0 |
| `bg/tavern_late.jpg` | The empty tavern late at night | CC BY 4.0 |
| `cg/toast.jpg`, `cg/wine.jpg`, `cg/table.jpg` | The event pictures: Brenna raising her mug, Wren with a glass of wine, and the counter with a glass and bottles | CC BY 4.0 |
| `brenna/`, `wren/`, `hedda/` | The standing picture of each character (`normal.png`) | CC BY 4.0 |
| `bgm/evening.ogg`, `bgm/town.ogg`, `bgm/gate.ogg`, `bgm/finale.ogg` | Music by the author from the author's own RPG, for the tavern, town, gate and reflective scenes | CC BY 4.0 |
| `faces.json` | The face box of each standing picture, written by `gozen art faces` (the face icons of the character book) | MIT |
| `fonts/mplus-1p-regular.ttf` | M+ 1p regular 1.060, M+ FONTS PROJECT (Latin and Japanese letters) | M+ FONTS LICENSE (below) |

The pictures and music are Copyright (c) 2026 CHIKAMATSU Naohiro and licensed under the
Creative Commons Attribution 4.0 International license
(https://creativecommons.org/licenses/by/4.0/; the text is in `game/assets/LICENSE`). Credit
"Gozen novel sample art and music, CHIKAMATSU Naohiro" when you reuse them. The pictures are
generated images by the same author, first made for another of the author's projects and
reused here; the standing pictures were cut to the figure and the backgrounds made 1920x1080.
The music comes from the author's own RPG and was rendered with FluidSynth and the GeneralUser
GS v2.0.3 SoundFont by S. Christian Collins
(http://www.schristiancollins.com), whose license lets the music made with it be used without
restriction; the SoundFont itself is not included.

## M+ FONTS LICENSE

```text
These fonts are free software. Unlimited permission is granted to use, copy, and
distribute them, with or without modification, either commercially or noncommercially.
THESE FONTS ARE PROVIDED "AS IS" WITHOUT WARRANTY.
```
