# ADR 0057: A phone on the screen

Status: Accepted and implemented (2026-11-06).

## Authors and the problem

Stories set today often turn on a phone: a message that arrives during a line,
a feed the character scrolls through, a clip a friend sends. An author could
only show a picture of a phone (`image`), which does not move, does not answer
the player's hand and cannot play anything.

## Decision

1. The event command `phone` shows a phone over the scene and waits until the
   player puts it down. It changes no state and is skipped by Skip and by a
   replay, so it is presentation only, like `call_view`.
2. `hold` is `portrait` (the default) or `landscape`. A phone on its side comes
   up upright and turns a quarter, as a hand turns a phone to watch a video.
3. `app` is one of three shapes most phones have, each with its own list:
   - `chat` with `messages`: bubbles from others on the left (`from`, a round
     `icon`) and the owner's (`mine: true`) on the right. The first message is
     there when the phone comes up; each press brings the next, another's after
     typing dots. A message is `text`, a `picture` or a `video`. A press on the
     newest video turns the phone on its side and plays it; Cancel turns it back
     to the chat.
   - `feed` with `posts`: the posts stacked top to bottom, as a photo app
     scrolls them (`who`, `icon`, a `picture` or a `video`, `text`, `likes`).
     The wheel, the up and down keys (a post at a time) and a finger or the
     mouse dragged up and down scroll them, gliding on after a drag and
     stopping at the top and the bottom; a click on a post's picture likes it;
     OK scrolls a post down and, at the bottom, puts the phone down; a post's
     video plays in its place, looping, while it is on the screen.
     `scroll: across` lays the posts side by side instead, a post a screen
     with a row of dots under it, as a photo app's carousel pages: a drag
     sideways follows the finger and settles on the nearest post or, flicked,
     on the next; left and right (and up, down, a notch of the wheel and OK)
     step a post. A post too tall for the screen across has its picture made
     shorter. `browse: true` makes the feed only looked at: a click likes
     nothing, for a story where the character scrolls past without reacting.
     (Added in v0.18.0; the default stays the column with likes.) A post's
     `set` is kept when the phone is put down with the post liked, as a page
     button's `set` is when it is pressed: the like the player gave as the
     phone goes down counts, so a second tap takes it back. Several posts can
     be liked at once, so a feed answers its event with one number, a bit for
     each post with a `set` (at most 31), and a save and a replay keep that one
     number as they keep a button's index. A feed has no `var`: a per-post
     `set` already says what each like means, and one var could not hold
     several likes without a list type the expressions lack. `browse: true`
     keeps nothing, and `gozen check` rejects a `set` under it. (Added in
     v0.19.0.) A caption
     too long for its lines is wrapped smaller and drawn at that size.
   - `video` with `video`: a player with the title, a pause on a press and a
     seek bar.
4. A `video` is a clip made of what a game already has: a `picture` pushed in
   slowly, and a `puppet` in front of it that breathes and sways as it does on
   the stage, `zoom` times the frame's height, jiggling at each second listed
   in `bounce`. `seconds` is its length (30 by default).

   A still pushed in barely moves, so a clip can also play footage: `frames`
   lists pictures under assets/ (a pattern such as `clips/battle/*.jpg` stands
   for the files it matches, sorted, as an image's `files` do) shown in order
   at `fps` (12 by default) with the same cover crop, over the `picture`, which
   becomes the poster. A feed's post loops them; elsewhere the clip stops on its
   last frame. `seconds` defaults to the frames' length. The frames are decoded
   with the phone's other pictures when it comes up; the engine decodes no
   video files, so a clip is cut into pictures beforehand (ffmpeg does it).
5. Every text is interpolated and translated when the command runs. `gozen
   check` rejects an unknown hold or app, a missing list or a list the app does
   not show, a message without exactly one content, a post without a picture or
   a video, a video without a picture, frames or a puppet, missing files, a frames
   pattern that matches nothing, negative
   numbers and a bounce outside the clip.

## Consequences

The phone is drawn by the engine in fixed, neutral colors, so it reads as a
phone in every game and needs no assets of its own; the apps are not copies of
any one product. A game with a clock may keep `var.clock_minutes`, which the
status bar shows. The phone is drawn from canvases it owns and frees when it is
put down. The command records nothing in a save but the result it was put
down with (a button's index, a call's answer, a feed's likes), kept among the
event's answers as a choice's is: Back over it shows the phone again from its
start, with no post liked.
