# Display, sound and startup API

This records the contracts of layers 5 and 6 for game authors. For the run conditions see [developing the screen](../development/graphics.md), and for the check before publishing see [the release checklist](../development/release-checklist.md).

| Public declaration | Who uses it for what, and the contract |
| --- | --- |
| `gfx.Font`, `gfx.Load`, `Font.Measure`, `Font.Draw` | An author drawing their own Ebitengine screen reads an FS font independently and uses it to measure width and height and to draw. The size is in pixels; a non-positive or non-finite size measures 0 and draws nothing. Draw does not change its options |
| `play.Main` | The game's main. Pass an fs.FS, a directory and the startup options (GNU style, `--dev` and so on) and play in a window. The output for help and reports, the save location and mute are in `play.Config` |
| `play.Config` (`Store`, `Mute`, `Output`) | The save location (a `gozen.Store` opened from the work's ID; nil is `gozen/<ID>` in the OS user config directory, and localStorage in a browser), silencing sound, and the output for help and reports |
| `play.Screen`, `play.New`, `Screen.Update/Draw/Layout/Close` | Show a `gozen.Session` on screen in your own Ebitengine loop or in a browser (GOOS=js). A freshly made Session starts from the start events, and a played Session continues from where it is. It follows the Session's `Config.Dev`. Close is required; game thread only |
| `puppet.Load`, `Rig`, `Rig.Size`, `Config`, `New`, `Puppet.Update/Draw/Speak/Stop/React/SetFace/Close` | An author who wants to move a one-picture character (a streamer game, a talk portrait) reads the YAML and difference pictures and draws blinking, mouth, breathing, sway of head and hair, reactions and face changes. Game thread only. [Moving portraits](puppet.md), [ADR 0020](../decisions/0020-moving-portraits.md) |

`puppet` is 17 declarations (ADR 0020). The T3 diff of `api/public.txt` is 35 declarations (types, fields, methods and constants counted each). No other screen, memo, audio mixer or dungeon API than the table's is added. `sound`, `play.Run` and `runner` were removed from the public API ([ADR 0021](../decisions/0021-query-and-commands.md); startup is `play.Main`).

## Startup and exit

`play.Main(ctx, fsys, dir, args, play.Config{Output: out})` opens a relative dir with fs.Sub. If `--dir` is given, that directory is used. Nil args means no arguments, and nil Output means no output. `--help` succeeds without loading the game, and if writing fails it returns that error.

Main is called on the main thread. A normal exit by closing the window is nil, cancellation is the context's error, and a failure to save or trace returns an error. For a host that uses Game.Update directly, script completion is notified with ebiten.Termination. The library does not call os.Exit or log.Fatal. Ebitengine cannot restart its loop within one process.

The FS is borrowed and is not changed or closed. Assets and definitions must stay stable while in use. The Writer is also owned by the caller and a synchronous write must return. Game is not concurrent, and only one instance can be open at a time. Even when New fails, resources initialized so far are cleaned up. Close is called after the loop stops, and from the second call on does nothing.

Asynchronous image prefetch goes only as far as decoding on the CPU, and the images for drawing are made on the using game's thread. A worker does not call a drawing API after screen initialization fails or the loop ends. Prefetch results also stay within the image cache budget, and Close waits for reading to finish before releasing (`internal/play/assets.go:180`, `194`, `402`, `internal/play/preload_test.go:12`).

The sound of play uses the process-wide audio.Context. If a Context already exists it is reused only at 44,100 Hz, and other rates are refused. Mute does not initialize a device. A Player of an independent Clip is outside the scope of Game.Close, so the user calls PauseAndStopReading.

## Saves and authoring information

Saves go to a `gozen.Store` (bytes per name). Normally this is a `gozen.DirStore` at `os.UserConfigDir()/gozen/<game-id>`, and in a browser (GOOS=js) the page's localStorage (the key is `gozen/<game-id>/<name>`). The save location of an existing work is not searched. `--save-dir` is the full output location and does not add the ID. For trial runs such as choosing a start position, scripts and replay, omitting the location puts the data in memory (`gozen.MemStore`) and leaves nothing. A failure to write the save location is not treated as success. When overwriting an old format or an unreadable save, a backup `<name>.before-<sha256>.bak` is left in the same place.

The authoring flags require `--dev`. The output locations of memos, traces, screenshots and reviews have no default derived from an authoring environment. Memos are on only when Notes or --memos is given. CollectBuildInfo or --build-info is false by default and, when allowed, adds only the revision of the embedded build information. There is no Git or source search by external commands.

External input on Linux is allowed explicitly with --external-input. If zenity is not found, in-screen input is used. Windows and macOS use in-screen input. If the user cancels input in an available zenity, that input is cancelled.

For the layout of the implementation and the decisions see [ADR 0011](../decisions/0011-presentation-ownership.md).
