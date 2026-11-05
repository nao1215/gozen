# Five Banners assets

Five Banners is the campaign sample, a small region-conquest war. Its Go code and YAML (`main.go`,
the tests, `game/game.yaml`, `game/data/`) are MIT like the rest of the repository. The files
under `game/assets/` are copies of earlier work by the same author, and pictures made for it.

| Files | What | Source | License |
|:--|:--|:--|:--|
| `maps/continent.jpg` | The picture map of the land, on which the five regions are laid out with `area` and `at` | Generated for this sample (Illustrious XL, takostyle LoRA 0.25) | CC0 1.0 |
| `maps/town.jpg` | The town map of Pellmere, kept as a second picture of the land | Go Zen (`examples/go-zen/game/assets/`, see [its ASSETS.md](../go-zen/ASSETS.md)) | CC0 1.0 |
| `corin/`, `hilde/`, `sable/`, `vessa/` (`normal.png`) | The generals' standing pictures | The author's earlier game, renamed for this sample | CC0 1.0 |
| `envoy/normal.png` | The Raven envoy | The author's earlier game, renamed for this sample | CC0 1.0 |
| `icons/` | The generals' round faces for the map's token, cut from their standing pictures at the boxes `gozen art faces` found | Made from the pictures above | CC0 1.0 |
| `foes/` | The foes who hold the regions in a clash | The author's earlier game, renamed for this sample | CC0 1.0 |
| `bg/field.jpg`, `bg/town.jpg`, `bg/castle.jpg` | The grounds behind a clash, by a region's terrain | Generated for this sample (Illustrious XL, takostyle LoRA 0.25) | CC0 1.0 |
| `armies/{men,women,monsters}_{1..4}.png` | The soldiers of each army kind, lined up in a clash, drawn facing right (the foe side is flipped by the engine). | Generated for this sample and cut out (BiRefNet) | CC0 1.0 |
| `fonts/mplus-1p-regular.ttf` | M+ 1p regular 1.060, M+ FONTS PROJECT | Go Zen | M+ FONTS LICENSE (in Go Zen's ASSETS.md) |
| `bgm/title.ogg`, `clash.ogg`, `win.ogg`, `lose.ogg`, `fanfare.ogg` | Music by the author from the author's own RPG | The author's own RPG | CC BY 4.0 |
| `sfx/hit.ogg`, `cutin.ogg`, `win.ogg` | Sound effects by the author from the author's own RPG, used for clash hits, skill cut-ins and victory | The author's own RPG | CC BY 4.0 |

No copyright is claimed in the pictures; to the extent any rights exist, they are dedicated to the public domain under CC0 1.0 Universal (https://creativecommons.org/publicdomain/zero/1.0/; the text is in `game/assets/LICENSE`): use them for any purpose, with or without credit. The music and sound effects are Copyright (c) 2026 CHIKAMATSU Naohiro and licensed under the Creative
Commons Attribution 4.0 International license (https://creativecommons.org/licenses/by/4.0/;
the text is in `game/assets/LICENSE`). Credit "Gozen sample music and sounds,
CHIKAMATSU Naohiro" when you reuse them.
