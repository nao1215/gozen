---
title: Production tools
description: gozen prompt assembles image-generation text from a catalog; gozen mosaic pixelates image regions by hand or with a user-supplied ONNX detection model.
weight: 90
toc: true
---

Two subcommands help with pictures. A game does not need either, and neither contacts a service, downloads a model unasked or changes your original images.

| Command | Does | Needs |
|:--|:--|:--|
| `gozen prompt` | Assembles the positive and negative text for an image generator from a catalog of words | A catalog YAML; nothing else |
| `gozen mosaic` | Pixelates regions of images, drawn by hand or found by a detection model | For detection: a cgo build, ONNX Runtime and your own model |

## gozen prompt

The catalog describes the words once, in layers (setting, subject, lighting, ...), with IDs, defaults, dependencies, conflicts, priorities and emphasis. `gozen prompt` picks the items for a scene, merges duplicates, resolves conflicts, applies the limits and prints the text. The JSON output records which item each word came from.

<!-- verify: each fixture=production -->
```shell
gozen prompt --config catalog.yaml --scene landscape --format json
gozen prompt --config catalog.yaml --plan plan.yaml           # a plan holds only item IDs
gozen prompt -c catalog.yaml --select setting=studio                 # choose an item of a layer by hand
gozen prompt -c catalog.yaml --scenes ids.txt --format jsonl  # item records followed by a summary
gozen prompt lint --config catalog.yaml                       # check every item, dependency and scene
```

