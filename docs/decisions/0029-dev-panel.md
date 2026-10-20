# ADR 0029: The development panel (F8)

Status: Accepted (2026-10-21). Stage 0 (the index of where names are written and read, `DB.StateUses`) is done; the panel is the stage "Panel" under "Plan". This continues [ADR 0017](0017-dev-balance-panel.md) (F7, the balance display). Home: `internal/layer2/gamedata` (the index), `internal/play` (the panel).

## Users and problem

The author of a novel game, and the agent that writes YAML for them, test a branch by setting flags and jumping to a scene. Under `--dev` the screen can reload (F5), take a note (F9) and show the balance (F7), but changing a flag means a script or a restart with `--flags`, and there is no view of which events write and read which flags.

## Decision

### F8 opens the panel; the frame meter moves into it

F8 under `--dev` already toggles the frame meter (`internal/play/meter.go`: ticks and frames a second, the longest and average Update and Draw). F8 becomes the panel, and the meter becomes one of its tabs, "Frames": choosing it hides the panel's body and leaves the meter in the corner, as it is drawn now, until F8 closes the panel. The browser's `?fps` meter does not change. Until the panel lands, F8 keeps toggling the meter.

Another key was considered (F6 is free), but two development keys for overlays that cannot be shown together would only add to the list of keys. One key opens what the development screen shows.

### Five tabs

1. State: the names declared in `state.names`, then the flags, vars, items, gold and choices (`chose`) set now. Enter toggles, steps or types a value. Each row shows where the name is written and read (the index below). An edit runs `- set: {...}` through the event runner, the same way as `Session.Do`, and writes `dev_set` to the trace.
2. Flags: a table with events as rows (in data order) and the declared flags as columns; a cell shows S (set), R (read) and the current value. The current event is highlighted. The arrows of a graph are left to `gozen book`.
3. Scenes: every event with its trigger, place, once and done, its `ran` count and its read rate ([ADR 0026](0026-read-lines-and-skipping.md)), and the gallery's scenes. Enter starts one from the current state (the same as `--event`, and as `Session.Do("- call: NAME")`); a number picks the line. The trace gets `jump {event, line}`.
4. Command: one line of YAML, checked like a script (`CheckScript`) and run.
5. Frames: the frame meter (above).

Nothing in the panel changes the state except through `set` and `call`, so every edit can be done headless with `Do`, and the script gains `jump EVENT [LINE]` and `do <yaml>` for the same work.

### One index of names, shared

`DB.StateUses` maps each name of the state (flag.x, var.y, item.z, ran.e, gold, ...) to the paths that write it and the paths that read it. The panel's rows, the flag table, `gozen book`'s flow of events and the checks that warn of a flag set but never read, or read but never set, use this one index, so they cannot disagree. `DB.Uses(verb)`, which tells whether a command is used anywhere, keeps its name and meaning.

## Alternatives

1. Edit the state directly (`State.SetPath`). It would bypass the checks of `set` and leave the trace without a record. Not chosen.
2. A separate window or a web page for the panel. It needs a second process or a server, and the browser build has neither. Not chosen.

## Consequences

- `api/public.txt` does not grow. `api/schema.txt` gains `DB.StateUses` (with its `NameUse` reached through it).
- The panel exists only under `--dev`.

## Plan

| Stage | Content |
| --- | --- |
| 0 | The index `DB.StateUses` (done) |
| 6 | The panel, the script's `jump` and `do`, `gozen book`'s flow, the warnings of unused and unset flags |
