# ADR 0039: Shared YAML path selection

Status: Accepted.

## Users and problem

`chat.Script.YAML` and `react.Book.YAML` expose fragments of opaque definitions. Their identical path walkers had separate copies of the same malformed-index bug. Neither public package may import the other, and a definition-layer helper would point upward from the foundation-layer chat package.

## Decision

Own the common selector in `internal/layer1/yamlpath`. It accepts source bytes and a logical path and returns ordered YAML or an error wrapping `fs.ErrNotExist`. It has no knowledge of characters, messages, game definitions, state or rendering. Both public entry points call it directly.

Allow the existing YAML vendor in the shared foundation layer. This is the only new dependency permission; no lateral dependency, reverse dependency, excluded source file or public identifier is added. The module already depends on this parser, so no dependency version changes.

Remove the unused `internal/dungeon` relay and move its contract tests to `internal/layer3/dungeon`, which already owns the rules. This completes the internal ownership described by ADR 0021 without growing the public API.

## Alternatives

- Keep duplicate selectors: fixes and edge-case coverage can diverge.
- Make chat or react own a public selector: creates a lateral dependency or an unnecessary public helper.
- Put the selector in layer 2: introduces an upward dependency from chat.
- Add a generic callback-based traversal abstraction: requires adapters solely to avoid naming the parser used by both consumers.

## Verification

Existing public YAML tests cover fragment extraction and missing paths. Shared tests reject incomplete indexes and empty path components. Architecture checks enforce the layer and vendor permissions, and the public API inventory stays unchanged.