A catalog, abridged from [docs/examples/production/catalog.yaml](https://github.com/nao1215/gozen/blob/main/docs/examples/production/catalog.yaml):

<!-- verify: parse abridged from docs/examples/production/catalog.yaml, which the commands above run -->
```yaml
version: 1
dialect: sdxl
emphasis: [soft, normal, strong, stronger, strongest]
default_emphasis: normal
priorities: {required: 0, subject: 1, detail: 2}
negative_limit: 2
layers:
  - id: setting
    defaults: [garden]
    items:
      garden:
        positive:
          - {word: quiet garden, priority: subject}
          - {word: soft light, priority: detail}
      studio:
        positive:
          - {word: plain backdrop, priority: subject}
        conflicts: [{layer: subject, id: traveler}]
  - id: subject
    person: true
    max: 1
    items:
      traveler:
        positive:
          - {word: traveler, priority: subject, emphasis: strongest}
        requires: [{layer: setting, id: garden}]
scenes:
  landscape: {select: {setting: [garden]}}
  portrait: {select: {setting: [garden], subject: [traveler]}}
```

| Option | Meaning |
|:--|:--|
| `-c, --config FILE` | The catalog; required |
| `--scene ID` | Assemble this scene of the catalog |
| `--plan FILE` | Assemble a plan file, which holds only item IDs |
| `--scenes FILE` | Assemble one scene ID per line; `-` reads stdin. Failed scenes are reported and the rest continue |
| `--select LAYER=ITEM` | Select an item of a layer; repeat for several |
| `--format FORMAT` | `text` (default), `json` (one aggregate), or `jsonl` (items and summary) |

- Emphasis has five steps, 0.9 to 1.5, in SDXL weight syntax. The token count is an estimate, not the model's tokenizer.
- Unknown fields, several YAML documents in one file, broken references and control characters are rejected.
- When a person layer is selected, the tool adds fixed age constraints that a catalog cannot replace or drop.
- `lint` fails on errors and passes on warnings alone.

JSON and JSONL use the version-1 result envelope, whose `data` is the assembled result, a batch array or the lint diagnostics. `gozen prompt` only prints the text; you send it to the generator.

## gozen mosaic

The main use is pixelating adult anime-style images distributed in Japan, as the distribution standard requires. With another model and classes it can also anonymize faces and the like.

`gozen mosaic` pixelates regions of the images in `<game-dir>/assets/` and writes the results and a `report.json` outside `assets/`, leaving the originals alone. Regions can come from any mix of:

- a fix file (`--fix`) written by hand, which needs no model;
- the manual subcommands, which edit a per-image instruction file by coordinates;
- a detection model (`--model`), optionally with face detection (`--face`), segmentation masks (`--seg`) and a tagger (`--tagger`).

<!-- verify: run fixture=production except=2 needs a detection model and ONNX Runtime -->
```shell
gozen mosaic auto --fix fixes.yaml -o processed game pattern.png              # hand-written regions, no model
gozen mosaic auto --model model.onnx --ort libonnxruntime.so -o processed game   # detect every referenced image
gozen mosaic inspect --fix fixes.yaml -o processed game pattern.png           # report, write nothing
gozen mosaic skip --manual edits.yaml game pattern.png                 # only regions drawn by hand: no model
gozen mosaic add --manual edits.yaml --rect 8,8,6,6 game pattern.png   # add a rectangle by hand
gozen mosaic view --manual edits.yaml --src masked game pattern.png    # draw the result with a scale
```

A fix file ([docs/examples/production/fixes.yaml](https://github.com/nao1215/gozen/blob/main/docs/examples/production/fixes.yaml)):

<!-- verify: same docs/examples/production/fixes.yaml -->
```yaml
pattern.png:
  skip: true          # no automatic detection for this image
  block: 4            # block size in pixels
  add: [[1, 1, 7, 7], [14, 10, 8, 9]]
```

| Subcommand | Does |
|:--|:--|
| `auto` | Find and pixelate regions; write the images, `report.json` and review sheets. `-n` writes nothing. |
| `inspect` | One JSON line per image: whether it needs pixelation, what was found, whether the output covers it, and a status (`none`, `todo`, `done`, `review`). Writes no file. |
| `list`, `show`, `view`, `add`, `drop`, `grow`, `skip`, `mark`, `clear` | Work on the regions of one image by coordinates, kept in the `--manual` file. `mark --checked` records that the image was looked at. |

How it pixelates:

- Each cell is the average of the original pixels in it. `--block 0` uses `max(4, ceil(long side / 100))` pixels; `1` leaves the image unchanged.
- Every cell a region touches is filled, so the outline is covered. Regions can be rectangles, boxes, polygons, ellipses or masks.
- Input is PNG, JPEG or WebP; output is PNG or JPEG (a WebP input is written as `<name>.png`).
- `IMAGE` is a path below `<game-dir>/assets/`, read the same way by every subcommand: a leading `assets/` and backslash separators are accepted, the asset wins over a file of the same name in the working directory, and a file in `--out` stands for the asset it was made from.
- A missing `--fix` file is an error; a missing `--manual` file is not, since the manual subcommands create it.
- When one image fails, `auto` records the finished images in `report.json` and exits 1.
- The class names of the `--model` output come from `--classes a,b,c`, else a `labels.json` (a JSON array of strings) beside the model, else the ONNX metadata `names` (the Ultralytics form `{0: 'a', 1: 'b'}`), else the built-in `nipple_f,penis,pussy` (`nipple_f`, a female nipple, is the class name of deepghs/anime_censor_detection). `--face` models are looked up the same way. `auto --dry-run` and `inspect` print the classes and their source (`--classes`, `labels.json`, `metadata` or `default`). An empty or duplicate name, or a count that does not match the model output, is an error.
- `--labels` picks the classes to pixelate. The default is `penis,pussy`, since the Japanese distribution standard does not pixelate nipples by default. With `--classes`, or a model whose classes differ from the default, and no `--labels`, every class is pixelated; `--model face.onnx --classes face` pixelates faces.
- `--threshold` and `--sure` (default 0.5) set the detection confidence; detections below `--sure` are pixelated and also listed for review. A person still has to look at the result.

`gozen mosaic <subcommand> --help` lists every option, including `--margin`, `--pad`, `--min`, `--dilate`, `--classes`, `--labels`, `--passes` and the `MOSAIC_*` environment variables.

### Fetching models

`gozen models fetch [NAME...]` downloads pinned revisions of models from Hugging Face into `os.UserCacheDir()/gozen/mosaic/<name>/` and checks each file against a SHA-256 built into gozen; on a mismatch it deletes the download and fails. A cached model with matching checksums is not downloaded again. A cached file is hashed once and recorded in `verified.json` beside it; later runs compare only its size and modification time and hash a changed file again. `gozen models list` shows which models are fetched.

| Name | Source (license) | Classes | Use |
|---|---|---|---|
| `censor` | deepghs/anime_censor_detection, censor_detect_v1.0_s (MIT) | `nipple_f`, `penis`, `pussy` | the main use: mosaic for the Japanese distribution standard |
| `face` | deepghs/anime_face_detection, face_detect_v1.4_s (MIT) | `face` | anonymizing people |
| `head` | deepghs/anime_head_detection, head_detect_v2.0_s (MIT) | `head` | anonymizing people |

Each model comes with its `labels.json`. The text detector deepghs/text_detection (MIT) is not offered: it is a DBNet model, not the YOLO output `[1, 4+classes, N]` that `--model` needs. To hide names, IDs or garbled signatures, pass a YOLO-format text detector with `--model` and `--classes`.

Without `--model`, the cached `censor` model is used. If it is not cached, gozen asks "Download the model? About N MB from URL to PATH [y/N]" when standard input and output are a terminal, and otherwise fails and points at `gozen models fetch`. `--yes` skips the question. Only `gozen models fetch` and a confirmed download use the network.

### Detection setup

Detection runs an ONNX model you supply through ONNX Runtime. Models, the runtime library and training data are not bundled. Check the license of any model you use.

1. Build `gozen` with cgo and a C compiler:

   ```shell
   CGO_ENABLED=1 go install github.com/nao1215/gozen/cmd/gozen@latest
   ```

2. Get the ONNX Runtime shared library from its [releases](https://github.com/microsoft/onnxruntime/releases). Gozen uses the 1.29.0 API through [onnxruntime_go](https://github.com/yalue/onnxruntime_go/tree/v1.36.0#readme) v1.36.0.

| OS | C compiler | Pass to `--ort` (or `ONNXRUNTIME_LIB`) |
|:--|:--|:--|
| Ubuntu / Debian | `sudo apt-get install build-essential` | `lib/libonnxruntime.so.1.29.0` |
| macOS | `xcode-select --install` | `lib/libonnxruntime.1.29.0.dylib` |
| Windows | MSYS2 MinGW64: `pacman -S mingw-w64-x86_64-gcc`, and its `bin` on `PATH` | `lib/onnxruntime.dll` and the DLLs beside it |

The model must be YOLO-style ONNX with an `images` input and an `output0` output shaped `[1, 4 + classes, N]`. A model that cannot be read is an error; Gozen does not fall back to another method.

Without a C compiler, `go install` builds `gozen` without inference. Help, hand-written regions and `inspect` of an existing report still work, and a command that needs detection says how to rebuild and exits 1.

## Recording voices

Voices are sound files under `assets/` that the lines name ([Add voices](/cookbook/#add-voices)), so they need no tool. A layout that keeps a recording session in order:

- one folder per actor, given as the actor's `voices` (`assets/keeper/voice/`);
- one file per line, named by the event and the line's number in it (`dinner_001.ogg`, `dinner_002.ogg`), so an actor's script sorts by event;
- Ogg Vorbis or WAV, mono or stereo, any sample rate (the engine resamples).

`gozen check` warns about lines of an actor with `voices` that have no voice file yet (GZN0109) and prints their paths; with `--format json` a script can turn them into the list of lines left to record.
