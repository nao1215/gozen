---
title: Play Go Zen in the browser
description: Play Go Zen, the Gozen sample game, in the browser - the same game as the desktop build, compiled to WebAssembly.
weight: 15
---

[Play Go Zen in the browser](/play/go-zen/). The page downloads about 55 MB (the program, 37 MB, and the game's pictures as WebP, music and font, 18 MB) and shows how much is left; the first scene takes some more seconds to draw. Saves stay in the browser.

The browser build is the same game as `examples/go-zen`: a `gozen.Session` on `play.New`, compiled with `GOOS=js GOARCH=wasm`. `make website` builds it into the site (it needs Python with Pillow built with WebP, `python3-pil`), and `make website-play-check` opens the built page in headless Chrome and fails when the game draws nothing.

| Input | Meaning |
|:--|:--|
| Arrows, Enter / Z | Choose and confirm |
| X / Esc | Back, or open the menu |
| Click | Choose, or walk to a place |
