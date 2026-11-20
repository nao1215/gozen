---
title: The edit-play loop
description: Tune a Gozen game while you play it. --dev, F5 reload, F6 character book, F7 win rates, F9 notes, test-play starts, autoplay, traces, replays and gozen notes.
weight: 50
toc: true
---

Gozen keeps the game running while you fix it and records what you saw so you can come back to it. Run a game with `gozen run game`; `--dev` turns on the author tools below, and F5 reloads YAML and asset changes.

Before playing, `gozen check game` reports broken references, wrong expressions, missing pictures and lines too long for their window, each with its file and line:

```text
data/world.yaml:26:5: locations.grotto.encounters: unknown troop "lessn" [GZN1101]
```

Between plays, `gozen sim battle` fights a battle many times and prints the win rate, and `gozen sim dive` plays a whole dungeon dive. The window, a headless session and these commands share the rules, so their numbers agree.

## Playtesting

![Play, script or autoplay to a trace; reproduce a screen, inspect its structure, read play metrics or section frames. Capture and compare with the previous capture.](/images/playtesting-flow.svg)

One recorded run lets you return to a screen and see what happened around it. The examples run in order from an empty directory with `gozen` on PATH; use new output directory names when you repeat them. Recording, replaying and capturing need a display (on Linux a virtual one works, see [graphics requirements](https://github.com/nao1215/gozen/blob/main/docs/development/graphics.md#requirements)); reading a trace does not.

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

This writes `shots/scene.png` and `play.jsonl`. Without `--script` you play by hand and close the window when done; for a reachable dungeon, `--autoplay 60` replaces the script (see [Autoplay](#autoplay)). All three write the same trace format. Do not change the game while recording a case to reproduce.

### Reproduce a screen

```shell
gozen trace play.jsonl --repro shots/scene.png --out repro
sh repro/repro.sh
```

The first command prints a shell command and writes `repro/repro.sh`, `repro/repro.json`, the trace up to the target and any initial saves. The script writes the PNG at the `screenshot` path in `repro.json` without overwriting the original save slots.

The target can also be a simulation tick (`--repro 120`) or a time (`--repro 2s`, rounded to a tick); a screenshot basename works only if it names one tick. `--format json` returns the command, seed, start options, target tick and the latest recorded screen structure, whose own tick may come before the target.

A trace that cannot be reproduced exactly (missing start details or initial saves, a review start, a live reload) is refused, and so is changed game data. The bundle copies no assets, so keep the game unchanged where it was recorded. Traces can hold saved player data.

### Read the screen structure

```shell
gozen trace play.jsonl --only screen
```

Each changed draw records a block with the tick, speaker, phone app, backgrounds, standing cast, text boxes and controls; a control has its label, rectangle, enabled state, focus and, when disabled, the reason. `--format jsonl` keeps the fields as JSON.

Rectangles are `[x, y, width, height]` in UI units on a screen 720 high. Headless simulations draw nothing and record no screens.

### Read play metrics

```shell
gozen trace play.jsonl --metrics --top 5 --stall-seconds 2
```

The report gives the text per decision, the longest stretch without a decision, repeated dialogue, the spacing of scene signals, stretches where the state or screen did not change, and the named sections. In JSON these are `text_per_decision`, `longest_read_only`, `repeated_text`, `climax_spacing`, `stalls` and `sections` in `data`, with unfinished dialogue in `trailing_text`. A short recording may have no decisions or repeats.

The numbers are observations, not verdicts, and times are simulation ticks divided by 60. Metrics read the whole trace, do not combine with display filters, `--check` or `--repro`, and never fail the command.

### Read per-section frame numbers

```shell
gozen trace play.jsonl --only section --format jsonl
```

The `section` objects name `opening` and `after-input` with `from_tick`, the ending `tick`, `ticks`, `frames`, `elapsed_seconds`, `fps`, `tps`, `slow_frame_share`, `heap_inuse_bytes`, `allocations` and `allocations_per_frame`. A final `scope: run` row covers the whole run, and `scope: mark` rows the sections, including the `run` section before the first mark.

A mark takes no tick. Frames are completed draws, not ticks. Timing and allocations depend on the machine. Headless scripts accept marks but measure no frames.

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

`current/index.html` lists changed rows first, with the previous and current pictures and the differences in pink, and folds the unchanged ones; with no edit, this example shows one unchanged capture. Edit the story and capture into a new directory to see a difference. `shots.json` keeps each job's `change`, `diff_pixels`, `diff_ratio` and `diff_bbox`.

Jobs are matched by ID; new and removed jobs count as changes, and with `--only` the jobs left out look removed. Pixels must match exactly, and GIFs also compare frames, delays and looping. Differences alone exit 0.

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

A `--dev` run that starts somewhere other than the beginning (`--start`, `--script`, `--replay`) uses a temporary save directory unless you pass `--save-dir`, so tests never touch a player's saves.

## Keys

| Key | With `--dev` |
|:--|:--|
| F5 | Reload the definitions. The game keeps your place: the location, the party, and the current line of an event. |
| F6 | Open or close the character book, with every page open: each character's profile, faces and relations ([profiles](/cookbook/#keep-a-character-book)). |
| F7 | Show or hide the balance panel: win rates of the battles at this place. |
| F8 | Open or close the development panel: the state (flip a flag, step a number), a table of the flags each event sets and reads, the events to jump to with their read rates, a line of YAML to run, and the Frames overlay (scene state, control rectangles, and Update and Draw times). |
| F9 | Write a note: a screenshot, the game state and your comment. |
| F10 | Tag the picture on screen, when picture review is on. |

The usual keys still work: O or F2 for settings, F11 for full screen, M to mute. In conversations, Q rolls back a line, holding Ctrl passes lines already read, N toggles skip mode, and Page Down skips a scene whose lines were all read (any scene when Text to skip is set to all; see [novel games](/writing-a-game/#novel-games)). A script presses these with `press back`, `press skip` and `press scene_skip`, and the novel band's save and load with `press save` and `press load`.

## F8: the development panel

| State: flip a flag, step a number | Flags: what each event sets and reads |
|:--|:--|
| ![flag.thanked_hedda and chose.toast in the development panel](/images/novel-panel.jpg) | ![the flags each event sets and reads](/images/novel-flags.jpg) |
| Scenes: jump to an event | Command: run a line of YAML |
| ![the Scenes tab jumps to the late night](/images/playtest-jump.mp4) | ![a set typed into the Command tab, then the State tab shows the flag](/images/playtest-command.mp4) |
| Frames: scene state, control rectangles, Update and Draw | |
| ![the Frames overlay over the tavern](/images/playtest-frames.jpg) | |

The panel pauses the game while open; left and right switch tabs. Every change goes through `set` or starts an event, like a headless test's `Do`, and the trace records it as `dev_set`, `jump` or `dev_do`. A script does the same with `jump EVENT [LINE]` (start an event from the current state and read on to the line) and `do YAML` (run commands, such as `do - set: {flag.met: true}`).

Left from State reaches Frames, which lets the game run until F8 closes it. It shows the tick, event and line, location, seed, cast, phone state, and the flags and variables the current event reads, and boxes pressable, disabled (with a reason) and focused controls. Update and Draw are CPU times, not GPU timings.

To open the same overlay in a script:

```text
press panel
press left
```

The flags table and the Flow section of `gozen book` come from the same index of where each name is written and read.

## F6: the character book

The book shows each character in `profiles` with the standing picture, the faces (Z switches them), the fields, the introduction and how they see the others. Under `--dev` every page is open, so you can check a line against the character. Left and right, L and R on a pad, the band's arrows and a swipe turn the page; X or F6 closes the book.

![Wren's page in the character book of After the Pass](/images/novel-characters.jpg)

## F5: reload

![Hedda's line is edited in the YAML and F5 shows the new text at the same place](/images/playtest-reload.mp4)

With `--dev`, the game watches `game.yaml`, `data/` and `assets/`. Pictures and sounds reload by themselves. For definitions, press F5: the game loads and checks them and swaps them in, and the field menu, settings, gallery and character book stay open with the new definitions. In a battle the reload waits until the battle ends; on a screen such as a shop or the save screen, until you close it.

If the edit has an error, the game keeps the old definitions and shows the error with its file and line. A change to the game `id` or a lower `data_version` cannot be reloaded; restart for those.

## F7: balance panel

![win rates by troop in the Cistern Hall](/images/go-zen-balance.jpg)

For each troop you can meet here (random encounters and battle actions, up to six), the panel shows:

- the win rate, from 200 AUTO battles with the party fully healed;
- the HP left after a win, as a band from the 10th to the 90th percentile with the median;
- the average number of turns;
- when someone is hurt or has a state, a second, darker row: the same battles from the party's current HP, MP and states;
- the wins in a row from the current HP without rest (up to 9);
- the win rate at each level from the current one to five above.

`gozen sim battle` prints the same numbers with the same seed. A note's `.save` file is the party at that moment:

<!-- verify: each fixture=go-zen -->
```shell
gozen sim battle game --troop crab --state notes/2026-10-19/001.save --hp
gozen sim battle game --troop crab --state notes/2026-10-19/001.save --as-is --streak 9
```

The panel measures for at most 3 ms a frame and never changes the game state or its random numbers. It measures again when you move or reload, or change the party's level, equipment or skills.

## F9: notes

![F9 opens a note, a comment is typed and saved](/images/playtest-memo.mp4)

F9 opens a note on the current moment. Pick a kind, type a comment (or none) and press Enter. The game writes `notes/<date>/<number>.png` (the screen), `.json` (the place, the line, the party, your comment) and `.save` (the state).

Collect a session's notes into one page:

<!-- verify: run fixture=cli -->
```shell
gozen notes notes -o notes-html
```

Start the game from a note to return to it. In an event, the game begins the event again and reads on to the same line; in a battle, it starts the same battle from the noted state:

<!-- verify: skip opens a game window and waits for input -->
```shell
gozen run game --dev --from-memo notes/2026-10-19/001.json
```

A round is then: play, press F9 at each problem, read the notes page, fix the YAML, start from the note with `--from-memo`, and press F5 until it reads right.

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

The game plays itself for 60 seconds at eight times speed. It walks into the dungeon reachable from the starting town (it needs one), fights, returns after `--auto-floors` floors to rest and shop, and goes down again. `--shot-every 5` saves a screenshot every five seconds into `--shots`. It shows how a long dive feels and how the numbers move. `gozen sim dive` does the same without a window.

## Trace and replay

`--trace FILE` records every line of dialogue, menu, battle, picture, song, state change and input, with the time, as JSON Lines. Read it with `gozen trace`:

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

A `layout` line says where a screen's standing pictures and texts were drawn, once the screen has stayed the same for ten ticks (a sixth of a second). Rectangles are `[x, y, width, height]` in UI units: 720 high whatever the pixels, and as wide as the screen's shape makes it.

- A figure (`figures`) has its `picture`, the `rect` its canvas is drawn in, the `content` rect of its opaque part, the `clip` it is drawn into (the screen, or a call view that cuts it) and `alpha`. `source_cut` names the edges (`top`, `bottom`) the opaque part already touches in the file, which tells a picture painted cut from one cut by where it stands.
- A text (`texts`) has `text`, `rect`, font `size`, `color`, the `back` color behind it on the finished screen (the median of the pixels not in the text's color) and `ratio`, the WCAG 2 contrast ratio.

Texts covered by later drawing are left out, and so are screens while they shake, flash or fade. A video call's person is recorded at rest, without its sway. Moving portraits (puppets) are not listed.

```json
{"t":"layout","tick":672,"since":662,"figures":[{"picture":"portraits/lead.png","rect":[-196,-10.6,1375.3,2010.1],"content":[81.7,7.1,925.7,1988],"clip":[16,76,916,628],"alpha":1}],"texts":[{"text":"Lead","rect":[130,490,60,42],"size":30,"color":"#82aaa9","back":"#333d48","ratio":4.35}]}
```

gozen records these without judging them, since a close-up, a figure leaving the screen or a cut-in may be cut on purpose. A game's own checks can read them: a `content` top above its `clip` top with no `top` in `source_cut` is a head cut by the layout, and a `ratio` under 4.5 (3 for large text) is hard to read. Layout is computed only while a trace is written.

`--replay` plays a trace again, checks that the game goes the same way, and reports the first line that differs with its tick:

<!-- verify: skip opens a game window and waits for input -->
```shell
gozen run game --dev --replay run.jsonl
gozen run game --dev --replay run.jsonl --replay-battle 3        # only the third battle
gozen run game --dev --replay run.jsonl --until 4200 --shots s   # stop at a tick and save the screen
```

After fixing a bug, replay the trace that showed it.

## Scripts and screenshots

A script presses buttons and takes screenshots. It needs `--shots`:

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

`wait N` waits N ticks (60 a second), `press BUTTON` presses `ok`, `cancel`, `up`, `down`, `left`, `right` or another button, `shot NAME` writes `NAME.png`, and `quit` ends the run. In a campaign, `order ID [GENERAL|-] [TARGET]` gives an order and `end_turn` ends the turn. GENERAL may list several, the first leading, each with its tactic (`corin:guard,hilde`), which skips the sortie screen; the trace writes `order`, `sortie`, `clash` and `end_turn` lines. `--record out.mp4` records a video of a scripted, automatic or replayed run (needs ffmpeg). On Linux, `xvfb-run -a` runs any of these without a screen.

[`gozen capture`](/commands/#capture) takes a list of screenshots, each from a fresh start. [`gozen sim script`](/commands/#sim-script) plays a script without a window and writes only the trace, which suits long plays; the same script and seed go the same way.

## All game options

`gozen run --help` prints the same list.

| Group | Options |
|:--|:--|
| Game | `--seed SEED`, `--mute`, `--fullscreen`, `--window SCALE`, `--save-dir DIR`, `--dev` |
| Test play (`--dev`) | `--start LOCATION`, `--battle TROOP`, `--party IDS`, `--level LEVEL`, `--flags FLAGS`, `--gold GOLD`, `--items ITEMS`, `--floor FLOOR`, `--calm`, `--scene SCENE`, `--event EVENT`, `--line LINE`, `--picture FILE`, `--answers ANSWERS` |
| Autoplay (`--dev`) | `--autoplay SECONDS`, `--report FILE`, `--shot-every SECONDS`, `--auto-floors FLOORS`, `--speed TIMES` |
| Scripts and records (`--dev`) | `--script FILE`, `--shots DIR`, `--record FILE`, `--trace FILE`, `--trace-shots`, `--replay FILE`, `--replay-battle N`, `--until TICK` |
| Notes and review (`--dev`) | `--memos DIR`, `--from-memo FILE`, `--allow-legacy-save`, `--gallery-all`, `--book-as-played`, `--review`, `--review-log FILE`, `--review-sources FILE`, `--build-info`, `--external-input` |

Options follow the GNU style: `--seed 7` and `--seed=7` both work, and a single-dash long option such as `-dev` is rejected.
