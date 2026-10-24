---
title: Using AI agents
description: Let a coding agent write a Gozen game - give it the schema, make gozen check the gate, measure balance with gozen sim battle, and review with gozen book.
weight: 70
---

A Gozen game is plain text with a checker that answers in file and line. That suits a coding agent: it writes YAML, runs `gozen check`, reads the errors, and fixes them until the check passes. You keep the parts an agent cannot judge: whether a line reads well and whether a fight feels right.

## The loop

1. Give the agent the contract: the [Schema](/schema/) page, or the JSON Schema in [data/schema/](https://github.com/nao1215/gozen/tree/main/data/schema), and a working example such as [Go Zen](https://github.com/nao1215/gozen/tree/main/examples/go-zen/game).
2. Give it one rule: `gozen check game` must pass before it says it is done.
3. Ask for numbers, not opinions: `gozen sim battle` and `gozen sim dive` say whether a fight or a dive is too hard, and `gozen sim campaign` whether a war is too easy.
4. Review the result as one document with `gozen book game > book.md`, then play it with `--dev` and leave F9 notes.
5. Hand the notes back: `gozen notes notes --out notes-html` gives a page of screenshots and comments, and each note's `.json` names the place and line.

<!-- verify: run fixture=cave -->
```shell
gozen check game                                       # references, expressions, pictures, text layout
gozen sim battle game --troop cave_boss --level 8                     # win rate of the party at level 8
gozen sim dive game --location cave --floors 5 --boss 5:cave_boss    # a whole dive: levels and wins per floor
gozen book game > book.md                              # everything, as one Markdown file
```

`check` exits 1 on any error, so it works as a gate in a script, a git hook, or CI ([setup-gozen](/cookbook/#check-and-measure-in-ci) installs it in GitHub Actions). `gozen check --format json game` prints the same errors and warnings as a JSON array (`file`, `line`, `col`, `path`, `code`, `message`, `severity`) for a tool that reads them.

## The gozen skill

[skills/gozen](https://github.com/nao1215/gozen/tree/main/skills/gozen) is an Agent Skill: a `SKILL.md` with short references for the minimal game, the required parts, the YAML traps, the error codes and `gozen sim`. Its rule is the loop above: edit, run `gozen check`, fix until it passes.

- Claude Code: copy `skills/gozen` to `~/.claude/skills/gozen`.
- Codex: copy `skills/gozen` to `~/.codex/skills/gozen`.

The agent loads it when you ask it to make or fix a Gozen game.

## Editor completion

The JSON Schema gives editors completion and inline errors for the YAML. With the YAML language server (VS Code's YAML extension, Neovim, Helix), add a line at the top of a file:

<!-- verify: load -->
```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/nao1215/gozen/main/data/schema/data.schema.json
locations:
  camp:
    name: Camp
```

Use `game.schema.json` for `game.yaml`. The JSON Schema checks shapes and types; references between files and expressions are checked only by `gozen check`.

## Tips

- Ask for small rounds: one place, one troop, one event at a time, each ending with a passing check.
- Ask the agent to quote the `gozen sim battle` output after a balance change, instead of describing it.
- Keep the agent away from `assets/` unless you want it to manage files; a missing picture is an error `check` will report, which keeps the agent honest.
- Treat the warnings of flags as part of the gate: a flag set but never read (GZN0112) or read but never set (GZN0113) is most often a typo. Declare a flag kept on purpose in `state.names`. `gozen book` has a Flow section of what each event sets, reads, calls and moves to, with a mermaid graph.
- For dialogue, have the agent draft and you rewrite. `gozen check --only layout` makes sure the result still fits the window.
