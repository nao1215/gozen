# ADR 0052: Stream voices, the radio and the chat's memory

Status: Accepted (2026-10-27). Home: `internal/layer2/gamedata` (the voices folder, the voice key and its check), `internal/react` (the lines as written), `internal/play` (playing the voices in the watch view).

## Context

The watch view of ADR 0048 reads its lines aloud only with a browser's `speechSynthesis`; the desktop build has no voice at all, and a synthesized voice is the same flat voice for every game. A stream game wants to record its lines ahead (with a speech engine or an actor) and play them in every build. A line can be said in two languages, its greeting has the viewer's name in it, and the reaction book's lines are filled with the comment, so a recorded line needs a name that the recording tool and the game compute alike from the text the author wrote.

## Decision

### Recorded voices

The streams file gains `voices: voice`, a folder under assets/ (the word of an actor's `voices`, ADR 0028). It holds a folder for each channel, named by the channel's id, and in it the voice of each line the channel says as `KEY.ogg`. The key is the first 16 hexadecimal digits (lower case) of the SHA-1 of the line in UTF-8, as it is shown (translated into the language shown, ADR 0027) and before its `{fields}` are filled: `Hello, everyone.` is `6f2529aa52538b73.ogg`, and `Welcome, {from}.` has one voice whoever the viewer is. A reaction book's line is keyed as the book writes it (`react.Reaction.Written` gives the lines as written beside the filled ones). A duo's line is in the folder of the host who says it. A tip's comment read out in quotes has no voice.

When the lines are read aloud and the line has its voice, the watch view plays it at the voice volume (and the actor's, ADR 0028) and waits for it to end, with a quarter of a second after it, before its next step; the mouth moves while it plays. A line without its voice is read by `speechSynthesis` in a browser build and only captioned elsewhere, lasting its timing. The read-aloud button shows when either can be used: in a browser, or in any build when the voices folder exists; reading aloud starts on when it exists (the button turns it off), and off otherwise as before.

The watch view already steps through its show by its own clock (each step waits for the one before, ADR 0048), so waiting for a voice delays only that view: the lobby's cards keep showing the line at the wall clock, a stream that goes on whether anyone listens, and the watch view lags behind them by the voices it waited for. Coming back to the lobby shows the show where the clock has it, and opening it again comes in there.

A desktop build reads a voice when its line begins, so a run and its replay time the lines the same; the trace records each voice played or missing (`voice`, with `file`, `who` and `ticks` or `missing`). A browser build reads it in the background: the voices are sounds, fetched after the other files (ADR 0051), and opening one still coming would hold the frame; the line waits for it at most its reading-aloud limit, and a voice that does not come is a line without one.

`gozen check` warns (GZN0119) of a voices folder that does not exist, of what in it is not a channel's folder, and of a file that is the voice of no line of its channel in any language offered (usually a line changed after it was recorded). A line without a voice is not reported: the voices are optional and may cover only some lines.

### The radio

The watch view has a radio, for a show heard without looking at the screen: a button over the chat column (beside the full screen's) or the auto key (F) turns it on, and the choice is a setting (`stream_radio` in the settings file) kept for every save, so a show opened from the lobby opens as the radio while it is on. In the radio:

- every line is read aloud, whatever the read-aloud button says: its recorded voice, else `speechSynthesis` in a browser (captioned only elsewhere), and the next step waits for it to end;
- the chat messages the show picks up are read in the viewers' voice, with `speechSynthesis` only (no recorded voice): a chat beat, the answer picked for a question (before the lines that answer it), and the player's comment the streamer answers (before the answer). The viewers' voice is 1.4 times the normal pitch, 0.7 when the host's own pitch is above 1, at 1.15 times the rate, so it is never the streamer's;
- a show that ends tunes, three seconds later, to the next show on the air in the order of the lineup (another channel's, or a duo's), as a new watch view (greeting the player, counting the visit);
- the screen is the face of the streamer speaking, the name, the episode's title, the line in large letters, the viewer's message being read under it, and two buttons: next (also right or OK), which tunes to the next show at once, and stop (also cancel or the auto key), which turns the radio off and shows the watch view;
- a browser build holds the Screen Wake Lock (`navigator.wakeLock.request("screen")`) so the screen does not turn off, asking again every five seconds when the page lost it (it does when hidden) and letting it go when the radio stops or the view closes; a browser without the API, or one that refuses, does nothing more.

The radio reuses the watch view, its steps and its chat; it only draws and reads differently, so a show heard in the radio counts as watched as it would on the screen.

## Consequences

A recording tool computes the same key from the texts of the streams file, its episodes, the translations and the reaction book, and writes the files where the game looks; the check tells it which files are stale. Voices cost a browser export their download; the game does not wait for them to start.
