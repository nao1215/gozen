<!-- ALL-CONTRIBUTORS-BADGE:START - Do not remove or modify this section -->
[![All Contributors](https://img.shields.io/badge/all_contributors-1-orange.svg?style=flat-square)](#contributors-)
<!-- ALL-CONTRIBUTORS-BADGE:END -->
![Coverage](https://raw.githubusercontent.com/nao1215/octocovs-central-repo/main/badges/nao1215/gozen/coverage.svg)
[![CI](https://github.com/nao1215/gozen/actions/workflows/ci.yml/badge.svg)](https://github.com/nao1215/gozen/actions/workflows/ci.yml)
[![reviewdog](https://github.com/nao1215/gozen/actions/workflows/reviewdog.yml/badge.svg)](https://github.com/nao1215/gozen/actions/workflows/reviewdog.yml)
[![Security](https://github.com/nao1215/gozen/actions/workflows/security.yml/badge.svg)](https://github.com/nao1215/gozen/actions/workflows/security.yml)
[![tested with atago](https://img.shields.io/badge/tested%20with-atago-7c3aed?logo=data:image/svg%2Bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCI%2BPHBhdGggZmlsbD0iI2ZmZiIgZD0iTTMuNiA0LjIgMTEuOSAxMmwtOC4zIDcuOC0xLjktMi4yTDcuOSAxMiAxLjcgNi40eiIvPjxyZWN0IGZpbGw9IiNmZmYiIHg9IjEyLjYiIHk9IjE3LjIiIHdpZHRoPSI5LjciIGhlaWdodD0iMi44IiByeD0iMS40Ii8%2BPC9zdmc%2B&logoColor=white)](https://github.com/nao1215/atago)
[![measured with himorime](https://img.shields.io/badge/measured%20with-himorime-d9480f?logo=data:image/svg%2Bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCI%2BPHBhdGggZmlsbD0ibm9uZSIgc3Ryb2tlPSIjZmZmIiBzdHJva2Utd2lkdGg9IjIuNCIgc3Ryb2tlLWxpbmVjYXA9InJvdW5kIiBkPSJNNC4yIDE4LjVBOSA5IDAgMSAxIDE5LjggMTguNSIvPjxwYXRoIGZpbGw9Im5vbmUiIHN0cm9rZT0iI2ZmIiBzdHJva2Utd2lkdGg9IjIuNCIgc3Ryb2tlLWxpbmVjYXA9InJvdW5kIiBkPSJNMTIgMTQuNSAxNi41IDkiLz48Y2lyY2xlIGZpbGw9IiNmZmYiIGN4PSIxMiIgY3k9IjE0LjUiIHI9IjIuMiIvPjwvc3ZnPg==&logoColor=white)](https://github.com/nao1215/himorime)
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

Gozen is a no-code toolkit for creating 2D RPGs, visual novels and strategy games with YAML and assets. Create a template, edit its text and rules, and play it with one command. No Go code or compiler is required to create and play a game. Built with Go and Ebitengine.

Documentation: https://nao1215.github.io/gozen/

| RPG: [Go Zen](https://nao1215.github.io/gozen/play/go-zen/) (play in browser) | Novel: [After the Pass](https://nao1215.github.io/gozen/play/novel/) (play in browser) |
|:--|:--|
| ![Go Zen, the Gozen sample game: the town map, a dungeon, a battle with a cut-in, and a talk](docs/images/go-zen.gif) | ![After the Pass, the novel sample: a line, a choice, Back over the choice, and Skip to the first line not read](docs/images/novel.gif) |
| Campaign: [Five Banners](https://nao1215.github.io/gozen/play/campaign/) (play in browser) | Monster collection: [Go Zen](https://nao1215.github.io/gozen/play/go-zen/) (play in browser) |
| ![Five Banners, the campaign sample: the war map, the sortie screen, a clash of two armies, and the region in the Crown's colour](docs/images/campaign.gif) | ![Breeding monsters in Go Zen: Yorn, the two parents, a silhouette reveal and the newborn Succubus](docs/images/go-zen-breeding.gif) |
| Management: [Harbor app team](examples/office) | |
| [![the office sample: the standup, the team, handing out work, a message, a video call with a shared screen, the calendar and the plan](docs/images/office.gif)](docs/images/office.mp4) | |

## Try it

Install the `gozen` binary from [Releases](https://github.com/nao1215/gozen/releases) or follow the [installation guide](https://nao1215.github.io/gozen/install/).

```sh
gozen new my-game --template rpg
gozen check my-game
```

<!-- verify: skip opens a window; generated templates are played by the graphics E2E -->
```sh
gozen run my-game
```

Choose `novel` or `campaign` for another template. Edit `game.yaml` and `data/world.yaml` in a text editor; no programming language is needed. YAML editing and terminal commands are the current interface, not a visual editor.

Play the larger samples in your browser: https://nao1215.github.io/gozen/play/

## Screens

All from [Go Zen](examples/go-zen) at 1920x1080.

| Conversation | Painted scene |
|:--|:--|
| ![Maren talks, a moving portrait that blinks and changes its face](docs/images/go-zen-face.gif) | ![Maren walks the plaza and Ox says a line in a balloon](docs/images/go-zen-plaza.gif) |
| Town map | First-person dungeon |
| ![the party's face walks the road to the well](docs/images/go-zen-map.gif) | ![steps and turns in a brick corridor](docs/images/go-zen-dungeon.gif) |
| Battle | Cut-in |
| ![four against the Brass Warden, with damage numbers](docs/images/go-zen-battle.gif) | ![Ox's Shield Rush cut-in](docs/images/go-zen-cutin.gif) |
| Shop | Skill tree |
| ![the shop's goods](docs/images/go-zen-shop.jpg) | ![Maren's skill tree with Ink Burst chosen](docs/images/go-zen-tree.jpg) |
| Title | |
| ![the title screen](docs/images/go-zen-title.jpg) | |

### Novel games

`ui.message: novel` makes a visual novel. All from [After the Pass](examples/novel), the novel sample, at 1920x1080.

| Roll back (Q, wheel) | Skip to the line not read (N) |
|:--|:--|
| ![Back goes over two lines and a choice, with the state](docs/images/novel-back.gif) | ![the option taken again, then Skip stops at the first line not read](docs/images/novel-skip.gif) |
| Two languages | Staging (`stand`, `effect`) |
| ![the settings switch to Japanese, the line on the screen turns Japanese at once, and so does the next](docs/images/novel-language.gif) | ![Brenna slides in and her portrait shakes](docs/images/novel-stage.gif) |
| Event picture with its line | |
| ![Brenna's ending: a picture of her cheering at the bar, with the line under it](docs/images/novel-picture.jpg) | |

#### Phone apps

`phone` shows a phone over the scene, upright or on its side, in a story or any other game ([A phone on the screen](https://nao1215.github.io/gozen/writing-a-game/#a-phone-on-the-screen)). All from the office sample.

| Chat: a picture, times and a clip | Feed: scrolled, a clip post, likes |
|:--|:--|
| ![a team chat on a phone: messages arrive one by one with times and a read mark, a picture opens over the screen, then a clip sent in the chat turns the phone on its side and plays a recorded call](docs/images/phone-chat.gif) | ![a photo feed scrolled down a post at a time, a battle clip looping in its post and Mia's dance clip after it](docs/images/phone-feed.gif) |
| Video on its side: footage | Chat: a dance practice clip (a puppet) |
| ![a video app: the phone turns on its side, nearly as wide as the screen, and plays footage of a battle from the Go Zen sample](docs/images/phone-video.gif) | ![Mia says in the chat that she is practicing the launch party dance and sends the clip; it plays on the phone on its side, her puppet breathing, swaying and jiggling](docs/images/phone-puppet.gif) |
| Page: a shop site with a picture grid | Page: a bank transfer to confirm |
| ![a browser page of a clothing shop: a grid of pictures, two list items with prices and a button pressed with the keys](docs/images/phone-shop.gif) | ![a bank app: the balance, a transfer to confirm and recent items; the keys ring the buttons and press Confirm](docs/images/phone-bank.gif) |
| Lock screen: notices arriving | Lock screen: an incoming call answered |
| ![the lock screen at 7:12: a bank notice, a chat preview and a calendar reminder arrive one a press](docs/images/phone-lock.gif) | ![a night on-call: two monitoring notices, an incoming call from the on-call app declined, ringing again and answered](docs/images/phone-call.gif) |
| Page: a sign-in approval | Feed: a timeline of text posts |
| ![a sign-in check asks to approve a sign-in to the production console; the keys ring Approve and Deny and press Approve](docs/images/phone-sso.gif) | ![a timeline of text posts with avatars and times about the booking app being down, with likes, replies and reposts, scrolled](docs/images/phone-timeline.gif) |
| Banner: a push notification over a scene | Page: a supporter site with locked posts |
| ![a push banner slides down over the novel window while the scene goes on, and a tap on it opens its event](docs/images/phone-banner.gif) | ![an illustrator's supporter page: tiers with prices, a Support button and locked posts that open when the page is shown again](docs/images/phone-support.gif) |
| Page: a contact to add with a QR code |  |
| ![an Add contact page with a QR code of the gozen repository; the keys ring and press Add, and Mia's first private message arrives](docs/images/phone-contact.gif) |  |

### Management workspaces

Workspace, call, calendar and timeline screens with a day `flow` (schema 10) make a management game. From the office sample ([examples/office](examples/office)); the YAML is in [workspace recipes](https://nao1215.github.io/gozen/workspaces/).

| Standup: busts, meters, dots, signs, burnup | People: press for a peek |
|:--|:--|
| ![six team cards with busts, progress meters, status dots and a burnup chart](docs/images/workboard-cards.jpg) | ![the team with Daniel's peek open](docs/images/workboard-peek.jpg) |
| Hand out work by dragging (or with keys) | Tickets, dependencies and a carousel |
| ![Acceptance tests is refused by Daniel, then Payments is handed to him](docs/images/workboard-drag.gif) | ![tickets joined by a dependency line, and the help carousel turning to an unavailable choice](docs/images/workboard-carousel.gif) |
| A message asks for a decision | Video call |
| ![Elena's message is opened and a choice appears](docs/images/workboard-message.gif) | ![the team call: the speaker on the stage, the others in the side column](docs/images/workboard-call.gif) |
| Screen sharing | First-visit guide |
| ![Elena shares the booking form in the call](docs/images/workboard-share.jpg) | ![Martin's guide line over the veiled standup](docs/images/workboard-guide.jpg) |
| Calendar with scheduled events | Plan timeline |
| ![the sprint calendar with the review and the demo](docs/images/workboard-calendar.jpg) | ![the sprint plan with five activities](docs/images/workboard-timeline.jpg) |
| Character grid | Glass panels |
| ![the character book in a grid](docs/images/workboard-profiles.jpg) | ![two overlapping glass panels over the office](docs/images/workboard-materials.jpg) |

### Campaigns

`regions`, `factions` and `campaign` (schema version 5) make a war of regions over turns. A campaign can share a game with dungeons and towns: generals are the same actors, and `move` goes between them. From [Five Banners](examples/campaign), the campaign sample.

| The war map | A clash of armies |
|:--|:--|
| ![the picture map of the land with the regions in the owners' colours, the band with the week, the window of a region and the generals](docs/images/campaign-map.jpg) | ![two armies of soldiers lined up on the terrain, with the leaders' cards in a row below](docs/images/campaign-clash.jpg) |
| A region's `area` and a point's `at` lay the regions on one picture of the land. | `clash.armies` gives each army kind its soldiers; each unit is a crowd that shrinks as it loses troops. |

`gozen sim campaign game` plays 200 campaigns and prints the win rate, the turns, the regions each side held turn by turn, and the orders given.

### Monster collection

Monsters ask to join after a battle, wait at the ranch, and two raised monsters, one male and one female, are bred into a new one at Yorn's altar, in the manner of Dragon Quest Monsters (schema version 6). The monster book (`ui.bestiary: true`) opens a page as the party meets each monster and shows how to breed it, two generations up.

| A monster asks to join | Breeding at Yorn's altar |
|:--|:--|
| ![a Drip Slime rises after the battle and asks to join, with Yes and No](docs/images/go-zen-recruit.jpg) | ![Yorn talks in the center, the father on the left and the mother on the right turn into light, then the Succubus is born](docs/images/go-zen-breeding.gif) |
| The monster book | The family tree, two generations up |
| ![the Succubus page of the monster book with its picture, lore and record](docs/images/go-zen-bestiary.jpg) | ![Ghoul and Night Bat make the Vampire, Incense Bloom and Pale Sylph make the Whisper Imp, and the two make the Succubus](docs/images/go-zen-lineage.jpg) |

<!-- verify: parse the short enemy example lists the fields used by the monster book -->
```yaml
enemies:
  succubus:
    name: Succubus
    image: enemies/succubus.png
    lore: "A night-dwelling spirit of the old ruins, known for swift magic."
    stats: {mhp: 420, mmp: 20, atk: 56, def: 24, mat: 45, mdf: 23, agi: 24, luk: 5}
```

`gozen check game` finds breeding rules that can never work, and `gozen sim breed game` lists every monster reachable from the ones that can be caught, with the shortest recipe. See [Collect, raise and breed monsters](https://nao1215.github.io/gozen/cookbook/#collect-raise-and-breed-monsters).

### Battles

Enemies can change form at an HP threshold, combine with allies, change pictures as HP falls, and move with a puppet portrait. Go Zen shows the Tollkeeper's second form, Drip Slimes gathering into King Slime, and a moving Succubus face with a soft sway region (schema version 7).

| Transform | Merge |
|:--|:--|
| ![The Tollkeeper changes into its second form](docs/images/go-zen-transform.gif) | ![Drip Slimes gather into King Slime](docs/images/go-zen-merge.gif) |
| Pictures by HP | Motion and sway |
| ![A damaged monster shows its low HP picture](docs/images/go-zen-hp-pictures.jpg) | ![The Succubus blinks, a soft region sways, and she reacts to a hit](docs/images/go-zen-motion.gif) |

See [the battle recipes](https://nao1215.github.io/gozen/cookbook/#transform-an-enemy).

### Multi-level maps

[![Walking up to the second and third floors across the town](docs/images/space-stairs.gif)](docs/media/stacked-space-short.mp4)

## Playtesting

![Play, script or autoplay to a trace; reproduce a screen, inspect its structure, read play metrics or section frames. Capture and compare with the previous capture.](docs/images/playtesting-flow.svg)

Record a play with `gozen run game --dev --trace run.jsonl`; add `--script input.txt --shots shots` or `--autoplay 60` to drive it automatically. Gozen reports what happened; the author decides what to change.

| Inspect | Example |
|:--|:--|
| Reproduce a screen from a recorded tick | `gozen trace run.jsonl --repro 120 --out repro` |
| Read the screen's text, cast and controls | `gozen trace run.jsonl --only screen` |
| Read text per decision, repetition and waiting spans | `gozen trace run.jsonl --metrics` |
| Read frame counts and timings between script `mark` commands | `gozen trace run.jsonl --only section --format jsonl` |
| Compare the same capture jobs with the previous output | `gozen capture game --config captures.yaml --out current --against previous` |

[Runnable examples, output and limits](https://nao1215.github.io/gozen/edit-play-loop/#playtesting) include the script and capture configuration. Reproduction refuses traces it cannot reproduce exactly; its bundle needs the unchanged game assets.

Start the game with `--dev` and these keys work while you play.

| F5: edit the YAML, the line changes in place | F9: a note with the screen and the state |
|:--|:--|
| ![Hedda's line is edited in the YAML and F5 shows the new text at the same place](docs/images/playtest-reload.gif) | ![F9 opens a note, a comment is typed and saved](docs/images/playtest-memo.gif) |
| F7: win rates of the battles here | F8 State: flip a flag, step a number |
| ![win rates by troop](docs/images/go-zen-balance.jpg) | ![flag.thanked_hedda and chose.toast in the development panel](docs/images/novel-panel.jpg) |
| F8 Flags: what each event sets and reads | F8 Scenes: jump to an event |
| ![the flags each event sets and reads](docs/images/novel-flags.jpg) | ![the Scenes tab jumps to the late night](docs/images/playtest-jump.gif) |
| F8 Command: run a line of YAML | F8 Frames: scene state, control boxes, Update and Draw |
| ![a set typed into the Command tab, then the State tab shows the flag](docs/images/playtest-command.gif) | ![the Frames overlay over the tavern](docs/images/playtest-frames.jpg) |
| F6: the character book | Tab: the relationship chart (Z focuses a face) |
| ![Wren's page in the character book: her picture, fields, introduction and how she sees the others](docs/images/novel-characters.jpg) | ![Ox focused at the center of the relationship chart with his neighbors in a circle](docs/images/go-zen-relations-focus.jpg) |

Outside the window, `gozen check game` reports a broken reference, a wrong expression, a missing picture or a line too long for its window with its file and line, such as `data/world.yaml:26:5: locations.grotto.encounters: unknown troop "lessn" [GZN1101]`. `gozen sim battle` fights a battle many times and prints the win rate (the numbers F7 shows), and `gozen sim dive` plays a whole dungeon dive. A note written with F9 can be opened again with `gozen run game --dev --from-memo` on its `.json`, at the same line. The window, a headless session and the command line follow the same rules.

All the tools: [the edit–play loop](https://nao1215.github.io/gozen/edit-play-loop/).

## Install

Go 1.26.10 or later:

```sh
go install github.com/nao1215/gozen/cmd/gozen@latest
```

With a C compiler, `CGO_ENABLED=1` adds the ONNX inference of `gozen mosaic`.

With Homebrew:

<!-- verify: skip installs from the Homebrew tap, outside the repository -->
```sh
brew install --cask nao1215/tap/gozen
```

[The release page](https://github.com/nao1215/gozen/releases) has archives for Linux, macOS and Windows (amd64, arm64) and `.deb`, `.rpm` and `.apk` packages. They have no ONNX inference.

The window needs the platform graphics libraries: [OS setup](https://nao1215.github.io/gozen/install/#what-the-window-needs).

In GitHub Actions, [setup-gozen](https://github.com/nao1215/setup-gozen) installs the release binary of the version in your `go.mod`, checked against the release checksums:

<!-- verify: skip runs in GitHub Actions -->
```yaml
- uses: actions/checkout@v6
- uses: nao1215/setup-gozen@1c378fc446f62ecb29a7bcd40aea6dff8565d698 # v0.1.1
- run: gozen check game
- run: gozen sim battle game --troop boss --format json  # boss: a troop id of your game
```

`version` also takes `latest` or a release such as `v0.2.0`, and `go-mod-path` points at a `go.mod` below the root. Without the action, `go run github.com/nao1215/gozen/cmd/gozen check game` builds the same version from `go.mod`.

## Recipes

Tested YAML recipes are in the [cookbook](https://nao1215.github.io/gozen/cookbook/).

With a coding agent, install the [gozen skill](skills/gozen/SKILL.md). It holds the schema links and one rule: run `gozen check` until it passes.

- Claude Code: copy `skills/gozen` to `~/.claude/skills/gozen`.
- Codex: copy `skills/gozen` to `~/.codex/skills/gozen`.

<!-- verify: run fixture=cave -->
```sh
gozen check game                                       # references, expressions, pictures, text layout
gozen sim battle game --troop cave_boss --level 8              # win rate at level 8
gozen sim dive game --location cave --floors 5 --boss 5:cave_boss # levels and wins per floor
```

`gozen book game > book.md` writes the whole game as one Markdown file.

Run the directory directly with `gozen run game`, or enable author tools with `gozen run game --dev`. No Go source is needed.

Test tools:

<!-- verify: skip opens a game window and waits for input -->
```sh
gozen run game --dev --memos notes                   # F9 note, F7 win rates, F5 reload
gozen run game --dev --start cave --level 8 --battle cave_boss  # jump to a fight
gozen run game --dev --autoplay 60 --speed 8 --trace run.jsonl  # autoplay and record
gozen trace run.jsonl                                # read the record
gozen notes notes --out notes-html                   # notes as one HTML page
```

## gozen

| Command | What it does |
|:--|:--|
| `new` | create a playable RPG, novel or campaign from a template |
| `check` | definitions, references, expressions, pictures, text layout (`--only structure` or `--only layout`) |
| `sim battle`, `sim dive`, `sim floors`, `sim design` | win rate; a dungeon dive; generated floors; scripted players |
| `run`, `trace`, `notes`, `capture` | run a game directory; read a trace; notes as HTML; screenshots of a list |
| `book` | the whole game as Markdown, JSON or JSONL |
| `export` | playable desktop directories; static browser games, using prebuilt runtimes |
| `release`, `trial` | approved pictures only; a demo |
| `prompt`, `mosaic`, `models fetch`, `faces`, `puppet` | image prompts; pixelation; models; balloon faces; moving portraits |
| `theme add`, `theme list` | copy a dungeon theme (walls, floor, ceiling, door) into a game; list the themes |
| `version`, `help` | the version; a command's help, or a diagnostic code |

`gozen help <command>` shows options and examples. `gozen help GZN1101` explains a code. [atago](https://github.com/nao1215/atago) tests every command end to end.

## Features

- YAML definitions with expressions, events and versioned saves
- Turn-based battles with a party, auto battle and enemy tactics
- Skill trees, shops, a field menu and a picture gallery
- First-person grid dungeons, fixed or generated, with themes (`theme: cave`, `gozen theme add`)
- Moving portraits from one picture ([portrait YAML](https://nao1215.github.io/gozen/cookbook/#give-a-character-faces-and-a-moving-portrait))
- Campaigns: regions held by factions over turns, orders with generals, a sortie screen with tactics, marches shown as clashes of armies (or decided by numbers or a battle), the other factions' rules, and `gozen sim campaign` ([recipes](https://nao1215.github.io/gozen/cookbook/#make-a-campaign))
- Management games: workspace cards, drag and drop, tickets, messages, video calls with screen sharing, calendars, timelines and first-visit guides ([recipes](https://nao1215.github.io/gozen/workspaces/))
- Visual novels: roll back, skip what was read, save in a scene, choices read as `chose`, staging with `stand` ([recipes](https://nao1215.github.io/gozen/cookbook/#let-the-player-roll-back-and-save-in-a-scene))
- Inspectable demo builds: `gozen trial` selects content using `trial.yaml`, reports unresolved conditions, and rejects explicitly forbidden definitions or assets before writing ([recipe](https://nao1215.github.io/gozen/cookbook/#check-the-contents-of-a-trial))
- Engine words in English and Japanese (`language` in game.yaml), and the game's text in two languages (`lang/`)

## Made with Gozen

| [Succubus Live](https://github.com/nao1215/succubus-live): a streaming-site game with succubus VTubers ([watch in your browser](https://nao1215.github.io/succubus-live/)) |
|:--|
| ![Succubus Live: channel listing and fullscreen streams featuring all seven characters](https://raw.githubusercontent.com/nao1215/succubus-live/main/doc/img/showcase.gif) |

## Limitations

- The supported interfaces are CLI commands, YAML definitions and saves. Go implementation packages are internal.
- Gameplay tests cover Linux, Windows and macOS. Browser E2E targets Chrome.
- There is no graphical editor.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md), the [Code of Conduct](CODE_OF_CONDUCT.md), [SUPPORT.md](.github/SUPPORT.md) and [SECURITY.md](SECURITY.md).

## Support

A star or a [GitHub Sponsors](https://github.com/sponsors/nao1215) sponsorship helps.

## License

The code is [MIT](LICENSE). The Go Zen, After the Pass and Five Banners pictures claim no copyright (CC0 1.0) and their music is CC BY 4.0 ([Go Zen](examples/go-zen/game/assets/LICENSE), [After the Pass](examples/novel/game/assets/LICENSE), [Five Banners](examples/campaign/game/assets/LICENSE)). Font notices are in [ASSETS.md](ASSETS.md).

## The name

Ebitengine is named after ebiten (海老天), shrimp tempura. In Japan, an ebiten gozen (御膳) is a set meal on one tray: shrimp tempura with soba, small side dishes, and a sweet such as dango. Gozen is that tray for Ebitengine: the engine served with everything an RPG needs. The name also starts with Go.

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
