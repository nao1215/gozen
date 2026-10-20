# ADR 0028: The words of staging and voice

Status: Accepted (2026-10-21). Staging (stage 3) is done; voice is the stage "Voice" under "Plan". Home: `internal/layer2/gamedata` (the words and the checks), `internal/event` (the effects), `internal/play` and `internal/layer5/sound` (the screen and the sound).

## Users and problem

A novel game is staged: who stands where, who leaves, a shake on one portrait, a fade to black, music that fades in and out, a wait the reader cannot skip, and voiced lines. Gozen places the portraits of a novel game by itself from the speakers of `say`, `effect` has three words (`shake`, `flash`, `fade`), `music` has no fade, and there is no voice.

## Decision

### One new verb, wider values for the others

- `stand` is the only new verb: `stand: {who, face, at, enter}` brings a portrait in (`at` is left, center, right or a fraction of the width), `stand: {who, leave}` takes it out and `stand: none` clears the stage. An event that uses `stand` once places its portraits only as written; one that does not keeps the automatic placement. `gozen check` warns when `stand` and `say.side` are mixed in one event.
- `effect` keeps its words and also takes a map: `{name, who, seconds, color, hold}` (a shake of one portrait, a fade with a color). A word that names a reaction of a moving portrait (`nod`, `hop`, ...) plays it on the last speaker.
- `music` gains `fade` (seconds) and `stop`. `wait` takes `{seconds, skip}` (`skip: false` holds even in skip mode). `sfx` takes `{name, wait}` (wait for the sound to end).
- Every map form keeps the string form (the same `UnmarshalYAML` as `image` and `say`).

### No inline markup in lines

Lines get no inline tags (`{w=0.5}` and the like): `{}` is already the expression of a line ([ADR 0021](0021-query-and-commands.md)), and a command between lines does the same work and is checked like any command.

### Voice

- `say.voice` names a sound: for an actor with `voices` (a folder under `assets/`), the name of a file in it; for anyone else, a path under `assets/`. Ogg Vorbis or WAV.
- The screen stops the previous voice when a line starts and plays the new one; skip mode, scene skip and the quiet replay play none (a roll back plays only the line it lands on). `ui.voice.cut` (stop at the next line) and `ui.voice.auto_wait` (auto waits for the voice) shape it, and the settings gain a voice volume and a volume per actor (`config.json`, added fields).
- `internal/layer5/sound` gains `PlayVoice` and `StopVoice`: decoded on demand with a small cache, for the browser's memory.
- The examples do not ship voices (the author's decision); the tests use short generated sounds.

### Effects are what the rules see

`stand` and `voice` reach the rules only as `Effect{Kind: "stand"}` and `Effect{Kind: "voice"}`: `Explore` lists them with the line they came at, and a headless test can ask who stands and which voice plays in a scene. The kinds are words, not identifiers, so `api/public.txt` does not grow; the godoc of `Effect.Kind` lists them.

## Checks

`gozen check` checks that `stand.who` is an actor (and its face exists), the words of `at` and `effect`, `music.fade >= 0`, that every `say.voice` file and `voices` folder exists with a known extension, and warns of the lines of a voiced actor that have no voice (`--format json` lists them with their paths). `gozen play trace --check` checks that a voice in the trace exists and that none played in skip mode.

### As built (staging)

- `stand` is drawn in the novel window only (the `box` window shows the speaker's portrait by each line, as before). The first `stand` of an event lets the portraits placed by the speakers leave, and the event's portraits are cleared when its events end. A stand that moves a portrait without `face` keeps its expression.
- `stand.at` is a word or a fraction; a name that is not an actor needs `face` the first time it stands (not checked, since a later stand may only move it).
- `effect` words: `shake`, `flash` and `fade` stay the screen's, and their string forms look as before. A reaction is one of the puppet kinds (`hop`, `shrink`, `nod`, `tilt`, `freeze`) or a reaction a moving portrait's file names; `shake` of one portrait is `{name: shake, who}`. A reaction on a portrait that does not move does nothing.
- A fade with a map goes to its color over `seconds`, stays `hold`, and comes back over `seconds`; it does not stop the event (a `wait` does).
- `music` `fade` crossfades the song asked for with the one playing, and `stop` fades out; both are words of the music command only (`gozen check` refuses them in a place's music).
- `sfx` `wait: true` is the request `wait` for the sound's length; `wait` keeps `skip: false` in the request, and skip mode holds there.
- None of these is a boundary of rolling back ([ADR 0025](0025-replay-to-roll-back-skip-and-resume.md)): the replay plays them again quietly.
- Not done: the warning of `stand` and `say.side` in one event.

## Alternatives

1. Several verbs (`show`, `hide`, `move`). More to learn for the same work. Not chosen.
2. Voice as a field of `Request`. The effect already carries it. Not chosen.

## Plan

| Stage | Content |
| --- | --- |
| 3 | `stand`, the map forms of `effect`, `music`, `wait` and `sfx` (done) |
| 4 | `say.voice`, `actors.<id>.voices`, `ui.voice`, the volumes, the checks and the trace |
