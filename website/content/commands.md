---
title: Commands
description: Every gozen command with its options and examples - check, sim (battle, dive, floors, design), play (trace, notes, shots), book, pack (release, trial), art (prompt, mosaic, fetch, faces, puppet), version and help.
weight: 60
toc: true
---

`gozen` reads a game directory (`game.yaml`, `data/`, `assets/`) and checks, simulates, documents and packages it. Results go to stdout and diagnostics to stderr.

<!-- verify: skip the form of a command line, not a command -->
```shell
gozen <command> [<sub-command>] [OPTIONS] [ARGUMENTS]
gozen help <command> [<sub-command>]   # the same as the command with --help
gozen help GZN1101                     # explain a diagnostic code
```

| Command | What it does |
|:--|:--|
| [check](#check) | Check definitions, references, expressions, assets and text layout |
| [sim](#sim) | Measure play without a window: [battle](#sim-battle), [dive](#sim-dive), [floors](#sim-floors), [design](#sim-design) |
| [play](#play) | Read and shoot real plays: [trace](#play-trace), [notes](#play-notes), [shots](#play-shots) |
| [book](#book) | Write the definitions and events as Markdown |
| [pack](#pack) | Build what you ship: [release](#pack-release), [trial](#pack-trial) |
| [art](#art) | Picture and asset tools: [prompt](/production-tools/#gozen-art-prompt), [mosaic](/production-tools/#gozen-art-mosaic), [fetch](#art-fetch), [faces](#art-faces), [puppet](#art-puppet) |
| [version and help](#version-and-help) | Print the version; show a command's help or explain a diagnostic code |

A group alone (`gozen sim`) exits 2 and lists its commands. `gozen sim --help` shows the group's page.

## Conventions

- Exit status: 0 for success and help, 1 when a check, I/O, simulation or capture fails, 2 for invalid arguments. A name the command line gives that the game does not have (a troop, a location, a persona, a shot) is an invalid argument.
- A diagnostic may end with a code such as `[GZN1101]`; its first digit is the exit status. `gozen help GZN1101` explains it, and [Error codes](/errors/) lists them all.
- Options follow the GNU style. `--seed 7` and `--seed=7` both work, a few common options have a one-letter form (`-n 100`, `-o DIR`, `-v`, `-h`), options may come before or after the arguments, and `--` ends the options. A single-dash long option such as `-seed` is rejected with exit status 2.
- Commands that write a directory (`sim design`, `play notes`, `pack release`, `pack trial`, `play shots`) refuse one that already exists, even if empty, and an output inside an input. They never overwrite.
- Every command except `play shots`, `art faces` and `art mosaic` detection needs nothing beyond the `gozen` binary: no cgo, display or sound device.

The examples below use `game` as the game directory, `lesson` as a troop and `grotto` as a dungeon location.

## check

Loads every definition, resolves references and expressions, checks that the assets exist, and lays out dialogue with the game's font. Warnings go to stderr and do not fail the check.

<!-- verify: each fixture=cli -->
```shell
gozen check game
gozen check --only structure game  # structure only, before the game has fonts
gozen check -- -draft              # a directory whose name starts with a dash
gozen check --format json game     # every error and warning as JSON, for a tool or an agent
```

```text
ok: 2 locations, 1 events, 1 actors, 2 skills, 0 items, 1 enemies, 1 troops, 0 ai sets
data/world.yaml:26:5: locations.grotto.encounters: unknown troop "lessn" [GZN1101]
```

| Option | Meaning |
|:--|:--|
| `--only structure` | Check definitions, references, expressions and assets only; do not read the font or check the text layout. stdout says so. |
| `--only layout` | Check only that dialogue and choices fit the message window, with the game's font and display rules. |
| `--format` | `text` (the default) or `json`: a JSON array on stdout with one object per problem, `{"file", "line", "col", "path", "message", "severity", "code"}`, where `severity` is `error` or `warning` and `code` is the [diagnostic code](/errors/) when the problem has one. An empty game check prints `[]`. It exits 1 when one is an error. |

```json
[
  {
    "file": "data/world.yaml",
    "line": 8,
    "col": 15,
    "path": "events.bad.do[0].battle",
    "message": "unknown troop \"wolves\"",
    "severity": "error",
    "code": "GZN1101"
  }
]
```

`--only layout` lays out every line of dialogue and every choice with the game's font and display rules, and reports the ones that overflow. Warnings alone exit 0.

<!-- verify: run fixture=cli -->
```shell
gozen check --only layout game
```

## sim

Plays the game by its rules alone, with no window or sound. `--seed`, `--turns` and `--format` mean the same in every `sim` command: the same `--seed` gives the same result.

### sim battle

Fights the troop with the starting party (or `--party`, or a saved state) under AUTO and prints the win, loss, flight and draw rates and the average turns. The numbers are the same as the [F7 balance panel](/edit-play-loop/#f7-balance-panel). The same `--seed` gives the same result.

<!-- verify: each fixture=cli -->
```shell
gozen sim battle game lesson -n 100
gozen sim battle game lesson -n 100 --party scout --level 5 --seed 7
gozen sim battle game lesson --state memo.save --hp                 # from a note, HP left after a win
gozen sim battle game lesson --state memo.save --as-is --streak 10  # keep the saved HP, count wins in a row
gozen sim battle game lesson -n 1 -v                                # the messages of the first battle
```

<!-- verify: output -->
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

### sim dive

Takes the starting party down a dungeon location, fighting `--battles` battles on each floor, buying better equipment from `--shops`, resting, and fighting the bosses. Prints the level, HP, gold and win rates of each floor.

<!-- verify: each fixture=cli -->
```shell
gozen sim dive game grotto --floors 3 --battles 5
gozen sim dive game grotto --floors 3 --boss 3:lesson --grind 0.8
gozen sim dive game grotto --floors 4 --rest 2 --boss 2:lesson --tries 50
```

<!-- verify: output -->
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

### sim floors

Generates the floors of a dungeon location and prints each map with the steps to the stairs and the expected encounters. With `--try`, compares seeds on one floor.

<!-- verify: each fixture=cli -->
```shell
gozen sim floors game grotto --from 1 --to 3
gozen sim floors game grotto --floor 1 --try 3
gozen sim floors game grotto --no-map
```

| Option | Meaning |
|:--|:--|
| `--from N`, `--to N` | The first and last floor to show (default: 1 to 20) |
| `--floor N` | Show only this floor; with `--try`, the floor whose seeds are compared |
| `--try N` | Compare seeds 0 to N-1 on `--floor`, one line each |
| `--no-map` | Print the numbers only |

### sim design

Plays the game headless with the personas of `designeval.yaml` (players, rewards and time weights) and writes `summary.json`, the timelines, `band.png` and `report.md` to a new directory. A simulation error keeps the report and exits 1. A small configuration is in [the CLI test fixture](https://github.com/nao1215/gozen/blob/main/e2e/atago/fixtures/game/designeval.yaml).

<!-- verify: each fixture=cli -->
```shell
gozen sim design game -o report
gozen sim design game -o report --only walker -v
```

| Option | Meaning |
|:--|:--|
| `--config FILE` | The personas and measures (default: `designeval.yaml` in the game directory) |
| `--only IDS` | Run only these personas |
| `-o, --out DIR` | Write the run to this new directory (default: `<root>/<date>`) |
| `--root DIR` | Where runs go when `--out` is not given (default: `build/designeval`) |
| `--memos DIR` | Write notes on marked places to this directory |
| `-v, --verbose` | Write each persona's dives, choices and stumbles to `debug.log` |

### sim campaign

Plays the game's campaign ([Campaigns](/writing-a-game/#campaigns)) many times, headless, with a strategy for the player, and prints the win rate, the turns the campaigns ended in, the mean of the regions each faction held at the start of each turn, the factions that fell and when, the orders given, and how the marches went. Campaign *i* uses the seed `--seed` + *i*, so the same `--seed` gives the same result.

<!-- verify: skip the CLI fixture has no campaign; e2e/atago/campaign.atago.yaml runs these -->
```shell
gozen sim campaign game
gozen sim campaign game --as random -n 500 --format json
gozen sim campaign game --tactic guard
```

| Option | Meaning |
|:--|:--|
| `-n, --campaigns N` | Campaigns to play (default: 200) |
| `--as STRATEGY` | `greedy` marches when the odds of the clash (its formulas with `rand()` at its middle) are good, else gives another order, one with a general first; `random` gives any order it can; `ai` plays the player's faction by its own `ai` rules (default: greedy) |
| `--tactic ID` | Every general marches with this tactic in a clash shown as units (`campaign.clash.tactics`, or the engine's `front`, `weakest`, `strongest`, `leader`, `guard`), and a march takes the most generals it can; the orders are counted as `order:tactic`. Compare the win rates of the tactics this way (default: each general's own) |
| `--orders N` | Orders a campaign may give before it is cut (default: 2000) |

## play

Reads and takes pictures of real plays. The game binary itself is run by its own `--dev`.

### play trace

Prints a trace written by the game's `--trace` in time order. With `--check`, lists faults of the standing pictures and the music instead and exits 1 if there are any.

<!-- verify: each fixture=cli -->
```shell
gozen play trace run.jsonl
gozen play trace run.jsonl --only say,battle --from 10 --to 60
gozen play trace run.jsonl --check
```

| Option | Meaning |
|:--|:--|
| `--only KINDS` | Print only these kinds, separated by commas: `say`, `scene`, `battle`, `menu`, `view`, `state`, `act`, `problem` and so on |
| `--from SECOND`, `--to SECOND` | The time window; `--to 0` is the end |
| `--input` | Also print the player's input |
| `--check` | List faults of standing pictures and music; exit 1 if any |
| `--music-late SECONDS` | With `--check`, a song heard later than this after it was asked for is late (default: 0.5) |

### play notes

Reads the notes the game writes with F9 and builds an HTML page with thumbnails in a new directory. The notes are not changed.

<!-- verify: each fixture=cli -->
```shell
gozen play notes notes                 # writes notes-html/
gozen play notes notes -o review-html
```

| Option | Meaning |
|:--|:--|
| `-o, --out DIR` | Write the HTML to this new directory (default: `<memo-dir>-html`) |

### play shots

Launches the game binary once per entry of a list and writes PNG, JPEG, an HTML index and `shots.json` to a new directory. Each shot runs with its own save directory.

<!-- verify: skip needs a game binary built with the runner and a display; e2e/atago/graphics.atago.yaml runs gozen play shots -->
```shell
gozen play shots game shots.yaml --bin ./mygame --raw -o shots
gozen play shots game shots.yaml --bin ./mygame --raw --only camp --timeout 1m
xvfb-run -a gozen play shots game shots.yaml --bin ./mygame --raw     # unattended on Linux
```

A list:

<!-- verify: parse read by gozen play shots, which needs a game binary and a display -->
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

`play shots` needs what the game window needs: a display and the graphics libraries (on Linux, Xvfb for unattended runs). A failed launch is recorded in the work tree and exits 1.

## book

Writes the locations, events, scenes, troops and items of the game to stdout as one Markdown document. A quick way to review a whole game, or what an agent wrote. The headings follow the game's `language`.

<!-- verify: run fixture=cli -->
```shell
gozen book game > book.md
```

## pack

Copies the game to a new directory. `--overlay` takes replaced assets from a directory, and `--inspect` reads the approval records every asset of the overlay must have.

### pack release

Checks the approval records and the assets the game refers to, then copies the game to a new directory, taking replaced assets from an overlay. Any unapproved reference fails the build.

<!-- verify: each fixture=cli -->
```shell
gozen pack release game release --inspect inspect.jsonl
gozen pack release game release --overlay overlay --inspect inspect.jsonl
```

| Option | Meaning |
|:--|:--|
| `--inspect FILE` | The inspection records (JSON Lines) from production; required |
| `--overlay DIR` | Take replaced assets from this directory (default: the game's approval settings) |
| `--prefix DIR` | The asset directory inside the game, separated by `/` (default: `assets`) |

### pack trial

Keeps only the definitions and assets reachable within the range of `trial.yaml` and writes them to a new directory: a demo version of the game.

<!-- verify: each fixture=cli -->
```shell
gozen pack trial game -n                            # count what would be kept, write nothing
gozen pack trial game trial --list trial.txt        # build, and list what was kept and dropped
gozen pack trial game trial --overlay overlay --inspect inspect.jsonl
```

| Option | Meaning |
|:--|:--|
| `--config FILE` | The range settings, relative to the game directory (default: `trial.yaml`) |
| `--overlay DIR`, `--inspect FILE` | Take replaced assets from an overlay; the two go together |
| `--list FILE` | Write what was kept and dropped to this new file |
| `-n, --dry-run` | Write nothing; print only how much would be kept |

## art

Tools for the pictures of a game. [art prompt](/production-tools/#gozen-art-prompt) assembles image-generation text from a catalog, and [art mosaic](/production-tools/#gozen-art-mosaic) pixelates regions of images; both are described on [Production tools](/production-tools/).

### art fetch

Downloads fixed revisions of the detection models (`censor`, `face`, `head`) into the user cache and checks each file against a SHA-256 written in gozen. `art mosaic` and `art faces` use a model in the cache when `--model` is omitted.

<!-- verify: skip downloads models from the network -->
```shell
gozen art fetch --list     # the models and whether each is in the cache
gozen art fetch face       # the face detector of art faces
```

### art faces

Finds the face in each standing picture a painted scene (`stage:`) cuts a balloon face from (the people placed in a stage and the actors' face pictures) with the anime face detector that `gozen art fetch face` downloads, and writes the boxes with each picture's size and SHA-256 to `assets/faces.json`. The game only reads that file and runs no model, so a web build shows the same faces. A doll's `icon` (a face drawn for the balloons) comes first and is not searched, then its `face_at`, then the box; a picture without a box gets a guessed face, and `gozen check` warns of it and of a picture changed since.

<!-- verify: skip downloads a model and needs a cgo build with ONNX Runtime -->
```shell
gozen art fetch face                          # download the model once
gozen art faces game                                 # find the faces
gozen art faces game --preview faces-check           # and look at the boxes and the cut faces
```

| Option | Meaning |
|:--|:--|
| `--model MODEL` | The face detection model (default: the one in the cache). |
| `--ort LIBRARY` | The ONNX Runtime shared library (default: `$ONNXRUNTIME_LIB`). |
| `--min SCORE` | The lowest score of a face (0.3). |
| `--preview DIR` | Also write each picture with its face box and its balloon face to this new directory. |

It needs a gozen built with cgo, an ONNX Runtime shared library and the model, as `gozen art mosaic` does; a build without cgo says so and fails.

### art puppet

Loads a moving portrait (a puppet YAML for the [puppet](/go-api/#moving-portraits-puppet) package) with the same checks as `puppet.Load` and prints where each face, closed-eyes and mouth picture lies on the base picture. A difference picture as large as the base is cut down to the part that differs.

<!-- verify: each fixture=puppet -->
```shell
gozen art puppet streamer/puppet.yaml                        # check it and show the positions
gozen art puppet streamer/puppet.yaml --out streamer-packed  # write it with small difference pictures
```

| Option | Meaning |
|:--|:--|
| `-o, --out DIR` | Write the puppet YAML and the cut pictures (each with `at`) to this new directory. The written YAML has no comments. |

It needs nothing beyond the gozen binary: no cgo, display or sound device.

## version and help

```shell
gozen version
gozen -v
```

Prints the version recorded by `go install`, or `devel` for a build from source.

`gozen help` takes the words of a command (`gozen help sim battle` is `gozen sim battle --help`), or a diagnostic code:

<!-- verify: each -->
```shell
gozen help sim battle
gozen help GZN2101
```

