# ADR 0026: Read lines, their record, and skipping

Status: Accepted (2026-10-21). Not implemented yet. Home: `internal/event` (the key of a line), `internal/play` (the record, skip mode, scene skip).

## Users and problem

A reader of a novel game skips what was read and stops at what was not. Gozen kept the read lines in `read.json` as a bare list of hashes of the line as shown (the speaker's name and the text after `{expressions}` were filled in). So a line with `{var.name}` was a new line for every value, a translated game would have its own read lines for each language, and the file had no version and no game id, unlike `gallery.json`. Fast forward (Ctrl) passed only read lines, but there was no way to keep skipping without holding a key, and no way to pass a whole scene that was read.

## Decision

### The key of a line is its source

The runner gives every line (`event.Say.Key`) a 64-bit FNV-1a hash of its source: the `who` and the `text` as written in the YAML, before `{expressions}` are filled in and before any translation. The screen keys read lines with it. A line whose text holds a variable is read once whatever the value, and the read lines are shared by every language of a game ([ADR 0027](0027-text-languages.md)). The key of a line the runner did not make (a choice's prompt shown alone) is the old key, the hash of the name and text as shown.

The old key stays readable for one version: a line counts as read when either key is in the record, and only the new key is written.

A choice remembers its options the same way: an option taken once is marked when the choice is asked again. Its key is the source of the prompt and the label (`"choice"`, prompt and label joined by a zero byte), in the same record, so it is the player's and does not roll back ([ADR 0025](0025-replay-to-roll-back-skip-and-resume.md)).

### The record has an envelope

`read.json` becomes `{"format_version": 1, "game_id": "...", "keys": [...]}`, the same shape as `gallery.json`. A bare list is read as the old record and the next write keeps it as a backup (`read.json.before-<sha256>.bak`). Another game's id, an unknown version or unknown fields are not read (logged, and the record starts empty without overwriting the file). Test runs (script, autoplay, `--start` and the like) still neither read nor write it.

### Skip mode, scene skip and the read rate

- Skip mode: a "Skip" button on the novel bar and a key (N) turn it on, and it stays on until a line that was not read (unless the setting "Text to skip: all" is on), a choice, a battle, the end of the events, or a click or the key again. While it is on, lines and waits pass as with Ctrl. Ctrl held keeps working as before.
- Scene skip: when every line of the running event was read (the lines are collected from the event's commands, into the branches of `if`, `choice` and `battle` and the events it calls, whatever the conditions), the novel bar shows "Skip scene" (key Page Down). It replays at once, quietly, to the next choice, the next line not read, a battle or the end of the event. It uses the replay of [ADR 0025](0025-replay-to-roll-back-skip-and-resume.md).
- The read rate of an event is the share of its lines that were read. The `--dev` panel lists it per event ([ADR 0029](0029-dev-panel.md)); `gozen book` does not (the author decided not to add `--read`).

### What the trace and the script see

A `say` line of the trace gains `"read": true|false` (the trace stays version 2; a replay of an older trace ignores the field when the recorded line has none). Skip mode and scene skip write a `skip` line (`{"on": true|false, "why": ...}`, `{"scene": true, "lines": ...}`). The script presses them with `press skip` and `press scene_skip`, and `gozen play trace --check` fails when skip mode passed a line that was not read.

The headless `Session` does not see read lines: they belong to the player's screen, not to the rules. The checks read the trace.

## Alternatives

1. A path key (`events.dinner.do[3]`). It survives a fixed typo but not a moved line, and it cannot be computed for a line made by a branch of a call. Not chosen; it can be added beside the source key later.
2. A key per translation. Players would lose their read lines when they switch languages. Not chosen.
3. A line id written by the author. It is exact but asks every line for an id; the engine has no line ids today. Not chosen.

## Consequences

- Two lines with the same speaker and the same source text are one read line (the limit of keying by the source, as in gettext). Write them differently if they must differ.
- Players of a game made with v0.1 keep their read lines (the old key is still read for one version).
- `api/public.txt` and `api/schema.txt` do not change.
