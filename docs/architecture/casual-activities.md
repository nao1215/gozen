# Phone games and extensible casual activities

Phone hosting, approved packages and explicit installation are available in Gozen 0.23.0. Runner gameplay and the casual activity model below remain proposals. Public catalog publication is tracked separately from the engine release.

## Delivery order

First, run one standalone Gozen game inside another game's phone without a VM or child process. Then reproduce a scrolling runner with Gozen definitions. General casual-game authoring follows those two working uses. One-button operation is an option, not a restriction on games.

The implemented phone host provides portrait or landscape framing, transformed input and an exit control outside the child's viewport. It suspends its own gameplay and resumes when the child closes. Child resources and saves are isolated; only one level of embedding is supported. [ADR 0063](../decisions/0063-phone-game-packages.md) defines ownership and trust. The [authoring recipe](../examples/phone-games.md) includes YAML and actual captured examples.

## Registry and installation

Host authors choose app IDs, unlock flags, bundled or optional installation, and exact versions or bounded update ranges. Players explicitly download or update an allowed app. New catalog entries do not add apps to a host's declarations, and updates preserve earlier installations and saves.

The repository maintains reviewed release metadata, including required custom PNG icons and immutable package hashes. Signed catalog refresh authenticates new releases against the host's pinned public key. Archive and storage checks preserve existing installations when a download fails. See the [registry contract](../registry/README.md) for package limits, browser mirrors, upstream metadata, review and publication.

Authors can submit games through the [registration form](https://github.com/nao1215/gozen/issues/new?template=game-registration.yml). Submission starts review and does not publish a game automatically.

## Casual activity proposal

A game author should be able to build a short touch-controlled challenge using YAML and assets, launch it from an ordinary story event or phone entry, and use its result in the story. Changing a runner into a catching challenge must not require recompiling Gozen. Authors configure these games through YAML under ADR 0041. The proposal adds no public Go plugin API.

The first test game is a scrolling runner with continuous lateral movement, pickups, obstacles, limited-use actions, increasing speed and course completion. The implementation must reproduce its rules and separately verify rendering and platform support. A second, stationary catching game must use the same rules and actions before the YAML fields are accepted. It must differ in gameplay, not just artwork.

## Input and game decisions

Device input becomes named game actions on simulation ticks. Keyboard and gamepad bindings must offer the same actions as touch. Coordinates are normalized to the activity viewport; resizing cannot change movement speed or swipe distance.

| Input | Candidate play | Author-controlled choice |
| --- | --- | --- |
| Press | Jump, change lane, reverse motion | Which single action fires |
| Hold and release | Charge a jump or time a launch | Charge bounds and release effect |
| Horizontal drag | Continuous steering while the world advances | Dead zone, speed and bounds |
| Directional swipe | Discrete lane change or dodge | Allowed directions and step size |
| Action button plus drag | Steer while spending a limited resource | Resource cost and cooldown |

Continuous drag and discrete swipe are separate modes. A pointer captured for steering cannot also fire a button on release. Swipes emit once after crossing a threshold; a long hold does not generate repeated swipes. Buttons capture independently when steering and action are simultaneous. Cancel, loss of focus and release outside the viewport clear held input without firing an action. The phone's back control is outside this capture region. Playing with one pointer needs another way to trigger the action, such as a separate action phase. Test this on a real touch device in the first prototype.

Players make choices about risk and timing: take a dangerous route for a reward, spend a limited action now or save it, trade speed for control, or release a charge inside a moving target window. A one-button game can offer these choices without growing an inventory or combat subsystem.

## What authors can extend

The proposed YAML fields cover these parts of a game:

- A playfield, fixed boundaries and a camera/scroll direction.
- Actor templates: asset, hit box, movement rule and collision category.
- Authored spawn patterns, repeated segments and seeded weighted choices.
- Named actions combining bounded movement, impulse, cooldown and resource cost.
- Collision rules for pickup, damage, blocking and destruction.
- Local counters, score rules, milestones, duration and completion conditions.
- Presentation bindings for animations, sounds and counter displays.

Templates and patterns use IDs and references so an author can compose reusable parts. Validate missing references, impossible bounds, non-finite numbers, unbounded spawn rates and recursive definitions before play. Put explicit limits on live actors and work per tick; report a bad definition instead of silently dropping gameplay objects. Effects have a fixed evaluation order, including simultaneous pickup/collision and success/failure precedence.

The first implementation supports only the rules and actions needed by the two test games. A new genre that needs a new movement solver or collision shape requires an engine change. Authors can combine the supported rules and actions, but cannot define arbitrary physics or scripts. Choose the YAML field names after both prototypes work with the same definitions.

## Gozen integration and ownership

The host event pauses while one activity owns input and drawing. Launching from a phone uses the same event boundary as launching from the story. Each attempt has its own state; no nested player, second event interpreter, network service or background game loop is needed.

Gamedata owns definitions and validation. A headless rules component below the presentation layer owns local state, seeded randomness and fixed-step updates. Presentation translates device input and draws snapshots; it does not decide scores or mutate story flags. Assign the concrete package and allowed imports in an implementation ADR before adding it.

An activity returns one outcome: completed, failed or cancelled, plus explicitly declared counters such as score. The waiting event applies authored assignments and branches once. Story rewards remain ordinary event logic; reading a score or reopening a result view cannot repeat a reward. Starting a new attempt is explicit. The story rules decide whether another attempt earns another reward. Local counters never implicitly overwrite story variables.

Initial scope excludes saving inside an attempt. Save before entry or after return. Resume after completion must not rerun a timed game to reconstruct its result: the event replay contract needs the recorded result, seed and definition identity. Replays for debugging additionally record normalized actions by tick. This is separate from phone history, which still needs only authored data and flags. Games without activities allocate no activity state and run no activity update loop.

## Implementation order

1. Specify how inputs compete, which component records results for replay, and resource limits. Check the runner's rules against the proposed actions and rules without importing its game-specific content into the engine.
2. Build a runner and a stationary catcher from neutral game definitions, with tests that run without graphics. Demonstrate a new pattern, action and scoring rule through data changes alone. Compare the port's fixed-seed state transitions with the source rules; record intentional differences.
3. Connect one shared host view to events and phone navigation. Verify cancelling, retrying, completing, saving after return, and applying each reward once.
4. Exercise actual touch, keyboard and gamepad operation, desktop/browser exports, graphics and deterministic scripted runs. Add schema, two examples, documentation and dango enforcement before declaring authoring supported.

Implement this after the embedded-game distribution work. Estimate the remaining time after testing the rules without graphics and trying the input prototype. Embedded Gozen games take priority over this activity model. The activity host must fit the ownership and exit contracts established by that work.
