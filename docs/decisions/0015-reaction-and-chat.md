# ADR 0015: Character reactions and chat

Status: Partly changed (2026-10-19). Home: `react` (layer 3, rules) and `chat` (layer 1, foundation). The public surface of the book (characters, rules, asks, memory) and of the script types is superseded by [ADR 0021](0021-query-and-commands.md), which makes them opaque types with `YAML(path)` and removes the custom `Analyzer` interface until a consumer needs it. Custom findings can be supplied with `Stimulus.Tags`.

## Users and problem

Let game authors build play in which characters react to what the player says or does. There are three kinds of users.

| User | Stimulus | Reaction |
| --- | --- | --- |
| A streamer game | Free-text comments, donation amounts, scrolling bot comments | Pick up, ignore, or react with a line and an expression; affection |
| An open-world resident | Events (stealing, equipping, a long-awaited visit) and being talked to | A line, an attitude, a rumor |
| Online versus play | The opponent's words | Masking before display, a bot's backchannel |

No LLM is used (everything must work from the shipped files alone, avoiding network cost and unpredictable output). Judgment accuracy may be low. In place of accuracy, the character's behavior makes it look smart.

1. When confidence is low, the character takes the initiative: teasing, asking back, changing the subject. A miss still holds together as personality.
2. Remember, and bring it up later. Insert earlier topics and events into lines.
3. Ask questions that narrow the answer. Right after a question, match against the expected answer first.
4. Do not repeat the same reaction. Detect runs of the same line and repeated words, and change the reaction.

## Decision

Reaction (judgment) and chat (flow) are two independent packages. They do not import each other. The game connects them.

### react: stimulus, analysis, rule, reaction

```
Stimulus (words, events, numbers) → Analyzer (code) → Tags with confidence
                                                       ↓
                       Memory (per character) → Rule (YAML, conditions in expr) → Reaction
```

| Type or function | Who uses it and why |
| --- | --- |
| `Stimulus{Kind, From, Text, Values, Tags}` | The game passes a stimulus. `Kind` is a string such as `say` (`KindSay`) or `event.steal`. For an event with no words, put known marks in `Tags` |
| `Analyzer` interface (one method: `Analyze(Stimulus, *Mind) []Tag`) | The game adds its own judgment. It can read `Mind`'s `Recall` and `Mood` |
| `Tag{Name, Score, Slot}` | The result of analysis. `Score` is a 0 to 1 confidence; `Slot` is the dictionary word that matched (inserted into memory or lines) |
| `Load(fsys, path)`, `Parse(b)` returning `(*Book, error)`, `(*Book).Validate` | Read the dictionary and character YAML and validate references |
| `New(book, id, cfg Config) (*Mind, error)` | Create the run state of one character. Random numbers, clock, custom analyzers and input limits are passed in `Config` |
| `(*Mind).React(Stimulus) (Reaction, error)` | React to one stimulus. Only a failure to evaluate a condition expression is an error |
| `(*Mind).Ask(id) (Reaction, error)` | The game makes the character ask a question (raise a topic) |
| `Reaction{Rule, Lines, Face, Delta, Mood, Confidence, Tags, Ask}` | The screen shows the lines and expression; the game uses mood and the like |
| `(*Mind).Mood`, `Recall`, `Save`, `Restore` | Read mood and memory, and save and restore them (a JSON byte slice) |
| `(*Mind).NewSession` | Start a new conversation with the same character, such as the next stream or the next visit. It forgets the recent words used for repeat detection and any question awaiting an answer, and keeps mood, memory and cooldowns (added 2026-10-19 so that the streamer game can span streams) |
| `Normalize(s)` | Use the same normalization as the dictionary in a game's own analyzer |

Standard analyzers (only processing outside the dictionary lives in code):

- Normalization: full-width to half-width, katakana to hiragana, Latin letters to lower case, remove whitespace, punctuation and long-vowel marks, and reduce runs of three or more identical characters to two. No morphological analysis is used (it keeps the WASM small, and substring matching is enough).
- Dictionary: look up a word list by substring match and attach tags. Each word can take an optional weight (confidence).
- Acrostic: with two or more lines, join the first character of each line, look it up in the dictionary, and attach `acrostic_<tag>`.
- Repeat: if the words equal the same sender's recent words, attach `repeat` (the count is `repeats` in conditions).
- Memory mention: if the text touches a remembered string (two or more characters, or one kanji), attach `recall`.
- Empty utterance: for `say` with no words, attach `empty`.
- Numbers: values such as a donation amount are read directly from conditions as `value.NAME`.

Names usable in condition expressions (`expr`): `tag.T` (confidence, 0 if absent), `slot.T`, `mem.K` (0 if absent), `value.N`, `mood`, `turn`, `kind`, `from`, `length`, `asked`, `answer`, `repeats`, `confidence`. Lines can insert `{from}`, `{mood}`, `{slot.T}` and `{mem.K}`. Validation treats unknown names, tags, expressions, asks and insertions as errors.

A character YAML (a shortened form of the example in `react/react_test.go`, with its Japanese dictionary words and lines given in English here):

