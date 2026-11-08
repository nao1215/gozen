# Phone games and extensible casual activities

Phone hosting, approved packages and explicit installation are available in Gozen 0.23.0. Runner gameplay and the casual activity model below remain proposals. Public catalog publication is tracked separately from the engine release.

## Delivery order

First, run one standalone Gozen game inside another game's phone without a VM or child process. Then reproduce a scrolling runner with Gozen definitions. General casual-game authoring follows those two working uses. One-button operation is an option, not a restriction on games.

The implemented phone host provides portrait or landscape framing, transformed input and an exit control outside the child's viewport. It suspends its own gameplay and resumes when the child closes. Child resources and saves are isolated; only one level of embedding is supported. [ADR 0063](../decisions/0063-phone-game-packages.md) defines ownership and trust. The [authoring recipe](../examples/phone-games.md) includes YAML and actual captured examples.

## Registry and installation

Host authors choose app IDs, unlock flags, bundled or optional installation, and exact versions or bounded update ranges. Players explicitly download or update an allowed app. New catalog entries do not add apps to a host's declarations, and updates preserve earlier installations and saves.

The repository maintains reviewed release metadata, including required custom PNG icons and immutable package hashes. Signed catalog refresh authenticates new releases against the host's pinned public key. Archive and storage checks preserve existing installations when a download fails. See the [registry contract](../registry/README.md) for package limits, browser mirrors, upstream metadata, review and publication.

