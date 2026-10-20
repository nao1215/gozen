# ADR 0019: Hold engine text in English and Japanese, selected by language in game.yaml

Status: Accepted. Date: 2026-10-19. Applies to the `text` package (layer 1), `data.Game`, and the `gozen` subcommands that produce documents.

## Users and problem

The users are authors who make English or Japanese games with Gozen. The author's decision (2026-10-19) is "Make Gozen support Japanese and English".

Until now the text the engine shows on screen (menus, battle messages, settings, saves, the gallery, dev displays) existed only as Japanese originals, so an English game had to replace all of it with `text` in game.yaml. Any text left out appeared in Japanese, and became tofu boxes in an English font. Wayfinder had more than 80 lines of replacements and still had Japanese in settings and some displays. The game documents produced by CLIs such as `gozen book` also had Japanese headings.

## Decision

- The text keys stay as they are: the engine's Japanese original (including the format verbs `%d` and `%s`). Existing `text` in game.yaml (including rpg's overrides) keeps working unchanged.
- The built-in English list lives in `internal/text/catalog`. The Japanese list is the keys themselves. A test (`internal/text/catalog`) reads the module's code as a syntax tree and checks that all keys passed to `text.T` match the English list (no missing keys and no unused keys), that the English reads the same arguments with the same verbs, aligns the `{name}` placeholders, and contains no Japanese characters. A call whose key is not a string literal fails unless the test table gives its location and the keys it can take.
- Add `language` to game.yaml. The values are `en` (English) and `ja` (Japanese), and omitting it means `en`. `text` is laid over the text of the selected language. Any other value is an error of `gozen check`.
- `text.Use(language, words)` replaces `Use(words)`, and `text.English` and `text.Japanese` are public. Before the first `Use`, `T` returns the key as it is (same as before). `Words`, which reads the English list, is not public, because only loading (the names of the default equipment slots) and the font check of `gozen check` use it.
- The runner reads only `language` and `text` first, before reading game.yaml, and also prints loading diagnostics in the game's language (`gamedata.UseWords`). The same holds for check, layout, book, design, sim, curve, floors, trial and shots of `gozen`. `gozen memos` and `gozen trace`, which take no game directory, follow the `language` recorded in the start line of the memo and the trace. Old memos and traces with no record are treated as written in Japanese.
- The diagnostics of `gozen` itself (the errors and summaries of release, trial and shots) are in English. Game documents (the headings of book, the report card of design, the HTML of memos and shots, the reading of trace) go through `text.T` and follow the game's language.
- When `language` is omitted but the title, subtitle or `text` is in Japanese, `gozen check` warns and recommends `language: ja`. It also warns when `fonts.regular` lacks the glyphs for the selected language's text and the characters of `text` (letters and digits only; symbols are taken as a font choice).

## Alternatives

1. Change the keys to IDs such as `battle.cannot_escape`. It would make translations easier to manage, but every existing `text` in game.yaml (the work's overrides) would stop working. rpg and Wayfinder would need rewriting, so it is not chosen.
2. Make Japanese the default when omitted. Existing Japanese games would need nothing, but as a public framework an English author would see Japanese first. The default is English, Japanese games are asked for the single line `language: ja`, and the `gozen check` warning lets them notice.
3. Embed the English list as YAML. The layer 1 `text` would depend on YAML parsing, and checks of keys and formats would be needed separately. It is a Go map checked by tests.

## Consequences

- Public API: the argument of `text.Use` changes, and `text.English`, `text.Japanese` and `data.Game.Language` are added (`api/public.txt`). `Game.language` is added to the schema ([schema](https://nao1215.github.io/gozen/schema/), `data/schema/types.schema.json`).
- Existing Japanese games show English engine text until `language: ja` is added to game.yaml. The CHANGELOG and the `gozen check` warning tell them.
- `language` is added to the memo (`memo.Memo.Language`) and to the trace start line. It is an optional item, and old records can still be read.
- The gallery's page-selection entry "from the beginning" used the Japanese key `はじめから`, the same key as the title's "New game". Because their English text must differ, the gallery key was changed to `最初から` (a different Japanese wording of the same meaning).
- When adding text, add it to the English list too. Forgetting fails the test.
