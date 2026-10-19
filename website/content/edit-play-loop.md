---
title: The edit–play loop
description: Tune a Gozen game while you play it. --dev, F5 reload, F7 win rates, F9 notes, test-play starts, autoplay, traces, replays and gozen memos.
weight: 50
toc: true
---

Most of making an RPG is the second draft: a line that reads wrong, a fight that is too hard. Gozen keeps the game running while you fix it, and keeps a record of what you saw so you can come back to it.

Every option on this page belongs to the game binary, not to `gozen`. Any game started with `runner.Run` has them, including your own; the examples use `go run .` in your game's module.

## Turn on the author tools

```shell
go run . --dev --dir game --memos notes
```

| Option | Why |
|:--|:--|
| `--dev` | Turns on the keys and options below. Players never need it. |
| `--dir game` | Reads the YAML and pictures from disk instead of the embedded copy, so edits can be reloaded. |
| `--memos notes` | Lets F9 write notes into `notes/`. |

Runs with `--dev` that start somewhere other than the beginning (`--start`, `--script`, `--replay`) use a temporary save directory unless you pass `--save-dir`, so testing never touches a player's saves.

## Keys

| Key | With `--dev` |
|:--|:--|
| F5 | Reload the definitions. The game keeps your place: the location, the party, and the current line of an event. |
| F7 | Show or hide the balance panel: win rates of the battles at this place. |
| F9 | Write a note: a screenshot, the game state and your comment. |
| F10 | Tag the picture on screen, when picture review is on. |

The usual keys still work: O or F2 for settings, F11 for full screen, M to mute.

## F5: reload

With `--dir`, the game watches `game.yaml`, `data/` and `assets/`. Pictures and sounds are reloaded on their own. For definitions, press F5: the game loads them, checks them, and swaps them in. If you are in a battle or a menu, it waits until you are back on the field.

If the edit has an error, the game keeps the old definitions and shows the error with its file and line, so a typo never ends the session. Changing the game `id` or lowering `data_version` cannot be reloaded; restart the game for those.

## F7: balance panel

![win rates by troop in the grotto, healed and as now](/images/wayfinder-balance.jpg)

For each troop you can meet here (random encounters and battle actions, up to six), the panel shows:

- the win rate, from 200 AUTO battles with the party fully healed;
- the HP left after a win, as a band from the 10th to the 90th percentile with the median;
- the average number of turns;
- when someone is hurt or has a state, a second, darker row: the same battles from the party's current HP, MP and states;
- the wins in a row from the current HP without rest (up to 9);
- the win rate at each level from the current one to five above.

These are the same numbers `gozen sim` prints, with the same seed, so you can check them in a terminal. A note's `.save` file is the party at that moment:

```shell
gozen sim game cobble --state notes/2026-10-19/001.save --hp
gozen sim game cobble --state notes/2026-10-19/001.save --as-is --streak 9
```

The panel measures a little each frame (at most 3 ms) and never changes the game state or its random numbers. It measures again when you move, reload, or change the party's level, equipment or skills.

## F9: notes

F9 opens a note on the current moment. Pick a kind, type a short comment (it may be empty) and press Enter. The game writes `notes/<date>/<number>.png` (the screen), `.json` (the place, the line, the party, your comment) and `.save` (the state).

Collect a session's notes into one page:

```shell
gozen memos notes -o notes-html
```

To come back to a note, start the game from it. In an event, the game begins the event again and reads on to the same line; in a battle, it starts the same battle from the state at the note:

```shell
go run . --dev --dir game --from-memo notes/2026-10-19/001.json
```

So a round is: play, press F9 on each problem, read the notes page, fix the YAML, `--from-memo` to the note, F5 until it reads right.

## Start anywhere

```shell
go run . --dev --start cave --level 8 --battle cave_boss   # straight into a fight
go run . --dev --start town --party hero,mage --gold 500 --items potion=3
go run . --dev --start cave --flags met_elder,var.trust=3 --calm   # no random battles
go run . --dev --event elder_talk --line 4                 # an event, from its 4th line
```

`--floor N` starts on a floor of a generated dungeon. `--scene` opens a gallery scene; `--picture FILE` stops at the line shown over a picture; `--answers 0,1` answers the questions on the way.

## Autoplay

```shell
go run . --dev --mute --autoplay 60 --speed 8 --trace run.jsonl --report run.txt
```

The game plays by itself for 60 seconds at eight times speed: it walks into the dungeon reachable from town, fights, goes back after `--auto-floors` floors to rest and shop, and goes down again. It needs a dungeon the party can reach from the starting town. `--shot-every 5` takes a screenshot every five seconds into `--shots`. Use it to see how a long dive feels, and how the numbers move, without playing it.

For a dive without a window, `gozen curve` does the same with the rules alone.

## Trace and replay

`--trace FILE` records what happens as JSON Lines: every line of dialogue, menu, battle, picture, song, state change and input, with the time. Read it with `gozen trace`:

```shell
gozen trace run.jsonl --only say,battle --from 10 --to 60
gozen trace run.jsonl --check    # standing pictures and music that went wrong
```

```text
00:00.02 scene      field  @camp
00:00.02 state      0G @camp  Walker Lv1 30/30 5/5
00:00.33 menu       [Talk: Courier] | Practice | Save journey | Rest | → Explore chamber | ...
```

A trace can be played again. `--replay` checks that the game goes the same way and reports the first line that differs, with its tick:

```shell
go run . --dev --replay run.jsonl
go run . --dev --replay run.jsonl --replay-battle 3        # only the third battle
go run . --dev --replay run.jsonl --until 4200 --shots s   # stop at a tick and save the screen
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

```shell
go run . --dev --mute --seed 7 --start camp --script script.txt --shots captures
```

`wait N` waits N ticks (60 a second), `press BUTTON` presses `ok`, `cancel`, `up`, `down`, `left`, `right` and the other buttons, `shot NAME` writes `NAME.png`, and `quit` ends the run. `--record out.mp4` writes a video of a scripted, automatic or replayed run (needs ffmpeg). On Linux, wrap any of these in `xvfb-run -a` to run without a screen.

To take a list of screenshots in one go, each from a fresh start, use [`gozen shots`](/commands/#shots).

## All game options

Run your game with `--help` to see the same list in the terminal.

| Group | Options |
|:--|:--|
| Game | `--dir DIR`, `--seed SEED`, `--mute`, `--fullscreen`, `--window SCALE`, `--save-dir DIR`, `--dev` |
| Test play (`--dev`) | `--start LOCATION`, `--battle TROOP`, `--party IDS`, `--level LEVEL`, `--flags FLAGS`, `--gold GOLD`, `--items ITEMS`, `--floor FLOOR`, `--calm`, `--scene SCENE`, `--event EVENT`, `--line LINE`, `--picture FILE`, `--answers ANSWERS` |
| Autoplay (`--dev`) | `--autoplay SECONDS`, `--report FILE`, `--shot-every SECONDS`, `--auto-floors FLOORS`, `--speed TIMES` |
| Scripts and records (`--dev`) | `--script FILE`, `--shots DIR`, `--record FILE`, `--trace FILE`, `--trace-shots`, `--replay FILE`, `--replay-battle N`, `--until TICK` |
| Notes and review (`--dev`) | `--memos DIR`, `--from-memo FILE`, `--allow-legacy-save`, `--gallery-all`, `--review`, `--review-log FILE`, `--review-sources FILE`, `--build-info`, `--external-input` |

Options follow the GNU style: `--seed 7` and `--seed=7` both work, and a single-dash long option such as `-dev` is rejected.
