# ADR 0064: Deterministic casual games inside ordinary Gozen games

Status: Proposed. Date: 2026-11-08. No casual-game YAML or runtime is implemented by this decision.

## Purpose

Authors need to build a scrolling runner, a ricochet target game and a tile-route puzzle with YAML and assets, then distribute each as an ordinary Gozen game. The existing phone host and package registry remain responsible for launching and installing those games. There is no second application format, VM, native plugin interface or nested engine loop.

These three uses establish priorities, not a sample-count limit. Additional complete examples are welcome when they reuse the capabilities and demonstrate a different authoring use. The [gameplay proposal](../architecture/casual-activities.md) defines their controls and rules; this decision defines implementation ownership and acceptance gates.

## Existing constraints

The continuous dungeon controller owns stacked-grid support, stairs, encounters and party position (`internal/layer3/dungeon/free.go:49`). Those rules do not describe a scrolling course or a launched ball. Reuse small geometry operations when their contracts match, but do not add runner modes to the dungeon controller or extract a universal physics framework before a second consumer needs it.

Events suspend through typed requests (`internal/event/event.go:154`). Dialogue saves reconstruct an event from its initial state and integer answers, while screen continuations use bounded request digests and integer responses (`internal/rpg/cursor.go:21`). Neither representation currently describes an activity's definition, seed and counters. A new activity cannot silently become a successful battle during reconstruction, and its real-time input must not be required to load a completed result.

The current touch adapter turns a second finger into a right-click (`internal/play/input.go:264`). Phone hosting transforms the ordinary pointer into child coordinates (`internal/play/phone_game.go:161`). That is insufficient for a game which steers with one finger while using another for an action. Input parity therefore requires an explicit device adapter as well as rules tests.

## Ownership

| Owner | Responsibility and consumer |
| --- | --- |
| `internal/layer2/gamedata` | Typed definitions, references, finite numeric values and authoring limits; consumed by validation, rules and presentation |
| `internal/layer3/casual` | Attempt state, fixed-step rules and immutable result values; consumed by event integration, presentation and headless tests |
| `internal/event` | A typed activity request, validation of its result and authored assignments; consumed by the player and headless session |
| `internal/rpg` | Bounded saved outcome records and structural validation; consumed by event reconstruction and save loading |
| `internal/play` | Device normalization, attempt screen, pause/exit handling, drawing and trace emission |

Saved outcome records are owned by `rpg`; event/player integration converts solver results into those records. `rpg` must not import the higher-layer solver. These dependencies fit the existing downward policy. Do not add an architecture exception. Introduce a package or export only with its first real consumer and meaningful contract coverage. Layer 3 must not import presentation, Ebitengine, wall-clock time or storage. Its state belongs to one attempt, not to a process-global engine. Definitions remain unchanged during play.

Start with a concrete scrolling-course implementation. Keep its update rules separate from static-ball contact and board resolution. Share an input or result type only where actual consumers agree. Avoid a generic entity/component scheduler, author callbacks or a rule-expression VM. Existing expressions can select an activity or handle its outcome; they do not run arbitrary per-contact programs.

The existing planning model already calls scheduled work items `activities`. Casual gameplay must not change that meaning or reuse its types. Choose unambiguous authoring names when the integrated prototype establishes the necessary fields.

## Attempt and input contract

One simulation step is exactly one sixtieth of a second. Drawing never advances gameplay. Presentation samples devices once and produces bounded logical actions for that tick. Attempt time excludes pause, focus loss and host suspension; resuming does not simulate missed wall-clock time. The seed, validated definition identity and ordered actions determine the state. Cosmetic randomness cannot consume gameplay randomness.

Actions distinguish held axes from one-shot presses and pointer begin, move, release and cancellation. Captured pointer IDs stay stable until release or cancellation. Transform each captured pointer through the child's viewport, rather than synthesizing multi-touch from a single mouse position. Game-specific input ownership disables the ordinary two-finger Back shortcut only while that input owner is active; the host's external exit control remains available. Entry, retry, focus loss and exit clear held input and prevent an old release from becoming a new action. A cancelled gesture cannot fire a shot or commit a board move.

An active attempt pauses on focus loss even when the general game's background-pause option is disabled. Forward an explicit cancellation when a captured pointer leaves the active viewport; absence of a held pointer is not a release action. Existing exact replay rejects focus pauses after startup. Keep that rejection until a recorded semantic cancellation and pause/resume contract can reproduce the affected activity state; do not claim that ordinary input tracing already covers it.

The first runner slice uses explicit finite course rows, horizontal steering, forward scrolling, obstacle contact, pickups and a finite finish. Specify and test input, movement, contact and finish ordering before making it author configurable. Preserve reference timing in the compatibility exercise; do not apply the ricochet solver's contact ordering to a row-based runner. Seeded course generation, limited actions, checkpoint rewind and progression follow as separate parity gates. A simple scrolling demo is not a complete port.

