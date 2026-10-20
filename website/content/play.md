---
title: Play the samples in the browser
description: Play Go Zen, the Gozen sample RPG, and After the Pass, the novel sample, in the browser - the same games as the desktop builds, compiled to WebAssembly.
weight: 15
---

[Play Go Zen in the browser](/play/go-zen/). The page downloads about 55 MB (the program, 37 MB, and the game's pictures as WebP, music and font, 18 MB) and shows how much is left; the first scene takes some more seconds to draw. Saves stay in the browser.

[Play After the Pass in the browser](/play/novel/), the novel sample: a short story at a tavern with two choices and three endings, in English or Japanese (Settings, Language). The page downloads about 42 MB (the program, 38 MB, and the game's pictures as WebP, music and font, 4 MB).

The browser builds are the same games as `examples/go-zen` and `examples/novel`: a `gozen.Session` on `play.New`, compiled with `GOOS=js GOARCH=wasm`. `make website` builds them into the site (it needs Python with Pillow built with WebP, `python3-pil`), and `make website-play-check` opens the built pages in headless Chrome and fails when a game draws nothing.

| Input | Meaning |
|:--|:--|
| Arrows, Enter / Z | Choose and confirm |
| X / Esc | Back, or open the menu |
| Click | Choose, or walk to a place |
| Q / mouse wheel up | In the novel, go back a line |
| N | In the novel, skip the lines already read |
