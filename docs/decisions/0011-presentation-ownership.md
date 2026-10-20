# ADR 0011: Ownership of presentation and the launch boundary

Accepted: 2026-10-18 (T3). The existing layers, the single module, and the content boundary are not changed. Partly changed: `play.Run` and `sound` were removed from the public API by [ADR 0021](0021-query-and-commands.md) because they have no users.

## Decision

The entry points for game authors are `runner.Run` and `play.Run`. For authors embedding into an existing Ebitengine app, we provide `play.New` and Update/Draw/Layout/Close of `Game`. Arguments and output destinations are explicit, and the library does not terminate the process. The app owns the exit code, signals, and the choice of standard output.

The many screen and production settings stay in `internal/play.Options`, and the public surface is narrowed to a small `play.Config` and an optional `play.Development`. Production features that have a write destination require explicit specification. Normal saving uses Gozen's directory under `savefile`, and trial runs such as scripts use a temporary save location unless one is explicitly specified.

The shared implementations of presentation are `internal/layer5/gfx` and `internal/layer5/sound`, and file recording of memos is `internal/layer1/memo`. Memos take the standard library's image and byte strings, and know nothing of the screen or game rules. dungeon uses `internal/layer3/dungeon` from T2. No sideways dependency between public packages is added.

`event.Runner.BlockAside` is a callback that suppresses inserted events depending on the screen state. Combined with `event.HostFunc`, it keeps suppression during recollections and image display without passing the upper Game type down to the lower event layer. The existing `AsideBlocker` is kept for backward compatibility.

## Ownership and alternatives

The font cache, translation, and audio settings inside the screen hold shared state, so only one Game can be open at a time. Close stops audio and releases the cache and the temporary save location. Running several games at the same time is not promised. The Ebitengine loop is also started only once per process.

gfx returns an independent Font, and sound uses an independent Clip and an audio.Context owned by the caller. The alternative of making all drawing helpers and global audio management public is not adopted because it adds compatibility contracts that game embedding does not need. The audio of play reuses the existing 44,100 Hz Context. A different rate is an error.

Build information is not recorded by default. The embedded revision is read only with an explicit opt-in. Git is not run, sources are not searched, and host information is not collected. Compatibility of reproduction data is decided by independent format and behavior versions.

## How to check

For the public contract and its users, see the [presentation API](../architecture/presentation-api.md), and for run conditions, see [Developing the screen](../development/graphics.md). Exclusions in the dependency policy are not added, and the deep dependency check is kept.
