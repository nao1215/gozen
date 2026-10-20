# The boundary between public and internal

What is published is the contract that a game author or a sample really needs. Capitalized identifiers are subject to compatibility, so implementation helpers stay internal.

| Location | Users and responsibility | Compatibility |
| --- | --- | --- |
| `expr`, `text`, `savefile`, `chat` | Authors who handle expressions, translation, the save location and the record of flowing messages | Under the v0 contract |
| `data`, `rpg` | Authors who load and validate definitions and handle state and saves | Under the v0 contract |
| `battle`, `event`, `world`, `dungeon`, `react` | Authors who use or extend the rules, and authors who make characters that react to words and happenings | Under the v0 contract |
| `headless` | Authors and the CLI who check progress without a screen | Under the v0 contract |
| `gozen` (root) | Authors who load a game, read one play with expressions and change it with commands (the minimal entry of ADR 0021) | Under the v0 contract. The older packages above move to internal in stage 6 |
| `gfx`, `play`, `puppet` | Authors who embed display, the screen (`play.Main`, `play.New`) and moving portraits | Under the v0 contract |
| `runner` | The old entry that only calls `play.Main` | Removed in stage 6 (ADR 0021) |
| `internal/<package>/`, `<package>/internal/` | The implementation of the owning package | Not public |
| `internal/layerN/` | Small implementations shared by layer N | Usable only from the same layer or above |
| `internal/testgame/` | Assembly of newly made neutral test data | Test only; a neutral game is newly made in T2 |
| `internal/dev/`, `api/` | API checks during development | Not used at run time |
| `cmd/gozen` | The only CLI. General authoring and checking, and dispatch to the authoring-support subcommands | Not used from the library |
| `x/` | A place for experiments. Create it when a concrete user appears (the empty reserved directory was removed in [ADR 0021](../decisions/0021-query-and-commands.md)) | No compatibility promised |
| `internal/promptgen/`, `internal/mosaic/` (including below) | Implementations dedicated to `gozen art prompt` and `gozen art mosaic`. The entry is each `command` package | Used only from `cmd/gozen` and their own implementation |
| `examples/` | Newly made general-audience samples | Usage examples of the public API |

The public API of layers 1 to 4 was made when the non-rendering core was implemented (T2), and layers 5 and 6 when the display layer was implemented (T3) (for the symbols see [the symbols of design decisions](../decisions/index.md#symbols)). Purposes and ownership are summarized in [the core API](core-api.md) and [the presentation API](presentation-api.md). `api/public.txt:1` is the current list of declarations. Identifiers of a future `x/` would also go in the list, but that does not make them a stable contract.

The dedicated internals for authoring support are not part of the shared implementation of the six layers; the corresponding CLI owns them. For how content is treated see [ADR 0004](../decisions/0004-content-boundary.md), and for the module structure [ADR 0009](../decisions/0009-single-module.md).

A shared internal is not a loophole around the layers. `internal/layerN/` itself may reference only the public packages below it and the shared internals further below. No reverse dependency on a public package of the same layer is made. An internal under an owning package is bound by the same constraints as the owner. Creating an unclassified general-purpose place directly under `internal/` is rejected by the check.

Test assembly is not embedded in the main code. An unpublished feature is internal by default, and if publishing is needed, show the user, purpose and contract and change the ADR and the API list. `dungeon` is published only within the scope of [ADR 0014](../decisions/0014-dungeon-host-adoption.md). The public API is reduced to the minimal form (`gozen`, `play`, `gfx`, `react`, `chat`, `puppet`) by the steps of [ADR 0021](../decisions/0021-query-and-commands.md). The table above is today's public packages. When dealing with the collection of development information or a limited-distribution mechanism, consider only the parts that can be explained generically, and do not include private aggregation machinery.
