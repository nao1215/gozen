# Changelog

Changes before the first public release are tracked under Unreleased. All packages and the gozen command share one module version. The v0 API is being established; experimental packages under `x/` carry no compatibility promise.

## Unreleased

### Added

- YAML game loading and validation, expressions, events, battle rules, world navigation and versioned saves.
- Headless execution, Ebitengine presentation, audio and an embedded-game runner.
- The gozen command with check, layout, sim, curve, floors, trace, book, design, memos, release, trial, shots, puppet, prompt, mosaic, version and help subcommands; ONNX inference in mosaic needs a cgo build and uses user-supplied models and runtime libraries.
- Original minimal, headless and graphical examples with asset source and license records. The Wayfinder pictures are CC BY 4.0; the code is MIT.
- Architecture dependency checks, public API inventory checks and CLI contract tests using atago.
- CI for lint, unit tests across three operating systems and minimum/stable Go versions, graphics, CLI contracts, native and cross-builds, examples, race detection, govulncheck and CodeQL.
- A development balance panel (F7 with `--dev`): win rate, HP left after a win and win rate by level for the battles at the current location, measured inside the game loop with the same functions and seeds as `gozen sim`, and measured again after a hot reload. `gozen sim` gains `--state` (start from a saved state such as an F9 note's `.save`), `--as-is` (keep its HP, MP and states), `--hp` and `--streak`. When the party is hurt the panel puts the current-HP win rate under the healed one and shows how many wins in a row the current HP lasts.
- GNU-style options for the gozen command, its prompt and mosaic subcommands and the game runner: `--name value` and `--name=value`, one-letter forms for common options (`-n`, `-o`, `-v`, `-h`), options before or after arguments, and `--` to end them. Single-dash long options such as `-seed` are rejected. Every `--help` has the same shape: a title line with the version, [Usage], [Examples], grouped [Options], notes with the requirements, and the project links.
- The engine's own words (menus, battle messages, settings, saves, gallery, development panels) come in English and Japanese. `language: en` (the default) or `language: ja` in game.yaml picks them, and `text` lays the game's own wording on top. A test keeps the English list in step with every key the code shows. `text.Use` now takes the language (`text.English`, `text.Japanese`). `gozen book`, `design`, `memos`, `shots` and `trace` write their documents in the game's language; gozen's own diagnostics are English. `gozen check` warns when `language` is unset but the game is written in Japanese, and when the regular font lacks letters of the chosen words.
- Moving portraits (`puppet`): one picture plus small difference pictures (closed eyes, mouth shapes, other faces) in YAML blink, move the mouth while speaking (`Speak`/`Stop`), breathe, sway and tilt the head, let the hair trail behind on a spring, play one-shot reactions (`React`: hop, shrink, shake, nod, tilt) and fade between faces (`SetFace`). The head and hair move by deforming a mesh, so no layered artwork or rig is needed. Full-size difference pictures are cut down to the part that differs when loading. `gozen puppet` checks a puppet, prints where each overlay lies, and with `--out` writes the small pictures.
- Contribution and security reporting guidance, plus Dependabot updates for Go modules and GitHub Actions.

### Changed

- Breaking: a game without `language` now shows the engine's words in English. Japanese games add `language: ja` to game.yaml. Wayfinder sets `language: en` and keeps only its own wording in `text`.

### Verification limits

- Published installation, Windows/macOS runtime checks and remote CI results remain unverified locally. Cross-builds do not establish runtime support.
- Real audio and ONNX inference need separate runtime verification; model-dependent test skips are not inference results.
