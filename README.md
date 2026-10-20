<!-- ALL-CONTRIBUTORS-BADGE:START - Do not remove or modify this section -->
[![All Contributors](https://img.shields.io/badge/all_contributors-1-orange.svg?style=flat-square)](#contributors-)
<!-- ALL-CONTRIBUTORS-BADGE:END -->
<!-- Coverage shows its value once the central repository collects the first report. CI, Security, reviewdog, Go Reference, license and OpenSSF Scorecard show their values once the repository is public (Scorecard after its first run on main). -->
![Coverage](https://raw.githubusercontent.com/nao1215/octocovs-central-repo/main/badges/nao1215/gozen/coverage.svg)
[![CI](https://github.com/nao1215/gozen/actions/workflows/ci.yml/badge.svg)](https://github.com/nao1215/gozen/actions/workflows/ci.yml)
[![reviewdog](https://github.com/nao1215/gozen/actions/workflows/reviewdog.yml/badge.svg)](https://github.com/nao1215/gozen/actions/workflows/reviewdog.yml)
[![Security](https://github.com/nao1215/gozen/actions/workflows/security.yml/badge.svg)](https://github.com/nao1215/gozen/actions/workflows/security.yml)
[![tested with atago](https://img.shields.io/badge/tested%20with-atago-7c3aed?logo=data:image/svg%2Bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCI%2BPHBhdGggZmlsbD0iI2ZmZiIgZD0iTTMuNiA0LjIgMTEuOSAxMmwtOC4zIDcuOC0xLjktMi4yTDcuOSAxMiAxLjcgNi40eiIvPjxyZWN0IGZpbGw9IiNmZmYiIHg9IjEyLjYiIHk9IjE3LjIiIHdpZHRoPSI5LjciIGhlaWdodD0iMi44IiByeD0iMS40Ii8%2BPC9zdmc%2B&logoColor=white)](https://github.com/nao1215/atago)
[![measured with himorime](https://img.shields.io/badge/measured%20with-himorime-d9480f?logo=data:image/svg%2Bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCI%2BPHBhdGggZmlsbD0ibm9uZSIgc3Ryb2tlPSIjZmZmIiBzdHJva2Utd2lkdGg9IjIuNCIgc3Ryb2tlLWxpbmVjYXA9InJvdW5kIiBkPSJNNC4yIDE4LjVBOSA5IDAgMSAxIDE5LjggMTguNSIvPjxwYXRoIGZpbGw9Im5vbmUiIHN0cm9rZT0iI2ZmIiBzdHJva2Utd2lkdGg9IjIuNCIgc3Ryb2tlLWxpbmVjYXA9InJvdW5kIiBkPSJNMTIgMTQuNSAxNi41IDkiLz48Y2lyY2xlIGZpbGw9IiNmZmYiIGN4PSIxMiIgY3k9IjE0LjUiIHI9IjIuMiIvPjwvc3ZnPg==&logoColor=white)](https://github.com/nao1215/himorime)
[![Go Reference](https://pkg.go.dev/badge/github.com/nao1215/gozen.svg)](https://pkg.go.dev/github.com/nao1215/gozen)
![GitHub](https://img.shields.io/github/license/nao1215/gozen)
[![GitHub Downloads (all assets, all releases)](https://img.shields.io/github/downloads/nao1215/gozen/total)](https://github.com/nao1215/gozen/releases)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/nao1215/gozen/badge)](https://scorecard.dev/viewer/?uri=github.com/nao1215/gozen)


<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/images/gozen-logo-dark.svg">
    <img src="docs/images/gozen-logo.svg" alt="Gozen" height="100">
  </picture>
</p>

# Gozen

Gozen is a 2D RPG framework for Go and Ebitengine. You write the game in YAML. `gozen check` finds broken references. `gozen sim battle` measures balance. While you play, F9 saves a note, F7 shows win rates, and F5 reloads the YAML.

The name: Ebitengine is named after ebiten (海老天), shrimp tempura. In Japan, an ebiten gozen (御膳) is a set meal on one tray: shrimp tempura with soba, small side dishes, and a sweet such as dango. Gozen is that tray for Ebitengine: the engine served with everything an RPG needs. The name also starts with Go.

Documentation: https://nao1215.github.io/gozen/

![Go Zen, the Gozen sample game: the town map, a dungeon, a battle with a cut-in, and a talk](docs/images/go-zen.gif)

## Try it

Go 1.26.9 or later. No display or assets needed:

```sh
go run github.com/nao1215/gozen/examples/minimal@latest
```

<!-- verify: output -->
```text
location=camp gold=2
```

That game is two YAML files and a 43-line Go host. The sample above is [Go Zen](examples/go-zen), a module of its own. Play it from a clone:

<!-- verify: skip needs a clone of the repository and opens the game window -->
```sh
git clone https://github.com/nao1215/gozen
cd gozen/examples/go-zen
go run . --mute
```

Play Go Zen in the browser: https://nao1215.github.io/gozen/play/

## The loop

| Step | Tool |
|:--|:--|
| Write | YAML: places, battles, skills, enemy tactics, events, dialogue. |
| Check | `gozen check` prints file and line: `data/world.yaml:26:5: locations.grotto.encounters: unknown troop "lessn" [GZN1101]`. |
| Measure | `gozen sim battle` prints a win rate. `gozen sim dive` plays a dungeon dive. F7 shows the same numbers in the game. |
| Play | `--dev` starts anywhere, autoplays, records and replays a trace. |
| Note | F9 saves a screenshot and the state. `--from-memo` starts at that line. |
| Fix | Edit the YAML under `--dir` and press F5. The game keeps your place. |

The window, a headless session and the command line share the rules.

## Screens

All from [Go Zen](examples/go-zen) at 1920x1080.

| Conversation | Event picture |
|:--|:--|
| ![Maren talks, a moving portrait that blinks and changes its face](docs/images/go-zen-face.gif) | ![Maren with a map by the dry fountain](docs/images/go-zen-picture.jpg) |
| Painted scene | Town map |
| ![Maren walks the plaza and Ox says a line in a balloon](docs/images/go-zen-plaza.gif) | ![the party's face walks the road to the well](docs/images/go-zen-map.gif) |
| First-person dungeon | Shop |
| ![steps and turns in a brick corridor](docs/images/go-zen-dungeon.gif) | ![the shop's goods](docs/images/go-zen-shop.jpg) |
| Battle | Cut-in |
| ![four against the Brass Warden, with damage numbers](docs/images/go-zen-battle.gif) | ![Ox's Shield Rush cut-in](docs/images/go-zen-cutin.gif) |
| Balance panel (F7) | Title |
| ![win rates by troop](docs/images/go-zen-balance.jpg) | ![the title screen](docs/images/go-zen-title.jpg) |
| Skill tree | |
| ![Maren's skill tree with Ink Burst chosen](docs/images/go-zen-tree.jpg) | |

## Install

Go 1.26.9 or later:

```sh
go install github.com/nao1215/gozen/cmd/gozen@latest
```

With a C compiler, `CGO_ENABLED=1` adds the ONNX inference of `gozen art mosaic`.

After the first release:

<!-- verify: skip the Homebrew cask is published with the first release -->
```sh
brew install --cask nao1215/tap/gozen
```

[The release page](https://github.com/nao1215/gozen/releases) will have archives for Linux, macOS and Windows (amd64, arm64) and `.deb`, `.rpm` and `.apk` packages. They have no ONNX inference.

As a library:

<!-- verify: run fixture=module -->
```sh
go get github.com/nao1215/gozen@latest
```

The window needs the platform graphics libraries: [OS setup](https://nao1215.github.io/gozen/install/#what-the-window-needs).

## Recipes

Tested YAML recipes are in the [cookbook](https://nao1215.github.io/gozen/cookbook/).

With a coding agent, install the [gozen skill](skills/gozen/SKILL.md). It holds the schema links and one rule: run `gozen check` until it passes.

- Claude Code: copy `skills/gozen` to `~/.claude/skills/gozen`.
- Codex: copy `skills/gozen` to `~/.codex/skills/gozen`.

<!-- verify: run fixture=cave -->
```sh
gozen check game                                       # references, expressions, pictures, text layout
gozen sim battle game cave_boss --level 8              # win rate at level 8
gozen sim dive game cave --floors 5 --boss 5:cave_boss # levels and wins per floor
```

`gozen book game > book.md` writes the whole game as one Markdown file.

Test tools of your game's binary:

<!-- verify: skip runs in your own game's module and opens the game window -->
```sh
go run . --dev --dir game --memos notes                   # F9 note, F7 win rates, F5 reload
go run . --dev --start cave --level 8 --battle cave_boss  # jump to a fight
go run . --dev --autoplay 60 --speed 8 --trace run.jsonl  # autoplay and record
gozen play trace run.jsonl                                # read the record
gozen play notes notes                                    # notes as one HTML page
```

Embed a game:

```go
//go:embed game
var files embed.FS

func main() {
	// game/ holds game.yaml, data/ and assets/; the options are the game's (--help, --dev ...)
	if err := play.Main(context.Background(), files, "game", os.Args[1:], play.Config{Output: os.Stdout}); err != nil {
		log.Fatal(err)
	}
}
```

Test it in Go with `gozen.Session`, which runs the rules without a window:

```sh
go run github.com/nao1215/gozen/examples/headless@latest
```

<!-- verify: output -->
```text
location=trail battles=1 wins=1 gold=3
```

## gozen

| Command | What it does |
|:--|:--|
| `check` | definitions, references, expressions, pictures, text layout (`--only structure` or `--only layout`) |
| `sim battle`, `sim dive`, `sim floors`, `sim design` | win rate; a dungeon dive; generated floors; scripted players |
| `play trace`, `play notes`, `play shots` | read a trace; notes as HTML; screenshots of a list |
| `book` | the whole game as Markdown |
| `pack release`, `pack trial` | approved pictures only; a demo |
| `art prompt`, `art mosaic`, `art fetch`, `art faces`, `art puppet` | image prompts; pixelation; models; balloon faces; moving portraits |
| `version`, `help` | the version; a command's help, or a diagnostic code |

`gozen help <command>` shows options and examples. `gozen help GZN1101` explains a code. [atago](https://github.com/nao1215/atago) tests every command end to end.

## Features

- YAML definitions with expressions, events and versioned saves
- Turn-based battles with a party, auto battle and enemy tactics
- Skill trees, shops, a field menu and a picture gallery
- First-person grid dungeons, fixed or generated
- Moving portraits from one picture ([puppet](https://pkg.go.dev/github.com/nao1215/gozen/puppet))
- Characters that react to typed words, and a chat log ([react](https://pkg.go.dev/github.com/nao1215/gozen/react), [chat](https://pkg.go.dev/github.com/nao1215/gozen/chat))
- Engine words in English and Japanese (`language` in game.yaml)

## Limitations

- The API may change before v1.
- Runtime checks run on Linux. Windows and macOS are built in CI, not played.
- There is no graphical editor.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md), the [Code of Conduct](CODE_OF_CONDUCT.md), [SUPPORT.md](.github/SUPPORT.md) and [SECURITY.md](SECURITY.md).

## Support

A star or a [GitHub Sponsors](https://github.com/sponsors/nao1215) sponsorship helps.

## License

The code is [MIT](LICENSE). The Go Zen pictures and music are [CC BY 4.0](examples/go-zen/game/assets/LICENSE). Font notices are in [ASSETS.md](ASSETS.md).

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

This project follows the [all-contributors](https://github.com/all-contributors/all-contributors) specification. Contributions of any kind are welcome.
