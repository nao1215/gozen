# Internal presentation and startup

`internal/play` owns the game window, menus, scripts, capture, sound and saves. `cmd/gozen` and the private `cmd/gozen-player` call it directly. Authors run `gozen run GAME` or export a game with a matching prebuilt runtime; there is no public Go screen API.

`Main` receives context, filesystem, arguments and `EntryConfig`. `Entry` registers the shared startup operation for `gozen run`; capture calls the internal batch renderer. These entry points do not invoke another CLI. Ebitengine runs one loop per process. Close and output failures reach the caller.

The renderer uses `internal/layer5` for graphics, sound and moving portraits. Game rules know definitions and state, not GPU images. Portrait YAML is checked by `gozen puppet`; games refer to it from actor or enemy definitions.

Saves default to `UserConfigDir/gozen/GAME-ID` on desktop and origin-local storage in browsers. `--save-dir` selects a separate desktop directory. Scripts isolate saves unless a directory is explicitly supplied.

`make graphics` runs the existing screen and portrait pixel tests. `make e2e-graphics` exercises actual CLI play, capture, distribution and save restoration. See [testing](../development/testing.md).
