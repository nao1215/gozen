# ADR 0047: Headless scripted runs

Status: Accepted (2026-10-26).

## Context

Balance tools drive long plays with the scripts of `gozen run --dev --script` and read the trace to decide the next script. A scripted run opens a window, keeps 60 ticks a second, and draws every frame, so unattended runs need Xvfb and spend most of their time drawing. The rule simulators (`sim battle`, `sim campaign`, `sim dive`) do not play the screens, so their numbers are not those of the scripted game.

## Decision

Add `gozen sim script`, backed by `play.Simulate`. It builds the same `play.Game` as `Run`, with the same options, script parser and trace, and calls `Update` in a loop as fast as it goes. It never calls `Draw`, never starts the Ebitengine loop, and opens no window or audio device. Every rule, event, battle, dungeon step and campaign order runs in `Update`, and the script's screenshot pause happens before the tick advances, so the same seed gives the same lines and ticks.

What is drawn is not reproduced: `shot` and the automatic screenshots of `lines`, `play` and walking take no picture, and the trace has no `shot`, `view`, `stand` or `menu` lines. `music` lines depend on how fast the songs decode in the background and are not compared. Pictures are not decoded ahead, and a picture asked for is an empty image of its size, read from the file header: the rules can depend on a picture's size (a tall portrait stands on the stage), never on its pixels.

With `--script -`, script lines come from the standard input after the game is ready for the next command; a `ready` line is written and the trace flushed before each read, a line that does not parse becomes a `script_error` line, and the end of the input ends the run. The script command `snapshot` writes the whole state as a `snapshot` line in both commands. These kinds are not compared by `--replay`.

## Consequences

A month of a strategy game that takes tens of seconds under Xvfb takes the time of loading the game. One process can be driven for many months. Behavior that would depend on what was drawn, such as a `click` on a position computed while drawing, is not verified. The graphics test `TestGraphicsSimulateMatchesRun` compares the two paths on talk, choices, battles, dungeon steps and campaign turns.
