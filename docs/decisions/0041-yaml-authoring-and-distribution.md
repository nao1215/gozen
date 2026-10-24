# ADR 0041: YAML authoring and distribution without a programming toolchain

Status: Accepted for phased implementation. Date: 2026-10-24.

## Users and product boundary

Gozen serves authors who make RPGs, visual novels and strategy games with YAML
and assets. Creating, checking, playing and eventually distributing a supported
game must not require Go source, a compiler, Git, Python or a custom host.
Go and Ebitengine remain implementation technologies.

The compatibility boundaries are documented CLI behavior, YAML schema versions,
game identity, and save formats. Existing Go packages remain available during
migration. Removing their public API is the final stage, after first-party games
and their tests have working replacements. Existing YAML and saves must keep
working through that removal.

This changes the embedding-first direction of ADR 0007 and ADR 0021. Their
current APIs remain implemented until the last stage. ADR 0016's single
author-facing CLI remains `gozen`; a private player executable may be added as
a release artifact, not as a second authoring interface. No dependency-layer
exception is authorized by this decision.

## Implementation sequence

1. **Create, check and play.** `gozen new DIR --template rpg|novel|campaign`
   generates small playable games containing YAML, original placeholder assets,
   and instructions. `gozen play run DIR` uses the same internal runtime as
   `play.Main`. Templates belong to command tooling under `cmd/internal/starter`.
   A new random game ID avoids collisions when two authors use the same folder
   name; `--id` supplies a deliberate identity. Never regenerate an existing
   game's ID. Publication uses `cli.Publish`: staging and overwrite protection,
   not a claim of atomic directory visibility.
2. **Desktop distribution.** Assemble a prebuilt private player, game data,
   runtime metadata and license notices. Resolve game data relative to the
   executable, not the current directory. Start with explicit local, matching
   runtime artifacts. Do not compile the author's game or download an unpinned
   runtime. Add platform release builds and real packaged-game smoke tests.
3. **Browser distribution.** Assemble one generic WASM player, its matching
   `wasm_exec.js`, HTML and a game manifest. Copy images without requiring Python
   conversion. Verify a real browser run, choices, persistence and subdirectory
   hosting. Browser saves remain origin-local.
4. **Simplify feature access.** Move first-party examples to the common player.
   Preserve gameplay already expressed in YAML. Add focused built-in features
   where authors need them; do not recreate arbitrary Go callbacks as a new
   programming language. Audit isolated chat, reaction and portrait demos.
5. **Retire public Go APIs.** Move implementation to its owning internal layers,
   update first-party imports, remove redundant wrappers and replace public-API
   inventory checks with boundary checks. Restore pre-migration save fixtures
   and continue playing them; package moves alone do not change save versions.

After API retirement, audit the complete authoring-to-distribution workflow for
bugs, refactor remaining dead or duplicate code, and reconcile all documents and
skills with actual behavior. These are separate final review stages; every
preceding change still includes its own tests and accurate documentation.

Desktop distribution uses a private player and an explicit local runtime bundle.
Browser distribution uses the same private player with a WASM build and the matching Go JavaScript runtime. The `player` startup component depends on presentation, game data and `webfs`; engine layers do not depend on it. Feature migration, API retirement and the final audits remain planned.
The detailed progress and acceptance criteria are in
[the migration plan](../development/no-code-migration.md).

## Distribution constraints

`pack release` keeps its existing approval-gated asset-tree meaning, and
`pack trial` keeps its content-selection meaning. Desktop and browser exporters
will consume prepared game trees. They must not bypass an asset approval policy.
A shared selector must distinguish runtime files and notices from Go sources,
repository metadata, test saves, captures and authoring records, reject unsafe
symlinks and reject output/input overlap.

The generic runtimes must identify their engine version and target. Runtime
bundles are built by maintainers, not authors. Cross-compilation alone does not
prove execution on the target platform. macOS signing and notarization must not
be implied by a plain directory export.

## Verification and messaging

The authoring loop is tested through the real CLI, including rendered games,
state changes and saves. Generated templates must pass full `gozen check` and
contain no Go source or module files. Existing destinations remain untouched.

The initial message is: create and play with YAML and assets, no programming
required. YAML editing and terminal use are still required; there is no visual
editor. Do not advertise toolchain-free export until its release artifacts and
commands work. Keep advanced Go documentation during the migration, outside the
beginner's path.
