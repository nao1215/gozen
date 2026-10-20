# ADR 0025: Replay to roll back, skip and resume

Status: Accepted (2026-10-21). Stages 0, 2 and 2b are done (`schema_version: 4`, `ui.rollback`, rolling back, the choice's `id` and `chose`, the save in the middle of an event, save format version 2). Home: `internal/play` (the history and the replay), `internal/rpg` (the save envelope), `internal/layer2/gamedata` (the YAML words).

## Users and problem

Gozen already shows novel games (`ui.message: novel`, with a backlog, auto and a read-line fast forward), and their authors ask for what a reader of a visual novel expects: stepping back a line with everything it changed, skipping what was read, and a save in the middle of a scene. The authors write YAML, and their checks run headless (`gozen.Session`, `Explore`, `gozen check`, `gozen play trace`). Each new feature must be checkable there. A novel game here is a game whose places show only events, so these features build on the existing events rather than on a separate engine. This is a design choice for novel games, not a limit on the kinds of games Gozen supports: other genres may add their own YAML vocabulary.

## Decision

### One mechanism: the state at the start of an event, the answers, and a replay to a line

The screen already resumes from the start of an event and reads on to a line without the player: a note of F9 (`--from-memo`), F5 (live reload) and the gallery's pages use the state saved when the event started (`evSnap`), the random generator's bytes at that moment (`evRand`), the count of lines (`evLines`) and the answers given (`evAnswers`), and replay them quietly (`live.quiet`: no sound, no read marks, no unlocks). Rolling back, skipping a scene, jumping in the `--dev` panel and loading a save made in the middle of an event all use this same replay. No new contract is added, and the runner's frames are never saved (`session.go` keeps its promise that a save holds only the state).

- A history entry is three integers: the line count, the number of answers and the backlog length. To go back to line k is to parse `evSnap`, restore `evRand`, start the event again and replay to line k with the first answers.
- Rolling back across events keeps a ring of the earlier events (`{evSnap, evRand, name, lines, answers}`), 8 by default. `ui.rollback` changes the size (0 to 64; 0 turns rolling back off). It is the first word of `schema_version: 4`.
- Boundaries: a battle, a shop, a save, an `open` screen, `move` and `clear` cannot be rolled back across; the history before them is dropped.
- What rolls back: flags, vars, items, the choices taken (`chose`), everything in the state. What does not: the read lines, the gallery's unlocks and the settings, which belong to the player, not to the play.
- The player steps back with a button on the novel bar, the wheel up when there is history (the backlog otherwise) and a key. The trace gets a `rollback` line and the script a `back` button.

### Choices are part of the state

`choice` gains an `id` beside `prompt` (on the command, not a new shape), and the state remembers the option taken for each id (`State.Chose`, the expression `chose.<id>`). It rolls back with the rest of the state.

### Save format version 2

The save envelope gains a `cursor` (`{event, line, answers}`, the same shape as a note's `resume`) and the random bytes (`rand`), and the state gains `chose`. `gozen.Session.Restore` and the screen start the event of the cursor and replay to its line, so a save made in the middle of a scene can be checked headless. A reader takes versions 1 and 2; an older engine stops with its existing message that it does not support save format version 2. Downstream games update on the same day, as with [ADR 0024](0024-command-tree-and-diagnostic-codes.md).

### Schema version 4

The words of novel games ([ADR 0025](0025-replay-to-roll-back-skip-and-resume.md) to [ADR 0029](0029-dev-panel.md)) need `schema_version: 4`. Using one in an earlier version is the error `<word> needs schema_version: 4` from `gozen check` (the same wiring as `needsVersion` for versions 2 and 3), so an old game keeps loading unchanged and an old engine reports a version, not an unknown field.

### As built

- The history of a line is the number of answers given before it (`evHist`); the backlog when the event began is kept with it and put back before the replay, which shows the lines again. An earlier event keeps its commands themselves, so an arrival (made on the spot, with no name) is rolled back the same way.
- A boundary is a battle, a shop, a save command, an `open` screen, `clear`, a game over, or a `move` effect. After one, the running event is neither rolled back nor saved with a cursor, and the earlier events are dropped.
- The keys: Q (S and Backspace are taken), the "Back" button of the novel band, and the wheel up when there is a line to go back to (the backlog otherwise). The band also gained "Save" and "Load", since a save in the middle of a scene needs a way to save there. The trace writes `rollback` after the replayed lines, and `gozen play trace --check` reports a rollback that did not go back (GZN1405).
- The cursor of an arrival is `{arrival: true}` with the state before the arrival was made: the loader arrives again, so the arrival's events, marked done when they were made, run again.
- `Session.Save` writes a cursor in the middle of an event that began with its own commands or with `- call: NAME` (from `Do` or `Start`), and `Restore` reads on to the line without reporting the requests on the way; its first `Next` is the line. A restored `call` begins the event's commands, so a second save has the same state at the line but may count the run before the replay instead of during it.

## Alternatives

1. A snapshot of the state for every line. It is simple, but memory grows with the length of a scene and the screen's pictures and music would still need replaying. Not chosen.
2. Saving the runner's frames (the program counter of each nested command list). It would make a save in the middle of a scene exact without a replay, but it ties the save format to the shape of the YAML, which changes as the author edits. A replay to a line count survives small edits, and when it cannot reach the line it stops at the last one (the rule F5 already follows).
3. A Ren'Py-like rollback that also undoes reads and unlocks. Players expect the read marks to stay; not chosen.

## Consequences

- Determinism becomes a contract: the same state, the same random bytes and the same answers reach the same line. A headless contract test fixes it (the `Save()` of a scene played to line k equals the one replayed from its start).
- `api/public.txt` does not grow. `Restore`'s godoc gains the cursor. `api/schema.txt` grows by the new YAML fields.

## Plan

| Stage | Content |
| --- | --- |
| 0 | This ADR, `schema_version: 4` and `ui.rollback` (done) |
| 2 | Rolling back on the screen, the trace and the script; the headless contract test (done) |
| 2b | `chose`, the cursor and the random bytes in the save; save format version 2; `Restore` resumes the event (done) |
