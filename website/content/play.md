---
title: Play the samples in the browser
description: Play Go Zen, the Gozen sample RPG, After the Pass, the novel sample, and Five Banners, the campaign sample, in the browser - the same games as the desktop builds, compiled to WebAssembly.
weight: 15
---

[Play Go Zen in the browser](/play/go-zen/). The page downloads the compressed WebAssembly program first, then fetches the game's files as Gozen opens them. It does not download unused pictures before play. Saves stay in the browser.

[Play After the Pass in the browser](/play/novel/), the novel sample: a short story at a tavern with two choices and three endings, in English or Japanese (Settings, Language). It also downloads the compressed program first and fetches pictures as needed.

[Play Five Banners in the browser](/play/campaign/), the campaign sample: three factions, five regions and ten weeks; choose a region, send your generals with their tactics, and watch the clash, in English or Japanese.

The browser builds are the same games as `examples/go-zen`, `examples/novel` and `examples/campaign`: a `gozen.Session` on `play.New`, compiled with `GOOS=js GOARCH=wasm`. A small `game/files.json` manifest lists game paths and their served files; the browser file system fetches each file on its first open and caches it for the rest of the session. The browser keeps saves in `localStorage`.

`make website` builds the compressed program files (`.wasm.gz`) and the game files into the site (it needs Python with Pillow built with WebP, `python3-pil`). The pages fetch the `.wasm.gz` files and decompress them with the browser's `DecompressionStream` before WebAssembly instantiation. The gzip files are ordinary files: static hosts can serve them without special `.wasm` compression settings. If you configure server-side compression instead, serve `.wasm` with `Content-Type: application/wasm` and a matching `Content-Encoding` (`gzip` or `br`), and remove the precompressed-file loader or have it fetch that representation consistently. `make website-play-check` opens the built pages in headless Chrome and fails when a game draws nothing.

| Input | Meaning |
|:--|:--|
| Arrows, Enter / Z | Choose and confirm |
| X / Esc | Back, or open the menu |
| Click | Choose, or walk to a place |
| Q / mouse wheel up | In the novel, go back a line |
| N | In the novel, skip the lines already read |
