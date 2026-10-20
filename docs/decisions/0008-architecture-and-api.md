# ADR 0008: Downward dependencies and a small API

Status: Accepted. Date: 2026-10-18. Corresponds to: D8.

## Context

We keep the core reusable without a screen, and keep the public API at a size that can be understood. Packages that refer to each other, or too much public surface, make use without drawing and compatibility harder.

## Decision

Dependencies are stacked in the order foundation, model, rules, progression, presentation, entry point, and only references from upper layers to lower layers are allowed. Sideways dependencies between public packages of the same layer, reverse dependencies, and dependencies from a stable package to `x/` are forbidden. Ebitengine is limited to presentation and the entry point.

An internal package with an owner is subject to the same constraints as its owning layer, and a shared internal package is placed under `internal/layerN/` and can be used from the same layer or above. A shared internal package itself depends only on lower layers. `savefile` is layer 1 and the `dungeon` rules are layer 3, but no promise is added to make the latter public.

The go-arch-lint v1.19.0 version 3 configuration expresses these dependencies, and runs in `make lint` and CI. In T1 empty reserved components are allowed, but unknown packages and vendor are not. Tests assemble the upper layers, so they are excluded from the runtime check, and dependencies from the library to test helpers are not allowed.

The API exposes only what real users need. It uses a small `Config`, a uniform `Load(fsys fs.FS, path string) (T, error)`, and extension interfaces of one or two methods, and aligns naming, context, and errors with the [API guidelines](../architecture/api-guidelines.md). A list of public identifiers produced with the standard go/ast is saved in `api/public.txt`, and tests detect additions and removals.

## Reasons

Drawing dependencies and growth of the public surface can be detected without relying only on the attention of reviewers. It avoids excess interfaces and configuration mechanisms, and keeps the entry points users must learn uniform.

## Consequences

A feature used by several packages splits its responsibilities into a shared internal implementation, a small contract, and assembly in an upper layer. Exceptions are not handled by adding them to the configuration. A necessary exception is decided only after its alternatives and reasons are recorded in an ADR. The identifier list is not a check of signature compatibility, so godoc and contract tests are maintained as well.
