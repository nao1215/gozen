# ADR 0031: The character book (profiles)

Status: Accepted (2026-10-21). Done. Home: `internal/layer2/gamedata` (the definitions and their checks), `internal/play` (the screen), `internal/layer3/scope` (the query).

## Users and problem

An author keeps who each character is (manner, the names they call the others, looks) in notes beside the game, and checks the game's lines against them by memory. A player of a novel or an RPG likes a page per character to look back at. Neither can be seen in the game.

## Decision

### YAML: `profiles.<id>` in data/

A data file may hold `profiles`, keyed by id, in the order written. A profile shows an actor (`actor`, by default the actor of the same id, with its faces or its puppet), an enemy (`enemy`, its picture) or anyone else (`picture`, a standing picture under `assets/`). It has `name` (by default the actor's or enemy's), `title`, `faces` (the faces the screen switches between), `when` (unlocks it; always unlocked without it), `fields` (a list of `label` and `value`, the labels the game's own), `text` (the introduction) and `relations` (`who`, `calls`, `text`: how the character sees another profile, an actor or `you`). The title, values, text and relations may hold `{expressions}`. Every text is a text of the game for `lang/` (ADR 0027). It needs schema version 4.

Unlocking is per character only. A condition for each field was considered; it costs the author a condition per line for little, and was dropped.

### The screen

"Characters": a band on top with the title and the switch to the previous and next character (◀ name n / N ▶), the list on the left (a face for an unlocked profile, a shadow and ??? for a locked one), the standing picture in the middle with a button per face under it (Z steps through them; a puppet moves), and the profile on the right: name, title, the fields, the introduction and the relations. Left and right, L and R of a pad, the arrows, a swipe and the list change the character.

It opens with `open: profiles`, from the title and the field menu ("Characters" after Gallery and after Load), from the novel bar, and with P while a line is shown. The menus and the bar offer it only when game.yaml asks for it with `ui.profiles: true`; under `--dev` the title and the menu offer it, P opens it, and every profile is unlocked, because the book is first the author's tool for checking the characters.

A profile unlocked once stays unlocked, in the record of the gallery (`gallery.json`, the key `profile:<id>`), checked with the gallery's unlocks.

### Checks and the query

`gozen check` reports the actor, enemy, picture and faces a profile points at and does not find (a face of a puppet is checked against the puppet), the names its `when` and `{expressions}` use (the state's names and `ran`), a relation to no one, and `open: profiles` in a game without profiles. A text without a translation is warned of like the others (GZN0111).

A session reads `profile.<id>.unlocked` (the profile's `when` on the state now) and `profile.<id>.name`. A session has no record of unlocks, so a profile unlocked in an earlier play is not unlocked there.

## Alternatives

1. Read the author's own notes format (one folder of YAML files per character). It ties the engine to one way of keeping notes; a game that has them can write `profiles` from them.
2. A tab of the gallery. The gallery is for pictures and scenes, unlocked by having seen them; a character is not a picture.

## Consequences

- `api/public.txt` does not grow. `api/schema.txt` gains `DB.Profiles`, `Profile`, `ProfileField`, `ProfileRelation` and `UIStyle.Profiles`.
- `make docs-images ONLY=a.jpg` remakes one picture of the ledger.
