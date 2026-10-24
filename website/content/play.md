---
title: Play the samples in the browser
description: Play Go Zen, the Gozen sample RPG, After the Pass, the novel sample, and Five Banners, the campaign sample, in the browser - the same games as the desktop builds, compiled to WebAssembly.
weight: 15
---

[Play Go Zen in the browser](/play/go-zen/). The page downloads the shared WebAssembly player and all files listed in the game's manifest before play. Saves stay in the browser.

[Play After the Pass in the browser](/play/novel/), the novel sample: a short story at a tavern with two choices and three endings, in English or Japanese (Settings, Language). It uses the same player and fetches its own game files before play.

[Play Five Banners in the browser](/play/campaign/), the campaign sample: three factions, five regions and ten weeks; choose a region, send your generals with their tactics, and watch the clash, in English or Japanese.

The pages run the same YAML and assets as `gozen run examples/NAME/game`.
They share one player under `play/runtime/`. Each game's `game/files.json` maps
original asset paths to served files. The browser fetches every listed file before
starting and keeps saves in `localStorage`.

`make website` builds the shared `.wasm.gz`, its runtime and license notices, and
compressed sample assets. It needs Go, Hugo, Python with Pillow/WebP, and
`go-licenses` v2.0.1. The pages decompress the player with the browser's
`DecompressionStream`; static hosts need no special compression configuration.
`make website-play-check` opens all three pages in Chrome, presses keys and checks
the rendered frames.

| Input | Meaning |
|:--|:--|
| Arrows, Enter / Z | Choose and confirm |
| X / Esc | Back, or open the menu |
| Click | Choose, or walk to a place |
| Q / mouse wheel up | In the novel, go back a line |
| N | In the novel, skip the lines already read |
