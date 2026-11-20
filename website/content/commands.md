---
title: Commands
description: Every gozen command with its options and examples - check, sim (battle, dive, floors, design, campaign, routes, breed, script, batch), run, trace, notes, capture, book, export, release, trial, prompt, mosaic, models (list, fetch), faces, puppet, theme (add, list), version and help.
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
| [sim](#sim) | Measure play without a window: [battle](#sim-battle), [dive](#sim-dive), [floors](#sim-floors), [design](#sim-design), [campaign](#sim-campaign), [routes](#sim-routes), [breed](#sim-breed), [script](#sim-script), [batch](#sim-batch) |
| [new](#new) | Create a playable template without Go source |
| [run](#run) | Run a game directory in a window |
| [trace](#trace), [notes](#notes) | Inspect traces and build HTML reports from test-play notes |
| [capture](#capture) | Capture PNG images and animated GIFs |
| [book](#book) | Write the definitions and events as Markdown, JSON or JSONL |
| [export](#export) | Export desktop and browser games with prebuilt runtimes |
| [release](#release), [trial](#trial) | Prepare approved asset trees and trial games |
| [prompt](/production-tools/#gozen-prompt), [mosaic](/production-tools/#gozen-mosaic), [models](#models-fetch), [faces](#faces), [puppet](#puppet) | Picture and asset tools |
| [theme](#theme) | Dungeon themes: [add](#theme-add) the pictures of a theme to a game, [list](#theme-list) the themes |
| [version and help](#version-and-help) | Print the version; show a command's help or explain a diagnostic code |

A group alone (`gozen sim`) exits 2 and lists its commands. `gozen sim --help` shows the group's page.

## Conventions

- Exit status: 0 for success and help, 1 when a check, I/O, simulation or capture fails, 2 for invalid arguments, including a name the game does not have (a troop, a location, a persona, a shot).
- A diagnostic may end with a code such as `[GZN1101]` whose first digit is the default exit status; `check --strict` and `--baseline` exit 1 on warnings without changing the code. `gozen help GZN1101` explains a code, and [Error codes](/errors/) lists them all.
- Options follow the GNU style: `--seed 7` and `--seed=7` both work, a few options have a one-letter form (`-n 100`, `-o DIR`, `-v`, `-h`), options may come before or after the arguments, and `--` ends the options. A single-dash long option such as `-seed` is rejected with exit status 2.
- Commands that write a directory (`sim design`, `notes`, `export`, `release`, `trial`, `capture`) never overwrite: they refuse a directory that exists, even an empty one, and an output inside an input.
- Only `export`, `run`, `capture`, `faces` and `mosaic` detection need more than the `gozen` binary (cgo, a display or a sound device). `export` also needs a matching prebuilt runtime bundle.

The examples use `game` as the game directory, `lesson` as a troop and `grotto` as a dungeon location. Configuration and output paths given on the command line are relative to the working directory; game asset paths, including `release --prefix`, are relative to the game. `--version` or `version` prints the version; `-v` means verbose.

## new

Creates a game directory from a bundled template; no Go compiler is needed. The destination must not exist. The game gets a random ID so saves do not collide; `--id` sets your own, which should not change after you distribute the game.

```sh
gozen new first-game --template rpg --title "My first game"
gozen check first-game
gozen new first-story --template novel
gozen new first-campaign --template campaign
```

Play it with `gozen run first-game`. The title is in `game.yaml`, dialogue and places in `data/world.yaml`, characters and battle rules in `data/rules.yaml`. The starter pictures are placeholders to replace.

`new` and `theme list` also accept `--format json` or `--format jsonl`:

```sh
gozen new machine-game --id machine-game --format json
gozen theme list --format jsonl
```

JSON is one versioned result document; the JSONL theme listing has one item per theme and a final summary. In these modes errors are JSONL on stderr with the same GZN codes and exit statuses, and the created game is the same.

## check

Loads every definition, resolves references and expressions, checks that the assets exist, and lays out dialogue with the game's font. Warnings go to stderr and do not fail the check unless you pass `--strict`, which exits 1 on any warning and keeps the `GZN0xxx` codes.

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
| `--strict` | Fail with exit 1 when warning counts exceed zero, or the supplied baseline. |
| `--baseline FILE` | Enforce recorded warning counts per code and source file; only growth fails. |
| `--write-baseline FILE` | Write a new JSON baseline after successful validation. Existing files are not overwritten. |
| `--only structure` | Check definitions, references, expressions and assets only; do not read the font or check the text layout. stdout says so. |
| `--only layout` | Check only that dialogue and choices fit the message window, with the game's font and display rules. |
| `--format` | `text` (default), `json` or `jsonl`. JSON returns a version-1 result whose `data` is the diagnostic array (`file`, `line`, `col`, `path`, `message`, `severity`, `code`). JSONL emits each diagnostic and a terminal summary. Validation errors set `status: failed` and exit 1, without duplicate stderr output. |

The `data` field contains diagnostics such as:

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

`--only layout` reports the lines and choices that overflow the window. Warnings alone exit 0.

<!-- verify: run fixture=cli -->
```shell
gozen check --only layout game
```

### Warning budgets

<!-- verify: run fixture=cli -->
```sh
gozen check game --strict
gozen check game --write-baseline warnings.json
gozen check game --baseline warnings.json
```

`--baseline FILE` turns on enforcement by itself. It allows the recorded number of warnings for each code and source file, and fails (exit 1) only when a number grows; a new code or file is allowed zero. A drop passes but does not rewrite the file. With `--strict` as well, the recorded numbers replace zero. Every warning is still printed in text, JSON and JSONL, and errors always fail.

`--write-baseline FILE` writes versioned JSON to a new file after a check without errors, with zero counts when the game is clean. It never overwrites a file, and it still applies `--strict` or `--baseline` given with it. The file path is relative to the working directory; the file names inside are relative to the game. A baseline records the `--only` scope, which must match when it is used. It counts warnings per code and file, so one warning replaced by another in the same bucket goes unnoticed.

## sim

Plays the game by its rules alone, with no window or sound. Every `sim` command but `sim script` (which writes a trace) accepts `--format text|json|jsonl`. JSON returns `{"version":1,"command":"sim ...","status":"ok","data":...}`; JSONL writes one item per entry and then a summary. Fatal diagnostics go to stderr as JSONL with their GZN codes. The format changes only the output, not what is simulated.

| Command | JSON `data` | JSONL items |
|:--|:--|:--|
| `sim battle` | Array of results, in `--state` order | One start's result |
| `sim dive` | Floors, skills used and party results | One floor |
| `sim floors` | Array of floor numbers, seeds, maps and notes | One floor or seed trial |
| `sim design` | `personas` and their measured results | One persona |
| `sim campaign` | Campaign totals or comparison rows | One comparison entry; ordinary totals use only a summary |
| `sim routes` | Paths, endings, unreachable content and errors | One reached ending |
| `sim breed` | `reachable` rows and `unreachable` IDs | One species |

### sim battle

Fights the troop on AUTO with the starting party (or `--party`, or a saved state) and prints the win, loss, flight and draw rates and the average turns, the same numbers as the [F7 balance panel](/edit-play-loop/#f7-balance-panel). The same `--seed` gives the same result.

<!-- verify: each fixture=cli -->
```shell
gozen sim battle game --troop lesson -n 100
gozen sim battle game --troop lesson -n 100 --party scout --level 5 --seed 7
gozen sim battle game --troop lesson --state memo.save --hp                 # from a note, HP left after a win
gozen sim battle game --troop lesson --state memo.save --as-is --streak 10  # keep the saved HP, count wins in a row
gozen sim battle game --troop lesson -n 1 -v                                # the messages of the first battle
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

Takes the starting party down a dungeon location: `--battles` battles on each floor, better equipment from `--shops`, rests and bosses. Prints the level, HP, gold and win rates of each floor.

<!-- verify: each fixture=cli -->
```shell
gozen sim dive game --location grotto --floors 3 --battles 5
gozen sim dive game --location grotto --floors 3 --boss 3:lesson --grind 0.8
gozen sim dive game --location grotto --floors 4 --rest 2 --boss 2:lesson --tries 50
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
gozen sim floors game --location grotto --from 1 --to 3
gozen sim floors game --location grotto --floor 1 --try 3
gozen sim floors game --location grotto --no-map
```

| Option | Meaning |
|:--|:--|
| `--from N`, `--to N` | The first and last floor to show (default: 1 to 20) |
| `--floor N` | Show only this floor; with `--try`, the floor whose seeds are compared |
| `--try N` | Compare seeds 0 to N-1 on `--floor`, one line each |
| `--no-map` | Print the numbers only |

### sim design

Plays the game headless with the personas of `designeval.yaml` (players, rewards and time weights). Only with `--out` does it write `summary.json`, timelines, `band.png` and `report.md`, to a new directory. A simulation error keeps the report and exits 1. [The CLI test fixture](https://github.com/nao1215/gozen/blob/main/e2e/atago/fixtures/game/designeval.yaml) has a small configuration.

<!-- verify: each fixture=cli -->
```shell
gozen sim design game -o report
gozen sim design game -o report --only walker -v
```

| Option | Meaning |
|:--|:--|
| `--config FILE` | The personas and measures (default: `designeval.yaml` in the game directory) |
| `--only ID` | Run this persona; repeat for multiple IDs |
| `-o, --out DIR` | Also write report files to this new directory |
| `--root DIR` | Read previous runs for comparison (default: `build/designeval`) |
| `--seed N` | Override every selected persona's seed; N must be positive |
| `--list` | Print the configuration without simulating; cannot be combined with `--out` |
| `--memos DIR` | Write notes on marked places to this directory; requires `--out` |
| `-v, --verbose` | Write each persona's dives, choices and stumbles to `debug.log`; requires `--out` |

### sim campaign

Plays the game's [campaign](/writing-a-game/#campaigns) many times, headless, with a strategy for the player. It prints the win rate, the turns the campaigns ended in, the mean number of regions each faction held at the start of each turn, which factions fell and when, the orders given, and how the marches went. Campaign *i* uses the seed `--seed` + *i*, so the same `--seed` gives the same result.

`--compare strategies` or `--compare tactics` plays each entry on the same seeds. The table gives the win rate, mean ending turn and mean regions held. For each region it gives the ownership changes per campaign, the mean turn of the first change, who held it at the end, and the player's win rate by the turn it held the region. `--format json` writes both as JSON.

<!-- verify: skip the CLI fixture has no campaign; e2e/atago/campaign.atago.yaml runs these -->
```shell
gozen sim campaign game
gozen sim campaign game --as random -n 500 --format json
gozen sim campaign game --tactic guard
gozen sim campaign game --compare strategies
gozen sim campaign game --compare tactics --format json
```

| Option | Meaning |
|:--|:--|
| `-n, --campaigns N` | Campaigns to play (default: 200) |
| `--as STRATEGY` | `greedy` marches when the odds of the clash (its formulas with `rand()` at its middle) are good, else gives another order, one with a general first; `random` gives any order it can; `ai` plays the player's faction by its own `ai` rules (default: greedy) |
| `--tactic ID` | Every general marches with this tactic in a clash shown as units (`campaign.clash.tactics`, or the engine's `front`, `weakest`, `strongest`, `leader`, `guard`), and a march takes the most generals it can; the orders are counted as `order:tactic`. Compare the win rates of the tactics this way (default: each general's own) |
| `--compare strategies\|tactics` | Compare all player strategies or declared tactics using the same seeds; reports per-entry and per-region balance numbers |
| `--orders N` | Orders a campaign may give before it is cut (default: 2000) |

### sim routes

Explores the game's starting event and automatic arrivals without a display. It reports the paths that reach a clear or a game over, choice options never offered on the explored paths, and flags that `gozen check` reports as set but never read (GZN0112). Path count, event requests per path and time bound the run, and JSON says whether the path limit cut it short. Findings exit 0 unless `--fail-unreachable` is set; runtime errors and timeouts always exit 1.

```sh
gozen sim routes game
gozen sim routes game --format json
```

| Option | Meaning |
| --- | --- |
| `--paths N` | Maximum paths to explore (default: 300) |
| `--depth N` | Maximum event requests per path (default: 20000) |
| `--timeout DURATION` | Maximum run time (default: 30s; for example `10s` or `2m`) |
| `--format text\|json\|jsonl` | Output form (default: text) |
| `--fail-unreachable` | Exit 1 if an unreachable ending or never-offered choice option, or a GZN0112 flag, is found |

### sim breed

`gozen sim breed <game-dir>` starts from the species with a positive recruit chance and follows the breeding rules. It prints each reachable species with the fewest generations and one recipe, then the unreachable species. `--format json` returns `reachable` rows (`id`, `family`, `rank`, `steps`, `parents`) and an `unreachable` ID list. A recipe gives the parents' sexes and the child's sex (`parent_sexes` and `sex` in JSON); the graph follows obtainable sexes and plus values. It assumes parents can be raised to any level, so try `min_plus` recipes in a real play too. See [the collection recipe](/cookbook/#collect-raise-and-breed-monsters).

`gozen sim breed examples/go-zen/game --format json` lists the sample's 40 non-boss species.

`gozen book GAME` describes the game in Markdown (`--format text` is the same). `--format json` returns the same sections, keyed by ID, in a version-1 envelope; `--format jsonl` writes one item per section and a summary.

`--section lineage` gives only the monster family trees. Its JSON `data.monsters` array has recipes, capture locations and nested lineage trees. Family recipes name concrete `parents` and `families`, a repeated species is a `ref`, and an unreachable species has a null lineage.

```sh
gozen book game --format json
gozen book game --section lineage --format json
```

### sim script

Plays a script of [`run --dev --script`](/edit-play-loop/#scripts-and-screenshots) as fast as it can, without a window, sound or screenshots, and writes the same [trace](/edit-play-loop/#trace-and-replay). With the same options and `--seed` it plays the same game as `run`: the same `say`, `request`, `order`, `dev_do`, `battle`, `autosave` and `state` lines at the same ticks, and the same state in `end`. Use it to measure long plays without Xvfb.

<!-- verify: each fixture=cli -->
```shell
gozen sim script game --start camp --script check.txt --trace sim.jsonl
gozen sim script game --start camp --party scout --level 3 --seed 7 --script check.txt --trace -
```

What needs the window is left out:

- `shot NAME` takes no picture, and neither do the automatic shots of `lines`, `play` and walking; the trace has no `shot` line.
- The trace lines that come from drawing, `view`, `stand`, `layout` and `menu`, are not written, and `--trace-shots` is not available. `music` lines say which song was asked for; whether it had started depends on how fast songs decode in the background.
- Every other command (`wait`, `gap`, `press`, `hold`, `read`, `until`, `walk`, `do`, `order`, `end_turn`, `skip`, `quit` and the rest) runs as in `run`, tick for tick. `click X Y` and `drag X Y TO_X TO_Y` (press, move over eight ticks, let go) run too, without checking positions known only once drawn.

With `--script -` the script is read line by line from the standard input and the trace goes to the standard output unless `--trace FILE` is given. Whenever the game is ready for the next command it writes `{"t":"ready","tick":N,"busy":false,"location":"camp","waiting":""}` and flushes the trace; `waiting` is `text`, `choice`, `command` (a battle waits for a command) or empty. A program can read up to `ready`, decide and send the next line, and keep one process playing as long as it likes. The command `snapshot` writes `{"t":"snapshot","state":{...}}` with the whole state, as in `end`; it works in `run --script` too. A line that does not parse is written as `script_error` and skipped. The end of the input, or `quit`, ends the run with `end`.

<!-- verify: skip reads the script from the standard input -->
```shell
gozen sim script game --start camp --script - < check.txt
```

Saves stay in memory unless `--save-dir` is given. The startup options of `run` work here too: `--start`, `--battle`, `--party`, `--level`, `--flags`, `--gold`, `--items`, `--floor`, `--calm`, `--event`, `--line` and `--answers`. The default `--seed` is 1. Without `--trace -`, it prints the ticks played and the lines written.

| Option | Meaning |
|:--|:--|
| `--script FILE` | The script; `-` reads it line by line from the standard input (required) |
| `--trace FILE` | Write the trace to this file; `-` is the standard output (default with `--script -`) |
| `--seed N` | The random seed (default: 1) |
| `--save-dir DIR` | Keep the saves in this directory (default: in memory) |

### sim batch

Plays many `sim script` cases in one process. It reads and checks each game directory once (again only when a file changes) instead of once a case, and plays every case from a new game.

The cases are JSON Lines from `--cases FILE` (`-` is the standard input). `args` are the arguments of `sim script`, game directory first, and `id` is a name the result carries back:

```json
{"id": "night", "args": ["game", "--start", "camp", "--seed", "5", "--script", "night.txt", "--trace", "night.jsonl"]}
{"id": "fight", "args": ["game", "--start", "camp", "--seed", "6", "--script", "fight.txt", "--trace", "fight.jsonl"]}
```

<!-- verify: skip needs the cases file above -->
```shell
gozen sim batch --cases cases.jsonl
```

When a case ends, stdout gets a version-1 JSONL envelope with `command: "sim batch"`, `type: "item"`, its `id` and `status` (`ok` or `failed`). Its `data` holds the case's `id`, `exit`, `stdout` and `stderr`, as `sim script` with the same arguments would have produced them. A final `summary` counts `succeeded` and `failed`, and any failed case makes the batch exit 1. A fatal input or processing error goes to stderr and may leave out the summary.

Each case writes its own `--trace` and has its own seed, saves and state; it cannot use `-` for its script or trace. A line that does not parse gets a result with exit 2 and the batch goes on. A program can stream cases and read each result before sending the next. Cases play one at a time; run several processes for parallel play.

| Option | Meaning |
|:--|:--|
| `--cases FILE` | The cases as JSON Lines; `-` reads the standard input (required) |
| `--games N` | Keep the definitions of at most this many game directories (default: 4) |

## run

Opens a game directory (`game.yaml`, `data/`, `assets/`) in a window with the same runtime as exported games. No Go source or game-specific executable is needed.

<!-- verify: skip opens a window; covered by e2e/atago/graphics.atago.yaml -->
```sh
gozen run game
gozen run game --fullscreen --mute
gozen run game --dev --save-dir test-saves
gozen run game --dev --script check.txt --shots shots --trace run.jsonl
```

Use `--` before a directory whose name starts with a dash. `gozen run --help` lists the game options.

`--dev` turns on the author tools and live reload. Scripted runs keep their saves apart unless `--save-dir` is given. Ordinary play saves under the game's ID in the user's Gozen configuration directory; `--save-dir` keeps separate copies apart. Relative output paths are relative to the working directory.

It needs a display and the platform graphics and audio libraries (use Xvfb for unattended Linux runs), and ffmpeg for recording. Closing the window exits 0, startup and I/O failures exit 1, and argument and startup option errors exit 2.

Assets are read as loose files, not packaged. Custom Go loops and save-store callbacks are not supported.

## trace

Prints a trace written by `--trace` in recorded order; `-` reads the standard input. `--check` lists faults of the standing pictures and the music, exits 1 if there are any, and takes no display filters.

`--format json` returns the selected records and a count of every kind in `data`; `--format jsonl` writes the records as items, then a summary with the counts. With `--check` it returns the issues with their GZN codes, ticks and messages, and `standing_pictures_checked` is false when the trace has no standing-picture records.

<!-- verify: each fixture=cli -->
```shell
gozen trace run.jsonl
gozen trace run.jsonl --only say --only battle --from 10 --to 60
gozen trace run.jsonl --check
```

| Option | Meaning |
|:--|:--|
| `--only KIND` | Select a kind such as `say`, `scene` or `battle`; repeat for multiple kinds |
| `--format text\|json\|jsonl` | Output form (default: text) |
| `--from SECOND`, `--to SECOND` | The time window; `--to 0` is the end |
| `--input` | Also print the player's input |
| `--check` | List faults of standing pictures and music; exit 1 if any |
| `--repro TICK\|TIME\|PNG` | Print a command to retake a recorded screen; JSON reports the starting saves and options |
| `--out DIR` | With `--repro`, write a cut trace, initial saves and reproduction script to a new directory |
| `--metrics` | Report play measurements as facts, never verdicts; text, decisions, repeated lines, signal intervals and unchanged observations |
| `--top N` | With `--metrics`, show the top N repeated lines (default: 10) |
| `--stall-seconds SECONDS` | With `--metrics`, minimum unchanged observation duration (default: 5 simulation seconds) |
| `--music-late SECONDS` | With `--check`, a song heard later than this after it was asked for is late (default: 0.5) |

## notes

Builds an HTML page with thumbnails from the notes written with F9, in a new directory. The notes are not changed.

<!-- verify: each fixture=cli -->
```shell
gozen notes notes --out notes-html
gozen notes notes -o review-html
```

| Option | Meaning |
|:--|:--|
| `-o, --out DIR` | Write the HTML to this new directory; required |

## capture

Takes PNG images and animated GIFs straight from the game YAML, without a game binary or ffmpeg. Each job starts from a fresh state with saves in memory.

<!-- verify: skip needs a display; e2e/atago/graphics.atago.yaml exercises direct capture -->
```shell
gozen capture game --config captures.yaml --out promo
gozen capture game --config captures.yaml --out selected --only camp --format json
xvfb-run -a gozen capture game --config captures.yaml --out unattended
```

<!-- verify: parse capture job configuration consumed by the graphics CLI -->
```yaml
seed: 7
shots:
  - id: camp
    start: camp
    wait: 30
  - id: conversation
    event: finish
    line: 1
    setup:
      - set: {gold: 42}
    wait: 30
  - id: greeting
    start: camp
    wait: 30
    gif:
      fps: 10
      duration: 3s
      width: 640
      script: |
        press ok
```

`id` names the output file. `start` picks a location, `event` or `scene` an event or gallery scene, and `line`, `picture` and `answers` a position in that event. `battle` needs `start` without `scene` or `event`. `party`, `level`, `floor`, `flags` and `calm` set up a location start. `setup` runs YAML commands that must finish without waiting for the player, after the gallery defaults.

The top-level `script` runs before `wait` (default: 120 simulation ticks), and `gif.script` during the animation. Scripts use the [input commands](/edit-play-loop/); script commands that write images are rejected, since the job writes the images.

| Option | Meaning |
| --- | --- |
| `--config FILE` | Capture jobs; required, relative to the working directory |
| `-o, --out DIR` | New output directory; required |
| `--only ID` | Select a job; repeat for multiple jobs |
| `--format text\|json\|jsonl` | Report encoding; images are unchanged |
| `--overlay DIR` | Approved asset replacements |
| `--raw` | Skip approval checks for local review; incompatible with `--overlay` |
| `--timeout DURATION` | Cooperative limit per job; default `4m` |
| `--against DIR` | Compare each job with the same job ID of an earlier capture directory |

The output holds the images, `shots.json` and `index.html`. A configuration or script error stops the batch before anything is drawn. A job that fails while running gets a reason in the report and no image, the other jobs go on, and the batch exits 1.

With `--against previous`, each row of `shots.json` also compares the job with `previous/shots.json`: `change` (`unchanged`, `changed`, `new` or `removed`), `changed`, `diff_pixels`, `diff_ratio` and `diff_bbox` (`[x, y, width, height]`, or null when no pixel changed). A difference does not change the exit status. The asset policy checks every sampled frame, GIF frames included; ordinary games need no `--raw`.

GIFs default to 10 FPS, three seconds and at most 640 pixels wide. The limits are 1 to 30 FPS, 10 seconds, 1280 pixels a side, 921600 pixels a frame, 2 to 300 frames and 64 MiB of paletted pixels, so a long or wide GIF may need a lower width, FPS or duration. GIFs loop without sound. Frame delays are in centiseconds, and the last one takes up any fraction left over.

Capture needs a display and the platform graphics libraries; use Xvfb for unattended Linux runs. The timeout is checked between updates and draws, so it cannot stop a stalled graphics driver.

## book

Writes the game's locations, events, scenes, troops and items to stdout as one Markdown document, for reviewing a whole game or what an agent wrote. The headings follow the game's `language`.

<!-- verify: run fixture=cli -->
```shell
gozen book game > book.md
```

## release

Checks the approval records against the assets the game uses and copies the game to a new directory, with replaced assets taken from an overlay. An unapproved reference fails the build.

<!-- verify: each fixture=cli -->
```shell
gozen release game --out release --inspect inspect.jsonl
gozen release game --out release --overlay overlay --inspect inspect.jsonl
```

| Option | Meaning |
|:--|:--|
| `-o, --out DIR` | Write the approved tree to this new directory; required |
| `--inspect FILE` | The inspection records (JSON Lines) from production; required |
| `--overlay DIR` | Take replaced assets from this directory (default: the game's approval settings) |
| `--prefix DIR` | The asset directory inside the game, separated by `/` (default: `assets`) |

## trial

Builds a demo tree from the range in `trial.yaml`. A branch it cannot prove unreachable is kept, with its condition reported. `forbid` patterns fail the build when a kept definition or asset must not ship; see [trial checks](/cookbook/#check-the-contents-of-a-trial).

<!-- verify: each fixture=cli -->
```shell
gozen trial game -n                            # count what would be kept, write nothing
gozen trial game --out trial --list trial.txt        # build, and list what was kept and dropped
gozen trial game --out trial --overlay overlay --inspect inspect.jsonl
```

| Option | Meaning |
|:--|:--|
| `-o, --out DIR` | Write the trial to this new directory; required unless `--dry-run` |
| `--config FILE` | The range settings, relative to the working directory (when omitted: `GAME/trial.yaml`) |
| `--overlay DIR`, `--inspect FILE` | Take replaced assets from an overlay; the two go together |
| `--list FILE` | Write what was kept and dropped to this new file |
| `-n, --dry-run` | Write nothing; print only how much would be kept |

The trial holds what [`gozen export`](#export) takes from a game: the pruned `game.yaml` and `data/`, the assets the kept part uses, and `lang/`, `streams/`, the license notices and `LICENSES/` unchanged. Translations are copied whole, so they still hold the text of what was cut. A symlink anywhere in the game fails the build, as in an export. The YAML is written anew, without comments or flow style and with every text quoted.

Without `--overlay`, a game's `asset-policy.yaml` stays in the trial, and `gozen export` refuses the trial as it refuses the game. `--overlay` with `--inspect` gives an approved tree without a policy. The list is written after the tree, and a failed run leaves neither.

## Picture tools

[prompt](/production-tools/#gozen-prompt) assembles image-generation text from a catalog and [mosaic](/production-tools/#gozen-mosaic) pixelates regions of images; [Production tools](/production-tools/) describes both.

### models fetch

Downloads pinned revisions of the detection models (`censor`, `face`, `head`) into the user cache and checks each file against a SHA-256 built into gozen. Without `--model`, `mosaic` and `faces` use the cached model.

<!-- verify: skip downloads models from the network -->
```shell
gozen models list     # the models and whether each is in the cache
gozen models fetch face       # the face detector of faces
```

### faces

Painted scenes (`stage:`) cut balloon faces from standing pictures. `faces` finds the face in each picture a stage uses with the anime face detector from `gozen models fetch face` and writes the boxes, with each picture's size and SHA-256, to `assets/faces.json`. The game only reads that file, so a web build shows the same faces. A doll's `icon` (a face drawn for the balloons) comes first, then its `face_at`, then the box; a picture without a box gets a guessed face, and `gozen check` warns about it and about pictures changed since.

<!-- verify: skip downloads a model and needs a cgo build with ONNX Runtime -->
```shell
gozen models fetch face                          # download the model once
gozen faces game                                 # find the faces
gozen faces game --preview faces-check           # and look at the boxes and the cut faces
```

| Option | Meaning |
|:--|:--|
| `--model MODEL` | The face detection model (default: the one in the cache). |
| `--ort LIBRARY` | The ONNX Runtime shared library (default: `$ONNXRUNTIME_LIB`). |
| `--min SCORE` | The lowest score of a face (0.3). |
| `--preview DIR` | Also write each picture with its face box and its balloon face to this new directory. |

Like `gozen mosaic`, it needs a cgo build, the ONNX Runtime shared library and the model; a build without cgo fails and says so.

### puppet

Loads a moving portrait ([portrait YAML](/cookbook/#give-a-character-faces-and-a-moving-portrait)) with the renderer's checks and prints where each face, closed-eyes and mouth picture sits on the base picture. With `--out`, a difference picture as large as the base is cut down to the part that differs.

<!-- verify: each fixture=puppet -->
```shell
gozen puppet streamer/puppet.yaml                        # check it and show the positions
gozen puppet streamer/puppet.yaml --out streamer-packed  # write it with small difference pictures
```

| Option | Meaning |
|:--|:--|
| `-o, --out DIR` | Write the puppet YAML and the cut pictures (each with `at`) to this new directory. The written YAML has no comments. |

It needs nothing beyond the gozen binary: no cgo, display or sound device.

## theme

A dungeon theme is a wall, floor, ceiling and door picture with matching colors. The `gozen` binary carries thirteen (`cave`, `ice`, `ruins`, `castle`, `crystal`, `temple`, `sewer`, `forest`, `tomb`, `keep`, `dream`, `mine`, `vine`). `gozen theme add` copies one into a game, and `theme: cave` in a [dungeon](/schema/#dungeon) or one of its [looks](/schema/#dungeonlook) fills whatever the YAML leaves out. The pictures are not in the game runtime, so a game without a theme does not carry them.

### theme list

Prints each theme's name with a line on how it looks.

<!-- verify: each fixture=cli -->
```shell
gozen theme list
```

### theme add

Copies `wall.jpg`, `floor.jpg`, `ceiling.jpg`, `door.jpg` (1024x1024), `theme.yaml` (the far fog and map colors) and `LICENSE.txt` to `assets/dungeon/themes/<name>/` in the game, offline. Files the game already has are kept.

<!-- verify: each fixture=cli -->
```shell
gozen theme add game --name cave          # the files of the cave theme go to game/assets/dungeon/themes/cave/
gozen theme add game --name cave --force  # replace the files that are already there
```

| Option | Meaning |
|:--|:--|
| `--force` | Replace the files the game already has. |

An unknown theme exits 2 (`GZN2402`) and a directory without `game.yaml` exits 1 (`GZN1110`). A game that says `theme: cave` without the folder fails `gozen check` with `GZN1111` and the line `run gozen theme add <game-dir> --name cave`. The pictures are CC0 1.0; see [ASSETS.md](https://github.com/nao1215/gozen/blob/main/ASSETS.md).

## version and help

```shell
gozen version
gozen --version
```

Prints the version recorded by `go install`, or `devel` for a build from source.

`gozen help` takes the words of a command (`gozen help sim battle` is `gozen sim battle --help`), or a diagnostic code:

<!-- verify: each -->
```shell
gozen help sim battle
gozen help GZN2101
```

## export

`--out DIR` is the new output directory and `--runtime DIR` the matching runtime bundle, both relative to the working directory. `--format json` returns one version-1 envelope whose `data` has `directory` and `target`, and `--format jsonl` one summary; the package is the same in every format. Errors go to stderr, with GZN codes where they have one.

### Desktop

Unpack the CLI archive and the `gozen-runtime` archive of the same release, choosing the runtime for the computer that will play the game.

<!-- verify: skip Uses a release runtime bundle; exercised by the desktop artifact E2E. -->
```sh
gozen export my-game --out output --runtime runtime --target windows-amd64
```

The output contains:

```text
output/
  gozen-player.exe
  game/
    game.yaml
    data/
    assets/
  runtime.json
  runtime-licenses/
```

Copy the whole output directory to the player's computer and run `gozen-player.exe` (`gozen-player` on Linux and macOS). Playing needs no Go, Python or authoring CLI, only a display and the platform graphics and audio libraries. Saves go to the per-user save directory under the game's ID, not into the package.

The target defaults to the current computer; the others are `linux-amd64`, `linux-arm64`, `windows-amd64`, `windows-arm64`, `darwin-amd64` and `darwin-arm64`. Packaging for another target does not run its binary. macOS packages are plain directories, not signed or notarized.

The command checks the runtime version, target and player checksum, and downloads and compiles nothing. The destination must not exist or overlap an input. A game with `asset-policy.yaml` must pass `gozen release` first, and you export the approved tree; a [trial](#trial) built without `--overlay` keeps its game's policy and is refused the same way.

Export copies `game.yaml`, the YAML under `data/`, `lang/` and `streams/`, runtime asset formats under `assets/`, the license notices at the root and `LICENSES/`. Other root directories such as `saves/` and `shots/` are left out, hidden paths are skipped and symlinks are rejected.

Under `assets/` it takes images (PNG, JPEG, GIF, WebP), audio (OGG, WAV, MP3), fonts (TTF, OTF, TTC), YAML, TXT, Markdown, `faces.json` and files named `LICENSE*` or `NOTICE*`. It picks files by path and extension without reading them, so keep author notes, captures and source art out of these directories. Assets are copied as they are, not converted or encrypted. Keep third-party notices in `LICENSES/` or next to their assets.

### Web

Unpack the `gozen-runtime_VERSION_js_wasm.tar.gz` archive of the same release as the CLI into `web-runtime`.

<!-- verify: skip Uses a release runtime bundle; exercised by the browser artifact E2E. -->
```sh
gozen export my-game --target web --out site --runtime web-runtime
```

Upload the contents of `site/` to a static HTTP or HTTPS host:

```text
site/
  index.html
  sw.js
  gozen-player.wasm.gz
  wasm_exec.js
  runtime.json
  runtime-licenses/
  game/
    files.json
    game.yaml
    data/
    assets/
```

One WASM player runs every exported game, so changing YAML or assets needs a new export, not a Go build. `wasm_exec.js` comes from the Go toolchain that built the player, and export checks both files' checksums. Opening the HTML through a `file:` URL does not work.

URLs are relative, so the site can live under a subdirectory such as `/games/my-game/`. The player is served gzipped and unpacked as it downloads. Pictures become WebP (paletted ones lossless), other files that gzip shrinks are served gzipped, and `game/files.json` lists how each file is served. The browser starts once everything but the pictures and sounds has arrived; recorded stream voices are fetched when the watch view reaches their lines. Files are chosen and approved as in a [desktop export](#desktop).

Saves are in browser local storage, kept for the same origin (scheme, host and port) and game ID. They do not sync across devices or follow a changed origin. The YAML and assets can still be downloaded; export does not hide them.

#### Phones

A phone plays the page in its browser. A tap is a click, a long press or a two-finger tap is the right click, and a swipe scrolls; the title explains the taps after the first touch. A phone held the other way from the game's screen is asked to turn, in the first of the device's languages the game offers, and may play as it is. Fullscreen in the settings also locks the screen to the game's orientation where the browser allows it.

A page added to the home screen opens fullscreen in the game's orientation. `web.icon` names its icon, a square PNG of at least 192x192 under `assets/`, copied next to the page as `icon.png` and named in `manifest.webmanifest`.

<!-- verify: parse game.yaml fragment; the icon is a picture of the game, checked by gozen check -->
```yaml
# game.yaml
schema_version: 9
web: {icon: icon.png}
```

Once played, a page opens again offline, from the home screen too. The export writes `sw.js`, a service worker that asks the host first, keeps every file the page reads (stream voices once heard), and answers from those copies when the host is unreachable. Online, the page plays what the host serves now, and the old copies are removed once a new export has fully arrived. Service workers need an HTTPS host or `localhost`/`127.0.0.1`; elsewhere the page plays online only.

When the page is hidden mid-play (the player switches apps, and the browser may close the page unseen), the game autosaves wherever the save screen could save, and the next visit continues from there.

## CPU profiles and reusable exports

`GOZEN_CPUPROFILE=cpu.pprof` before a command records its CPU activity:

<!-- verify: skip needs a game checkout and a script of your own; internal/screen/play's WorkspaceFrameTime test profiles the same way -->
```sh
GOZEN_CPUPROFILE=cpu.pprof gozen sim script examples/workshop/game --start workshop --script tour.txt
go tool pprof cpu.pprof
```

The profile includes setup and decoding, so compare runs on the same fixture and seed; for allocations use `go test -benchmem`.

`gozen export game --target web --runtime web-runtime --out site2 --reuse site` reuses what it can of an earlier export; leave out `--reuse` for a fresh one.

## Command and option index

This list is checked against `gozen help --format json`. It names every option of each command, short forms included; the command's `--help` gives values, defaults and requirements. The image tools share many options, whose effect depends on the action.

| CLI command | Purpose | Options |
| --- | --- | --- |
| `gozen` | Play, check, simulate, and package Gozen games | `--help`, `-h`, `--version` |
| `gozen book` | Write the game synopsis as Markdown or structured data | `--help`, `-h`, `--format`, `--section` |
| `gozen capture` | Capture PNG images and animated GIFs from a game | `--help`, `-h`, `--config`, `--overlay`, `--raw`, `--out`, `-o`, `--only`, `--timeout`, `--against`, `--format` |
| `gozen check` | Check the definitions, assets, and text layout of a game | `--help`, `-h`, `--only`, `--format`, `--strict`, `--baseline`, `--write-baseline` |
| `gozen export` | Export a playable game with a prebuilt runtime | `--help`, `-h`, `--runtime`, `--target`, `--out`, `-o`, `--reuse`, `--format` |
| `gozen faces` | Find the faces for the balloons and icons | `--help`, `-h`, `--model`, `--ort`, `--min`, `--preview` |
| `gozen help` | Describe commands or diagnostic codes | `--help`, `-h`, `--format` |
| `gozen models` | Inspect the inference model cache | `--help`, `-h` |
| `gozen models fetch` | Download the pinned detection models into the cache | `--help`, `-h`, `--format` |
| `gozen models list` | List pinned models and their cache status | `--help`, `-h`, `--format` |
| `gozen mosaic` | Pixelate image regions, with optional ONNX detection | `--help`, `-h`, `--format` |
| `gozen mosaic add` | Add regions by hand | `--help`, `-h`, `--fix`, `--manual`, `--tags`, `--model`, `--yes`, `--ort`, `--face`, `--seg`, `--tagger`, `--classes`, `--labels`, `--threshold`, `--sure`, `--passes`, `--threads`, `--shape`, `--block`, `--margin`, `--pad`, `--min`, `--dilate`, `--out`, `-o`, `--quality`, `--rect`, `--box`, `--poly`, `--ellipse`, `--norm`, `--note`, `--size`, `--step`, `--format` |
| `gozen mosaic auto` | Detect and pixelate regions, and write images | `--help`, `-h`, `--fix`, `--manual`, `--tags`, `--model`, `--yes`, `--ort`, `--face`, `--seg`, `--tagger`, `--classes`, `--labels`, `--threshold`, `--sure`, `--passes`, `--threads`, `--shape`, `--block`, `--margin`, `--pad`, `--min`, `--dilate`, `--out`, `-o`, `--quality`, `--dry-run`, `-n`, `--format` |
| `gozen mosaic clear` | Remove the manual instructions of an image | `--help`, `-h`, `--fix`, `--manual`, `--tags`, `--model`, `--yes`, `--ort`, `--face`, `--seg`, `--tagger`, `--classes`, `--labels`, `--threshold`, `--sure`, `--passes`, `--threads`, `--shape`, `--block`, `--margin`, `--pad`, `--min`, `--dilate`, `--out`, `-o`, `--quality`, `--note`, `--size`, `--step`, `--format` |
| `gozen mosaic drop` | Drop automatic regions | `--help`, `-h`, `--fix`, `--manual`, `--tags`, `--model`, `--yes`, `--ort`, `--face`, `--seg`, `--tagger`, `--classes`, `--labels`, `--threshold`, `--sure`, `--passes`, `--threads`, `--shape`, `--block`, `--margin`, `--pad`, `--min`, `--dilate`, `--out`, `-o`, `--quality`, `--rect`, `--box`, `--region-id`, `--norm`, `--note`, `--size`, `--step`, `--format` |
| `gozen mosaic grow` | Grow or shrink regions | `--help`, `-h`, `--fix`, `--manual`, `--tags`, `--model`, `--yes`, `--ort`, `--face`, `--seg`, `--tagger`, `--classes`, `--labels`, `--threshold`, `--sure`, `--passes`, `--threads`, `--shape`, `--block`, `--margin`, `--pad`, `--min`, `--dilate`, `--out`, `-o`, `--quality`, `--rect`, `--region-id`, `--by`, `--left`, `--top`, `--right`, `--bottom`, `--norm`, `--note`, `--size`, `--step`, `--format` |
| `gozen mosaic inspect` | Report if images need and have pixelation | `--help`, `-h`, `--fix`, `--manual`, `--tags`, `--model`, `--yes`, `--ort`, `--face`, `--seg`, `--tagger`, `--classes`, `--labels`, `--threshold`, `--sure`, `--passes`, `--threads`, `--shape`, `--block`, `--margin`, `--pad`, `--min`, `--dilate`, `--out`, `-o`, `--quality`, `--pretty`, `--redetect`, `--format` |
| `gozen mosaic list` | List the images to review | `--help`, `-h`, `--fix`, `--manual`, `--tags`, `--model`, `--yes`, `--ort`, `--face`, `--seg`, `--tagger`, `--classes`, `--labels`, `--threshold`, `--sure`, `--passes`, `--threads`, `--shape`, `--block`, `--margin`, `--pad`, `--min`, `--dilate`, `--out`, `-o`, `--quality`, `--format` |
| `gozen mosaic mark` | Mark an image as reviewed | `--help`, `-h`, `--fix`, `--manual`, `--tags`, `--model`, `--yes`, `--ort`, `--face`, `--seg`, `--tagger`, `--classes`, `--labels`, `--threshold`, `--sure`, `--passes`, `--threads`, `--shape`, `--block`, `--margin`, `--pad`, `--min`, `--dilate`, `--out`, `-o`, `--quality`, `--checked`, `--note`, `--format` |
| `gozen mosaic show` | Print the regions and instructions of an image | `--help`, `-h`, `--fix`, `--manual`, `--tags`, `--model`, `--yes`, `--ort`, `--face`, `--seg`, `--tagger`, `--classes`, `--labels`, `--threshold`, `--sure`, `--passes`, `--threads`, `--shape`, `--block`, `--margin`, `--pad`, `--min`, `--dilate`, `--out`, `-o`, `--quality`, `--format` |
| `gozen mosaic skip` | Use only regions added by hand for an image | `--help`, `-h`, `--fix`, `--manual`, `--tags`, `--model`, `--yes`, `--ort`, `--face`, `--seg`, `--tagger`, `--classes`, `--labels`, `--threshold`, `--sure`, `--passes`, `--threads`, `--shape`, `--block`, `--margin`, `--pad`, `--min`, `--dilate`, `--out`, `-o`, `--quality`, `--note`, `--size`, `--step`, `--format` |
| `gozen mosaic view` | Draw an image with a coordinate scale | `--help`, `-h`, `--fix`, `--manual`, `--tags`, `--model`, `--yes`, `--ort`, `--face`, `--seg`, `--tagger`, `--classes`, `--labels`, `--threshold`, `--sure`, `--passes`, `--threads`, `--shape`, `--block`, `--margin`, `--pad`, `--min`, `--dilate`, `--out`, `-o`, `--quality`, `--src`, `--region`, `--norm`, `--size`, `--step`, `--view-out`, `--format` |
| `gozen new` | Create a playable game without Go source | `--help`, `-h`, `--format`, `--template`, `--id`, `--title` |
| `gozen notes` | Turn test-play notes into HTML with thumbnails | `--help`, `-h`, `--out`, `-o` |
| `gozen prompt` | Assemble image-generation text from a catalog | `--help`, `-h`, `--config`, `-c`, `--format`, `--scene`, `--plan`, `--scenes`, `--select` |
| `gozen prompt lint` | Check every catalog item and scene | `--help`, `-h`, `--config`, `-c`, `--format` |
| `gozen puppet` | Check a moving portrait and cut its pictures | `--help`, `-h`, `--out`, `-o` |
| `gozen release` | Build a release tree with approved assets only | `--help`, `-h`, `--out`, `-o`, `--inspect`, `--overlay`, `--prefix` |
| `gozen run` | Run a game directory in a window | `--help`, `-h`, `--dev`, `--build-info`, `--external-input`, `--seed`, `--script`, `--shots`, `--record`, `--mute`, `--fullscreen`, `--gallery-all`, `--book-as-played`, `--window`, `--save-dir`, `--start`, `--battle`, `--party`, `--level`, `--flags`, `--gold`, `--items`, `--floor`, `--calm`, `--autoplay`, `--report`, `--shot-every`, `--auto-floors`, `--speed`, `--trace`, `--replay`, `--replay-battle`, `--memos`, `--from-memo`, `--allow-legacy-save`, `--until`, `--trace-shots`, `--scene`, `--event`, `--line`, `--picture`, `--answers`, `--review`, `--review-log`, `--review-sources` |
| `gozen sim` | Measure play without a window | `--help`, `-h` |
| `gozen sim batch` | Play many sim script cases in one process | `--help`, `-h`, `--cases`, `--games` |
| `gozen sim battle` | Fight AUTO battles against a troop | `--help`, `-h`, `--troop`, `--battles`, `-n`, `--party`, `--level`, `--seed`, `--verbose`, `-v`, `--turns`, `--no-tree`, `--gear`, `--state`, `--as-is`, `--streak`, `--hp`, `--format` |
| `gozen sim breed` | Explore the monster breeding graph | `--help`, `-h`, `--format` |
| `gozen sim campaign` | Play a campaign many times and measure it | `--help`, `-h`, `--campaigns`, `-n`, `--seed`, `--as`, `--tactic`, `--orders`, `--format`, `--compare` |
| `gozen sim design` | Play the game with personas and write a report | `--help`, `-h`, `--config`, `--root`, `--out`, `-o`, `--only`, `--memos`, `--verbose`, `-v`, `--seed`, `--list`, `--format` |
| `gozen sim dive` | Simulate a dive floor by floor | `--help`, `-h`, `--location`, `--floors`, `--battles`, `--boss`, `--shops`, `--tries`, `--grind`, `--seed`, `--turns`, `--rest`, `--supplies`, `--events`, `--rest-cost`, `--rest-event`, `--back-item`, `--worn-hp`, `--worn-mp`, `--scene-event`, `--scene-when`, `--guards`, `--retreat`, `--guards-only`, `--no-town`, `--floor-event`, `--arrival-saves`, `--format` |
| `gozen sim floors` | Show generated floors: maps, steps, and encounters | `--help`, `-h`, `--location`, `--from`, `--to`, `--floor`, `--try`, `--format`, `--no-map` |
| `gozen sim routes` | Find unreachable endings and choice options | `--help`, `-h`, `--paths`, `--depth`, `--timeout`, `--format`, `--fail-unreachable` |
| `gozen sim script` | Play a script without a window and write its trace | `--help`, `-h`, `--script`, `--trace`, `--seed`, `--save-dir`, `--start`, `--battle`, `--party`, `--level`, `--flags`, `--gold`, `--items`, `--floor`, `--calm`, `--event`, `--line`, `--answers` |
| `gozen theme` | Inspect and install dungeon themes | `--help`, `-h` |
| `gozen theme add` | Copy a dungeon theme into a game | `--help`, `-h`, `--name`, `--force` |
| `gozen theme list` | List the dungeon themes | `--help`, `-h`, `--format` |
| `gozen trace` | Print or check a JSON Lines trace of a play | `--help`, `-h`, `--metrics`, `--top`, `--stall-seconds`, `--only`, `--format`, `--from`, `--to`, `--input`, `--check`, `--music-late`, `--repro`, `--out` |
| `gozen trial` | Build a trial tree with the reachable part | `--help`, `-h`, `--out`, `-o`, `--config`, `--overlay`, `--inspect`, `--list`, `--dry-run`, `-n` |
| `gozen version` | Print the gozen version | `--help`, `-h` |
