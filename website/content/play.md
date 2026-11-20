---
title: Play the samples in the browser
description: Play Go Zen, the Gozen sample RPG, After the Pass, the novel sample, and Five Banners, the campaign sample, in the browser - the same games as the desktop builds, compiled to WebAssembly.
weight: 15
---

[Play Go Zen in the browser](/play/go-zen/), the RPG sample.

[Play After the Pass in the browser](/play/novel/), the novel sample: a short story at a tavern with two choices and three endings, in English or Japanese (Settings, Language).
[Play Five Banners in the browser](/play/campaign/), the campaign sample: three factions, five regions and ten weeks; choose a region, send your generals with their tactics, and watch the clash, in English or Japanese.

The pages run the same YAML and assets as `gozen run examples/NAME/game` and share one WebAssembly player under `play/runtime/`. Each game's `game/files.json` maps the original asset paths to the served files; the browser fetches every listed file before starting and keeps saves in `localStorage`.

`make website` builds the shared `.wasm.gz`, its runtime and license notices, and the compressed sample assets; it needs Go, Hugo, Python with Pillow/WebP, and `go-licenses` v2.0.1. The pages unpack the player with the browser's `DecompressionStream`, so a static host needs no compression settings. `make website-play-check` opens all three pages in Chrome, presses keys and checks the frames drawn.

| Input | Meaning |
|:--|:--|
| Arrows, Enter / Z | Choose and confirm |
| X / Esc | Back, or open the menu |
| Click | Choose, or walk to a place |
| Q / mouse wheel up | In the novel, go back a line |
| N | In the novel, skip the lines already read |
