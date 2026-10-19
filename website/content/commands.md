---
title: Commands
description: Every gozen subcommand with its options and examples - check, layout, sim, curve, floors, trace, book, design, memos, release, trial, shots, prompt, mosaic and version.
weight: 60
toc: true
---

`gozen` reads a game directory (`game.yaml`, `data/`, `assets/`) and checks, simulates, documents and packages it. Results go to stdout and diagnostics to stderr.

```shell
gozen <command> [OPTIONS] [ARGUMENTS]
gozen help <command>          # the same as gozen <command> --help
```

| Command | What it does |
|:--|:--|
| [check](#check) | Check definitions, references, expressions, assets and text layout |
| [layout](#layout) | Check that dialogue and choices fit the message window |
| [sim](#sim) | Fight AUTO battles against a troop and print the win rate |
| [curve](#curve) | Simulate a dive floor by floor: growth, battles and bosses |
| [floors](#floors) | Show generated dungeon floors: maps, steps and encounters |
| [trace](#trace) | Print or check a JSON Lines trace of a play |
| [book](#book) | Write the definitions and events as Markdown |
| [design](#design) | Play the game with personas and write a design report |
| [memos](#memos) | Turn test-play notes into HTML with thumbnails |
| [release](#release) | Build a release tree with approved assets only |
| [trial](#trial) | Build a trial tree with the reachable part of the game |
| [shots](#shots) | Launch the game and take the screenshots of a list |
| [prompt](/production-tools/#gozen-prompt) | Assemble image-generation text from a catalog |
| [mosaic](/production-tools/#gozen-mosaic) | Pixelate regions of images, with optional ONNX detection |
| [version](#version) | Print the gozen version |

`gozen prompt` and `gozen mosaic` work on pictures rather than the game, and are described on [Production tools](/production-tools/).

## Conventions

- Exit status: 0 for success and help, 1 when a check, I/O, simulation or capture fails, 2 for invalid arguments.
- Options follow the GNU style. `--seed 7` and `--seed=7` both work, a few common options have a one-letter form (`-n 100`, `-o DIR`, `-v`, `-h`), options may come before or after the arguments, and `--` ends the options. A single-dash long option such as `-seed` is rejected with exit status 2.
- Commands that write a directory (`design`, `memos`, `release`, `trial`, `shots`) refuse one that already exists, even if empty, and an output inside an input. They never overwrite.
- Every command except `shots` and `mosaic` detection needs nothing beyond the `gozen` binary: no cgo, display or sound device.

The examples below use `game` as the game directory, `lesson` as a troop and `grotto` as a dungeon location.

## check

Loads every definition, resolves references and expressions, checks that the assets exist, and lays out dialogue with the game's font. Warnings go to stderr and do not fail the check.

```shell
gozen check game
gozen check --no-layout game     # structure only, before the game has fonts
gozen check -- -draft            # a directory whose name starts with a dash
```

```text
ok: 2 locations, 1 events, 1 actors, 2 skills, 0 items, 1 enemies, 1 troops, 0 ai sets
data/world.yaml:26:5: locations.grotto.encounters: unknown troop "lessn"
```

| Option | Meaning |
|:--|:--|
| `--no-layout` | Check definitions, references, expressions and assets only; do not read the font or check the text layout. stdout says so. |

## layout

Lays out every line of dialogue and every choice with the game's font and display rules, and reports the ones that overflow. Warnings alone exit 0. `gozen check` runs the same layout check.

```shell
gozen layout game
```

## sim

Fights the troop with the starting party (or `--party`, or a saved state) under AUTO and prints the win, loss, flight and draw rates and the average turns. The numbers are the same as the [F7 balance panel](/edit-play-loop/#f7-balance-panel). The same `--seed` gives the same result.

```shell
gozen sim game lesson -n 100
gozen sim game lesson -n 100 --party scout --level 5 --seed 7
gozen sim game lesson --state memo.save --hp                 # from a note, HP left after a win
gozen sim game lesson --state memo.save --as-is --streak 10  # keep the saved HP, count wins in a row
gozen sim game lesson -n 1 -v                                # the messages of the first battle
```

```text
lesson: 100 battles  win 100%  lose 0%  fled 0%  draw 0%  avg turns 1.0  avg standing after a win 1.0
```

| Option | Meaning |
|:--|:--|
| `--party IDS` | The party, as actor IDs separated by commas (default: the starting party) |
| `--level N` | The party's level (default: each actor's own) |
| `--gear IDS` | Items to give the party and wear where they fit best |
| `--no-tree` | Do not spend skill points (by default they go evenly to the open nodes of the lowest level) |
| `--state FILE` | Start from a saved state (a note's `.save` from F9) with everyone recovered |
| `--as-is` | With `--state`, keep the saved HP, MP and states |
| `-n, --battles N` | Battles to fight (default: 200) |
| `--turns N` | A battle longer than this is a draw (default: 60) |
| `--seed N` | The random seed (default: 1) |
| `--streak N` | Also fight again and again without recovery, up to N wins, and print the wins in a row |
| `--hp` | Also print the party's HP left after a win (10th, 50th and 90th percentile) |
| `-v, --verbose` | Print the messages of the first battle |

## curve

Takes the starting party down a dungeon location, fighting `--battles` battles on each floor, buying better equipment from `--shops`, resting, and fighting the bosses. Prints the level, HP, gold and win rates of each floor.

```shell
gozen curve game grotto --floors 3 --battles 5
gozen curve game grotto --floors 3 --boss 3:lesson --grind 0.8
gozen curve game grotto --floors 4 --rest 2 --boss 2:lesson --tries 50
```

```text
floor Lv   battles lost   turns  hp_left mp     gold   scenes boss    troops
B1    1    5       0      1.0    1.00    1.00   15     0      -       lesson
B2    1    5       0      1.0    1.00    1.00   30     0      -       lesson
B3    1    5       0      1.0    1.00    1.00   45     0      -       lesson
```

| Option | Meaning |
|:--|:--|
| `--floors N` | How many floors to go down (default: 20) |
| `--battles N` | Battles fought on each floor (default: 7) |
| `--boss FLOOR:TROOP` | Bosses, separated by commas, fought before going down from that floor |
| `--tries N` | Battles against each boss to measure its win rate (default: 30) |
| `--grind RATE` | Before a boss, fight more on its floor until the win rate reaches RATE (0 to 1) |
| `--turns N` | A battle longer than this is a draw (default: 60) |
| `--seed N` | The random seed (default: 1) |
| `--shops IDS` | Shops whose equipment is bought when better and affordable |
| `--rest N` | Rest (HP and MP full) only every N floors and after a boss; 0 rests on every floor |
| `--supplies ITEM:COUNT` | Items bought at each rest (with `--rest`) |
| `--rest-cost EXPR` | The gold a rest costs, such as `"members * 10"` |
| `--rest-event EVENT` | An event run at each rest |
| `--back-item ITEM` | The item used up to go back to rest |
| `--worn-hp RATE`, `--worn-mp RATE` | Go back early when the party's HP or MP is below RATE |
| `--events` | Run the game's `after_win` event after each win and `game_over` after a loss |
| `--scene-event EVENT`, `--scene-when EXPR` | An event the player sees after a won battle whenever EXPR holds |

## floors

Generates the floors of a dungeon location and prints each map with the steps to the stairs and the expected encounters. With `--try`, compares seeds on one floor.

```shell
gozen floors game grotto --from 1 --to 3
gozen floors game grotto --floor 1 --try 3
gozen floors game grotto --no-map
```

| Option | Meaning |
|:--|:--|
| `--from N`, `--to N` | The first and last floor to show (default: 1 to 20) |
| `--floor N` | Show only this floor; with `--try`, the floor whose seeds are compared |
| `--try N` | Compare seeds 0 to N-1 on `--floor`, one line each |
| `--no-map` | Print the numbers only |

## trace

Prints a trace written by the game's `--trace` in time order. With `--check`, lists faults of the standing pictures and the music instead and exits 1 if there are any.

```shell
gozen trace run.jsonl
gozen trace run.jsonl --only say,battle --from 10 --to 60
gozen trace run.jsonl --check
```

| Option | Meaning |
|:--|:--|
| `--only KINDS` | Print only these kinds, separated by commas: `say`, `scene`, `battle`, `menu`, `view`, `state`, `act`, `problem` and so on |
| `--from SECOND`, `--to SECOND` | The time window; `--to 0` is the end |
| `--input` | Also print the player's input |
| `--check` | List faults of standing pictures and music; exit 1 if any |
| `--music-late SECONDS` | With `--check`, a song heard later than this after it was asked for is late (default: 0.5) |

## book

Writes the locations, events, scenes, troops and items of the game to stdout as one Markdown document. A quick way to review a whole game, or what an agent wrote. The headings follow the game's `language`.

```shell
gozen book game > book.md
```

## design

Plays the game headless with the personas of `designeval.yaml` (players, rewards and time weights) and writes `summary.json`, the timelines, `band.png` and `report.md` to a new directory. A simulation error keeps the report and exits 1. A small configuration is in [the CLI test fixture](https://github.com/nao1215/gozen/blob/main/e2e/atago/fixtures/game/designeval.yaml).

```shell
gozen design game -o report
gozen design game -o report --only walker -v
```

| Option | Meaning |
|:--|:--|
| `--config FILE` | The personas and measures (default: `designeval.yaml` in the game directory) |
| `--only IDS` | Run only these personas |
| `-o, --out DIR` | Write the run to this new directory (default: `<root>/<date>`) |
| `--root DIR` | Where runs go when `--out` is not given (default: `build/designeval`) |
| `--memos DIR` | Write notes on marked places to this directory |
| `-v, --verbose` | Write each persona's dives, choices and stumbles to `debug.log` |

## memos

Reads the notes the game writes with F9 and builds an HTML page with thumbnails in a new directory. The notes are not changed.

```shell
gozen memos notes                 # writes notes-html/
gozen memos notes -o review-html
```

| Option | Meaning |
|:--|:--|
| `-o, --out DIR` | Write the HTML to this new directory (default: `<memo-dir>-html`) |

## release

Checks the approval records and the assets the game refers to, then copies the game to a new directory, taking replaced assets from an overlay. Any unapproved reference fails the build.

```shell
gozen release game release --inspect inspect.jsonl
gozen release game release --overlay overlay --inspect inspect.jsonl
```

| Option | Meaning |
|:--|:--|
| `--inspect FILE` | The inspection records (JSON Lines) from production; required |
| `--overlay DIR` | Take replaced assets from this directory (default: the game's approval settings) |
| `--prefix DIR` | The asset directory inside the game, separated by `/` (default: `assets`) |

## trial

Keeps only the definitions and assets reachable within the range of `trial.yaml` and writes them to a new directory: a demo version of the game.

```shell
gozen trial game -n                            # count what would be kept, write nothing
gozen trial game trial --list trial.txt        # build, and list what was kept and dropped
gozen trial game trial --overlay overlay --inspect inspect.jsonl
```

| Option | Meaning |
|:--|:--|
| `--config FILE` | The range settings, relative to the game directory (default: `trial.yaml`) |
| `--overlay DIR`, `--inspect FILE` | Take replaced assets from an overlay; the two go together |
| `--list FILE` | Write what was kept and dropped to this new file |
| `-n, --dry-run` | Write nothing; print only how much would be kept |

## shots

Launches the game binary once per entry of a list and writes PNG, JPEG, an HTML index and `shots.json` to a new directory. Each shot runs with its own save directory.

```shell
gozen shots game shots.yaml --bin ./mygame --raw -o shots
gozen shots game shots.yaml --bin ./mygame --raw --only camp --timeout 1m
xvfb-run -a gozen shots game shots.yaml --bin ./mygame --raw     # unattended on Linux
```

A list:

```yaml
shots:
  - id: camp
    title: Camp overview
    start: camp
    wait: 30
```

`id` becomes the file name. An entry can also start a `scene` or an `event`, stop at a `picture`, `line` or `answers`, or run a `script`.

| Option | Meaning |
|:--|:--|
| `--bin FILE` | The game binary, built with the Gozen runner; required |
| `--raw` | Use the original assets without checking approval (for local review) |
| `--overlay DIR` | Take approved replacements from this directory and check approval; pending approval exits 1 |
| `--timeout DURATION` | The time limit of one shot, such as `90s` or `2m` (default: 4m) |
| `-o, --out DIR` | Write the shots to this new directory (default: `build/shots/<date-time>`) |
| `--only IDS` | Take only these shots |

`shots` needs what the game window needs: a display and the graphics libraries (on Linux, Xvfb for unattended runs). A failed launch is recorded in the work tree and exits 1.

## version

```shell
gozen version
gozen -v
```

Prints the version recorded by `go install`, or `devel` for a build from source.
