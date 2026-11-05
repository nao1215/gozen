# ADR 0055: First-visit screen guides and header faces

Status: Accepted and implemented (2026-11-04).

## Authors and the problem

A planning game teaches its screens with permanent text: "press a face to
choose", "this uses a phase", "drag a ticket onto a person". The text stays
after the player has learned it, and it describes the controls instead of
letting their shape, place and emphasis show what they do. A player new to a
screen still needs to be told where to look first, once.

A state of the player character ("tired", "fine") written as a word in the
header has the same problem: it reads as a manual, not as the character.

## Decision

1. `screens.<name>.guide` (schema 10) is a first-visit walk-through of a
   full-screen surface: `when` (a pure condition that holds it back) and one to
   eight `steps`. A step is `who` (an actor; the square face and name head the
   line), `text` (interpolated), `focus` and `until`.
2. The line shows in the notification corner (the chat notification's place,
   moved to another corner when a lit place lies under it). The inbox's
   notification waits until the guide is finished.
3. `focus` names entry marks (`entries[].mark`, shared by the rows of an
   `each` or by several entries) or `next`, the flow's next action. While a
   line with focus shows, the screen is veiled except the lit places, which
   are ringed; any click, wheel, key or cancel aimed elsewhere is refused (and
   traced as `guide_blocked`). Save and load wait. The keyboard focus moves to
   the first lit place (a carousel member turns to the middle; left and right
   still turn the carousel).
4. A line with focus goes on when a lit place runs its action (the screen
   starts running an event or becomes another screen). With `until`, it goes on
   when the condition holds instead, so a drag or a carousel can be used any
   number of times first. A line without focus is pressed (anywhere, or OK) to
   go on.
5. Pointing a guide at marks, not at positions or labels, keeps it independent
   of the screen's layout: another author may move or rebuild the entries; only
   the mark has to stay.
6. When the last line goes on, or the screen is left for one not over it, the
   guide is recorded as `guide:<screen>` with the gallery's unlocks
   (`gallery.json`). It never shows again, in this play or a later new game,
   like a profile's `met`. It is not part of a save.
7. Scripted, replayed and automatic plays start as a player who has seen every
   guide, so rule scripts and captures are unchanged. The script command
   `guides on` (and `guides off`) shows them as to a new player; the trace has
   `guide` (screen, step, who, text, focus) and `guide_done`.
8. `view.header_face` (an interpolated picture path) draws a small square face
   crop right after the date in a full-screen view's header, and `view.header_tint`
   (an interpolated `#rrggbb`) a ring around it, so a state reads from the face
   and its color instead of a word.

## Consequences

- A game removes its permanent instructions and keeps the shapes; the guide
  says once, in a character's voice, where to look next.
- `gozen check` refuses a guide on a screen that is not a full-screen surface,
  a line without a speaker or text, a focus that is not a mark of the screen
  (or `next` on a step without a next action), `until` without focus, and a
  mark named `next` or on an entry without bounds.
- A popup, a peek, a running action or a spoken line pauses the guide.
