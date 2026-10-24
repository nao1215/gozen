# No-code migration

The product direction and boundaries are recorded in [ADR 0041](../decisions/0041-yaml-authoring-and-distribution.md).

| PR | Scope | Completion gate | Status |
| --- | --- | --- | --- |
| 1 | Test filenames and maintenance rules | Content-preserving renames; no runtime change | Merged in #45 |
| 2 | `play run` and shared startup | Real CLI plays, records and restores saves | Merged in #46 |
| 3 | `new`, three templates and no-code onboarding | Templates validate and play without Go source | Merged in #47 |
| 4 | Desktop export and private player | Move the package, hide its source, play and restore with no toolchain; native tests for claimed platforms | Merged in #48 |
| 5 | Web export and generic WASM runtime | Serve under a subdirectory, play a choice, save and reload in a real browser | Implemented in this change |
| 6 | First-party samples and useful library-only features | Samples use the common player; retained features have YAML/CLI coverage | Planned |
| 7 | Public Go API retirement | Existing YAML and pre-migration saves still play; no unintended public application API | After distribution and migration |
| 8 | Post-migration bug audit | End-to-end authoring/distribution, boundary inputs, oversized data and save compatibility; fixes include reproductions | Planned |
| 9 | Refactoring | Remove dead code, duplication and unused abstractions; organize tests by implementation or integration behavior | Planned |
| 10 | Documentation and skill audit | README, site, help, architecture, AGENTS.md and Gozen skill match the implementation; authoring-to-distribution instructions execute | Planned |

Each earlier PR includes its own necessary tests and documentation. The last
three PRs audit the integrated result; they do not defer known bugs, essential
cleanup or accurate instructions until the end.

## Export interfaces

Desktop and web exports will be separate from the existing approval-tree command.
`gozen pack desktop` and `gozen pack web` take a game directory, a new output
directory and an explicit local runtime bundle. The runtime's version and target must match;
there is no implicit download or author-side compilation.

A desktop player locates `game/` beside itself. A web player consumes
`game/files.json`, serves its title from game data, and uses the existing browser
save store. `internal/webfs` currently prefetches every manifest entry; do not
claim progressive lazy loading. Existing sample build recipes use Go and Python
and are maintainer workflows until the generic runtime replaces them.

## Scope decisions

The authoring templates use a fresh random ID by default so identically named
folders do not share saves. An explicit `--id` supports intentional continuity.
Titles are serialized as YAML strings, never substituted as unquoted YAML.
Templates are original neutral examples, not copies of the large showcase games.

Custom Ebitengine loops and arbitrary Go callbacks are not automatically converted
into YAML features. Evaluate their actual game-author use before retaining them.
The no-code workflow must remain understandable without learning a new plugin
language. Preserve the useful engine implementation and its tests internally.