Authors can submit games through the [registration form](https://github.com/nao1215/gozen/issues/new?template=game-registration.yml). Submission starts review and does not publish a game automatically.

## Initial casual-game scope

Prioritize one existing runner port and two original casual games. These are priorities, not a limit on the number of samples. The phone host is implemented; the gameplay capabilities below are proposed and must pass the acceptance checks before being advertised. A catching game or charge-jump game may follow when it can reuse the implemented capabilities and demonstrate a distinct authoring use without requiring a large dedicated subsystem.

| Game | Player decisions | Primary controls | What it verifies |
| --- | --- | --- | --- |
| Runner compatibility exercise | Steer through obstacles, choose risky pickups and spend limited actions | Continuous horizontal steering; the reference game's boost and action bindings | Scrolling patterns, movement, contact timing, pickups, retry state and progression |
| Ricochet targets | Aim a limited number of shots; take a direct target or use a wall rebound to reach a harder one | Pull back, aim and release | Captured aim vectors, bounded launch power, static-wall reflection and target rules |
| Tile-route puzzle | Move a held tile through neighboring cells to arrange several matches before committing | Pick up a tile, drag through adjacent cells and release | Ordered swaps, simultaneous matching, falling tiles, seeded refill and cascades |

The runner is a compatibility target, not another newly designed showcase. Its movement, collisions, retry behavior, progression and input feel require comparison with the reference implementation. A short neutral fixture can test engine rules, but does not establish a complete port. Keep project-specific compatibility records outside this repository; engine fixtures and assets must be original and suitable for general audiences.

Each original casual sample is a small complete game with a start screen, a brief control explanation, a finite round, clear success/failure, results, retry and exit. Use a few authored stages to demonstrate meaningful choices rather than building a campaign. Give each distributable game an original icon and licensed assets. Run it standalone and as an ordinary phone package. Counter and Trail remain phone-host integration examples; they do not prove these new gameplay capabilities.

## Capabilities instead of required genres

Authors choose capabilities through YAML and assets. They can change a course, wall arrangement, target rule, tile set, score rule or round objective without recompiling Gozen. The samples are examples of compositions, not mandatory genre templates.

Keep two rule families separate:

- Continuous movement and contact rules for the runner and ricochet game.
- Discrete board operations for the tile puzzle.

Share action recording, seeded randomness, counters, timers, resource costs, presentation and lifecycle outcomes where their contracts agree. Do not force board swaps into a physics solver or require a party, attributes, combat, character collection or special attacks. A game can be only a score challenge. Authors may use the declared outcome in ordinary events within that same game.

Supported rules are finite engine capabilities, not an arbitrary scripting language or a public Go plugin API. A genuinely new solver or matching rule may require an engine change. Decide exact YAML names after the three uses demonstrate the proposed contracts; do not publish speculative fields as supported schema.

## Input ownership

Device adapters produce named actions on simulation ticks. Keyboard and mouse operation must reach the same gameplay decisions as touch. Coordinates belong to the game's logical viewport, so phone scaling and letterboxing do not change steering or shot power.

| Game | Touch or mouse | Keyboard equivalent |
| --- | --- | --- |
| Runner | Steering and action bindings matching the reference game | Arrow-key steering and explicitly bound boost/action keys |
| Ricochet targets | Press the launcher, pull within a bounded aiming region, release to fire | Left/Right change angle, Up/Down change power, Space or Enter fires |
| Tile-route puzzle | Press a tile, drag through neighboring cells, release to commit | Space or Enter picks up/drops; arrows move the held tile one adjacent cell |

A start/retry press cannot also become the next round's first action. A captured aiming or board gesture cannot activate a UI button when released. Cancellation, release outside the active viewport and focus loss clear held input without firing or committing. Loss of focus pauses an active attempt. For a board gesture, restore the pre-drag board on cancellation. Keep the phone host's exit control outside child capture.

The two original samples must be completable with one pointer. Do not impose that restriction on the reference runner if it has different established controls. Multi-touch, gamepad bindings and actual touch feel need explicit runtime verification; keyboard success alone proves none of them. Dedicated directional-swipe and hold-duration charge mechanics need a playable consumer before becoming supported capabilities; an additional sample may provide that evidence.

## Ricochet rules

Use one launched ball at a time, static walls and static targets. Pull distance controls bounded shot power; pulling does not imply a hold-duration charging mechanic. While a ball is active, aiming another shot is disabled. Targets can be removed or change a counter on contact; this is an authored rule, not structural destruction physics.

Use fixed-step movement and swept contact handling so fast shots cannot pass through thin walls. Define nearest-contact ordering, corner reflections and separation after impact. In the initial sample, each target scores at most once per shot. Bound speed, flight duration and reflections; the sample ends a shot when it reaches its declared limit and returns to aiming. Costs and limits are visible to the player.

Authors can change target positions, walls, shot allowance, target values and the completion threshold independently. The initial engine does not promise moving rigid bodies, spinning objects, joints, stacks or collapsing structures.

## Tile-route rules

Use a bounded rectangular grid and author-defined tile match keys. During a gesture, entering an orthogonally adjacent cell swaps the held tile with its occupant. Revisiting a cell follows the same swap rule. Fast pointer movement must traverse crossed cells in a deterministic order; it cannot jump directly to a distant cell. Specify a stable tie rule for exact grid corners. Keyboard input uses the same adjacent-swap operation.

The introductory stage uses a move budget without a dragging deadline. A second configuration of the same sample demonstrates an optional drag timer, rather than making timed dragging mandatory for every game. Start that timer on the first valid swap for both pointer and keyboard input. At expiry, accept no further swaps and commit the current arrangement exactly once. A committed move consumes one move; cancelling or dropping without any swap consumes none. The board resolves only after commitment:

1. Find horizontal and vertical runs meeting the declared minimum length.
2. Clear their union simultaneously, counting intersections once.
3. Apply gravity in each column.
4. Refill from seeded tile choices in a fixed cell order.
5. Repeat matching until stable, then update the result and allow the next move.

The sample scores cleared tiles and cascades toward a visible target. Authors can change grid size, tile keys, minimum match length, refill weights, score values and the round objective. Fixed introductory boards demonstrate solutions; changing colors alone is not sufficient evidence of extensibility.

No diagonal matching, irregular boards, special-tile combo taxonomy, team system or damage formula is required initially. Bound board size, path work per tick, resolution passes and generated tiles. Exceeding an engine safety budget produces an explicit failed attempt with a diagnostic; it must not silently drop matches or fabricate a win. The rules engine grants no story progress itself; ordinary authored handling of a failed outcome remains available. Choose validated limits during prototype measurements and test the exhaustion path.

## Runtime and result ownership

Gamedata owns definitions and validation. Rules below the presentation layer own attempt state and deterministic updates. Presentation translates input and renders snapshots; it does not decide scores or grant story rewards. Select concrete packages and dependencies in an implementation ADR before changing package boundaries.

An activity within a game returns one completed, failed or cancelled outcome and explicitly declared counters. The waiting event applies assignments once. Reopening a result screen cannot repeat a reward. An independently installed phone game has its own state and saves under ADR 0063; its score does not automatically modify its parent game.

Initial persistence covers game progression and results between attempts. Do not promise saving at an arbitrary instant inside an attempt. The runner's in-memory retry/checkpoint behavior is a separate requirement and must preserve the reference rules. Event restoration must use the recorded outcome, definition identity and seed instead of replaying a timed challenge to reconstruct a reward. Debug replays additionally record normalized actions by tick.

Games that do not use these capabilities allocate no attempt state and run no activity update loop. Keep explicit bounds on live actors, board cells and work per tick. Reject invalid references, non-finite values, recursive definitions and impossible configured bounds before play.

## Delivery and acceptance

1. Establish the runner's compatibility cases and implement only its required reusable rules. Compare deterministic state transitions and separately check presentation and controls. Record differences rather than calling a partial port complete.
2. Build the ricochet sample with static reflections and limited shots. Prove that a changed layout and scoring objective need only YAML and assets.
3. Build the tile-route sample with the separate board rules. Prove data-only changes to board dimensions, matching and refill/scoring choices.
4. For every supported game, verify starting, cancelling, losing focus, retrying, completing, restoring progression and returning from the phone. Test deterministic replay, failed I/O and resource limits, then desktop and browser exports and real touch operation. Add schema documentation and dango coverage for observable contracts.

Evaluate additional samples by completion cost, reuse and the distinct authoring capability they verify, rather than by a fixed count. Defer additions that require a large dedicated subsystem. Arbitrary physics and arbitrary code remain outside this proposal. The implementation is ready to advertise only when each promised capability has a playable example and the corresponding runtime evidence.
