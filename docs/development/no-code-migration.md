# No-code migration

The product direction and boundaries are recorded in [ADR 0041](../decisions/0041-yaml-authoring-and-distribution.md).

| PR | Scope | Completion gate | Status |
| --- | --- | --- | --- |
| 1 | Test filenames and maintenance rules | Content-preserving renames; no runtime change | Merged in #45 |
| 2 | `play run` and shared startup (now `run`) | Real CLI plays, records and restores saves | Merged in #46 |
| 3 | `new`, three templates and no-code onboarding | Templates validate and play without Go source | Merged in #47 |
| 4 | Desktop export and private player | Move the package, hide its source, play and restore with no toolchain; native tests for claimed platforms | Merged in #48 |
| 5 | Web export and generic WASM runtime | Serve under a subdirectory, play a choice, save and reload in a real browser | Merged in #49 |
| 6 | First-party samples and useful library-only features | Samples use the common player; retained features have YAML/CLI coverage | Merged in #50 |
| 6a–6f | [CLI contract migration](../decisions/0042-command-and-output-contract.md): reporting, primary commands, capture, analysis, production tools, help | Each batch updates actual CLI E2E and docs; no CLI-to-CLI execution | 6a merged in #51; `run` and `export` merged in #52. Named selectors and direct report/distribution commands complete 6b. Capture, analysis output, production tools and machine-readable help remain planned |
| 7 | Public Go API retirement | Existing YAML and pre-migration saves still play; no unintended public application API | After distribution, samples and CLI migration |
| 8 | Post-migration bug audit | End-to-end authoring/distribution, boundary inputs, oversized data and save compatibility; fixes include reproductions | Planned |
| 9 | Refactoring | Remove dead code, duplication and unused abstractions; organize tests by implementation or integration behavior | Planned |
| 10 | Documentation and skill audit | README, site, help, architecture, AGENTS.md and Gozen skill match the implementation; authoring-to-distribution instructions execute | Planned |

Each PR includes its necessary tests and documentation. The remaining work is
grouped into three PRs to avoid repeated changes to the same CLI dispatchers and
documentation:

1. Direct PNG/GIF capture and real CLI graphics tests (6c).
2. Analysis and production command output, machine-readable help, and the full
   CLI contract audit (6d–6f).
3. Public API retirement (7), followed by the bug, refactoring, documentation and
   skill audits (8–10) on the resulting implementation.

PR #53 completes 6b. Prepare the next PR while CI runs for its predecessor;
rebase it after that predecessor merges. Run focused checks during implementation
and the required complete checks before review. API retirement remains last;
grouping the audits does not remove their completion gates.

## Export interfaces

Desktop and web exports use `gozen export GAME --out DIR --runtime DIR`.
`--target web` selects a browser package; a desktop OS-ARCH target selects a native player. The runtime's version and target must match;
there is no implicit download or author-side compilation.

A desktop player locates `game/` beside itself. A web player consumes
`game/files.json`, serves its title from game data, and uses the existing browser
save store. `internal/webfs` currently prefetches every manifest entry; do not
claim progressive lazy loading. The showcase site builds one common WASM player and uses Python/Pillow to compress
its images. These are maintainer tasks; `export --target web` needs neither Go nor Python.

## Scope decisions

The authoring templates use a fresh random ID by default so identically named
folders do not share saves. An explicit `--id` supports intentional continuity.
Titles are serialized as YAML strings, never substituted as unquoted YAML.
Templates are original neutral examples, not copies of the large showcase games.

Custom Ebitengine loops and arbitrary Go callbacks are not automatically converted
into YAML features. Evaluate their actual game-author use before retaining them.
The no-code workflow must remain understandable without learning a new plugin
language. Preserve the useful engine implementation and its tests internally.

## First-party feature audit

| Consumer | Author-facing replacement or retained purpose |
| --- | --- |
| RPG, novel, campaign and stacked-space samples | `gozen run examples/NAME/game`; browser pages share `cmd/gozen-player` |
| Development flags, capture scripts and hot reload | `gozen run GAME --dev`; documentation images exercise this command |
| Moving portraits | Actor `puppet`, `pose` and `effect` YAML use the built-in renderer; the standalone puppet program remains a maintainer animation inspection tool |
| Headless and minimal Go examples | Temporary API contract examples, to be folded into internal integration tests during PR 7; authors use `gozen sim` |
| Standalone `chat` and `react` APIs | No first-party game consumes these packages; retain their internal rules and tests during API retirement without adding an unused YAML scripting interface |

The sample integration tests still cover story branches, translations and campaign
outcomes. Graphics tests start the real CLI; image capture builds it once for all
four game directories. No sample-specific native or WASM startup remains.
