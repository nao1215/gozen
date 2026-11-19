# ADR 0066: Weather and tone

Status: Accepted and implemented (2026-11-19). Home: `internal/model/gamedata` (`WeatherCue`, `ToneCue` and their checks), `internal/model/rpg` (`State.Look`), `internal/rules/event` (the commands), `internal/screen/play` (`look.go`, drawing).

## Context

Authors could shake the screen, flash it and fade it through a color (ADR 0028), but nothing lasted. Rain over a scene, snow on a hill, a memory told in a cold gray, or a night that darkens the place had no word. Similar engines have weather and a screen tint, and authors otherwise faked them with pictures prepared for each background.

## Decision

- Two event commands, from schema version 12:
  - `weather` makes `rain`, `snow`, `petals` or `embers` fall over the scene, with `power` (0 to 1, 0.5 by default), `wind` (-1 to 1) and `color`. `none` stops it.
  - `tone` colors the scene with a preset (`gray`, `sepia`, `night`, `dusk`) or with its parts: `color` multiplies, `gray` takes the color away, `bright` darkens toward black or lightens toward white, and `vignette` darkens the edges. `none` restores the scene as drawn.
  - Each takes `seconds` for the change (1 by default, 0 at once, at most 60). A word is the kind or the preset; a map sets the parts. `gozen check` refuses an unknown word and a number out of its range with the usual codes.
- Both last from the command on, over every place and line, until another command changes them. The state keeps what they left in `look` (the kind, power, wind and color of the weather; the parts of the tone), so a save, a load and a roll back show the same scene. The state does not keep the time a change takes. A look that arrives without its command (a load, a roll back, a new game) is shown at once.
- No location field. An arrival event writes the weather of a place, so the YAML already says where it rains, and a second source of truth would have to be ordered against the command.
- The screen draws the tone over the place, its pictures and the novel window's portraits, and the weather over that. Both go under the windows, the message and the menus. A battle draws neither, because its screen is not the place's.
- The tone is one full-screen Kage pass (`toneShader`) over a copy of the scene. The drops come from a generator with a fixed seed and move only on ticks, so two captures of the same play draw the same drops, and the documentation pictures compare equal when remade. At most 900 drops are on the screen.
- The schema number 12 was used by the casual-game runner, which was removed before any release (the hygiene tombstone kept its constant name from returning). No released game has schema 12, so the weather and tone commands take the next number rather than skipping one. The runner's other names stay removed.

## Consequences

Authors write weather and tone in YAML with no pictures of their own. The examples use them: After the Pass lets snow fall on the hill and colors Brenna's memory, and Go Zen colors the memory of the brass wall and scatters petals over the final square. The headless session sees each command as a visual effect named `weather` or `tone`, and tests read `look`. The pictures are checked by the documentation pictures (`novel-weather`, `novel-tone`) and their remake, not by a pixel test of their own.

`TestWeatherAndToneWords` checks the words, ranges and version. `TestWeatherAndToneLook` checks the state, the save and the effects. `TestLookChanges` checks how the screen eases to a look, shows a loaded one at once, and drops the same snow twice.
