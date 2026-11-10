# CLI reference

Use the [command guide](https://nao1215.github.io/gozen/commands/) for recipes.
Run help before using an option on an older installed version.

## Command and option index

This complete inventory is checked against `gozen help --format json`.
Options are listed per command, including short aliases; use that command
with `--help` for values, defaults and requirements. Shared implementation
options can appear on several image tools; their effect depends on the action.

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
