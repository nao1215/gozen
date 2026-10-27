# ADR 0052: Stream voices, slots and the chat's memory

Status: Accepted (2026-10-27). Home: `internal/layer2/gamedata` (the voices folder, the voice key and its check), `internal/react` (the lines as written, the lines said lately), `internal/layer3/broadcast` (the slot, the episodes seen) and `internal/rpg` (the visits), `internal/play` (the voices, the replies and the record of unanswered comments).

## Context

The watch view of ADR 0048 reads its lines aloud only with a browser's `speechSynthesis`; the desktop build has no voice at all, and a synthesized voice is the same flat voice for every game. A stream game wants to record its lines ahead (with a speech engine or an actor) and play them in every build. A line can be said in two languages, its greeting has the viewer's name in it, and the reaction book's lines are filled with the comment, so a recorded line needs a name that the recording tool and the game compute alike from the text the author wrote. A stream game's episodes are also short talks of 10 to 15 minutes, while a stream on the air is an hour of them; ADR 0048 played one episode a stream.

## Decision

### Recorded voices

The streams file gains `voices: voice`, a folder under assets/ (the word of an actor's `voices`, ADR 0028). It holds a folder for each channel, named by the channel's id, and in it the voice of each line the channel says as `KEY.ogg`. The key is the first 16 hexadecimal digits (lower case) of the SHA-1 of the line in UTF-8, as it is shown (translated into the language shown, ADR 0027) and before its `{fields}` are filled: `Hello, everyone.` is `6f2529aa52538b73.ogg`, and `Welcome, {from}.` has one voice whoever the viewer is. The lines with voices are a channel's greetings (`greet`, `first`, `streak`, `stayed`), its `bridges`, the lines and questions and answers it says in its episodes and duos, and its reaction book's lines. A reaction book's line is said, and keyed, as the other lines are: translated into the language shown when it is said, before its fields are filled. The book answers in its source (`react.Reaction.Written`, with what each line's fields were filled with in `Fields`, so a reply waiting to be said, the rest of its lines or a comment read back later, follows a change of language and is filled with `react.Fill` as it was; an event's `respond` says each line as a line of the source with those values, `data.Say.Values`, so it is translated, shown again in another language and read by its source's key as any line, ADR 0027), and its lines are texts of the game to translate (`lang/*.yaml`, GZN0110 and GZN0111 count them). A duo's line is in the folder of the host who says it. A tip's comment read out in quotes has no voice.

When the lines are read aloud and the line has its voice, the watch view plays it at the voice volume (and the actor's, ADR 0028) and waits for it to end, with a quarter of a second after it, before its next step (a question then gives the viewers `timing.ask` to answer, however its line was read: with its voice, loaded late or not at all, or by speech synthesis); the mouth moves while it plays. A line without its voice is read by `speechSynthesis` in a browser build and only captioned elsewhere, lasting its timing. The read-aloud button shows when either can be used: in a browser, or in any build when the voices folder exists; reading aloud starts on when it exists (the button turns it off), and off otherwise as before. While the lines are read aloud a browser build holds the Screen Wake Lock (`navigator.wakeLock.request("screen")`), so a stream can be listened to with the screen untouched; it asks again every five seconds when the page lost it (it does when hidden) and lets it go when reading aloud is turned off or the view closes, and a browser without the API, or one that refuses, does nothing more.

The watch view already steps through its show by its own clock (each step waits for the one before, ADR 0048), so waiting for a voice delays only that view: the lobby's cards keep showing the line at the wall clock, a stream that goes on whether anyone listens, and the watch view lags behind them by the voices it waited for, or goes ahead of them when its voices are shorter than the lines' timing. Coming back to the lobby shows the show where the clock has it, and opening it again comes in there, or where the watch view left it when that is later (ADR 0048), so what was heard is not said again.

A desktop build reads a voice when its line begins, so a run and its replay time the lines the same; the trace records each voice played or missing (`voice`, with `file`, `who` and `ticks` or `missing`). A browser build reads it in the background: the voices are sounds, fetched after the other files (ADR 0051), and opening one still coming would hold the frame; the line waits for it at most its reading-aloud limit, and a voice that does not come is a line without one.

`gozen check` warns (GZN0119) of a voices folder that does not exist, of what in it is not a channel's folder, and of a file that is the voice of no line of its channel in any language offered (usually a line changed after it was recorded). A line without a voice is not reported: the voices are optional and may cover only some lines.

### Slots

A live stream is a slot of episodes (`slot: {minutes: 60}` in the streams file, 60 when left out, more than 0 and at most 1440). A channel (or a duo) that is live plays its episodes one after another, from the next the player has not seen and round again after the last, until the slot's minutes have passed, and ends with the episode it is in then. Between two episodes the host says one of the channel's `bridges` in turn (a line with no `{fields}`, such as a short break before the next topic); without bridges the next episode follows at once. At the end of the slot the host thanks a viewer who watched most of it (`stayed`, as before), and the channel goes offline until the game opens again; with `slot.rest` (minutes, 0 to 1440) it starts its next slot that many minutes after the end instead (`rest: 0`, at once), timed from the end so the lobby and the watch view agree, again from the next episode the player has not seen. The episodes are taken round: a channel whose episodes have all been seen plays them again, the oldest first. A game that writes `live: {share: 1}` and `slot: {rest: 0}` has every channel on the air whenever it is open (share 1 draws every channel on every opening), and its lobby has no row of channels offline.

The rules of ADR 0048 run on the slot as they did on an episode: the beat at a time is found from the beats of all its episodes and bridges, so the lobby's card shows the line of the episode playing (and that episode's lobby lines) and a show opened from the lobby comes in where the clock has the slot (or where its watch view was left, when later); who is live is decided when the game opens, and a show started at most four minutes before. An episode counts as seen when 60% of its beats from where the player came in are watched, each episode of the slot apart; the channel then goes on to the episode after it, and episodes the player came in too late for stay behind, as a stream does not wait. Reaching the end of the slot marks the episode it ends with seen. The watch view shows the title of the episode playing. GZN0118 now warns of an episode shorter than 10 minutes.

