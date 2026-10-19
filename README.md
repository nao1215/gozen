<!-- ALL-CONTRIBUTORS-BADGE:START - Do not remove or modify this section -->
[![All Contributors](https://img.shields.io/badge/all_contributors-1-orange.svg?style=flat-square)](#contributors-)
<!-- ALL-CONTRIBUTORS-BADGE:END -->
<!-- CI, Security, Go Reference, license and OpenSSF Scorecard show their values once the repository is public (Scorecard after its first run on main). Add the GitHub Downloads badge after the first release. -->
[![CI](https://github.com/nao1215/gozen/actions/workflows/ci.yml/badge.svg)](https://github.com/nao1215/gozen/actions/workflows/ci.yml)
[![Security](https://github.com/nao1215/gozen/actions/workflows/security.yml/badge.svg)](https://github.com/nao1215/gozen/actions/workflows/security.yml)
[![tested with atago](https://img.shields.io/badge/tested%20with-atago-7c3aed?logo=data:image/svg%2Bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCI%2BPHBhdGggZmlsbD0iI2ZmZiIgZD0iTTMuNiA0LjIgMTEuOSAxMmwtOC4zIDcuOC0xLjktMi4yTDcuOSAxMiAxLjcgNi40eiIvPjxyZWN0IGZpbGw9IiNmZmYiIHg9IjEyLjYiIHk9IjE3LjIiIHdpZHRoPSI5LjciIGhlaWdodD0iMi44IiByeD0iMS40Ii8%2BPC9zdmc%2B&logoColor=white)](https://github.com/nao1215/atago)
[![measured with himorime](https://img.shields.io/badge/measured%20with-himorime-d9480f?logo=data:image/svg%2Bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCI%2BPHBhdGggZmlsbD0ibm9uZSIgc3Ryb2tlPSIjZmZmIiBzdHJva2Utd2lkdGg9IjIuNCIgc3Ryb2tlLWxpbmVjYXA9InJvdW5kIiBkPSJNNC4yIDE4LjVBOSA5IDAgMSAxIDE5LjggMTguNSIvPjxwYXRoIGZpbGw9Im5vbmUiIHN0cm9rZT0iI2ZmIiBzdHJva2Utd2lkdGg9IjIuNCIgc3Ryb2tlLWxpbmVjYXA9InJvdW5kIiBkPSJNMTIgMTQuNSAxNi41IDkiLz48Y2lyY2xlIGZpbGw9IiNmZmYiIGN4PSIxMiIgY3k9IjE0LjUiIHI9IjIuMiIvPjwvc3ZnPg==&logoColor=white)](https://github.com/nao1215/himorime)
[![Go Reference](https://pkg.go.dev/badge/github.com/nao1215/gozen.svg)](https://pkg.go.dev/github.com/nao1215/gozen)
![GitHub](https://img.shields.io/github/license/nao1215/gozen)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/nao1215/gozen/badge)](https://scorecard.dev/viewer/?uri=github.com/nao1215/gozen)

# Gozen

Gozen is a 2D RPG framework for Go and Ebitengine built for a fast edit–play loop. You (or your coding agent) write the game as YAML. `gozen check` and `gozen sim` catch broken references and bad balance before anyone plays. While you play, you leave notes on the exact moment, watch live win rates, and reload the YAML without restarting.

Documentation: **https://nao1215.github.io/gozen/**

![Wayfinder, the Gozen sample game](docs/images/wayfinder.gif)

## Try it in 30 seconds

If you have Go 1.26.9 or later, paste this. It needs no display and no assets:

```sh
go run github.com/nao1215/gozen/examples/minimal@latest
```

```text
location=camp gold=2
```

That game is two YAML files and a 35-line Go host. To play the sample above in a window:

```sh
go run github.com/nao1215/gozen/examples/play@latest --mute
```

## Why Gozen?

Making an RPG is mostly tuning: a line that reads wrong, a boss that is too hard, a dungeon that is too long. Gozen shortens each round of that loop.

| Step | What Gozen gives you |
|:--|:--|
| Write | The whole game is plain YAML: places, battles, skills, the enemies' tactics, events and dialogue. Text is easy for you and for a coding agent to write, diff and review. |
| Check | `gozen check` reports broken references, wrong expressions, missing pictures and text that overflows its window, with file and line: `data/world.yaml:26:5: locations.grotto.encounters: unknown troop "lessn"`. |
| Measure | `gozen sim` fights a battle hundreds of times and prints the win rate. `gozen curve` plays a whole dungeon dive. The same numbers appear in the game with F7. |
| Play | `--dev` adds test tools: start anywhere, autoplay at high speed, record and replay a trace. |
| Note | F9 saves a note with a screenshot and the game state. `--from-memo` starts the game again at that exact line. |
| Fix | Run the game with `--dir`, edit the YAML and press F5. The game reloads its definitions and keeps your place. |

The rules are shared by the window, a headless driver and the command line, so a test in CI and a player see the same game.

## Screens

<p align="center"><img src="docs/images/puppet.gif" width="300" alt="a streamer-style portrait that blinks, talks, sways and reacts"></p>
<p align="center">Portraits that blink, talk, breathe and react, from one picture and a few small patches (<a href="examples/puppet">examples/puppet</a>).</p>

| A party of three | Cut-in on a special skill |
|:--|:--|
| ![a battle with a party of three](docs/images/wayfinder-party.jpg) | ![a cut-in when Bram uses Shield Rush](docs/images/wayfinder-cutin.jpg) |
| **First-person dungeon** | **Balance panel (F7)** |
| ![a first-person dungeon with a map](docs/images/wayfinder-dungeon.jpg) | ![win rates by troop, at full HP and at the current HP](docs/images/wayfinder-balance.jpg) |
| **Conversation** | **Village** |
| ![conversation](docs/images/wayfinder-talk.jpg) | ![village](docs/images/wayfinder-village.jpg) |

## How to install

The `gozen` command is distributed with `go install`, prebuilt archives and Linux packages on the release page, a Homebrew cask in `nao1215/tap`, and winget. The library is added with `go get`. Gozen has no release yet: until the first one, use `go install` or `go get`; the other channels start with it.

### Use "go install"

Building from source needs Go 1.26.9 or later.

```sh
go install github.com/nao1215/gozen/cmd/gozen@latest
```

With a C compiler, `CGO_ENABLED=1 go install ...` also builds the ONNX inference of `gozen mosaic`; without one, everything else works and the commands that need inference say how to reinstall.

### Use Homebrew

```sh
brew install --cask nao1215/tap/gozen
```

### Use winget (Windows)

```sh
winget install --id nao1215.gozen
```

### Install from a package or binary

[The release page](https://github.com/nao1215/gozen/releases) contains `.tar.gz` archives for Linux and macOS, `.zip` archives for Windows (amd64 and arm64), and `.deb`, `.rpm` and `.apk` packages for Linux. The prebuilt binaries have no ONNX inference; use `go install` with cgo for that.

### As a library

```sh
go get github.com/nao1215/gozen@latest
```

The graphical game needs the platform graphics libraries (on Linux, X11 and OpenGL). See [OS setup](https://nao1215.github.io/gozen/install/#what-the-window-needs).

## Verifying release integrity

Every release ships supply-chain metadata so you can verify what you download:

- Signed checksums: `checksums.txt` is signed with [cosign](https://github.com/sigstore/cosign) (keyless), producing `checksums.txt.sigstore.json`.
- SBOM: an SPDX Software Bill of Materials is attached to each release archive.
- Build provenance: SLSA build provenance is attested via GitHub OIDC, and is also attached to the release as `multiple.intoto.jsonl`.
- Licenses: each archive and package carries the license texts of the linked dependencies under `THIRD_PARTY_LICENSES/`.

Verify the signed checksums (then check your archive against `checksums.txt`):

```sh
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp 'https://github.com/nao1215/gozen/\.github/workflows/release\.yml@refs/tags/.*' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
  checksums.txt
sha256sum --check --ignore-missing checksums.txt
```

Verify the build provenance of a downloaded archive with the GitHub CLI:

```sh
gh attestation verify gozen_<version>_<os>_<arch>.tar.gz --repo nao1215/gozen
```

Or verify it against the `multiple.intoto.jsonl` release asset with [slsa-verifier](https://github.com/slsa-framework/slsa-verifier):

```sh
slsa-verifier verify-artifact gozen_<version>_<os>_<arch>.tar.gz \
  --provenance-path multiple.intoto.jsonl \
  --source-uri github.com/nao1215/gozen \
  --source-tag v<version>
```

## Recipes

### Let a coding agent write the game

Point your agent at the [schema](https://nao1215.github.io/gozen/schema/) and give it one rule: run `gozen check` until it passes. A typical round:

```sh
gozen check game                                  # references, expressions, pictures, text layout
gozen sim game cave_boss --level 8                # win rate of the party at level 8
gozen curve game cave --floors 5 --boss 5:cave_boss  # a whole dive: levels and wins per floor
```

`gozen book game > book.md` writes every place, enemy, skill and line of dialogue as one Markdown file, which is a quick way to review what the agent wrote.

### Play with the test tools

```sh
go run . --dev --dir game --memos notes             # F9 leaves a note, F7 shows win rates, F5 reloads
go run . --dev --start cave --level 8 --battle cave_boss  # jump straight to a fight
go run . --dev --autoplay 60 --speed 8 --trace run.jsonl  # play by itself, fast, and record
gozen trace run.jsonl                                # read what happened
gozen memos notes                                    # all notes as one HTML page
```

### Embed a game in your program

```go
//go:embed game
var files embed.FS

func main() {
	fsys, err := fs.Sub(files, "game") // game.yaml, data/, assets/
	if err != nil {
		log.Fatal(err)
	}
	if err := runner.Run(context.Background(), fsys, ".", runner.Config{Args: os.Args[1:], Output: os.Stdout}); err != nil {
		log.Fatal(err)
	}
}
```

### Test the game in Go

`headless` runs the same rules without a window: move, choose, fight, save and load in a normal Go test.

```sh
go run github.com/nao1215/gozen/examples/headless@latest
```

```text
location=trail battles=1 wins=1 gold=3
```

## gozen

| Command | What it does |
|:--|:--|
| `check` | validate definitions, references, expressions, pictures and text layout |
| `layout` | check that dialogue and choices fit the message window |
| `sim` | fight a battle many times and report the win rate |
| `curve`, `floors` | simulate a dungeon dive and the size of its floors |
| `trace` | read a play trace as a short timeline |
| `book`, `design` | write the whole game as Markdown; let scripted players finish it |
| `memos` | collect test players' notes into one HTML page |
| `trial`, `release` | cut a demo version; package approved pictures |
| `shots` | take screenshots of listed scenes |
| `puppet` | check a moving portrait; cut its difference pictures small |
| `prompt`, `mosaic` | build image-generation prompts; pixelate parts of pictures |
| `version`, `help` | print the version; show the options and examples of a command |

`gozen help <command>` shows the options and examples of each command. Every command is tested end to end with [atago](https://github.com/nao1215/atago).

## Features

- YAML definitions with expressions, conditional events and versioned saves
- Turn-based battles with a party, auto battle and per-enemy tactics (gambits)
- Skill trees, shops, a field menu and a picture gallery
- First-person grid dungeons, fixed or generated floors
- Moving portraits from one picture and a few difference pictures: blinking, talking, breathing, head and hair sway, reactions and face changes ([puppet](https://pkg.go.dev/github.com/nao1215/gozen/puppet))
- Characters that react to typed words and events, and a chat log ([react](https://pkg.go.dev/github.com/nao1215/gozen/react), [chat](https://pkg.go.dev/github.com/nao1215/gozen/chat))
- Engine words (menus, battle messages, settings) in English and Japanese, chosen by `language` in game.yaml
- A headless driver that shares the rules with the window

## Limitations

- Gozen is young. The API may change before v1.
- Runtime checks are done on Linux. Windows and macOS are built in CI but not yet played.
- There is no graphical map or event editor; the game is text.

## Contributing

Contributions are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) and the [Code of Conduct](CODE_OF_CONDUCT.md). For questions, see [SUPPORT.md](.github/SUPPORT.md). Report vulnerabilities as described in [SECURITY.md](SECURITY.md).

## Support

If Gozen helps you, a star or a [GitHub Sponsors](https://github.com/sponsors/nao1215) sponsorship keeps it going.

## License

The code is [MIT](LICENSE). The pictures of the Wayfinder sample are [CC BY 4.0](examples/play/game/assets/LICENSE). The Go Regular font notice is in [ASSETS.md](ASSETS.md).

## Contributors ✨

Thanks goes to these wonderful people ([emoji key](https://allcontributors.org/en/reference/emoji-key/)):

<!-- ALL-CONTRIBUTORS-LIST:START - Do not remove or modify this section -->
<!-- prettier-ignore-start -->
<!-- markdownlint-disable -->
<table>
  <tbody>
    <tr>
      <td align="center" valign="top" width="14.28%"><a href="https://debimate.jp/"><img src="https://avatars.githubusercontent.com/u/22737008?v=4?s=75" width="75px;" alt="CHIKAMATSU Naohiro"/><br /><sub><b>CHIKAMATSU Naohiro</b></sub></a><br /><a href="https://github.com/nao1215/gozen/commits?author=nao1215" title="Code">💻</a> <a href="https://github.com/nao1215/gozen/commits?author=nao1215" title="Documentation">📖</a></td>
    </tr>
  </tbody>
</table>

<!-- markdownlint-restore -->
<!-- prettier-ignore-end -->

<!-- ALL-CONTRIBUTORS-LIST:END -->

This project follows the [all-contributors](https://github.com/all-contributors/all-contributors) specification. Contributions of any kind welcome!
