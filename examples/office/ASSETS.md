# Office sample assets

The office sample (a module of its own, so its pictures stay out of the gozen module) is a small app team's sprint on the workspace screens: a
morning standup, the team, handing out work by dragging, tickets, a carousel,
a message, a video call and the sprint plan. `docs/images/workboard-*` are
captured from it (`scripts/capture-office.txt`). The YAML (`game/game.yaml`,
`game/data/`) is MIT like the rest of the repository. The files under
`game/assets/` are listed here.

| Files | What | License |
|:--|:--|:--|
| `martin/`, `elena/`, `daniel/`, `iris/`, `nora/`, `mia/` | The standing picture of each team member (`normal.png`) | CC0 1.0 |
| `icons/` | Face crops of the same pictures, for the carousel | CC0 1.0 |
| `bg/office.jpg`, `bg/meeting.jpg`, `bg/pantry.jpg` | The office floor, the meeting room and the pantry, darkened for the glass panels | CC0 1.0 |
| `phone/photo1.jpg` to `phone/photo10.jpg` | Mia's photos on the phone's feed (standing pictures on a plain backing) | CC0 1.0 |
| `clip1/` to `clip3/` | The standing pictures and puppets of the phone's video clips | CC0 1.0 |
| `clips/battle/`, `clips/cutin/`, `clips/call/` | The footage the phone's clips play: 640 px JPEG frames cut with ffmpeg from `docs/images/go-zen-battle.mp4`, `go-zen-cutin.mp4` and `workboard-call.mp4` (every third frame), captures of the Go Zen and office samples, which are made from their CC0 pictures | CC0 1.0 |
| `faces.json` | The face box of each standing picture, written by `gozen faces` | MIT |

No copyright is claimed in the pictures. To the extent any rights exist, they
are dedicated to the public domain under CC0 1.0 Universal
(https://creativecommons.org/publicdomain/zero/1.0/; the text is in
`game/assets/LICENSE`): use them for any purpose, with or without credit. They
were made for another of the author's projects and not used there; the standing
pictures were cut to the figure and the backgrounds made 1920x1080.
