# Design decisions

This directory records design decisions about Gozen's layout, public contract, dependencies, and checks. They are listed in number order. There are three statuses.

- Accepted: the decision as written is still in effect.
- Partly changed: a later ADR changed part of the decision. The later ADR is listed in the "Changed or superseded by" column, and the changed parts are also noted at the top of the original ADR.
- Superseded: a later ADR replaced the whole decision. Read the replacing ADR for the current decision.

| ADR | Decision | Status | Changed or superseded by |
| --- | --- | --- | --- |
| [0001](0001-modules-and-layout.md) | D1 Modules and layout | Partly changed | The multi-module layout is in [0009](0009-single-module.md), the CLI layout in [0016](0016-single-cli.md) |
| [0002](0002-public-api-scope.md) | D2 Scope of the v0 public API | Partly changed | New public packages are in [0014](0014-dungeon-host-adoption.md), [0015](0015-reaction-and-chat.md), and [0020](0020-moving-portraits.md). Where experiments live is in [0017](0017-dev-balance-panel.md). The list of public packages and the reservation of `x/` are in [0021](0021-query-and-commands.md) |
| [0003](0003-publication-content.md) | D3 Published content and check data | Accepted | |
| [0004](0004-content-boundary.md) | D4 Boundary between the core and authoring tools | Partly changed | Layout is in [0009](0009-single-module.md), the CLI entry point in [0016](0016-single-cli.md) |
| [0005](0005-examples-and-assets.md) | D5 Examples and assets | Accepted | |
| [0006](0006-platforms-and-ci.md) | D6 Supported environments and CI | Partly changed | Per-module checks are in [0009](0009-single-module.md). The current CI layout is in [CI layout](../development/ci.md) |
| [0007](0007-adoption.md) | D7 Embedding through the public API | Accepted | |
| [0008](0008-architecture-and-api.md) | D8 Layers and the minimal API | Accepted | |
| [0009](0009-single-module.md) | D9, D10 Adoption path and the change to a single module | Accepted | |
| [0010](0010-core-ownership.md) | T2 Owner of the shared model and rules | Partly changed | The public dungeon package is in [0014](0014-dungeon-host-adoption.md). The YAML type contract, the shape of effects and listeners, and the save entry point are in [0021](0021-query-and-commands.md) |
| [0011](0011-presentation-ownership.md) | T3 Ownership of presentation and the launch boundary | Partly changed | The public `play.Run` and `sound` were removed in [0021](0021-query-and-commands.md) |
| [0012](0012-cli-ownership-and-output.md) | T4 Ownership and output of the CLI | Partly changed | The entry point is in [0016](0016-single-cli.md), where win-rate calculation lives in [0017](0017-dev-balance-panel.md), option syntax in [0018](0018-gnu-options-and-help.md), and diagnostic codes in [0024](0024-command-tree-and-diagnostic-codes.md) |
| [0013](0013-battle-hooks-and-meter-limits.md) | SW1 Battle events and variable limits | Accepted | |
| [0014](0014-dungeon-host-adoption.md) | SW2 Custom dungeon actions and adoption by host games | Partly changed | The public `dungeon` was folded into commands and expressions in [0021](0021-query-and-commands.md) |
| [0015](0015-reaction-and-chat.md) | Character reactions and chat | Partly changed | The public book and script types became opaque types in [0021](0021-query-and-commands.md) |
| [0016](0016-single-cli.md) | A single `gozen` CLI | Partly changed | The shape of options and help is in [0018](0018-gnu-options-and-help.md), the command tree in [0024](0024-command-tree-and-diagnostic-codes.md) |
| [0017](0017-dev-balance-panel.md) | Development balance display and shared win-rate calculation | Accepted | |
| [0018](0018-gnu-options-and-help.md) | GNU-style CLI options and a consistent help format | Partly changed | The command tree, `check --only` and `gozen help` with several words are in [0024](0024-command-tree-and-diagnostic-codes.md) |
| [0019](0019-engine-languages.md) | Engine text in English and Japanese, selected by language | Accepted | |
| [0020](0020-moving-portraits.md) | Moving a single illustration with diffs and mesh deformation (puppet) | Accepted | |
| [0021](0021-query-and-commands.md) | Read with expressions, change with commands (constrained but simple; the public API in its smallest form) | Accepted (stages 0 to 4 done) | |
| [0022](0022-maps-and-painted-scenes.md) | Maps and painted scenes: rules are points, looks are pictures | Accepted (stages 0 to 4 done) | |
| [0023](0023-verified-docs.md) | Checking the images and steps in the README and the site (image ledger, running the steps, links) | Accepted | |
| [0024](0024-command-tree-and-diagnostic-codes.md) | A command tree of six tools, and diagnostic codes | Accepted | |
| [0025](0025-replay-to-roll-back-skip-and-resume.md) | Replay to roll back, skip and resume (`schema_version: 4`, save format version 2) | Accepted (stages 0, 2 and 2b done) | |
| [0026](0026-read-lines-and-skipping.md) | Read lines keyed by their source, a versioned record, skip mode and scene skip | Accepted (done) | |
| [0027](0027-text-languages.md) | The languages of a game's text (source to translation) | Accepted (done) | |
| [0028](0028-staging-and-voice-words.md) | The words of staging and voice (`stand`, the map forms of `effect`, `music`, `wait`, `sfx`; `say.voice`) | Accepted (done) | |
| [0029](0029-dev-panel.md) | The development panel (F8) and one index of where names are written and read | Accepted (stage 0 done) | |

The source of truth for the current public packages is `api/public.txt` and [Boundaries](../architecture/boundaries.md), not this list of ADRs.

When changing a decision, record the current constraint, the concrete users, the alternatives, and the impact. A change that hides a dependency violation by adding configuration exclusions is not accepted. When an ADR changes, fix its status line and the status and change columns of this table in the same change.

## Symbols

The symbols in ADRs and design documents are stages of the initial implementation before publication and design decision numbers. They are not version or API boundaries. Do not use them in new documents. Describe the content instead, for example "when the presentation layer was implemented".

| Symbol | Meaning |
| --- | --- |
| D1 to D10 | The 10 design decisions made before the initial implementation. They are written up in ADR 0001 to 0009 (the "Decision" column of the table above) |
| T1 | Foundation. Modules, layer dependency checks, the public API list, development documents |
| T2 | The core that does not draw (layers 1 to 4) and a neutral game for tests |
| T3 | Drawing, sound, and running (layers 5 and 6), and the dungeon |
| T4 | The generic CLI and the atago E2E |
| T5 | Authoring support (`gozen prompt`, `gozen mosaic`) and the ONNX cgo build |
| T6 | The sample game, the README, and capturing the introduction images |
| T7 | CI and repository policy (CONTRIBUTING, SECURITY, CHANGELOG) |
| SW1 | Stage 1 of putting a host game on Gozen. Common battle events and a variable meter limit (ADR 0013) |
| SW2 | Stage 2 of the same. Custom dungeon actions and adoption by host games (ADR 0014) |
