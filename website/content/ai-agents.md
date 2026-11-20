---
title: Using AI agents
description: Let a coding agent write a Gozen game - give it the schema, make gozen check the gate, measure balance with gozen sim battle, and review with gozen book.
weight: 70
---

A Gozen game is plain text with a checker that answers in file and line, so a coding agent can write YAML, run `gozen check`, and fix the errors until the check passes. You judge whether a line reads well and whether a fight feels right.

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

`check` exits 1 on any error, so it can gate a script, a git hook or CI ([setup-gozen](/cookbook/#check-and-measure-in-ci) installs it in GitHub Actions). `gozen check --format json game` prints a version-1 result (`version`, `command`, `status`, `data`) whose `data` is the array of errors and warnings (`file`, `line`, `col`, `path`, `code`, `message`, `severity`).

## The gozen skill

[skills/gozen](https://github.com/nao1215/gozen/tree/main/skills/gozen) is an Agent Skill: a `SKILL.md` with short references for the minimal game, the required parts, the YAML traps, the error codes and `gozen sim`, and the loop above as its rule.

- Claude Code: copy `skills/gozen` to `~/.claude/skills/gozen`.
- Codex: copy `skills/gozen` to `~/.codex/skills/gozen`.

The agent loads it when you ask it to make or fix a Gozen game.

## Editor completion

With the YAML language server (VS Code's YAML extension, Neovim, Helix), the JSON Schema gives completion and inline errors. Add a line at the top of a file:

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
- Keep the agent out of `assets/` unless you want it to manage files; `check` reports a missing picture as an error.
- Treat flag warnings as part of the gate: a flag set but never read (GZN0112) or read but never set (GZN0113) is usually a typo. Declare a flag kept on purpose in `state.names`. The Flow section of `gozen book` shows what each event sets, reads, calls and moves to, with a mermaid graph.
- For dialogue, let the agent draft and rewrite it yourself; `gozen check --only layout` confirms it still fits the window.