```yaml
lexicon:
  cat: [cat, kitten, {meow: 0.4}]
  dog: [dog, puppy]
  rude: [idiot, stupid]
tags: [gift]            # tags the game attaches with Stimulus.Tags
people:
  - id: host
    mood: 50
    faces: [normal, smile, angry, think]
    likes: {cat: 10, rude: -20}
    asks:
      - id: pet
        say: [Dogs or cats?]
        expect: [cat, dog]
    rules:
      - id: answer_cat
        when: asked == "pet" && answer == "cat"
        say: [You get it.]
        remember: [{key: pet, set: "{slot.cat}"}]
      - id: gift
        kind: event
        when: tag.gift > 0 && value.amount >= 1000
        say: ["Thanks, {from}!"]
        delta: 15
      - id: callback
        when: tag.recall > 0
        say: ["I remember what you said about {mem.pet}."]
      - id: cat
        when: tag.cat >= 0.5
        say: [Cats are nice.]
        remember: [{key: cats, add: 1, turns: 3}]
        ask: pet
        cooldown: 2
    dodge:
      repeat: [You said that already.]
      unsure: [Hm. So?]
      unknown: [Say that again in plain words.]
```

Rules are evaluated from the top, and the first rule whose `kind` and cooldown fit and whose condition holds is taken. If none holds, a line is picked from the `dodge` set in this order: `repeat` (a repeated line), `unsure` (some tag is present), `unknown` (nothing). Lines are chosen so as to avoid the same one as last time. Mood moves by the sum of `likes` times confidence plus the rule's `delta`, and is clamped to `mood_min` to `mood_max` (default 0 to 100). When a question is asked, on the next stimuli (`turns` times) the `expect` tags get confidence 1, readable as `asked` and `answer`.

### chat: scrolling messages

| Type or function | Who uses it and why |
| --- | --- |
| `Message{ID, From, Text, At, Values}` | One message |
| `New(cfg Config) *Log`, `Post`, `Pull`, `Recent`, `Since` | Keep the recent messages up to a limit; the screen scrolls and picks them up. Long text is cut |
| `Source` interface (one method: `Poll(now time.Duration) []Message`) | Stream player input, bots and online opponents in the same form |
| `Filter` interface (one method), `NewMask(words) *Mask` | Mask or drop before display. `Mask` matches ignoring full/half width, kana, case, whitespace, symbols and long-vowel marks, and turns the original characters into `*`. Words between people always pass |
| `Load`, `Parse` returning `(*Script, error)`, `NewCrowd(script, CrowdConfig) *Crowd` | The bot script (names, average count per second, lines per mood). `Mood` changes the mood and `Burst` issues a simultaneous reaction to an event. After a long pause, at most 20 are emitted at once |

Online communication is not implemented. Only the `Source` hook is provided; WebRTC or a relay server is the user's choice.

### Layers

`react` sits in layer 3 (rules) and uses only `expr`, YAML and `golang.org/x/text/width`. It does not use Ebitengine and is tested without drawing. `chat` sits in layer 1 (foundation) and uses only the standard library, YAML (the bot script) and `golang.org/x/text/width`. Following the ban on sideways dependencies within a layer, the two do not import each other. The caller assembles the display.

## Alternatives

- LLM (an external API or a small in-browser model): not chosen because of network cost, distribution size and difficulty of controlling output.
- Morphological analysis (kagome and the like): adds tens of MB of dictionary and does not suit WASM. A prototype will check that substring matching is enough.
- Place it as an experiment under `x/` and promote it once there are two users: Gozen is not yet public and has no external users, so changing the API has a small cost even in a public layer. Instead it is checked in "Review before Gozen goes public" below (the author's decision, 2026-10-19).
- Merge reaction and chat into one: an open-world game uses no chat and online versus play uses no reaction, so they are kept apart.

## Verification

- `react/react_test.go`: a table of conversation sequences with the expected rules, lines and mood (random seed fixed). Asks and bringing up memory, cooldowns, memory expiry (by count and by time), save and restore, normalization, and 13 kinds of validation errors. `FuzzReact` checks that mood and confidence stay in range for long or broken input.
- `chat/chat_test.go`: limits and IDs, truncation, dropping Filters and spelling variants of masking, bot frequency, mood, bursts, the cap after a pause, and script validation. `FuzzMask` checks that masking does not change the character count.
- Mask matching normalizes the text with NFKC first, so the dakuten of half-width katakana (ﾊﾞ) and a voiced mark written as its own character are merged; the original characters of a match are hidden.
- Not handled: a CLI for authors to talk with a character and check the rules taken (`gozen talk`) is left for later consideration.

## Decided (the author's answers, 2026-10-19)

- Affection and mood are held by the character (`Mind`). They can be used directly from rule conditions (`mood`) and from lines, and are saved with memory. The game reads `Reaction.Delta` and the values of `Mind`.
- Each memory can choose its expiry as a number of conversations or as in-game time. The game passes time through the clock in `Config`. If omitted, it is never forgotten.
- Dictionary words can take an optional weight. If omitted it is 1. A tag's confidence uses the weight of the matched word (example: `{cat: 1, meow: 0.4}`).

## Review before Gozen goes public

Before Gozen is made public, check the following. Anything that fails gets its API fixed or is taken out of the public surface.

1. Two users of different nature are actually using it: the streamer game and a second one (for example, the townspeople of an RPG, or a sample). Do not keep an API that only suits one of them.
2. The YAML format is documented in the [schema](https://nao1215.github.io/gozen/schema/), and the validation in `Load` and `gozen check` find mistakes.
3. It withstands untrusted input. It accepts free text and online opponents' words, so normalization, analyzers and masking have fuzz tests, and time and memory are bounded for large input.
4. It has been confirmed to run on WASM in a user's game.
