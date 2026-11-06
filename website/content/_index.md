---
title: Gozen
---

Gozen is a no-code toolkit for making 2D RPGs, visual novels and strategy games with YAML and assets. Start with a playable template, edit its dialogue and rules, then check and play it. No Go code or compiler is required for this workflow. YAML and terminal commands are the interface; there is no visual editor. Go and Ebitengine power the engine behind it.

The name: Ebitengine is named after ebiten (海老天), shrimp tempura. In Japan, an ebiten gozen (御膳) is a set meal on one tray: shrimp tempura with soba, small side dishes, and a sweet such as dango. Gozen is that tray for Ebitengine: the engine served with everything a game needs. The name also starts with Go.

| RPG: [Go Zen](https://github.com/nao1215/gozen/tree/main/examples/go-zen) | Novel: [After the Pass](https://github.com/nao1215/gozen/tree/main/examples/novel) |
|:--|:--|
| ![Go Zen, the Gozen sample game: the town map, a dungeon, a battle with a cut-in, and a talk](/images/go-zen.mp4) | ![After the Pass, the novel sample: a line, a choice, Back over the choice, and Skip to the first line not read](/images/novel.mp4) |
| Campaign: [Five Banners](https://github.com/nao1215/gozen/tree/main/examples/campaign) ([play in browser](/play/campaign/)) | Monster collection: [Go Zen](https://github.com/nao1215/gozen/tree/main/examples/go-zen) ([play in browser](/play/go-zen/)) |
| ![Five Banners, the campaign sample: the war map, the sortie screen, a clash of two armies, and the region in the Crown's colour](/images/campaign.mp4) | ![Breeding monsters in Go Zen: Yorn, the two parents, a silhouette reveal and the newborn Succubus](/images/go-zen-breeding.mp4) |
| Management: [Harbor app team](https://github.com/nao1215/gozen/tree/main/examples/office) | |
| ![the office sample: the standup, the team, handing out work, a message, a video call with a shared screen, the calendar and the plan](/images/office.mp4) | |

## Create your first game

[Install Gozen](/install/), then create a playable template:

```shell
gozen new my-game --template rpg
gozen check my-game
```

<!-- verify: skip opens a window; generated templates are played by the graphics E2E -->
```shell
gozen run my-game
```

Choose `novel` or `campaign` for a different starting point. Follow [Getting started](/getting-started/) to edit your game, or [play the showcase games in your browser](/play/).

On Linux the window needs a graphics environment; see [Install](/install/).

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
| ![Maren talks, a moving portrait that blinks and changes its face](/images/go-zen-face.mp4) | ![Maren walks the plaza and Ox says a line in a balloon](/images/go-zen-plaza.mp4) |
| Town map | First-person dungeon |
| ![the party's face walks the road to the well](/images/go-zen-map.mp4) | ![steps and turns in a brick corridor](/images/go-zen-dungeon.mp4) |
| Battle | Cut-in |
| ![four against the Brass Warden, with damage numbers](/images/go-zen-battle.mp4) | ![Ox's Shield Rush cut-in](/images/go-zen-cutin.mp4) |
| Shop | Skill tree |
| ![the shop's goods](/images/go-zen-shop.jpg) | ![Maren's skill tree with Ink Burst chosen](/images/go-zen-tree.jpg) |
| Title | |
| ![the title screen](/images/go-zen-title.jpg) | |

### Novel games

`ui.message: novel` makes a visual novel. All from [After the Pass](https://github.com/nao1215/gozen/tree/main/examples/novel), the novel sample, at 1920x1080 ([Novel games](/writing-a-game/#novel-games)).

| Roll back (Q, wheel) | Skip to the line not read (N) |
|:--|:--|
| ![Back goes over two lines and a choice, with the state](/images/novel-back.mp4) | ![the option taken again, then Skip stops at the first line not read](/images/novel-skip.mp4) |
| Two languages | Staging (`stand`, `effect`) |
| ![the settings switch to Japanese, the line on the screen turns Japanese at once, and so does the next](/images/novel-language.mp4) | ![Brenna slides in and her portrait shakes](/images/novel-stage.mp4) |
| Event picture with its line | |
| ![Brenna's ending: a picture of her cheering at the bar, with the line under it](/images/novel-picture.jpg) | |

#### Phone apps

`phone` shows a phone over the scene, upright or on its side, in a story or any other game ([A phone on the screen](/writing-a-game/#a-phone-on-the-screen)). All from the office sample.

| Chat: a picture, times and a clip | Feed: scrolled, a clip post, likes |
|:--|:--|
| ![a team chat on a phone: messages arrive one by one with times and a read mark, a picture opens over the screen, then a clip sent in the chat turns the phone on its side and plays a recorded call](/images/phone-chat.mp4) | ![a photo feed scrolled down a post at a time, a battle clip looping in its post and Mia's dance clip after it](/images/phone-feed.mp4) |
| Video on its side: footage | Chat: a dance practice clip (a puppet) |
| ![a video app: the phone turns on its side, nearly as wide as the screen, and plays footage of a battle from the Go Zen sample](/images/phone-video.mp4) | ![Mia says in the chat that she is practicing the launch party dance and sends the clip; it plays on the phone on its side, her puppet breathing, swaying and jiggling](/images/phone-puppet.mp4) |
| Page: a shop site with a picture grid | Page: a bank transfer to confirm |
| ![a browser page of a clothing shop: a grid of pictures, two list items with prices and a button tapped](/images/phone-shop.mp4) | ![a bank app: the balance, a transfer to confirm and recent items; Confirm is tapped and the story goes on from it](/images/phone-bank.mp4) |
| Lock screen: notices arriving | Lock screen: an incoming call answered |
| ![the lock screen at 7:12: a bank notice, a chat preview and a calendar reminder arrive one a press](/images/phone-lock.mp4) | ![a night on-call: two monitoring notices, an incoming call from the on-call app declined, ringing again and answered](/images/phone-call.mp4) |
| Page: a sign-in approval | Feed: a timeline of text posts |
| ![a sign-in check asks to approve a sign-in to the production console; Approve is tapped and the story branches on it](/images/phone-sso.mp4) | ![a timeline of text posts with avatars and times about the booking app being down, with likes, replies and reposts, scrolled](/images/phone-timeline.mp4) |
| Banner: a push notification over a scene | Page: a supporter site with locked posts |
| ![a push banner slides down over the novel window while the scene goes on, and a tap on it opens its event](/images/phone-banner.mp4) | ![an illustrator's supporter page: tiers with prices, a Support button and locked posts that open when the page is shown again](/images/phone-support.mp4) |
| Page: a contact to add with a QR code |  |
| ![an Add contact page with a QR code of the gozen repository; the keys ring and press Add, and Mia's first private message arrives](/images/phone-contact.mp4) |  |

### Management workspaces

Workspace, call, calendar and timeline screens with a day `flow` (schema 10) make a management game. From the office sample ([examples/office](https://github.com/nao1215/gozen/tree/main/examples/office)); the YAML is in [workspace recipes](/workspaces/).

| Standup: busts, meters, dots, signs, burnup | People: press for a peek |
|:--|:--|
| ![six team cards with busts, progress meters, status dots and a burnup chart](/images/workboard-cards.jpg) | ![the team with Daniel's peek open](/images/workboard-peek.jpg) |
| Hand out work by dragging (or with keys) | Tickets, dependencies and a carousel |
| ![Acceptance tests is refused by Daniel, then Payments is handed to him](/images/workboard-drag.mp4) | ![tickets joined by a dependency line, and the help carousel turning to an unavailable choice](/images/workboard-carousel.mp4) |
| A message asks for a decision | Video call |
| ![Elena's message is opened and a choice appears](/images/workboard-message.mp4) | ![the team call: the speaker on the stage, the others in the side column](/images/workboard-call.mp4) |
| Screen sharing | First-visit guide |
| ![Elena shares the booking form in the call](/images/workboard-share.jpg) | ![Martin's guide line over the veiled standup](/images/workboard-guide.jpg) |
| Calendar with scheduled events | Plan timeline |
| ![the sprint calendar with the review and the demo](/images/workboard-calendar.jpg) | ![the sprint plan with five activities](/images/workboard-timeline.jpg) |
| Character grid | Glass panels |
| ![the character book in a grid](/images/workboard-profiles.jpg) | ![two overlapping glass panels over the office](/images/workboard-materials.jpg) |

### Campaigns

`regions`, `factions` and `campaign` (schema version 5) make a war of regions over turns, with generals, orders and clashes of armies. From [Five Banners](https://github.com/nao1215/gozen/tree/main/examples/campaign), the campaign sample.

| The war map | A clash of armies |
|:--|:--|
| ![the picture map of the land with the regions in the owners' colours, the band with the week, the window of a region and the generals](/images/campaign-map.jpg) | ![two armies of soldiers lined up on the terrain, with the leaders' cards in a row below](/images/campaign-clash.jpg) |

### Monster collection

Monsters ask to join after a battle, wait at the ranch, and two raised monsters, one male and one female, are bred into a new one at Yorn's altar, in the manner of Dragon Quest Monsters (schema version 6). The monster book opens a page as the party meets each monster and shows how to breed it, two generations up. The recipe is [Collect, raise and breed monsters](/cookbook/#collect-raise-and-breed-monsters).

| A monster asks to join | Breeding at Yorn's altar |
|:--|:--|
| ![a Drip Slime rises after the battle and asks to join, with Yes and No](/images/go-zen-recruit.jpg) | ![Yorn talks in the center, the parents turn into light, then the Succubus is born](/images/go-zen-breeding.mp4) |
| The monster book | The family tree, two generations up |
| ![the Succubus page of the monster book](/images/go-zen-bestiary.jpg) | ![the Succubus family tree with its parents and grandparents](/images/go-zen-lineage.jpg) |

### Battles

Enemies can change form at an HP threshold, combine with allies, change pictures as HP falls, and move with a puppet portrait. Go Zen shows the Tollkeeper's second form, Drip Slimes gathering into King Slime, and a moving Succubus face with a soft sway region (schema version 7).

| Transform | Merge |
|:--|:--|
| ![The Tollkeeper changes into its second form](/images/go-zen-transform.mp4) | ![Drip Slimes gather into King Slime](/images/go-zen-merge.mp4) |
| Pictures by HP | Motion and sway |
| ![A damaged monster shows its low HP picture](/images/go-zen-hp-pictures.jpg) | ![The Succubus blinks, a soft region sways, and she reacts to a hit](/images/go-zen-motion.mp4) |

See [the battle recipes](/cookbook/#transform-an-enemy).

### Multi-level maps

[![Walking up to the second and third floors across the town](/images/space-stairs.mp4)](https://github.com/nao1215/gozen/raw/main/docs/media/stacked-space-short.mp4)

## Playtesting

Start the game with `--dev` and these keys work while you play.

| F5: edit the YAML, the line changes in place | F9: a note with the screen and the state |
|:--|:--|
| ![Hedda's line is edited in the YAML and F5 shows the new text at the same place](/images/playtest-reload.mp4) | ![F9 opens a note, a comment is typed and saved](/images/playtest-memo.mp4) |
| F7: win rates of the battles here | F8 State: flip a flag, step a number |
| ![win rates by troop](/images/go-zen-balance.jpg) | ![flag.thanked_hedda and chose.toast in the development panel](/images/novel-panel.jpg) |
| F8 Flags: what each event sets and reads | F8 Scenes: jump to an event |
| ![the flags each event sets and reads](/images/novel-flags.jpg) | ![the Scenes tab jumps to the late night](/images/playtest-jump.mp4) |
| F8 Command: run a line of YAML | F8 Frames: ticks, frames, the slowest Update and Draw |
| ![a set typed into the Command tab, then the State tab shows the flag](/images/playtest-command.mp4) | ![the frame meter over the tavern](/images/playtest-frames.jpg) |
| F6: the character book | Tab: the relationship chart (Z focuses a face) |
| ![Wren's page in the character book: her picture, fields, introduction and how she sees the others](/images/novel-characters.jpg) | ![Go Zen's relationship chart with face icons and coloured connections](/images/go-zen-relations.jpg) |

More on each key in [the edit–play loop](/edit-play-loop/).

## Where to go

- [Install](/install/): the gozen command and what each OS needs for the window.
- [Getting started](/getting-started/): run the samples, then start your own game.
- [Writing a game](/writing-a-game/): how the YAML is laid out, with expressions and events.
- [Schema](/schema/): every YAML field, its type, default and references.
- [The edit–play loop](/edit-play-loop/): `--dev`, F5, F7, F9, autoplay, traces and notes.
- [Commands](/commands/): every `gozen` subcommand and its options.
- [Using AI agents](/ai-agents/): let an agent write the game and keep `gozen check` green.

## Features

- YAML definitions with expressions, conditional events and versioned saves
- Turn-based battles with a party, auto battle and per-enemy tactics
- Skill trees, shops, a field menu and a picture gallery
- First-person grid dungeons, fixed or generated floors
- Management games: workspace cards, drag and drop, tickets, messages, video calls with screen sharing, calendars, timelines and first-visit guides ([recipes](/workspaces/))
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

The [install page](/install/) lists the graphics libraries for each OS.
