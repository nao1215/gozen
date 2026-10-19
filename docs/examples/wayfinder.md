# Wayfinder

Wayfinder is an original, small game assembled with the public Gozen API. Practice with a friendly stone companion, light a trail marker, and walk to Lantern ridge, where Sela the lamplighter joins you. Bram waits in the village with an inn and joins too. A grotto has a wild Pebble to fight again (plus a sturdier Cobble and a rockfall of three for balance checks), and the observation halls are a first-person maze with branches, dead ends, a chest, a sentry fight where Bram's Shield Rush shows a cut-in, and a wheel that opens the gate to the lantern. The story, names, dialogue and pictures were made for this repository; the pictures are CC BY 4.0, see [ASSETS.md](../../ASSETS.md).

![Title screen](../images/wayfinder-title.jpg)
![Conversation with the courier](../images/wayfinder-talk.jpg)
![Practice battle in the grotto background](../images/wayfinder-battle.jpg)
![Lamplight village](../images/wayfinder-village.jpg)
![The observation halls, a Pebble sentry ahead](../images/wayfinder-dungeon.jpg)
![A party of three fights the sentries](../images/wayfinder-party.jpg)
![Bram's Shield Rush cut-in](../images/wayfinder-cutin.jpg)

## Run

From the repository root, with Go 1.26.4 or later and a [graphics environment](../development/graphics.md#requirements):

```sh
go run ./examples/play --mute
```

The example embeds its YAML and pictures (`examples/play/game/`, laid out like a game directory) in the binary. It needs no downloaded assets, ONNX models, audio device or external game directory. `--mute` disables audio initialization. There is no sample music or sound recording.

## Play through

1. Select **New journey**, then **Talk: Courier**. Enter advances the two lines.
2. Select **Practice**. In battle, select **Auto** to let the engine fight, or choose **Attack**, then Pebble as the target, each turn. Winning unlocks **Light the marker**.
3. Select **Light the marker**, read the message, then **Walk to the ridge**. The arrival event awards 4G once (practice awarded 3G), and Sela joins the party.
4. Select **Save journey**, choose slot 1 and wait for confirmation. Press Esc to open the menu, choose **Return to title**, confirm, and use **Continue** to restore the journey. Menu **Load** also restores a slot after confirmation.
5. Return to camp and select **Visit the village**. Talk to Bram and he joins; the innkeeper talks and the inn heals. **Enter the grotto** fights a wild Pebble again; it also offers **Face the Cobble** and **Brave the rockfall**, which are much harder at level 1.
6. Return to camp and select **Explore chamber**. Up walks forward, Left/Right turns, Down turns around, and walking into a chest, a sentry or a wheel uses it. A chest at a dead end holds 5G. The Pebble sentries (**!** on the map) guard the east hall; Auto in their fight lets Bram open with Shield Rush, a skill with `cutin: true`. The iron gate stays shut until the wheel (**W**) in the west hall is turned; the lantern (**L**) behind it reads the window event, and the door **C** leads back to camp. The minimap fills as you walk.

The ridge path is hidden until its marker is lit. Revisiting the ridge after loading does not award its arrival reward again. These transitions, the companions joining, the halls (chest, sentries, wheel and gate, a save and load inside them) and their return route are checked against the same definitions in `examples/play/journey_test.go:16`. The halls are a hand-written `dungeon.map` in `examples/play/game/data/world.yaml` using only tile definitions (`solid`, `once` with `after`, `when`, `trigger: bump`, `look: wall` and `look: door`, pictures and textures); the cut-in uses the standing picture, since the actors have no separate cut-in picture.

## Controls and saves

| Input | Meaning |
| --- | --- |
| Up / Down, Enter / Z / Space | Menu selection and confirmation |
| X / Esc | Back, or open the field menu |
| O / F2 | Settings |
| F11 / Alt+Enter | Fullscreen |
| M | Mute |
| F7 (with `--dev`) | Balance panel: win rate (healed and as now), HP left, wins in a row and win rate by level for the battles here ([details](../development/graphics.md#バランス表示)) |
| Up, Left / Right, Down in the halls | Forward (or use what is ahead), turn, turn around |

Normal saves use the platform config directory under `gozen/wayfinder-demo`: typically `$XDG_CONFIG_HOME` or `~/.config` on Linux, `%AppData%` on Windows, and `~/Library/Application Support` on macOS. An explicit `--save-dir` is the complete destination. The scripted capture uses a private temporary directory and removes it when finished. A development run with `--start` also isolates saves unless a destination is explicit (`runner/runner.go:128`, `internal/play/play.go:356`).

The bundled Go Regular font supports the English sample text. Some optional settings and status labels still use the engine's Japanese defaults. Full localization and a larger font are left to the game author. Main journey dialogue, battle commands and save prompts have English overrides in `examples/play/game/game.yaml:18`.

## Capture

On Linux, install Go, Python 3, ffmpeg, Xvfb and xauth (ffmpeg must include ffprobe), plus the libraries in [graphics setup](../development/graphics.md#requirements). Python uses only its standard library here. Run from the repository root:

```sh
xvfb-run --auto-servernum --error-file=/dev/stderr bash scripts/capture-example.sh
```

The script builds the actual sample, fixes its seed, selects its menu actions, talks to the courier, runs the practice battle, lights the marker, walks to the ridge (Sela joins), saves, returns to the title, restores the slot, visits the village (Bram joins), and walks the observation halls: the sentry fight with the cut-in, the chest, the gate, the wheel and the lantern. The game writes PNG frames from its Draw loop, five per second of animation; the battles' frames come from the game while it plays them. Before encoding, Python checks that the actual save contains the ridge location, the completion flag, 7G and Sela, that the trace ends at the lantern in the halls with 18G after the chest and the fight, that the halls' messages and a cut-in were shown, and that every generated picture was shown. It encodes a 13-second, 640×360 GIF of about 66 frames (about 1.4 MB, 112 colors) to `docs/images/wayfinder.gif` and writes seven 960×540 JPEG screenshots (`wayfinder-title.jpg`, `-talk.jpg`, `-battle.jpg`, `-village.jpg`, `-dungeon.jpg`, `-party.jpg`, `-cutin.jpg`). The party and cut-in stills are the fight's frames just before and during the cut-in, found by the ticks in the trace. The game writes some frames with an alpha channel, so every frame is converted to RGB before the GIF's single palette is made; mixed frames made every GIF frame alike, and the script now also checks that the GIF frames differ. The GIF is a sequence of real captured frames played at five per second, not a continuous screen recording. The generated executable stays in ignored `build/capture/`; temporary saves, traces and intermediate frames are removed. Existing showcase files are replaced.

`capture-example.txt` is the input sequence, `capture-example.sh` owns temporary paths and prerequisites, and `capture-example.py` validates progress and encodes the images. Failures return a nonzero status. The script checks the final room position in the trace and the encoded GIF dimensions and duration with ffprobe. The game run has a 240-second timeout. `scripts/capture-balance.sh` (with `scripts/capture-balance.txt`) fights the Cobble once, then captures the development balance panel for the hurt party in the grotto to `docs/images/wayfinder-balance.jpg` in the same 960×540 JPEG form.

![Balance panel in the grotto](../images/wayfinder-balance.jpg)

For independent screen checks, run:

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
