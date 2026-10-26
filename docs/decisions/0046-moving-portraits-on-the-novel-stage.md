# ADR 0046: Moving portraits on the novel stage

Status: Accepted (2026-10-26). Home: `internal/layer2/gamedata` (`ui.stage_puppets` and its check), `internal/play` (the stage), `internal/layer5/puppet` (the reduced composition).

## Users and problem

A novel game stands large portraits with `stand` and frames them by their heads with `ui.stage_framing` ([ADR 0028](0028-staging-and-voice-words.md)). An actor can already have a moving portrait ([ADR 0020](0020-moving-portraits.md)), but only the `box` window's speaker portrait moved: the novel stage drew the face pictures, still, and a new face replaced the picture at once. Authors of novel games want the standing figures to breathe, blink, sway, talk while they speak and change faces softly, with high-resolution pictures (4096 by 6144) filling the screen.

## Decision

- `ui.stage_puppets: true` in game.yaml (schema version 8) lets the novel stage draw an actor's puppet in place of the face picture. It is off by default.
- A figure is drawn by its puppet when the figure is an actor with `puppet` and the puppet has a face of the face picture's name (the `<face>` of `assets/<faces>/<face>.png`, as for the `box` window). Otherwise the stage draws the picture as before, so a face the puppet lacks, a figure that is no actor, or a puppet that fails to load still shows.
- The puppet's base picture is placed by the same rules as the picture it replaces: `stand.at` or the automatic placement, `stage_framing` keyed by the face picture's path, `stage_scale`, `stage_drop`, mirroring, the fit to the screen height, the dimming of those not speaking, entering, leaving and `{name: shake, who}`. The base size takes the place of the picture's size, so the base should have the canvas of the face pictures.
- Every figure drawn by a puppet steps once a tick: breathing, sway, springs, `soft_sway`, hair and blinking. The mouth moves only for the figure of the line's speaker while the text is typing.
- A face change of a figure standing on the stage, by a line's `face` or by `stand.face`, fades over the puppet's `fade`. A figure that comes on, or comes back after half a second away, starts on its face. Reactions (`effect: nod` and the other puppet kinds) play on the puppet drawn.
- A stage puppet composes its picture at a power-of-two fraction of the base size that is still no smaller than the drawn size (the base and overlays box-filtered, aligned to the base). A 4096 by 6144 portrait drawn 1.2 screens tall on a 720-line screen is composed at 1024 by 1536.
- A stage puppet does not load the face pictures it replaces.

## Why opt-in

Without the setting the novel stage keeps drawing the face pictures, and the puppet is used where it was (the `box` window, the character book, battles). An existing novel game may name a puppet for its character book whose base does not share the canvas of its face pictures, or whose faces differ from the pictures; drawing the puppet there would move or change its portraits. Opting in keeps every existing game as it looks. The key is game-wide rather than per actor, because a stage of moving and still figures of one cast is rarely wanted and the per-actor choice is already made by writing `puppet`.

## Alternatives

1. Always draw a puppet when an actor has one. Simplest, but it changes the look of existing novel games (above). Not chosen.
2. A setting per `stand`. More to write for the same result, and lines that bring speakers on by themselves would need another word. Not chosen.
3. Composing at the base size, as the `box` window does. A 4096 by 6144 base and its canvas take 200 MB of GPU memory, and a face, blink or mouth change redraws 25 million pixels. Not chosen for the stage; the `box` window keeps its composition, so its pixels do not change.

## Consequences

The stage costs one mesh draw per moving figure and a recomposition of the reduced canvas when a face, blink or mouth frame changes. Measured with the graphics benchmark `BenchmarkGraphicsLargePuppet` (a 4096 by 6144 figure, 1.2 screens tall on 1280 by 720, Radeon 8060S, waiting for the GPU every frame, two runs): about 2 ms a frame idle and 4 ms recomposing every frame, against 2.5 ms and 6.4 ms composed at the base size and 3 to 10 ms for the still picture; the first frame takes 40 to 75 ms against 210 to 250 ms. Loading a puppet (decoding its pictures) happens on the game thread the first time its actor stands (about 0.9 s for a 4096 by 6144 base and one whole face picture), as it does for the `box` window.
