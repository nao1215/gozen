---
title: Using AI agents
description: Let a coding agent write a Gozen game - give it the schema, make gozen check the gate, measure balance with gozen sim, and review with gozen book.
weight: 70
---

A Gozen game is plain text with a checker that answers in file and line. That suits a coding agent: it writes YAML, runs `gozen check`, reads the errors, and fixes them until the check passes. You keep the parts an agent cannot judge: whether a line reads well and whether a fight feels right.

## The loop

1. Give the agent the contract: the [Schema](/schema/) page, or the JSON Schema in [data/schema/](https://github.com/nao1215/gozen/tree/main/data/schema), and a working example such as [Wayfinder](https://github.com/nao1215/gozen/tree/main/examples/play/game).
2. Give it one rule: `gozen check game` must pass before it says it is done.
3. Ask for numbers, not opinions: `gozen sim` and `gozen curve` say whether a fight or a dive is too hard.
4. Review the result as one document with `gozen book game > book.md`, then play it with `--dev` and leave F9 notes.
5. Hand the notes back: `gozen memos notes` gives a page of screenshots and comments, and each note's `.json` names the place and line.

```shell
gozen check game                                       # references, expressions, pictures, text layout
gozen sim game cave_boss --level 8                     # win rate of the party at level 8
gozen curve game cave --floors 5 --boss 5:cave_boss    # a whole dive: levels and wins per floor
gozen book game > book.md                              # everything, as one Markdown file
```

`check` exits 1 on any error, so it works as a gate in a script, a git hook, or CI.

## An AGENTS.md for your game

Put a file like this at the root of the game's repository. Most coding agents read `AGENTS.md` (or `CLAUDE.md`) before they start.

````markdown
# Game rules for agents

This is a Gozen game. The game is data: game/game.yaml and game/data/*.yaml.
Pictures, fonts and sounds are in game/assets/ and are referred to by file name.

## Before you write

- Read the schema: https://nao1215.github.io/gozen/schema/
- Read game/data/ to see the IDs that already exist. Reuse them; do not invent near-duplicates.
- Every game needs the skills attack and guard.

## While you write

- Put new places in their own file under game/data/ (one area per file).
- Conditions and formulas are expressions: flag.NAME, var.NAME, gold and item.ID in events;
  a.atk and b.def in battle formulas; level in class stats.
- Do not add pictures that do not exist. Use an existing one and leave a TODO comment.

## Before you say you are done

Run these and fix every error. Do not stop while check fails.

    gozen check game
    gozen sim game <troop> --level <expected level>

A boss should win between 60% and 80% at the level the player is expected to have.
Report the sim numbers for every troop you added or changed.
````

Adjust the targets to your game: the expected level at each place, the win rates you want, and which files the agent may change.

## Editor completion

The JSON Schema gives editors completion and inline errors for the YAML. With the YAML language server (VS Code's YAML extension, Neovim, Helix), add a line at the top of a file:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/nao1215/gozen/main/data/schema/data.schema.json
locations:
  camp:
    name: Camp
```

Use `game.schema.json` for `game.yaml`. The JSON Schema checks shapes and types; references between files and expressions are checked only by `gozen check`.

## Tips

- Ask for small rounds: one place, one troop, one event at a time, each ending with a passing check.
- Ask the agent to quote the `gozen sim` output after a balance change, instead of describing it.
- Keep the agent away from `assets/` unless you want it to manage files; a missing picture is an error `check` will report, which keeps the agent honest.
- For dialogue, have the agent draft and you rewrite. `gozen layout` makes sure the result still fits the window.
