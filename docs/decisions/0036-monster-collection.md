# ADR 0036: Collect, raise and breed monsters

Status: Accepted (2026-10-22). Home: `internal/layer2/gamedata` for definitions and reachability, `internal/rpg` for saved instances and reserve, `internal/event` for transitions, `internal/play` for staging and ranch, and `cmd/gozen` for author checks.

## Users and problem

An RPG author wants a defeated creature to ask to join, wants to raise distinct individuals, and wants to combine them into new species. Players need to see the offer and the fusion, manage a party of four and a reserve, and consult the monster book. Authors need to find bad recipes and unreachable species from a terminal before playtesting.

## Decision

Schema version 6 adds `monster_families` (IDs to display names), `enemies.<id>.family`, `rank`, `monster_class`, and optional `recruit: {chance, class, level, bonus_item, bonus_skill, bonus}`. A recruit class overrides the species' `monster_class`; the latter also lets a species available only by breeding have growth. Each class uses the existing stat formulas, experience curve and skills learned by level. A positive recruit chance is rolled once for each defeated member of a won troop. Owning the named item or having the named skill in the active party adds `bonus` to the chance; both bonuses can apply, and the final probability is capped at one. The item is not consumed. Bosses cannot be recruited or bred.

The shared event runner stages the monster's rise with a portrait entrance and flash, says its line, then asks Yes/No as a regular `choice` request. A Yes creates a unique, saved actor instance whose name defaults to the species name. A full active party sends it to the reserve. `bestiary.<id>.caught` counts each accepted recruit and bred child, while `seen` and `defeated` keep their ADR 0035 meaning. The book shows family, rank, recruitability and the caught count.

The active party remains at most four. All joined actors outside it are in the reserve. The ranch screen is opened with `open: ranch`, offered in the Go Zen inn, and lists stats and active or reserve placement. `reserve: {in: INSTANCE, out: INSTANCE}` swaps a reserve actor into an active slot. `release: INSTANCE` permanently removes an owned monster, with a confirmation on screen. These commands preserve the party limit; release cannot leave an empty active party. `reserve` and `monster.<instance>` expressions read the state, including the species' `family` and the instance's `plus`.

`breeding: {min_level, skill_slots, pairs, families}` holds the recipes. `breed: {parents: [INSTANCE, INSTANCE], skills: [SKILL]}` requires two distinct owned monsters at or above the minimum level. A specific species pair is symmetric and wins over a family pair; it may require a higher `min_level` or combined `min_plus`. A family rule is symmetric and picks the highest ranked listed child whose rank is no greater than the mean parent rank, breaking ties by species ID. No eligible child means the pair cannot breed. Selected skills must be distinct, known by a parent and within `skill_slots`. The two parents leave; the level-one child keeps the selected skills. Its `plus` is the sum of parental plus values plus at least one point from their levels; each plus point adds a small amount to each stat as the child grows. The ranch previews the child, showing `?` until the book has seen it, and confirms before a fade, flash and child appearance. All mutation goes through the event command, including when the screen initiated it.

`gozen check` checks probabilities, classes, family IDs, ranks, unknown species and skills, duplicate unordered pairs, direct parent-to-self recipes, and invalid requirements. It warns when a non-boss species has no path from a capturable species. `gozen sim breed <game-dir> [--format json]` prints capturable species, the fewest generations and one recipe for reached species, and unreachable species. `gozen book` includes recruitment and breeding definitions. GZN1112 reports invalid rules and GZN0116 reports unreachable species.

## Alternatives

1. Keep recruited monsters as a count of each species. This cannot preserve separate levels, inherited skills, names or reserve placement.
2. Make capture and breeding Go methods on the public Session. YAML commands keep them available to scripts, the policy, exploration and traces without growing the public Go API (ADR 0021).
3. Let breeding copy the parents. Consuming them makes the choice consequential and keeps the reserve bounded by play rather than a second hard limit.
4. Choose family offspring randomly. A rank rule is reproducible, previewable and inspectable with `gozen sim breed`.
5. Treat every cycle as an error. Family breeding can intentionally return an existing species with a higher plus value; only a direct specific pair yielding one of its parents is rejected as a likely typo.

## Consequences and limits

- Instance IDs are stable within a save and the loaded game definition is copied for each session. Restoring a save reconstructs instance templates from current enemy and class definitions.
- `gozen sim breed` treats capturable species as repeatable and parents as raisable to their class maximum. It tracks inherited plus values up to the largest recipe requirement. Its generation count is a recipe graph measure, not a prediction of battles or time needed to level them.
- A species' rank is an authoring choice, not a battle power guarantee. Balance still needs `gozen sim battle` and `gozen sim dive`.
- No new public Go entry point is added. Schema types and commands grow under schema version 6.
