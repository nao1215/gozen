# ADR 0056: A phone on the screen

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
   - `feed` with `posts`: one post fills the screen (`who`, `icon`, a `picture`
     or a `video`, `text`, `likes`). Left and right, or a swipe, turn the posts;
     a click likes the post on the screen; a post's video plays in its place,
     looping.
   - `video` with `video`: a player with the title, a pause on a press and a
     seek bar.
4. A `video` is a clip made of what a game already has: a `picture` pushed in
   slowly, and a `puppet` in front of it that breathes and sways as it does on
   the stage, `zoom` times the frame's height, jiggling at each second listed
   in `bounce`. `seconds` is its length (30 by default).
5. Every text is interpolated and translated when the command runs. `gozen
   check` rejects an unknown hold or app, a missing list or a list the app does
   not show, a message without exactly one content, a post without a picture or
   a video, a video without a picture or a puppet, missing files, negative
   numbers and a bounce outside the clip.

## Consequences

The phone is drawn by the engine in fixed, neutral colors, so it reads as a
phone in every game and needs no assets of its own; the apps are not copies of
any one product. A game with a clock may keep `var.clock_minutes`, which the
status bar shows. The phone is drawn from canvases it owns and frees when it is
put down. The command does not record anything in a save: Back over it shows
the phone again from its start.
