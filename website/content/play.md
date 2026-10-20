---
title: Play Go Zen in the browser
description: Play Go Zen, the Gozen sample game, in the browser - the same game as the desktop build, compiled to WebAssembly.
weight: 15
---

[Play Go Zen in the browser](/play/go-zen/). The page downloads about 60 MB (the program and the game's pictures and music) before the title appears. Saves stay in the browser.

The browser build is the same game as `examples/go-zen`: a `gozen.Session` on `play.New`, compiled with `GOOS=js GOARCH=wasm`. `make website` builds it into the site.

| Input | Meaning |
|:--|:--|
| Arrows, Enter / Z | Choose and confirm |
| X / Esc | Back, or open the menu |
| Click | Choose, or walk to a place |
