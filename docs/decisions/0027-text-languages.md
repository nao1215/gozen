# ADR 0027: The languages of a game's text

Status: Accepted (2026-10-21). Done (stage 5). This extends [ADR 0019](0019-engine-languages.md) from the engine's words to the game's own text. Home: `internal/layer2/gamedata` (the files and the checks), `internal/text` (the active language).

## Users and problem

An author writes a game in one language and wants to offer it in another. ADR 0019 already lets `game.yaml`'s `language` choose the engine's words and `text:` override them, keyed by the engine's source words. The game's own text (lines, names, choices, places, items) has no such way: the author would copy every data file.

## Decision

### Source to translation (gettext)

- `game.yaml` gains `written_in` (the language the files under `data/` are written in; defaults to `language`) and `languages` (the languages the player may choose; defaults to `[language]`). `language` stays the language shown.
- `lang/<code>.yaml` holds `text:`, a map from the source text to its translation, in the same key space as the engine's words of ADR 0019. `{expressions}` stay in the translation as they are.
- What is translated: the text and the speaker of `say`, the label and prompt of `choice`, `notice`, the names and descriptions of places, people, actions, exits, items, skills, states, troops and classes, the title and subtitle, gallery titles and the labels of note kinds. Everything shown as text.
- Where: once, just before the text is shown (before `{expressions}` are filled in). The active language belongs to the process, as the engine's words already do ([ADR 0021](0021-query-and-commands.md), decision 7: text is not per Session).
- The settings screen gains a "Language" row when `languages` has two or more. The choice is kept in `config.json` and wins over `game.yaml` at start.
- Read lines are shared across languages, because their key is the source ([ADR 0026](0026-read-lines-and-skipping.md)).

### Checks

`gozen check` reports, per language: a translation whose `{expressions}` differ from the source (an error), a key that is no source text of the game (a warning), the text not yet translated (a warning with the first few; `--format json` lists every one with its path, so an agent can fill `lang/<code>.yaml`), the layout of the message window, and the glyphs missing from the fonts (ADR 0019's warning, per language). `gozen book` gains no `--language` (the author decided the JSON list of untranslated text is enough).

Headless checks of a translated game load it with an overlay that sets `language` (`gozen.Load` with an overlay), so `Request.Text` is the translation and `DefaultPolicy.Prefer` names translated labels.

### As built

- The translations belong to the loaded game (`DB.Words`, `DB.Show`, `DB.Tr`), not to the process: the engine's words stay process-wide (`text.Use`) as before, and a game loaded with `language: ja` is a Japanese game for its sessions. This keeps a test that loads the game twice, once in each language, simple.
- The names and descriptions of the definitions (places, people, actions, exits, items, skills, states, enemies, classes, shops, the title, the subtitle and the gallery's scenes) are translated in place by `Show`, which keeps their sources to translate again when the player changes the language. The lines, speakers who are not actors, choices, prompts and notices are translated by the runner as they are shown, so the key of a read line is its source.
- The languages are those of the engine's catalog, `en` and `ja`. `languages` must hold the language shown.
- `lang/<code>.yaml` has one field, `text`; a key that is a word of the engine is not a stray key. A language offered without its file has every text untranslated.
- `gozen check`: a translation with other `{expressions}` than its source is an error; a key that is no text of the game is GZN0110, each text not translated GZN0111 (with its path; `--format json` lists them all). The layout check lays out the lines in each language offered (the files tagged `[ja]`), and the glyph check reads `lang/` too. `gozen book` stays in the language written (the author decided against `--language`).
- The settings have a "Language" row when two or more are offered; `config.json` keeps `language`, which wins over `game.yaml` at start.
- Until a language is kept in `config.json`, a game that offers two or more opens in the first of the device's languages it offers (a browser's `navigator.languages`; the desktop's `LC_ALL`, `LC_MESSAGES` and `LANG`, then macOS's `AppleLanguages` or Windows's user locale), a region not counting (`en-US` is `en`), else in `language`; a run checked again (a script, the autopilot, a replay) keeps `language`.
- Changing the language shows what is on the screen in it at once: the open line, its speaker, the question and its options, and the backlog. A line keeps its source and the values its `{expressions}` had when it was shown (`gamedata.Phrase`), so it is translated again without evaluating them again.

## Alternatives

1. A map per string (`text: {en: ..., ja: ...}`). Every file grows with every language and a translator edits the author's files. Not chosen.
2. Line ids. Exact, but the engine has no ids and authors would write one per line. Not chosen.
3. Path keys (`events.dinner.do[3].say`). A moved line loses its translation. Not chosen.

## Consequences

- One source text has one translation. When the same words must be translated differently in two places, the author writes the sources differently (the limit of gettext; documented).
- Several Sessions in different languages in one process are not supported (the existing constraint of ADR 0021).
- `api/public.txt` does not grow. `api/schema.txt` gains `Game.WrittenIn` and `Game.Languages`.

## Plan

| Stage | Content |
| --- | --- |
| 5 | `lang/`, `written_in`, `languages`, the translation before showing, the settings row, the checks (done) |
