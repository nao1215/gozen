---
title: Gozen
---

Gozen is a 2D RPG framework for Go and [Ebitengine](https://ebitengine.org/). You, or your coding agent, write the game as YAML. `gozen check` and `gozen sim battle` catch broken references and bad balance before anyone plays. While you play, you leave a note on the exact moment, watch live win rates, and reload the YAML without restarting.

The name: a gozen (御膳) is a Japanese meal served on a tray. Ebitengine takes its name from ebiten, shrimp tempura; Gozen sets that ebiten on a tray with everything an RPG needs, and the name starts with Go.

![Go Zen, the Gozen sample game: a talk with moving portraits](/images/go-zen.gif)

## Try it in 30 seconds

With Go 1.26.9 or later, paste this. It needs no display and no assets:

```shell
go run github.com/nao1215/gozen/examples/minimal@latest
```

<!-- verify: output -->
```text
location=camp gold=2
```

That game is two YAML files and a 43-line Go host. The sample above is [Go Zen](https://github.com/nao1215/gozen/tree/main/examples/go-zen), a module of its own. Play it from a clone:

<!-- verify: skip needs a clone of the repository and opens the game window -->
```shell
git clone https://github.com/nao1215/gozen
cd gozen/examples/go-zen
go run . --mute
```

Or [play Go Zen in the browser](/play/).

On Linux the window needs X11 and OpenGL; see [Install](/install/).

## The loop

Making an RPG is mostly tuning: a line that reads wrong, a boss that is too hard, a dungeon that is too long. Gozen shortens each round.

| Step | What you do |
|:--|:--|
| Write | Put places, battles, skills, enemy tactics, events and dialogue in YAML. Text is easy to write, diff and review, for you and for an agent. |
| Check | `gozen check game` reports broken references, wrong expressions, missing pictures and text that overflows its window, with file and line. |
| Measure | `gozen sim battle` fights a battle hundreds of times and prints the win rate. `gozen sim dive` plays a whole dungeon dive. |
| Play | Run the game with `--dev`: start anywhere, autoplay at high speed, record and replay a trace. |
| Note | Press F9 to save a note with a screenshot and the game state. `--from-memo` starts the game again at that line. |
| Fix | Edit the YAML and press F5. The game reloads its definitions and keeps your place. F7 shows the win rates of the battles here. |

```text
data/world.yaml:26:5: locations.grotto.encounters: unknown troop "lessn" [GZN1101]
```

The rules are shared by the window, a headless driver and the command line, so a test in CI and a player see the same game.

## Screens

All from [Go Zen](https://github.com/nao1215/gozen/tree/main/examples/go-zen) at 1920x1080.

| Conversation | Event picture |
|:--|:--|
| ![Maren talks, a moving portrait](/images/go-zen-talk.jpg) | ![Maren with a map by the dry fountain](/images/go-zen-picture.jpg) |
| Painted scene | Town map |
| ![the plaza with a balloon](/images/go-zen-plaza.jpg) | ![the town map with name plates](/images/go-zen-map.jpg) |
| First-person dungeon | Shop |
| ![a brick corridor and the map](/images/go-zen-dungeon.jpg) | ![the shop's goods](/images/go-zen-shop.jpg) |
| Battle | Cut-in |
| ![four against the Brass Warden](/images/go-zen-battle.jpg) | ![Ox's Shield Rush cut-in](/images/go-zen-cutin.jpg) |
| Balance panel (F7) | Title |
| ![win rates by troop](/images/go-zen-balance.jpg) | ![the title screen](/images/go-zen-title.jpg) |

## Where to go

- [Install](/install/): the gozen command, the library, and what each OS needs for the window.
- [Getting started](/getting-started/): run the samples, then start your own game.
- [Writing a game](/writing-a-game/): how the YAML is laid out, with expressions and events.
- [Schema](/schema/): every YAML field, its type, default and references.
- [The edit–play loop](/edit-play-loop/): `--dev`, F5, F7, F9, autoplay, traces and notes.
- [Commands](/commands/): every `gozen` subcommand and its options.
- [Using AI agents](/ai-agents/): let an agent write the game and keep `gozen check` green.
- [Go API](/go-api/): headless tests, embedding the runner, reactions and chat, moving portraits.

## Features

- YAML definitions with expressions, conditional events and versioned saves
- Turn-based battles with a party, auto battle and per-enemy tactics
- Skill trees, shops, a field menu and a picture gallery
- First-person grid dungeons, fixed or generated floors
- Moving portraits from one picture and a few difference pictures: blinking, talking, breathing, head, hair and swaying parts, reactions and face changes
- Characters that react to typed words and events, and a chat log
- Engine words in English and Japanese, chosen by `language` in game.yaml
- A headless driver that shares the rules with the window

## Limitations

- Gozen is young. The API may change before v1.
- Runtime checks are done on Linux. Windows and macOS are built in CI but not yet played.
- There is no graphical map or event editor; the game is text.

## Install

```shell
go install github.com/nao1215/gozen/cmd/gozen@latest
```

As a library: `go get github.com/nao1215/gozen@latest`. The [install page](/install/) lists the graphics libraries for each OS.
