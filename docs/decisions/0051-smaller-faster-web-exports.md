# ADR 0051: Smaller and faster web exports

Status: Accepted (2026-10-27). Home: `cmd/internal/distribute` (what a web export serves), `internal/webfs` (how the player downloads it), `internal/expr` and `internal/layer5/puppet` (the startup's work), `internal/layer5/picdecode` (WebP).

## Context

The stream game of ADR 0048 is mostly opened on a phone. Its web export served a 42.5 MB player and 9.4 MB of game files as they were (PNG puppets of 1024×1536, JPEG rooms, YAML, a 1.7 MB font), and a static host without compression (Python's `http.server`) sent all 52 MB. Measured in Chrome as a phone (390×844, device pixel ratio 3, the CPU slowed four times, DevTools' Fast 4G: 9 Mbps, 165 ms), the lobby was first drawn after 105 s: 43 s for the player, 10 s for the files, then 52 s of startup. A profile of the startup showed:

- go-yaml hands a `TextUnmarshaler` its text by formatting the node again from every token of the file (`unmarshalableText`), so each condition (`expr.Expr`) of the 190 kB reaction book cost the whole file: 62% of the startup, 277 ms natively against 15 ms.
- A puppet face drawn in a part (`at`) copied the whole base (6.3 MB for 1024×1536) for each face and each eye and mouth picture, needed only when a later picture was a whole one.
- The pictures came as PNG and JPEG, a fifth to a half larger than WebP of the same look.

## Decision

- `gozen export --target web` serves the player as `gozen-player.wasm.gz`. The page counts the bytes as they come (against `Content-Length`), undoes the gzip with `DecompressionStream` and compiles the player while it downloads (`instantiateStreaming`); it reads the first two bytes, across pieces of the download however they split, to tell gzip, and a response that is already WASM (a host that undid the gzip) is used as it is. No host needs to be set up.
- The game's files are made smaller on the way (`shrinkWeb`): an 8-bit PNG or JPEG becomes a WebP of the same pixels, served as `NAME.webp` under its own name (the game reads a picture by its content): lossy at quality 95 with sharp RGB to YUV and its alpha lossless (at quality 85 with the default conversion the thin lines of a painted puppet, its eyes, lips and strands, were smeared on a phone's screen: the pixels of the lines of the example puppet kept 31 dB, now 33 dB, the picture a fifth of its PNG rather than a seventh), lossless when the PNG is paletted (pixel art), and left as it was when WebP is not smaller or the picture is not 8-bit (a 16-bit depth map). Another file that gzip makes a tenth smaller (YAML, a font) is served as `NAME.gz` with `"gzip": true` in `files.json`, which `internal/webfs` undoes. The files under the game do not change; a desktop export is as before.
- WebP is encoded and decoded with `github.com/deepteams/webp` (MIT, pure Go, no dependencies). `golang.org/x/image/webp` read a lossy WebP's limited-range YUV as full range: its pictures came out 8 levels brighter on average than the browser's, and it is slower. It is the decoder of every build now, so a picture decoded in Go before the first frame matches the ones the browser decodes after it.
- `internal/webfs` fetches each file whole with the browser's `fetch` and `arrayBuffer`, so the browser reads it while the game works (read in Go through `net/http`, a file came only as fast as the busy game took its pieces), and the player no longer links `net/http` (35.4 MB, 8.3 MB gzipped, from 42.5). It fetches the pictures and sounds last (by the name the game reads, so a sound served gzipped is still a sound), and the recorded voices of the streams (the streams file's `voices` folder, ADR 0052) not at all: `files.json` marks them `"later": true`, they are fetched when the game opens them and left out of the progress, so a game with voices in two languages (hundreds of megabytes) opens as quickly as without and returns once the other files have come: the game reads its YAML while its media still come, and opening a file still coming waits for it. Progress is reported as each file comes; once the game has drawn its first frame and the loading page is gone, the page keeps the bytes and shows nothing, and an error the page throws does not stop the game.
- `expr.Expr` reads itself from YAML as a string (`UnmarshalYAML`), which go-yaml decodes where the node is, so a condition costs its own text. Scalars read as before; a list or a map is an error as before.
- A puppet makes the whole picture of a face only when one of its eyes or mouth pictures is a whole picture.
- `make web-runtime` and `make desktop-runtime` build with `-ldflags="-s -w"`, as the release does. A Go panic still prints its stack (from the program's own tables); a browser's profiler no longer names the functions.

## Consequences

Measured as above on the same machine, the runs of each row one after another:

| Phone, CPU slowed | Before | After |
| --- | --- | --- |
| Downloaded until the lobby is drawn (player + files) | 52.0 MB (42.5 + 9.4) | 11.8 MB (8.3 + 3.4) |
| 4×, Fast 4G: player in / files in / lobby drawn | 42.8 s / 53.4 s / 105.6 s | 8.8 s / 19.2 s / 24.9 s |
| 6×, Fast 4G: lobby drawn | 134.0 s | 29.9 s |
| 4×, no network limit: lobby drawn (two runs) | 54.3 s, 56.6 s | 13.0 s, 15.3 s |
| 4×: frames a second in the lobby, the watch view | 41 to 46, 60 | 38 to 58, 59 to 60 |

The frame rates moved with the GPU's other work, not with this change.

Pictures are lossy in a web export, so one compared pixel by pixel (a whole face picture cut against the base, ADR 0020) differs a little everywhere; the cut already allows for pictures redrawn by image generation. Each file is a request with its round trip (89 in this game); one archive would save those on an HTTP/1.1 host but lose fetching the media last. The pictures of the first frame are still decoded in Go (`picdecode.BrowserAfter`: before it, the sound being made keeps the page so busy that the browser's decoder answers late); in this game the browser decoding them was about 2 s sooner at four times slower, which was not measured for the other games.
