# ADR 0049: Games without menus

Status: Accepted (2026-10-27). Home: `internal/layer2/gamedata` (the key and its check), `internal/layer3/field` (a place's entries), `internal/play` (the opening and the menu inputs).

## Context

Every game opens on the title screen (new game, continue, gallery, settings) and has the game menu (status, items, save, load, settings, back to the title), opened with the menu key, the right click, a two-finger tap, a long press in the stream lobby or the menu entry of a place. A game that is one screen kept in the autosave, such as the stream lobby (ADR 0048), has nothing for the player to choose there: the title only stands between opening the game and the lobby, and the menu offers save slots the game does not need.

## Decision

`game.yaml` `menus: none` (schema version 9) leaves out the title screen and the game menu together, so an author writes in one word that the game has neither and keeps the play in the autosave alone. The default (the key left out) keeps both.

- The game opens where the title would be: on the newest readable save, the autosave included, chosen by the same rule as the title's load list (the slot written last; the first readable slot when the store keeps no times), or, with no readable save, a new game at `start` with its `start.event`. A save that cannot be read is skipped; writing over it keeps its backup as every save does.
- Wherever the play goes back to the title (a game over without `game_over.event`, an ending, or `open: title`), it opens the same way instead.
- No input opens the game menu or the system menu of a place with `no_menu`, and a place's entries leave out the menu entry. The lists of a map and a painted scene still open; they list the place, not the menu.
- `gozen check` reports `open: save`, `open: load` and `open: title` with `menus: none` as errors (GZN1104): there is no slot to choose and no title. Other screens (`open: config`, `open: status` and the author's `screens`) still open, so a game reaches its settings, such as the language and the volume, through an entry of a place that runs `open: config`.

## Consequences

A stream game opens straight on the lobby and saves only by its autosaves (ADR 0048). The settings are no longer reachable by default in such a game; the author places them as an entry. The replays and test runs are unchanged: a trial play uses a memory store without saves, so it opens on a new game. Title screen keys (`title_background`, `title_music` and the rest) mean nothing with `menus: none`; the title music is not played.
