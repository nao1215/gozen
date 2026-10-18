# Wayfinder

Wayfinder is an original, small game assembled with the public Gozen API. Practice with a friendly stone companion, light a trail marker, and walk to Lantern ridge. A side room demonstrates first-person grid movement. The story, names, dialogue and geometric pictures were written for this repository; see [ASSETS.md](../../ASSETS.md).

## Run

From the repository root, with Go 1.26.4 or later and a [graphics environment](../development/graphics.md#requirements):

```sh
go run ./examples/play --mute
```

The example embeds YAML and generates its PNG assets in memory. It needs no downloaded assets, ONNX models, audio device or external game directory. `--mute` disables audio initialization. There is no sample music or sound recording.

## Play through

1. Select **New journey**, then **Talk: Courier**. Enter advances the two lines.
2. Select **Practice**. In battle, select **Auto** to let the engine fight, or choose **Attack**, then Pebble as the target, each turn. Winning unlocks **Light the marker**.
3. Select **Light the marker**, read the message, then **Walk to the ridge**. The arrival event awards 4G once; practice awarded 3G.
4. Select **Save journey**, choose slot 1 and wait for confirmation. Press Esc to open the menu, choose **Return to title**, confirm, and use **Continue** to restore the journey. Menu **Load** also restores a slot after confirmation.
5. Return to camp and select **Explore chamber**. Up walks forward, Left/Right turns, Down turns around. Reach **L** to read the window event; **C** leads back to camp. The minimap fills as you walk.

The ridge path is hidden until its marker is lit. Revisiting the ridge after loading does not award its arrival reward again. These transitions and the room's return route are checked against the same definitions in `examples/play/journey_test.go:16`.

## Controls and saves

| Input | Meaning |
| --- | --- |
| Up / Down, Enter / Z / Space | Menu selection and confirmation |
| X / Esc | Back, or open the field menu |
| O / F2 | Settings |
| F11 / Alt+Enter | Fullscreen |
| M | Mute |
| Up, Left / Right, Down in the room | Forward, turn, turn around |

Normal saves use the platform config directory under `gozen/wayfinder-demo`: typically `$XDG_CONFIG_HOME` or `~/.config` on Linux, `%AppData%` on Windows, and `~/Library/Application Support` on macOS. An explicit `--save-dir` is the complete destination. The scripted capture uses a private temporary directory and removes it when finished. A development run with `--start` also isolates saves unless a destination is explicit (`runner/runner.go:128`, `internal/play/play.go:356`).

The bundled Go Regular font supports the English sample text. Some optional settings, detailed battle explanations and status labels still use the engine's Japanese defaults. Full localization and a larger font are left to the game author. Main journey dialogue, battle commands and save prompts have English overrides in `examples/play/game/game.yaml:18`.

## Capture

On Linux, install Go, Python 3, ffmpeg, Xvfb and xauth (ffmpeg must include ffprobe), plus the libraries in [graphics setup](../development/graphics.md#requirements). Python uses only its standard library. Run from the repository root:

```sh
xvfb-run --auto-servernum --error-file=/dev/stderr bash scripts/capture-example.sh
```

The script builds the actual sample, fixes its seed, selects its menu actions, runs the battle, lights the marker, walks to the ridge, saves, returns to the title, restores the slot, saves it in a second slot, and moves into the room. The game writes nine PNGs from its Draw loop. Before encoding, Python checks that both actual saves contain the ridge location, the completion flag and 7G. It copies the camp frame to `docs/images/wayfinder.png` (960×540) and encodes a nine-second, 480×270 GIF slideshow to `docs/images/wayfinder.gif`. This is a sequence of real screenshots, not a continuous video. The generated executable stays in ignored `build/capture/`; temporary saves, traces and intermediate frames are removed. Existing showcase PNG/GIF files are replaced.

`capture-example.txt` is the input sequence, `capture-example.sh` owns temporary paths and prerequisites, and `capture-example.py` validates progress and encodes the images. Failures return a nonzero status. The script checks the final room position in the trace and the encoded GIF dimensions, frame count and duration with ffprobe. The loop has a 90-second timeout. For independent screen checks, run:

```sh
xvfb-run --auto-servernum --error-file=/dev/stderr make graphics
```

Windows/macOS are intended interactive targets; runtime verification is still pending. The capture wrapper is for Linux. Capture success does not verify audio, every menu, or those other operating systems. See the [release checklist](../development/release-checklist.md).

## Small examples

`examples/minimal` is a 35-line Go host with 17 lines of YAML across `game.yaml` and `data/world.yaml`. It loads an embedded `fs.FS`, creates state and runs the first arrival event without graphics. The two default battle skill IDs must exist even in a game without battles. Its only output is `location=camp gold=2`.

`examples/headless` demonstrates deterministic movement, a choice, battle and a save/restore round trip in memory. Its output is `location=trail battles=1 wins=1 gold=3`. Neither example writes player saves.

```sh
go run ./examples/minimal
go run ./examples/headless
```
