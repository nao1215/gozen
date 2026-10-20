# README and site pictures and steps

The README.md and the GitHub Pages site (`website/`) show only what works with the current code. Only pictures that can be remade from a sample are kept, and the code snippets of the steps are actually run in CI. The reasons are in [ADR 0023](../decisions/0023-verified-docs.md); the checks are implemented by the tests in `internal/dev/docs`.

After changing the README or the site, pass the following before committing.

```sh
go test ./internal/dev/docs   # ledger and references, marks, YAML examples, README links (no display, a few seconds)
make docs-images              # remake the pictures (rewrites only those that changed)
make docs-steps               # run the steps of the README and site
make website website-check    # build the site and look for broken link and image references
```

`make docs` runs `docs-images-check`, `docs-steps`, `website` and `website-check` together. Pictures and steps use a display, so on Linux Make wraps them in `xvfb-run -a -s "-screen 0 1920x1080x24"` (no window appears on your screen). You need ffmpeg, python3 and the setup in [Graphics development](graphics.md#requirements). The site needs hugo (the version in `website.yml`).

## The picture ledger

All pictures of the README, the site and `docs/` are in `docs/images/`, and each has its recipe in [docs/images/manifest.yaml](../images/manifest.yaml).

| Item | Content |
| --- | --- |
| `samples` | The name of a sample and its Go package (such as `./examples/go-zen`; a sample with its own `go.mod` is built in its directory). Built first |
| `runs` | The arguments for one run of a sample. `{script}` is the script file and `{out}` is the new directory of that run (where shots, saves and traces go). `check` runs after the run with that run's directory as its last argument, and confirms that play went as planned |
| `images` | For each image, its run, the source frames (`shot: NAME` is `shot NAME` of the script, `pick` takes one frame by a glob of names and an event of the trace, `frames` is the sequence of frames of a moving image) and the encoding (`jpeg`, `gif`). `tolerance` is the share of pixels allowed to differ in the staleness check (default 2%) |

To change the sample, change `samples` and `runs`; the names, references and checks of the images stay as they are. To add an image, add a `shot` to the sample's script, write one line in the ledger, run `make docs-images`, and reference it from a document. Hand-made images cannot be placed, with one exception: a picture that is not a capture, such as the logo (`gozen-logo*.svg`, `gozen-favicon*.svg`, `gozen-ogp.png`), is listed under `static` with its source and license and is not remade. `go test` fails on each of these:

- the README or the site references a file outside `docs/images` as an image
- a referenced image is not in the ledger, or its file is missing
- an image in the ledger is referenced by no document, or its file is missing
- `docs/images` has a file that is not in the ledger, or `website/` contains an image file (the site's images are served from `docs/images` as `/images`)
- the sample, script or `shot` name of the ledger does not exist

## Remaking and the staleness check

`make docs-images` builds the samples, runs the runs in order, encodes with ffmpeg and writes to `build/docs-images/out/`. Images that cannot be told apart from the current ones (within the range allowed by the comparison below) are not rewritten, so images that do not change when remade leave no diff. Look at the changed images before committing.

`make docs-images-check` (the images job of the Docs workflow in CI) builds the same way and compares with the committed images.

- Still image: the size is the same, and the share of pixels where any color differs by more than 48/255 is at most `tolerance`
- GIF and video: the number of frames is the same, and the first, 1/4, half, 3/4 and last frames meet the condition above (a GIF is compared as it looks with the difference rectangles composited; a video is split into frames with ffmpeg)

If stale, it fails and points to `make docs-images`. In CI the generated images are kept as the artifact `docs-images`. The Go Zen images on the author's machine were remade and checked to show no difference (the whole remake takes about 10 minutes, most of it the F7 measurement).

## Verifying the steps

The code snippets of README.md and `website/content/*.md` are read from the top.

- Shell snippets (`sh`, `shell`, `bash`) without a mark are run as they are. The snippets of one page run one after another in the same directory, and `cd` carries over to the next snippet (the same as a reader going on in one terminal).
- `go install`, `go run …@latest` and `go get` are not rewritten. A local module proxy that serves the local checkout as `v0.0.0-docs` is placed at the front of `GOPROXY`, so they run with the same content as the published module zip. Dependencies come from the module cache, then from the original `GOPROXY`. `HOME` and `GOPATH` are fresh every time, and `gozen` is built from the checkout and put on PATH.
- A YAML snippet must always have a mark. A `text` snippet is checked only if it has a mark. Go snippets are type-checked against this checkout by `go test` (TestDocGo) unless marked `skip REASON`: a whole file as it is; declarations (`func`, `var`, `const`, `type`) with the imports of `snippetImports` added; statements inside a function whose parameters are the names a game has at hand (`ctx`, `t`, `files`, `fsys`, `screen`, `s`, `g`). Unused variables and imports are allowed, since an example shows a part of a program; an unknown name, a wrong method or a wrong number of results fails.

A mark is written on the line just before the snippet (it is not shown on GitHub or on the site).

| Mark | Meaning |
| --- | --- |
| `<!-- verify: run [fixture=NAME] [except=N,M REASON] -->` | Run in the directory of the page (the same as a shell snippet without a mark). `fixture` first copies `internal/dev/docs/testdata/fixtures/NAME/`. `except` leaves out the Nth command (counted from 1, not counting blank lines and comment lines) |
| `<!-- verify: each [fixture=NAME] [except=N,M REASON] -->` | Run the commands one at a time, each in a fresh directory (a copy of the fixture). Use it for a snippet that lists alternatives and uses the same output location many times |
| `<!-- verify: build REASON -->` | Turn a `go run` that opens a window and waits for a person into `go install` (with `@`) or `go build`, and check only that it builds |
| `<!-- verify: skip REASON -->` | Do not run. The reason is required |
| `<!-- verify: output -->` | Each line of a `text` snippet appears, in this order, in the standard output of the snippet run just before (the number of spaces does not matter) |
| `<!-- verify: file PATH -->` | Write the snippet's content to PATH in the page's directory (such as `game/game.yaml` or `main.go`, used by later snippets) |
| `<!-- verify: load [group=NAME] [under=KEY] [as=game.yaml\|as=assets/PATH] -->` | Overlay a YAML example as a data file on `internal/dev/docs/testdata/docgame` and read it with `gozen.Load`, together with the other load examples of the page, or of the same `group` (each recipe of the cookbook is its own game). Picture and sound names in the example are placed as stub files. `under` puts an indented fragment under `KEY:`; `as=game.yaml` merges the example's keys into the docgame's `game.yaml` instead, and `as=assets/PATH` places the example as that file (such as a moving portrait), its pictures stubbed beside it |
| `<!-- verify: same PATH -->` | The YAML example has the same values as the file in the repository (comments and style do not matter) |
| `<!-- verify: parse REASON -->` | Check only that it reads as YAML |

`go test` fails when a mark that needs a reason (skip, build, parse, except) has none, on an unknown mark, on a mark with no snippet, and on a YAML example without a mark. `copy` in `fixture.yaml` copies files of the repository (such as `game: e2e/atago/fixtures/game`). A `go.mod` in a fixture takes everything below it out of the Gozen module, so place `go.mod.txt` and make it `go.mod` with `copy`. docgame is a small neutral game that has only the IDs an example uses without defining them; add to it when an example uses a new ID.

The current exclusions (shown in the log of `docs-steps`):

| Snippet | Reason |
| --- | --- |
| Installing with Homebrew | Published with the first release |
| cosign, `gh attestation`, slsa-verifier | Need the files of a published release (and a GitHub login) |
| `go run .` of your own game and starting with `--dev` (the README's "Play with the test tools", the edit–play loop page) | Opens a window in your own game's module and waits for input. `go run . --dev --dir game` in getting-started is a build, which checks that the game you made builds |
| `gozen play shots` | Needs a game built by the runner and a display. `e2e/atago/graphics.atago.yaml` runs it |
| `gozen art mosaic auto --model`, `gozen art faces` | Need a detection model and ONNX Runtime (and cgo) |
| `sudo apt-get install xvfb xauth` | Installs system packages. The capture inside Xvfb that follows is run |
| `gozen <command> [OPTIONS] [ARGUMENTS]` | Explains the form of a command |
| The YAML of the verb `roll_dice` and the screen `album`, and the 3 expression examples | Need registration in Go code / separate expressions, not one file |
| The shots list and the excerpt of the catalog YAML | parse only (shots for the same reason as above; the catalog is the full `docs/examples/production/catalog.yaml`, which the steps run) |

## The Schema page

The Schema page lists every type and field of the YAML. It is built from `data/schema/types.schema.json`, which a test generates from the Go types (`GOZEN_UPDATE_SCHEMA=1 go test ./internal/layer2/gamedata -run '^TestSchemaCoverage$'`): each field carries its type label (`x-type`), its contract (`x-contract`: default, what 0 means, required, what it refers to) and its description. The contracts are written in Japanese in the generating test; it writes them in English with `website/data/schema_en.yaml` and takes every type and field description from `website/data/schema_fields/*.yaml`, so the JSON is English and fails to generate when one has no English. After changing a description or a wording there, run the same command again: `go test ./internal/layer2/gamedata` fails while the JSON differs. The page adds:

- a description of every type (`_`) and field in `website/data/schema_fields/*.yaml`;
- the English wording of the contract cells in `website/data/schema_en.yaml`;
- the order of the types by topic in `website/data/schema_sections.yaml`. A type no topic names is appended under "Other types" by itself, so every type a row links to has a section.

A type or field without a description, a description for a field that no longer exists, a description that is not English, or a topic that names no type fails `go test ./internal/dev/docs` (TestSchemaDescriptions) and the site build. `make website-check` also checks that the built page has a section for every type and a row for every field (TestSiteSchema).

## Language and vocabulary

Documents are English only. `go test ./internal/dev/docs` (TestDocumentLanguage) fails on Japanese in any Markdown or JSON file of the repository (outside `testdata/`); a word used on purpose (the origin of the name, an engine text key quoted as data) goes in `japaneseAllowed` in `internal/dev/docs/language_test.go`, by file. Go comments and the engine's own Japanese text (its text catalog) are not documents and are not checked.

The vocabulary of adult content belongs only to the mosaic tool ([ADR 0004](../decisions/0004-content-boundary.md)): its code (`internal/mosaic/`) and the `## gozen art mosaic` sections of `website/content/production-tools.md` and `docs/development/production-tools.md`. The same test fails when one of the words of `adultWords` (English and Japanese) appears in any other text file of the repository.

## The site

`make website` builds into `website/public/` with hugo (it fails on a schema cell without a translation or a missing image), and `make website-check` (run by the build of `website.yml`) checks every `href` and `src` of all built pages.

- The target of a link or image inside the site exists as a page or file. The id after `#` exists on that page
- Paths that start with `/` are under the path of `baseURL` (`/gozen/`), so that they do not break on GitHub Pages
- The targets of `https://github.com/nao1215/gozen/(tree|blob)/main/…` exist in the checkout
- Pagefind's `pagefind/` is made after hugo, so it is not checked. Links to outside sites are not checked

`go test` checks the README links (relative repository paths, GitHub main paths, and that the site pages exist in `website/content`).

## CI time estimates

On the author's machine (Linux, Xvfb), remaking and comparing images takes about 3 minutes (most of it the Go Zen runs), running the steps about 40 seconds (with the build cache; the first time takes several minutes for fetching dependencies and building Ebitengine), and building the site and checking links a few seconds. The Docs workflow in CI runs images and steps side by side. While private, it follows the budget rule of [CI configuration](ci.md) and runs only after the repository is public and on manual runs. The checks that need no display (ledger and references, marks, YAML examples, README links) are part of the usual `go test ./...`.
