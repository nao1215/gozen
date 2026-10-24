# Product and implementation boundaries

| Surface | Contract |
| --- | --- |
| CLI | Documented operands, options, output envelopes, diagnostic codes and exit values |
| Game YAML | Versioned by `schema_version`; definitions and assets remain game-relative |
| Saves | Game identity, format version and supported restoration behavior |
| Go packages | Internal implementation; no public application API |
| `cmd/gozen` | Author-facing executable |
| `cmd/gozen-player` | Private runtime for desktop and browser exports |
| `examples/` | Neutral game directories and maintainer integration tools |

`make boundary` rejects application packages outside `internal/`, including files selected only by other operating systems or build tags. `make arch` checks downward dependencies. Shared code must have an owning layer; internal placement alone does not authorize an import.

Production CLI adapters depend on their domains, CLI I/O and diagnostics. Domain packages cannot import adapters. The content boundary remains [ADR 0004](../decisions/0004-content-boundary.md): production-specific capabilities belong only to the permitted prompt and mosaic packages.

Test assembly may compose layers without becoming runtime code. Keep fixtures original and neutral. Public Go wrappers, their inventory generator and the session-to-screen bridge were removed under [ADR 0041](../decisions/0041-yaml-authoring-and-distribution.md); use the CLI for game authoring and distribution.
