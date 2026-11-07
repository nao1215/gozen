# ADR 0059: Hair kept in a call's view, a backdrop behind a picture self view, and notices on nearly opaque cards

Status: Accepted and implemented (2026-11-07).

## Authors and the problem

The layout lines of a trace (ADR 0058) showed three faults in a management game
built on gozen, none of them the game's own:

1. A call's view frames a person by the face found in the picture: the face box
   at 22% of the view's height and 48% of it high. It never looked at the hair.
   A picture with long or tall hair had its head cut by the view's top, by up to
   69 UI units on the full call's stage.
2. A self view that is a picture (`call.self: me.png`, with `self_stage`) had no
   way to put anything behind the person: on the stage the torso floated on the
   dark tile of the call, while every participant (an entity) stands on its
   backdrop.
3. Under `ui.material` with glass, a screen guide's line and an inbox
   notification stood on the card role's glass. The text of the screen under them
   showed through, and their own text fell to a contrast of 3.6 to 4.4.

## Decision

1. The call keeps its framing while the hair fits. When the opaque part of the
   picture (the content box the picture cache already holds) would rise above the
   view's top, the face moves down until the hair is 2% of the height inside, by
   at most a tenth of the height (the face's top from 22% to 32%); if that is not
   enough the face shrinks until it fits. A picture whose opaque part touches its
   own top edge is drawn cut there (`source_cut: top` in the trace) and keeps the
   old framing. The rule is the minimal one: no picture that fitted moves.
2. `call.self_backdrop` names a picture drawn behind a self view that is a guest
   actor or a picture, small and on the stage, as an entity's backdrop
   (`view.backdrop`) is drawn behind an entity. Omitted, the view keeps the plain
   tile: the look of every existing game stays, and it is also what an entity with
   no backdrop value shows. `check` refuses it without `self`, for a missing file,
   and when `self` is an entity, whose view takes the entity's own backdrop.
3. The card under a notice (a guide's line, an inbox notification and the bubble
   it opens) is drawn at the card role's opacity raised to 0.94. At 0.94 the
   light card over a black backdrop still keeps the light palette's text and
   accent above 4.5:1. The card keeps its material (glass, acrylic, mica) and its
   edge; only how much shows through changes. Other cards and panels keep what
   `ui.material` sets.

## Consequences

- Existing games change only where a head was cut by a call's view and where a
  notice stood on glass less opaque than 0.94.
- A self view that is a picture can stand in a room like the participants.
- The floor is not authorable: a notice exists to be read over whatever screen
  it lands on, and a game that wants a different look of it recolors the light
  palette (`ui.light`).

## Considered and not chosen

- Fitting the hair by always shrinking the face: it would change every call in
  every game, also those whose hair fits.
- Defaulting a picture self view's backdrop to the call's colors or a blur of the
  stage: no picture is a better default than one the author did not choose, and
  the entity's default (no backdrop value) is the plain tile too.
- A new `ui.material.opacities.notice` role: one more key to get wrong, when no
  game needs a notice it cannot read.
