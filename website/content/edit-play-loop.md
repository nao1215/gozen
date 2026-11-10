---
title: The edit–play loop
description: Tune a Gozen game while you play it. --dev, F5 reload, F6 character book, F7 win rates, F9 notes, test-play starts, autoplay, traces, replays and gozen notes.
weight: 50
toc: true
---

Most of making a game is the second draft: a line that reads wrong, a fight that is too hard. Gozen keeps the game running while you fix it, and keeps a record of what you saw so you can come back to it.

Run a game directory with `gozen run game`. Add `--dev` to enable the author tools below. YAML and asset changes can be reloaded with F5.

Before playing, `gozen check game` reads the whole game and reports a broken reference, a wrong expression, a missing picture or a line too long for its window, each with its file and line:

```text
data/world.yaml:26:5: locations.grotto.encounters: unknown troop "lessn" [GZN1101]
```

Between plays, `gozen sim battle` fights a battle many times and prints the win rate, and `gozen sim dive` plays a whole dungeon dive; the window, a headless session and these commands follow the same rules, so their numbers agree.

## Playtesting

![Play, script or autoplay to a trace; reproduce a screen, inspect its structure, read play metrics or section frames. Capture and compare with the previous capture.](/images/playtesting-flow.svg)

Use one recorded run to revisit a screen and inspect what happened around it.
The following examples run in order from an empty working directory, with
`gozen` on PATH. Recording, replaying and capturing need a display; reading a
trace does not. On Linux a virtual display works too; see
[graphics requirements](https://github.com/nao1215/gozen/blob/main/docs/development/graphics.md#requirements).
Use new output directory names when repeating the examples.

### Record a short play

Create a small story to try the tools:

```shell
gozen new playtest-game --template novel
```

Save this as `input.txt`. Each `mark` begins a named section; `shot` writes a PNG.

<!-- verify: file input.txt -->
```text
mark opening
wait 80
press ok
wait 30
shot scene
mark after-input
wait 60
quit
```

```shell
gozen run playtest-game --dev --mute --seed 7 --script input.txt --shots shots --trace play.jsonl
```

This writes `shots/scene.png` and `play.jsonl`. For manual play, omit `--script`
and close the window when finished. For a reachable dungeon, replace the script
with `--autoplay 60`; see [Autoplay](#autoplay). Both can write the same trace
format. Keep the game unchanged while recording a reproduction case.

### Reproduce a screen

```shell
gozen trace play.jsonl --repro shots/scene.png --out repro
sh repro/repro.sh
```

The first command prints a shell command and writes `repro/repro.sh`,
`repro/repro.json` and the trace up to the target. Initial saves, if present,
are included too. Running the script writes the PNG at the `screenshot` path
in `repro.json`. Original save slots are not overwritten.

A target can also be a simulation tick (`--repro 120`) or time
(`--repro 2s`, rounded to a tick). A screenshot basename works only if it
identifies one tick. `--format json` returns the command, seed, start options,
target tick and latest recorded screen structure in the result's `data`.
The screen record retains its own tick; it may precede the target.

Reproduction refuses traces it cannot reproduce exactly: missing start details
or initial save bytes, unsupported review starts and live reloads need a new
recording. Replay rejects changed game data. The bundle copies no game assets;
the unchanged game directory must remain at its recorded location even if the
bundle moves. Traces can contain saved player data. A required picture that
cannot finish loading fails the replay instead of producing a placeholder.

### Read the screen structure

```shell
gozen trace play.jsonl --only screen
```

Each changed draw records an indented block with the tick, speaker, phone app,
backgrounds, standing cast, text boxes and controls. Controls include their
label, rectangle, enabled state, focus and reason when disabled. Use
`--format jsonl` to keep the fields as JSON objects.

Rectangles are `[x, y, width, height]` in UI units, with a screen height of 720.
These are drawing facts: an overlay can cover recorded text, icon-only controls
can have empty labels, and moving puppets are not standing-picture entries.
Headless simulations have no drawn screen records. The separate `layout`
records below measure contrast after the screen settles.

### Read play metrics

```shell
gozen trace play.jsonl --metrics --top 5 --stall-seconds 2
```

The text report lists text per decision, the longest span without a decision,
repeated dialogue, spacing between observed scene signals, unchanged-state or
screen spans, and named sections. `--format json` puts these under
`text_per_decision`, `longest_read_only`, `repeated_text`, `climax_spacing`,
`stalls` and `sections` in `data`; unfinished dialogue is `trailing_text`.
A short recording may have no decisions or repeated lines.

Text counts describe offered dialogue, not proof of reading. Signal spacing
uses observed battles, music, pictures and calls, not emotional intensity.
An unchanged screen is not necessarily a fault. Times here are simulation
ticks divided by 60, not wall time. Older traces cannot supply unrecorded
choices or screens. Metrics read the whole trace and cannot be combined with
display filters, `--check` or `--repro`; repetition and waiting do not fail the
command.

### Read per-section frame numbers

```shell
gozen trace play.jsonl --only section --format jsonl
```

The `section` objects name `opening` and `after-input`, with `from_tick`, the
ending `tick`, `ticks` and `frames`. They also contain `elapsed_seconds`,
`fps`, `tps`, `slow_frame_share`, `heap_inuse_bytes`, `allocations` and
`allocations_per_frame`. The final `scope: run` row covers the entire run;
`scope: mark` rows cover sections (including the initial `run` section before
the first mark).

A mark consumes no tick. Frame counts are completed draws, not a frame number
for every simulation tick; updates and draws need not be one-to-one. Timing
and allocation values depend on the machine and include process activity.
Ratios per frame are absent if there was no completed draw. Headless scripts
accept marks but do not collect frame performance.

### Compare captures

Save this job list as `captures.yaml`:

<!-- verify: file captures.yaml -->
```yaml
title: Story opening
seed: 7
shots:
  - id: opening
    event: opening
    wait: 80
```

```shell
gozen capture playtest-game --config captures.yaml --out previous
gozen capture playtest-game --config captures.yaml --out current --against previous
```

Open `current/index.html`: changed rows come first, with previous and current
pictures and pink difference highlights; unchanged rows are collapsed. With
no edit, the example should show an unchanged capture. Edit the story, then
repeat into a new directory to inspect the difference. `shots.json` keeps each
job's `change`, `diff_pixels`, `diff_ratio` and `diff_bbox` alongside its image.

Jobs are matched by ID. New or removed jobs count as changes; `--only` makes
omitted previous jobs appear removed. Pixels are compared exactly, without a
tolerance. GIFs also compare frames, delays and loop settings. Text/control
structure differences appear when both captures contain structure; the first
comparison against an older capture establishes it for the next comparison.
Differences alone exit successfully; capture and file errors still fail.

| Detail | Reference |
|:--|:--|
| Reproduction targets and bundle contents | [Graphics: reproduce a trace screen](https://github.com/nao1215/gozen/blob/main/docs/development/graphics.md#reproduce-a-trace-screen) |
| Screen fields and coordinates | [Graphics: structured screen records](https://github.com/nao1215/gozen/blob/main/docs/development/graphics.md#structured-screen-records) |
| Metric counting and missing observations | [Graphics: play metrics](https://github.com/nao1215/gozen/blob/main/docs/development/graphics.md#play-metrics-from-traces) |
| F8 state and control rectangles | [Graphics: development overlay](https://github.com/nao1215/gozen/blob/main/docs/development/graphics.md#f8-development-overlay) |

## Turn on the author tools

<!-- verify: skip opens a game window and waits for input -->
```shell
gozen run game --dev --memos notes
```

| Option | Why |
|:--|:--|
| `--dev` | Turns on the keys and options below. Players never need it. |
| `--memos notes` | Lets F9 write notes into `notes/`. |

Runs with `--dev` that start somewhere other than the beginning (`--start`, `--script`, `--replay`) use a temporary save directory unless you pass `--save-dir`, so testing never touches a player's saves.

## Keys

| Key | With `--dev` |
|:--|:--|
| F5 | Reload the definitions. The game keeps your place: the location, the party, and the current line of an event. |
| F6 | Open or close the character book, with every page open: each character's profile, faces and relations ([profiles](/cookbook/#keep-a-character-book)). |
| F7 | Show or hide the balance panel: win rates of the battles at this place. |
| F8 | Open or close the development panel: the state (flip a flag, step a number), a table of the flags each event sets and reads, the events to jump to with their read rates, a line of YAML to run, and the Frames overlay (scene state, control rectangles, and Update and Draw times). |
| F9 | Write a note: a screenshot, the game state and your comment. |
| F10 | Tag the picture on screen, when picture review is on. |

The usual keys still work: O or F2 for settings, F11 for full screen, M to mute. In conversations, Q rolls back a line, Ctrl held passes the lines read, N turns skip mode on and off, and Page Down skips a scene whose lines were all read (any scene when the setting Text to skip is all; [novel games](/writing-a-game/#novel-games)); a script presses them with `press back`, `press skip` and `press scene_skip`, and the novel band's save and load with `press save` and `press load`.

## F8: the development panel

| State: flip a flag, step a number | Flags: what each event sets and reads |
|:--|:--|
| ![flag.thanked_hedda and chose.toast in the development panel](/images/novel-panel.jpg) | ![the flags each event sets and reads](/images/novel-flags.jpg) |
| Scenes: jump to an event | Command: run a line of YAML |
| ![the Scenes tab jumps to the late night](/images/playtest-jump.mp4) | ![a set typed into the Command tab, then the State tab shows the flag](/images/playtest-command.mp4) |
| Frames: scene state, control rectangles, Update and Draw | |
| ![the Frames overlay over the tavern](/images/playtest-frames.jpg) | |

The panel holds the game while it is open; left and right change its tab. Every change it makes goes through `set` or starts an event, the same as a headless test's `Do`, and the trace records it: `dev_set` for a change, `jump` for a jump, `dev_do` for a command. A script does the same with `jump EVENT [LINE]` (start an event from the state as it is, read on to the line) and `do YAML` (run a line of commands, such as `do - set: {flag.met: true}`). From State, press Left once to select Frames; the game continues until F8 closes it. Frames shows the tick, event and dialogue line, location, seed, cast and phone state, plus flags and variables read by the current event. Its control boxes show pressable, disabled (with a reason) and focused controls through labels and colors. Update and Draw are CPU times, with averages over their latest calls, not GPU timings. The top band can cover controls without intercepting their input.

To open the same overlay in a script, use:

```text
press panel
press left
```

Collection stops when Frames closes. The flags table and `gozen book`'s Flow section come from one index of where each name is written and read.

## F6: the character book

The book shows each character of `profiles` with the standing picture, the faces to switch between (Z), the fields, the introduction and how they see the others. Under `--dev` every page is open, so you can check a line you write against the character. Left and right, L and R of a pad, the arrows on the band and a swipe go to the next character; X or F6 closes it.

![Wren's page in the character book of After the Pass](/images/novel-characters.jpg)

## F5: reload

![Hedda's line is edited in the YAML and F5 shows the new text at the same place](/images/playtest-reload.mp4)

With `--dev`, the game watches `game.yaml`, `data/` and `assets/`. Pictures and sounds are reloaded on their own. For definitions, press F5: the game loads them, checks them, and swaps them in. The field menu, the settings, the gallery and the character book are rebuilt with the new definitions at once and stay open. In a battle the reload waits until the battle ends, and on another screen, such as a shop or the save screen, until you close it.

If the edit has an error, the game keeps the old definitions and shows the error with its file and line, so a typo never ends the session. Changing the game `id` or lowering `data_version` cannot be reloaded; restart the game for those.

## F7: balance panel

![win rates by troop in the Cistern Hall](/images/go-zen-balance.jpg)

For each troop you can meet here (random encounters and battle actions, up to six), the panel shows:

- the win rate, from 200 AUTO battles with the party fully healed;
- the HP left after a win, as a band from the 10th to the 90th percentile with the median;
- the average number of turns;
- when someone is hurt or has a state, a second, darker row: the same battles from the party's current HP, MP and states;
- the wins in a row from the current HP without rest (up to 9);
- the win rate at each level from the current one to five above.

These are the same numbers `gozen sim battle` prints, with the same seed, so you can check them in a terminal. A note's `.save` file is the party at that moment:

<!-- verify: each fixture=go-zen -->
```shell
gozen sim battle game --troop crab --state notes/2026-10-19/001.save --hp
gozen sim battle game --troop crab --state notes/2026-10-19/001.save --as-is --streak 9
```

The panel measures a little each frame (at most 3 ms) and never changes the game state or its random numbers. It measures again when you move, reload, or change the party's level, equipment or skills.

## F9: notes

![F9 opens a note, a comment is typed and saved](/images/playtest-memo.mp4)

F9 opens a note on the current moment. Pick a kind, type a short comment (it may be empty) and press Enter. The game writes `notes/<date>/<number>.png` (the screen), `.json` (the place, the line, the party, your comment) and `.save` (the state).

Collect a session's notes into one page:

<!-- verify: run fixture=cli -->
```shell
gozen notes notes -o notes-html
```

To come back to a note, start the game from it. In an event, the game begins the event again and reads on to the same line; in a battle, it starts the same battle from the state at the note:

<!-- verify: skip opens a game window and waits for input -->
```shell
gozen run game --dev --from-memo notes/2026-10-19/001.json
```

So a round is: play, press F9 on each problem, read the notes page, fix the YAML, `--from-memo` to the note, F5 until it reads right.

## Start anywhere

<!-- verify: skip opens a game window and waits for input -->
```shell
gozen run game --dev --start cave --level 8 --battle cave_boss   # straight into a fight
gozen run game --dev --start town --party hero,mage --gold 500 --items potion=3
gozen run game --dev --start cave --flags met_elder,var.trust=3 --calm   # no random battles
gozen run game --dev --event elder_talk --line 4                 # an event, from its 4th line
```

`--floor N` starts on a floor of a generated dungeon. `--scene` opens a gallery scene; `--picture FILE` stops at the line shown over a picture; `--answers 0,1` answers the questions on the way.

## Autoplay

<!-- verify: skip opens a game window and waits for input -->
```shell
gozen run game --dev --mute --autoplay 60 --speed 8 --trace run.jsonl --report run.txt
```

The game plays by itself for 60 seconds at eight times speed: it walks into the dungeon reachable from town, fights, goes back after `--auto-floors` floors to rest and shop, and goes down again. It needs a dungeon the party can reach from the starting town. `--shot-every 5` takes a screenshot every five seconds into `--shots`. Use it to see how a long dive feels, and how the numbers move, without playing it.

For a dive without a window, `gozen sim dive` does the same with the rules alone.

## Trace and replay

`--trace FILE` records what happens as JSON Lines: every line of dialogue, menu, battle, picture, song, state change and input, with the time. Read it with `gozen trace`:

<!-- verify: each fixture=cli -->
```shell
gozen trace run.jsonl --only say --only battle --from 10 --to 60
gozen trace run.jsonl --check    # standing pictures and music that went wrong
```

```text
00:00.02 scene      field  @camp
00:00.02 state      0G @camp  Walker Lv1 30/30 5/5
00:00.33 menu       [Talk: Courier] | Practice | Save journey | Rest | → Explore chamber | ...
```

A `layout` line says where the standing pictures and the texts of a screen were drawn, written once the screen has stayed the same for ten ticks (a sixth of a second). Rectangles are `[x, y, width, height]` in UI units: the screen is 720 units high whatever its pixels, and as wide as its shape makes it. A figure (`figures`) has the `picture`, the `rect` its canvas is drawn in, the `content` rect of its opaque part, the `clip` it is drawn into (the screen, or a call's view that cuts it) and `alpha`; `source_cut` names the edges (`top`, `bottom`) the opaque part already touches in the picture file, so a picture drawn cut is told apart from one cut by where it stands. A text (`texts`) has `text`, `rect`, font `size`, `color`, the `back` color behind it read from the finished screen (the median of the pixels that are not the text's color) and `ratio`, the WCAG 2 contrast ratio of the text on it. A text covered by what is drawn over it is left out, and so is a screen while it shakes, flashes or fades. A video call's person is recorded at rest, without its small sway. Moving portraits (puppets) are not listed.

```json
{"t":"layout","tick":672,"since":662,"figures":[{"picture":"portraits/lead.png","rect":[-196,-10.6,1375.3,2010.1],"content":[81.7,7.1,925.7,1988],"clip":[16,76,916,628],"alpha":1}],"texts":[{"text":"Lead","rect":[130,490,60,42],"size":30,"color":"#82aaa9","back":"#333d48","ratio":4.35}]}
```

gozen only records these; it does not judge them, because a close-up, a figure leaving the screen or a cut-in may be cut on purpose. A game's own checks read them: a `content` top above its `clip` top with no `top` in `source_cut` is a head cut by the layout, and a `ratio` under 4.5 (3 for large text) is hard to read. The layout is measured only while a trace is written; without `--trace` nothing of it is computed.

A trace can be played again. `--replay` checks that the game goes the same way and reports the first line that differs, with its tick:

<!-- verify: skip opens a game window and waits for input -->
```shell
gozen run game --dev --replay run.jsonl
gozen run game --dev --replay run.jsonl --replay-battle 3        # only the third battle
gozen run game --dev --replay run.jsonl --until 4200 --shots s   # stop at a tick and save the screen
```

Fix a bug, replay the trace that showed it, and see the difference.

## Scripts and screenshots

A script drives the game with button presses and takes screenshots. It needs `--shots`:

```text
wait 80
press ok
wait 30
shot field
quit
```

<!-- verify: skip opens a game window and waits for input -->
```shell
gozen run game --dev --mute --seed 7 --start camp --script script.txt --shots captures
```

`wait N` waits N ticks (60 a second), `press BUTTON` presses `ok`, `cancel`, `up`, `down`, `left`, `right` and the other buttons, `shot NAME` writes `NAME.png`, and `quit` ends the run. In a campaign, `order ID [GENERAL|-] [TARGET]` gives an order and `end_turn` ends the turn; GENERAL may be several, the first leading, each with its tactic (`corin:guard,hilde`), and then the sortie screen is not shown; the trace writes `order`, `sortie`, `clash` and `end_turn` lines. `--record out.mp4` writes a video of a scripted, automatic or replayed run (needs ffmpeg). On Linux, wrap any of these in `xvfb-run -a` to run without a screen.

To take a list of screenshots in one go, each from a fresh start, use [`gozen capture`](/commands/#capture). To play a script without a window and only read its trace, for example to measure a long play, use [`gozen sim script`](/commands/#sim-script): the same script and seed go the same way, without the screenshots.

## All game options

Run `gozen run --help` to see the same list in the terminal.

| Group | Options |
|:--|:--|
| Game | `--seed SEED`, `--mute`, `--fullscreen`, `--window SCALE`, `--save-dir DIR`, `--dev` |
| Test play (`--dev`) | `--start LOCATION`, `--battle TROOP`, `--party IDS`, `--level LEVEL`, `--flags FLAGS`, `--gold GOLD`, `--items ITEMS`, `--floor FLOOR`, `--calm`, `--scene SCENE`, `--event EVENT`, `--line LINE`, `--picture FILE`, `--answers ANSWERS` |
| Autoplay (`--dev`) | `--autoplay SECONDS`, `--report FILE`, `--shot-every SECONDS`, `--auto-floors FLOORS`, `--speed TIMES` |
| Scripts and records (`--dev`) | `--script FILE`, `--shots DIR`, `--record FILE`, `--trace FILE`, `--trace-shots`, `--replay FILE`, `--replay-battle N`, `--until TICK` |
| Notes and review (`--dev`) | `--memos DIR`, `--from-memo FILE`, `--allow-legacy-save`, `--gallery-all`, `--book-as-played`, `--review`, `--review-log FILE`, `--review-sources FILE`, `--build-info`, `--external-input` |

Options follow the GNU style: `--seed 7` and `--seed=7` both work, and a single-dash long option such as `-dev` is rejected.
