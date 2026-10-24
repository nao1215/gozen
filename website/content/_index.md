---
title: Gozen
---

Gozen is a YAML-first framework for 2D RPGs, visual novels and strategy games in Go and [Ebitengine](https://ebitengine.org/). You, or your coding agent, write the game as YAML: places, battles and skills for an RPG; lines, choices and staging for a novel, with roll back, skip and a second language; regions, generals and orders for a campaign. `gozen check`, `gozen sim battle` and `gozen sim campaign` catch broken references and bad balance before anyone plays. While you play, you leave a note on the exact moment, watch live win rates, and reload the YAML without restarting.

The name: Ebitengine is named after ebiten (海老天), shrimp tempura. In Japan, an ebiten gozen (御膳) is a set meal on one tray: shrimp tempura with soba, small side dishes, and a sweet such as dango. Gozen is that tray for Ebitengine: the engine served with everything a game needs. The name also starts with Go.

| RPG: [Go Zen](https://github.com/nao1215/gozen/tree/main/examples/go-zen) | Novel: [After the Pass](https://github.com/nao1215/gozen/tree/main/examples/novel) |
|:--|:--|
| ![Go Zen, the Gozen sample game: the town map, a dungeon, a battle with a cut-in, and a talk](/images/go-zen.gif) | ![After the Pass, the novel sample: a line, a choice, Back over the choice, and Skip to the first line not read](/images/novel.gif) |
| Campaign: [Five Banners](https://github.com/nao1215/gozen/tree/main/examples/campaign) ([play in browser](/play/campaign/)) | Monster collection: [Go Zen](https://github.com/nao1215/gozen/tree/main/examples/go-zen) ([play in browser](/play/go-zen/)) |
| ![Five Banners, the campaign sample: the war map, the sortie screen, a clash of two armies, and the region in the Crown's colour](/images/campaign.gif) | ![Breeding monsters in Go Zen: Yorn, the two parents, a silhouette reveal and the newborn Succubus](/images/go-zen-breeding.gif) |

## Try it in 30 seconds

With Go 1.26.10 or later, paste this. It needs no display and no assets:

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

Or [play Go Zen and After the Pass in the browser](/play/).

On Linux the window needs X11 and OpenGL; see [Install](/install/).

## The loop

Making a game is mostly tuning: a line that reads wrong, a boss that is too hard, a dungeon that is too long, a war that one side always wins. Gozen shortens each round.

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

| Conversation | Painted scene |
|:--|:--|
| ![Maren talks, a moving portrait that blinks and changes its face](/images/go-zen-face.gif) | ![Maren walks the plaza and Ox says a line in a balloon](/images/go-zen-plaza.gif) |
| Town map | First-person dungeon |
| ![the party's face walks the road to the well](/images/go-zen-map.gif) | ![steps and turns in a brick corridor](/images/go-zen-dungeon.gif) |
| Battle | Cut-in |
| ![four against the Brass Warden, with damage numbers](/images/go-zen-battle.gif) | ![Ox's Shield Rush cut-in](/images/go-zen-cutin.gif) |
| Shop | Skill tree |
| ![the shop's goods](/images/go-zen-shop.jpg) | ![Maren's skill tree with Ink Burst chosen](/images/go-zen-tree.jpg) |
| Title | |
| ![the title screen](/images/go-zen-title.jpg) | |

### Novel games

`ui.message: novel` makes a visual novel. All from [After the Pass](https://github.com/nao1215/gozen/tree/main/examples/novel), the novel sample, at 1920x1080 ([Novel games](/writing-a-game/#novel-games)).

| Roll back (Q, wheel) | Skip to the line not read (N) |
|:--|:--|
| ![Back goes over two lines and a choice, with the state](/images/novel-back.gif) | ![the option taken again, then Skip stops at the first line not read](/images/novel-skip.gif) |
| Two languages | Staging (`stand`, `effect`) |
| ![the settings switch to Japanese, the line on the screen turns Japanese at once, and so does the next](/images/novel-language.gif) | ![Brenna slides in and her portrait shakes](/images/novel-stage.gif) |
| Event picture with its line | |
| ![Brenna's ending: a picture of her cheering at the bar, with the line under it](/images/novel-picture.jpg) | |

### Campaigns

`regions`, `factions` and `campaign` (schema version 5) make a war of regions over turns, with generals, orders and clashes of armies. From [Five Banners](https://github.com/nao1215/gozen/tree/main/examples/campaign), the campaign sample.

| The war map | A clash of armies |
|:--|:--|
| ![the picture map of the land with the regions in the owners' colours, the band with the week, the window of a region and the generals](/images/campaign-map.jpg) | ![two armies of soldiers lined up on the terrain, with the leaders' cards in a row below](/images/campaign-clash.jpg) |

### Monster collection

Monsters ask to join after a battle, wait at the ranch, and two raised monsters, one male and one female, are bred into a new one at Yorn's altar, in the manner of Dragon Quest Monsters (schema version 6). The monster book opens a page as the party meets each monster and shows how to breed it, two generations up. The recipe is [Collect, raise and breed monsters](/cookbook/#collect-raise-and-breed-monsters).

| A monster asks to join | Breeding at Yorn's altar |
|:--|:--|
| ![a Drip Slime rises after the battle and asks to join, with Yes and No](/images/go-zen-recruit.jpg) | ![Yorn talks in the center, the parents turn into light, then the Succubus is born](/images/go-zen-breeding.gif) |
| The monster book | The family tree, two generations up |
| ![the Succubus page of the monster book](/images/go-zen-bestiary.jpg) | ![the Succubus family tree with its parents and grandparents](/images/go-zen-lineage.jpg) |

### Battles

Enemies can change form at an HP threshold, combine with allies, change pictures as HP falls, and move with a puppet portrait. Go Zen shows the Tollkeeper's second form, Drip Slimes gathering into King Slime, and a moving Succubus face with a soft sway region (schema version 7).

| Transform | Merge |
|:--|:--|
| ![The Tollkeeper changes into its second form](/images/go-zen-transform.gif) | ![Drip Slimes gather into King Slime](/images/go-zen-merge.gif) |
| Pictures by HP | Motion and sway |
| ![A damaged monster shows its low HP picture](/images/go-zen-hp-pictures.jpg) | ![The Succubus blinks, sways and reacts to a hit](/images/go-zen-motion.gif) |

See [the battle recipes](/cookbook/#transform-an-enemy).

### Multi-level maps

[![Walking up to the second and third floors across the town](/images/space-stairs.gif)](https://github.com/nao1215/gozen/raw/main/docs/media/stacked-space-short.mp4)

## Playtesting

Start the game with `--dev` and these keys work while you play.

| F5: edit the YAML under `--dir`, the line changes in place | F9: a note with the screen and the state |
|:--|:--|
| ![Hedda's line is edited in the YAML and F5 shows the new text at the same place](/images/playtest-reload.gif) | ![F9 opens a note, a comment is typed and saved](/images/playtest-memo.gif) |
| F7: win rates of the battles here | F8 State: flip a flag, step a number |
| ![win rates by troop](/images/go-zen-balance.jpg) | ![flag.thanked_hedda and chose.toast in the development panel](/images/novel-panel.jpg) |
| F8 Flags: what each event sets and reads | F8 Scenes: jump to an event |
| ![the flags each event sets and reads](/images/novel-flags.jpg) | ![the Scenes tab jumps to the late night](/images/playtest-jump.gif) |
| F8 Command: run a line of YAML | F8 Frames: ticks and frames a second, the slowest Update and Draw |
| ![a set typed into the Command tab, then the State tab shows the flag](/images/playtest-command.gif) | ![the frame meter over the tavern](/images/playtest-frames.jpg) |
| F6: the character book, every page open: each character's profile, faces and relations | |
| ![Wren's page in the character book: her picture, fields, introduction and how she sees the others](/images/novel-characters.jpg) | |
| Tab: relationship chart; arrows choose a face, Z opens its page, and the wheel zooms | |
| ![Go Zen's relationship chart with face icons and coloured connections](/images/go-zen-relations.jpg) | |

More on each key in [the edit–play loop](/edit-play-loop/).

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
- Campaigns: regions held by factions over turns, orders with generals, a sortie screen with tactics, clashes of armies, and `gozen sim campaign` for the win rate
- Dungeon themes: `theme: cave` fills a dungeon's pictures, and `gozen theme add` copies them into the game
- Visual novels: roll back with the state, skip what was read, save in the middle of a scene, choices read as `chose`, staging with `stand`, and a development panel (F8) for the flags
- Engine words in English and Japanese, chosen by `language` in game.yaml, and the game's own text in two languages
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