Attempt state progresses from ready to running, optionally paused, then to one terminal result: completed, failed or cancelled. Retry creates a fresh attempt or uses an explicitly defined in-memory checkpoint policy. It does not reopen a consumed result. Bound course storage, active objects, action count and contact work. Reject impossible definitions before play; exhausting a runtime work budget produces a failed result with a diagnostic. No overflow, unbounded catch-up loop or silent collision omission is permitted.

## Events, outcomes and saves

Use a dedicated typed request and result submission rather than encoding score data into a choice integer or putting mutable RPG state inside the solver. The request identifies the invocation and validated definition and supplies its seed. Derive invocation identity from deterministic event execution and occurrence, not a process-local token. Allocate the seed once from the game RNG and reproduce that allocation during reconstruction so later story randomness is unchanged; per-tick simulation uses a separate attempt RNG. A result carries the matching identity, terminal status and only the declared bounded counters. The waiting event accepts one result for that invocation; repeated, stale or mismatched submissions fail without applying assignments. Results are local gameplay records, not proof against a player editing a save.

Extend the existing cursor with bounded typed outcome records in encounter order. Each record binds the invocation, definition fingerprint, seed, terminal status and counters. Reconstruction consumes the matching recorded result and runs authored outcome handling on the reconstructed event state once. It neither launches the activity nor injects a fabricated result. Missing, reordered, changed or malformed records fail restoration before replacing live state. Fingerprints must include the referenced gameplay definition, not only the event command that names it.

This also requires auditing dialogue rollback, scene skip, developer seeking and session screen continuations. Stop at an unresolved activity when no recorded result exists. A frontend that has not implemented outcome restoration must reject that continuation explicitly. Preserve the current loading behavior for older schemas and saves; do not introduce migration commands. Decide the new schema/save version with the implementation and tests.

Initial disk persistence is between attempts. Block saves from entry, including the ready screen, until the terminal result has been accepted and the event continuation is reconstructible. A terminal screen with an unsubmitted result is still transient. Do not write a save that implies the attempt can resume at an arbitrary frame. Existing completed saves remain usable. An in-memory retry checkpoint can retain consumed pickups and limited resources according to its rules without becoming a disk-save format. After a result, save/load must preserve its authored progression without awarding it again. Trace replay separately records normalized actions and verifies the resulting state; ordinary save reconstruction needs no action recording.

A phone child owns all these records in its own save namespace. Its completion does not submit an activity result to the parent game. Closing the child discards its unfinished attempt and resumes the parent under [ADR 0063](0063-phone-game-packages.md).

## Implementation gates

1. Build a neutral finite runner through the full authoring path: validation, event request, headless rules, player input/drawing, result handling and save-after-result reconstruction. Two meaningfully different courses must use the same rules. Retry in this first slice starts a fresh attempt; reference checkpoint rewind belongs to the next gate. Keep unfinished fields out of the supported schema and documentation.
2. Establish compatibility with reference movement, collision timing, pickup accounting, limited actions, checkpoint rewind, progression and device controls. Keep reference-specific records outside tracked public material. Record differences and do not advertise full parity until all cases pass.
3. Add the original ricochet game, then the separate board resolver and tile-route game. Extract shared operations only when these consumers demonstrate the need. Each sample includes instructions, finite rounds, results, retry and exit, and works standalone and as a phone package.

For the first gate, test deterministic action sequences, simultaneous contacts and finish, repeated result submission, cancellation, save/load after outcomes, changed-definition rejection, missing/extra/reordered outcome records, restoration of a cancelled outcome, preservation of subsequent story RNG, failed save I/O and bounded-work exhaustion. Check that nonparticipating games create no attempt and run no simulation work. Add enforcing dango tests for these observable contracts when the implementation exists; an ADR-presence test is not enforcement.

Runtime acceptance includes keyboard and pointer operation, focus loss, releasing outside the viewport, restarting without leaked input, parent exit during play, portrait and landscape phone framing, desktop export and browser export. Physical touch and multi-touch require separate device evidence. Unit tests and cross-compilation do not establish that evidence.

## Alternatives and consequences

Hard-coded game templates would make the first examples easier but would not demonstrate author-controlled courses and rules. A general physics or scripting engine would create a much larger compatibility and resource-control contract before these games need it. Extending the dungeon controller would couple unrelated rules and risk existing walking behavior.

The selected design adds a small rules family and explicit result persistence. The persistence and input work are part of the first usable feature, not follow-up polish. It costs more than displaying a moving sprite, but keeps phone hosting unchanged and gives later games a tested lifecycle without requiring an RPG party or combat model.