### The chat's memory

The reaction book (ADR 0015) already remembers, keeps its memory in the save across shows and fills a line with what it remembers; the stream adds only the values its rules need and two changes of how a reply is said. The player is the one viewer whose comments reach the book, so a memory is the player's.

- A comment's stimulus carries `value.hour` (the hour of the stream's clock, 0 to 23), `value.visits` (how many times the player opened the channel's shows, kept as `visits` in the channel's progress of the save), `value.days` (the days in a row), `value.step` (the affection step) and `value.family` (1 while an episode with `family: true` plays, one a family is in with a child among the guests, else 0, so the book's rules can keep to lines fit for it, as in `when: value.family == 0`); a tip's has them with `value.amount`, and a bot's tip only `value.amount`, `value.hour` and `value.family`. Remembering and recalling need nothing new:

```yaml
lexicon:
  snack: [chocolate, cookies, chips]
people:
  - id: lise
    rules:
      - id: snack_again              # a later comment on snacks, on any later show
        when: tag.snack > 0 && mem.snack != 0
        say: ["{from}, the {mem.snack} fan, back again?"]
      - id: snack                    # the first: remember which snack
        when: tag.snack > 0
        say: ["{slot.snack}? Good choice."]
        remember: [{key: snack, set: "{slot.snack}"}]
      - id: night_owl
        when: value.hour >= 23 || value.hour < 5
        say: ["Still up, {from}?"]
      - id: regular
        when: value.visits >= 10 && mem.snack != 0
        say: ["Ten visits! Have some {mem.snack}, {from}."]
```

  A memory not written reads as 0 in a condition, so `mem.snack != 0` holds once the snack is remembered.
- A list of lines (a rule's `say`, a question, a dodge) does not say again any line it said the last half of its length times (by their first line, for an entry of several lines): a list of two alternates, a list of ten says five others before it may repeat one. The lines said lately are part of the reaction state, saved with the game (`used`), and replace the single last line the book avoided before.
- A comment the book found nothing in (`dodge.unknown`) is not answered at once: it is read back, as before only when short (`echo.max_runes`) and by the chance `echo.chance`, two beats later, as if the streamer came upon it in the chat; otherwise it flows by unanswered. A show that ends first reads it back before its end.

### The comments the book could not answer

A comment the player typed that the reaction book found nothing in (no tag: the rule `dodge.unknown`, or no rule and no dodge line) is written to `stream_unanswered.json` beside the saves, read back or not: a file of the save folder on the desktop (`--save-dir`, or the game's folder under the OS configuration directory), a key of the page's localStorage in a browser (`gozen/<game id>/stream_unanswered.json`). It is a JSON list of `{at, channel, language, text}`, the oldest dropped beyond 200, and a damaged file starts again. It is for the author, who reads it to grow the lexicon and the rules; the game never sends it anywhere and shows it nowhere. A trial run (`--script`) keeps it in memory with its saves.

## Consequences

A recording tool computes the same key from the texts of the streams file, its episodes, the translations and the reaction book, and writes the files where the game looks; the check tells it which files are stale. Voices cost a browser export their download; the game does not wait for them to start.
