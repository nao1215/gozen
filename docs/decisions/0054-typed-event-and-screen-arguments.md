# ADR 0054: Typed arguments for reusable events and screens

Status: Accepted for phased implementation (2026-11-01). Event calls and plain screen arguments implemented; screen continuation saves and expression-form target fields remain planned.

See [the executable event-call slice](../development/event-arguments.md) for supported syntax and remaining limits.

## Authors and current constraint

Workshop, farming and training authors need one assignment screen for several
activities and one conversation for several residents. Copying a screen or event
for every ID or date makes small rule changes hard to review. Arguments must name
real objects, survive a suspended choice and resume, and never depend on a mutable
global variable containing the last selection.

Current evidence (before implementation):

- `internal/layer2/gamedata/types.go:1493` defines events without parameters;
  `types.go:1707` defines author screens without parameters and `types.go:1719`
  binds an entry to a literal entity ID.
- `internal/layer2/gamedata/command.go:98` and `:125` store literal `call` and
  `open` names. `internal/event/event.go:1022` pushes the named command list;
  `event.go:313` has no argument environment per frame.
- `internal/play/author_screen.go:48` renders screen entries against state alone.
- `internal/rpg/cursor.go:12` records an event-start replay cursor, not execution
  frames. [ADR 0025](0025-replay-to-roll-back-skip-and-resume.md) defines replay
  boundaries, including `open`.

This decision extends [narrative authoring](0043-narrative-authoring.md) and
[planning](0053-planning-components.md). It does not claim that the syntax below
loads today. Existing literal forms and parameterless games keep their behavior.
No package moves, dependency exceptions or public Go interfaces are required.

## Declaration and invocation

An event or author screen declares `params`, a mapping of names to types. Every
parameter is required; there are no defaults, nullable values or implicit captures.
Names are identifiers, unique within the declaration. The initial types are:

| Type | Accepted value and validation |
| --- | --- |
| `number` | Finite numeric scalar; no bool or string conversion |
| `bool` | Boolean scalar only |
| `string` | String scalar only |
| `entity` | String ID of a declared planning entity |
| `activity` | String ID of a declared planning activity |
| `actor` | String ID of a declared narrative actor, including classless actors |
| `date` | Valid ISO calendar date `YYYY-MM-DD`; no implicit timezone |

Reference types are validated IDs, not mutable objects or pointers. An entity and
an actor remain separate namespaces; no matching-name conversion is inferred.
A valid date is not automatically an open day or inside a booking range. The
operation using it validates those constraints. References to inactive residents
or cancelled activities still exist; action eligibility belongs to authored guards
and operation contracts, not to the type system. The expression form of `say.who`
requires an actor ID; the existing literal form still permits a display name.

Keep `call: inspect` and `open: candidates` as shorthand for an invocation with
no arguments. Add mapping forms `call: {event: inspect, args: {...}}` and
`open: {screen: candidates, args: {...}}`. Names of events and screens remain
literal, so the dependency graph is statically inspectable. Each `args` value is
an ordinary Gozen expression, as in planning rule arguments; a literal string
therefore includes expression quotes. Unknown or missing arguments are errors.

Evaluate arguments once, in declared parameter-name order (ascending), against the
caller's environment. Evaluation must be pure and consume no random draws. Validate
all values before creating the callee frame or screen. The immutable result is
visible through `arg.name`; expressions in a call read the caller's `arg`, not a
partially built callee environment. Computed argument keys are not supported.
Reject unknown `arg` fields and use outside a declared scope. Do not silently
coerce a reference from another typed namespace, even if its ID spelling matches.

Fields that currently accept literal object IDs retain that form and may add an
explicit expression form, for example `entity: {expr: 'arg.resident'}`. Use it for
`plan.entity`, `plan.activity`, `say.who` and `ScreenEntry.target` as each consumer
is implemented. Validate the expression's result against the receiving field's
type. Ordinary string literals, including strings with braces, never become ID
macros. Expressions return the ID scalar for comparisons and existing lookups;
static parameter metadata retains its reference type for checking typed uses.
No new general evaluator, arbitrary callback or text substitution engine is added.

## Neutral author example

These are **future schema fragments**, not a runnable example for the current CLI.
Assume planning entities `potter` and `weaver`, activities `prepare` and `pack`,
actor `guide`, and a declared numeric entity attribute `vigor`. Event declarations
belong in a data file; `screens` belongs in `game.yaml`.

```yaml
events:
  encourage:
    params: {resident: entity, speaker: actor, amount: number}
    do:
    - say:
        who: {expr: 'arg.speaker'}
        text: 'Let us prepare for the next task.'
    - plan:
        op: set
        entity: {expr: 'arg.resident'}
        attribute: vigor
        value: 'self.vigor + arg.amount'
    - call:
        event: report
        args: {resident: 'arg.resident'}
  report:
    params: {resident: entity}
    do:
    - notice: 'Ready: {arg.resident}'
  visit:
    do:
    - call:
        event: encourage
        args: {resident: '"potter"', speaker: '"guide"', amount: '2'}
    - call:
        event: encourage
        args: {resident: '"weaver"', speaker: '"guide"', amount: '2'}
```

A candidate screen reuses a task argument; its entries may call a shared event.
The two fixed candidates keep this example small. Collection-generated rows are a
later feature, not a requirement to duplicate the screen for each task.

```yaml
screens:
  candidates:
    params: {task: activity}
    title: 'Assign {arg.task}'
    entries:
    - label: Potter
      do:
      - call:
          event: assign_task
          args: {task: 'arg.task', resident: '"potter"'}
    - label: Weaver
      do:
      - call:
          event: assign_task
          args: {task: 'arg.task', resident: '"weaver"'}
```

```yaml
events:
  assign_task:
    params: {task: activity, resident: entity}
    do:
    - choice:
      - label: Confirm assignment
        do:
        - plan:
            op: assign
            activity: {expr: 'arg.task'}
            entity: {expr: 'arg.resident'}
      - label: Cancel
  choose_work:
    do:
    - open: {screen: candidates, args: {task: '"prepare"'}}
    - open: {screen: candidates, args: {task: '"pack"'}}
```

The ordinary choice supplies explicit confirmation on every screen kind. This
proposal does not reinterpret entry text as confirmation on screens that currently
execute immediately. A date parameter uses the same invocation shape, for example
`args: {day: '"2030-06-12"'}` with `params: {day: date}`; booking is separate.

## Scope, continuation and confirmation

A named event owns its parameter scope. Its `if`, choice branches and other inline
command lists inherit that scope. A named call creates a fresh scope and restores
the caller on return. A named callee cannot read undeclared caller parameters. A `plan rule` invocation evaluates
its supplied arguments in the event scope, then uses only its own existing `arg`
scope inside the rule body; it must not overwrite the suspended event bindings.
Follow-up events receive values only through explicit `args`; returning, closing,
or cancelling discards their scope. Independent auto/arrival events do not capture
an unrelated event's arguments. Initially, trigger, schedule, policy, checkpoint,
aside and custom-command entry points with only a literal name may target only
parameterless events. The checker must diagnose every such reference to a required
parameter, not fill it with nil. Later invocation support must extend these sites
explicitly, including their saved representation.

Opening a screen freezes its own arguments and suspends its opener. Entry commands
inherit the screen scope; nested named calls get their explicit bindings. Closing
resumes the opener exactly once with its original scope. A nested screen cannot
replace a parent's arguments. Refresh reevaluates labels and eligibility using
current state with the same bound IDs. No selection writes game variables.

Preview, navigation and cancellation change no simulation state or RNG. A pending
confirmation carries the selected IDs, not row offsets. Confirming rechecks the
entry's visibility and target eligibility, then consumes that pending action before
starting its event. Held keys, duplicate clicks and resume cannot confirm it twice.
Authors still guard against a *new* deliberate action when repetition is forbidden;
this is not global idempotency for every call. No screen refresh may retarget an
already pending confirmation to a different entity. If its target becomes ineligible,
cancel that preview and explain why without applying its commands.

## Saving and deterministic resume

Retain event-start snapshot plus deterministic replay. Do not serialize Go runner
frames, closures, expression environments or pointers into YAML command slices.
A resumable event root records its event ID and evaluated typed argument values
alongside the existing line/answer cursor and RNG snapshot. Nested bindings are
reconstructed by replaying explicit calls from that root. Replay must not reevaluate
root arguments from the latest global state or consume additional random draws.
A screen entry root is identified by screen ID, authored entry index, and frozen
screen arguments. In an unchanged game the authored index is stable; a missing or
changed action on reload must fail validation rather than run a neighboring entry.
Store canonical definition fingerprints for the replay root and its reachable
called events to detect changed actions before replay; a fingerprint is independent
of YAML whitespace and mapping order, but includes command order and expressions.

`open` remains a rollback boundary. At that boundary, resumable context must retain
the screen ID/arguments and a declarative opener continuation: its root invocation,
start snapshot, RNG, answer prefix and ordinal of the opening command. Restore
reconstructs that suspended opener privately by replaying to the boundary, verifies
the screen invocation, and installs the saved current state. It must not apply
pre-opening effects to that current state a second time. Nested screens use a
bounded list of these descriptors, not recursively embedded whole save envelopes;
use the runner's existing nesting limit. Completed entry effects belong to current
state; an active entry has its own replay root. The same model is required in
interactive play and non-rendering sessions.

Save pending confirmation identity and whether its action has started. A save before
confirmation restores the preview without executing it; a save during dialogue
restores the active action; after completion it restores the refreshed screen.
Presentation details such as scroll position can reset. Bound IDs, date, selected
action, opener continuation and committed changes cannot reset. Checkpoint restore
abandons all suspended screens as today; parameterized checkpoint targets remain
rejected until their saved invocation is explicitly supported.

Introduce schema version 10 for these forms and save format 3 for bound invocation
context. The event-call slice introduces these version gates; old formats load with empty bindings, while
an older engine rejects the new format instead of silently dropping its context.
Validate saved types, target existence, action fingerprints, cursor reachability and
nesting limits before replacing the live state. A changed declaration or malformed
save produces an error and leaves the existing session usable. No automatic
argument migration, partial restore or fallback to another target is allowed.

## Errors, ownership and implementation sequence

`gozen check` reports `file:line`, the offending parameter or reference, expected
type, and how to correct it. Check declarations, all invocation sites, argument
expressions and typed target fields; retain runtime validation for dynamic values.
A rejected binding changes neither state, RNG, caller scope nor screen stack.
A later command failure follows the existing event contract: earlier completed
commands are not rolled back as one giant transaction. Each planning operation
retains its own atomicity. Do not promise whole-event transactions across choices.

Gamedata owns the schema and static validation. Event execution owns argument scopes
and propagation. RPG/save owns value serialization and validation; it must not
import event or play. Session/play own replay orchestration, while play alone owns
input and rendering. Share lower-level invocation values where a real consumer
needs them; do not add an unused framework or an expression-to-command dependency.
Planning receives resolved typed targets through its existing operations.

Implement event calls first, including two residents, nested follow-up dialogue and
save/resume. Then implement screen calls, confirmation and continuations with two
activities using the same screen. Only then migrate author data. Collection rows,
aggregation, random role selection and date queries have separate contracts. This
ADR provides their scope boundary, not a claim that their APIs are already decided.

## Acceptance cases before implementation

Each implementation slice first fixes the legacy path with a rule test or atago
scenario, then adds the new observable contract and proves it fails before the fix.

| Case | Required observable result |
| --- | --- |
| Same event, two residents | Only the passed resident changes; no global selection variable |
| Nested call and choice | Explicit child arguments survive a choice; caller bindings return intact |
| Invalid invocation | Missing/extra name, wrong reference kind, unknown ID, non-finite number and malformed date fail without state/RNG change |
| Save during dialogue | Resumed effects, next random result and follow-up speaker match uninterrupted execution; save again and resume again |
| Same screen, two tasks | Preview and cancel leave both assignments unchanged; confirm changes only the bound task |
| Duplicate input | One confirmation applies once; held input cannot start another action during refresh |
| Screen save phases | Before confirm, during action and after completion restore the same target and changes without duplicate effects |
| Nested screen and close | Child arguments remain isolated; opener continues once, including commands following `open` |
| Changed or damaged save | Unknown references, changed parameter types/action fingerprint and excessive depth reject without replacing live state |
| Legacy invocations and saves | Existing literal calls, screens, checkpoint and save-format-2 replay still pass |

Use neutral fixtures and an atago path through actual CLI save/resume for each new
invocation route. Input-level screen confirmation needs the existing player script
harness; package tests alone do not prove pointer behavior or rendering. Compare
complexity diagnostics without loosening thresholds when migrating examples or game
data. This decision itself introduces no runtime test coverage for future syntax.

## Screen continuation prerequisite found in the legacy path

The screen-call slice must first extend the save model; adding argument maps to
`Open` and `AuthorScreen` alone is insufficient. The existing
`TestAuthorScreenDialogueSaveAndOpener` originally characterized all three save phases:
preview, entry dialogue, and completed entry. Without loading, closing the screen
executes the opener's following command once. After loading, each phase loses the
screen and opener. Active entry dialogue still resumes and completes once; committed
effects and the next random draw survive. That characterization demonstrated a legacy limitation, not an accepted result
for typed screens. The current test instead requires refusing these unsafe saves,
then verifies saving after close and event completion.

The concrete ownership gaps are:

- `internal/play/author_screen.go`, `activate` and `update`, suspend a runner and
  request in memory. The `done` callback resumes the opener only in that live view.
- `internal/play/rollback.go`, `cursor` and `saveBytes`, save either an active entry
  root or current state. Neither representation includes the suspended screen.
- `internal/play/save_report.go`, `loadSavedGame`, reconstructs current state and
  a single event cursor. `startState` in `play.go` replaces the scene stack.
- `internal/session/session.go`, `Next` and `Save`, treat `open` as a boundary and
  save only current state after it. The session currently has no screen selection
  or close lifecycle. Extending only play would leave the non-rendering contract
  unimplemented.

The continuation implementation must distinguish three identities: a replay root,
the particular opening request reached within that root, and the selected authored
entry within the opened screen. The root includes frozen arguments and its start
snapshot and RNG; the opening ordinal distinguishes repeated opens of the same
screen. The entry index belongs to the unchanged definition fingerprint, not the
filtered display row. The suspended opener's replay metadata must be restored along
with its runner after an entry ends; retaining only the runner loses the information
needed by the next save.

Reconstruction must execute the opener against private state up to the recorded
opening request, check its screen and bindings, then attach the continuation to the
saved current state. Replaying pre-open effects onto current state is forbidden.
Do not serialize callbacks or runner frames, embed recursively nested save envelopes,
or silently discard an unrepresentable root. Named events, screen entry roots, and
arrival roots need explicit handling; unsupported roots must reject saving with a
diagnostic. Validate all descriptors before replacing a live session. Checkpoint
restore must discard the entire suspended chain.

The first implementation slice refuses saves while an author screen is on the
play stack, including beneath another screen or the save menu. `saveBytes` enforces
the same refusal as the UI before touching a slot, so callers cannot bypass it.
It also enforces the existing refusal for dialogue following a replay boundary.
`Session.Save` refuses a running root after an `open` request, including its later
dialogue. Normal completion permits saving again. Ordinary autosave already waits
for an idle location; this is a serialization guard, not a change to that schedule.

For example, this neutral screen can be used now, but the player must close it and
finish `inspect` before saving:

```yaml
screens:
  stock:
    title: Stock
    entries: [{label: Boards, text: 'Boards: {var.boards}'}]
```

```yaml
events:
  inspect:
    do:
      - open: stock
      - say: Inspection complete.
```

This deliberately limits availability rather than accepting a save that drops the
opener. It does not implement screen arguments or continuation restoration and does
not repair previously written incomplete saves. No save or schema version changes.
The rejection leaves the slot, live action and random stream unchanged. Tests cover
preview, active entry dialogue, completed entry, nested screens, the save menu,
repeated rejection, and successful save/restore after the event finishes.

The next continuation slice must replace refusal with successful restoration, not
remove the guard while still writing state-only saves. Restore the opener and active
screen identity in all three phases; preserve exactly-once entry effects and next
random draws. Also cover save-after-restore, nested opens, two activities through one
screen, cancellation, repeated confirmation input, and malformed or edited
continuation rejection. Input and CLI coverage must use the existing player script
and atago harness; ordinary play package tests do not establish rendering success.

## Alternatives and consequences

Global selection variables are short to write but leak across nested calls and
resume. Generated copies and ID replacement macros preserve the maintenance cost
and cannot validate reference kinds. Saving runtime frames couples saves to internal
implementation and changes ADR 0025's replay model unnecessarily. A separate script
language duplicates expression, event and save responsibilities.

Typed invocations add explicit save and checker work, especially around suspended
screens. That cost is intentional: one authored body and one candidate screen should
replace copies while preserving target identity. Do not ship screen arguments with
only display reuse and postpone save/confirmation correctness to a later cleanup.

## Live authored screens in non-rendering sessions

The session lifecycle now suspends an opener until its authored screen is closed.
Previously `Session.Next` returned an `open` notification and immediately continued
on the following call, without any way to select the screen's entries. This could
not serve as an oracle for screen actions or future continuation restoration.

First-party integration tests use the existing `Next` and `Answer` interface:

- An authored `open` request contains its title and visible entry labels. Repeated
  `Next` calls leave it open. Returned labels belong to the caller.
- `Answer(n)` selects visible entry `n`; `Answer(-1)` closes the screen. Invalid
  indices and entries without commands leave it open. Selection retains the
  authored index and rechecks its condition before executing commands.
- Entry execution suspends the opener and uses a separate runner. Confirmation
  remains an ordinary authored choice. A second answer before the next request
  cannot enqueue the action again. Completed entries refresh the screen; nested
  screens close back to their caller. The screen stack is limited to 64 entries.
- Moving to another location closes the completed action's screen. Checkpoint
  restoration discards suspended screens and their continuations.
- `Start` and `Do` reject new scripts while an authored screen is active. The
  default policy used by unattended `Do` closes screens without taking actions,
  preserving its earlier outcome. A custom policy may answer `open` requests.
  Built-in screens remain presentation-only notifications; `Explore` and CLI
  headless drivers do not acquire screen exploration through this change.

A neutral stock screen uses existing YAML syntax:

```yaml
screens:
  stock:
    title: Stock
    entries:
    - label: Collect boards
      when: 'var.boards == 0'
      do:
      - choice:
        - label: Confirm
          do: [{set: {var.boards: 'var.boards + 1'}}]
        - label: Cancel
```

No screen argument schema or save representation is introduced by this slice.
Saving while any authored screen is suspended is still rejected, including while
an entry's conversation has its own replay cursor. Closing the screen and finishing
its opener permits saving again. Restoring an open screen, typed screen arguments,
presentation preview restoration and CLI screen E2E remain required follow-up work.

## Definition checks before restoring screens

The replay fingerprint now follows authored `open` edges as well as `call` edges,
including commands nested inside choices and conditions. A screen contributes its
whole definition and the events and screens reachable from its entries. An entry
root also contributes its owning screen: two entries with identical commands but
different labels or guards must not become interchangeable after reordering.
Visited event and screen sets terminate cycles independently.

For example, these existing-schema fragments connect an event to a screen and
then to another event:

```yaml
screens:
  stock:
    title: Stock
    entries:
    - label: Boards
      do: [{call: report}]
```

```yaml
events:
  visit:
    do:
    - say: Ready.
    - open: stock
  report:
    do: [{say: Checked.}]
```

A schema-10 save at `Ready.` rejects edits to `stock` or `report` before replay.
Whitespace and mapping order do not change the fingerprint. Unreachable definitions
do not contribute. Screen presentation fields do contribute, so editing a reachable
title or label also requires restarting the event. Built-in screens have no authored
definition to include.

Existing schema-10 cursor saves whose roots reach authored screens, or are screen
entries, used the narrower fingerprint and are now rejected; restart those events.
Fingerprints for roots with no authored screen dependency remain unchanged. Legacy
save-format and schema gates are unchanged. This check does not enable saves while
a screen is open, implement screen arguments, or reconstruct opener continuations.
Those remain the next implementation step.

## Rejecting damaged answer histories

Before extending replay to suspended screens, both session and interactive saved
replay now require an exact answer prefix. A missing answer, an index outside the
visible choice, or unused answers beyond the saved position are errors. Interactive
loading also reports an unreachable line instead of treating the nearest available
position as a successful resume. A failed replay stops its runner
and clears the pending dialogue; normal gameplay cannot continue with an invented
answer. Non-rendering restoration returns an incompatible-save error and no session.

For example, a workshop choice with `Collect` and `Cancel` followed by a receipt
line accepts exactly one preceding answer, `0` or `1`, at that receipt. An empty
history must not collect by default; `[2]` must not become `Cancel`; `[0, 0]` must
not silently drop the extra confirmation. A save at the choice can carry its one
valid answer when saved between answering and requesting the next line. Restoration consumes that answer once without
showing the choice again; additional answers are rejected. Valid histories retain
the same effects, bindings and deterministic random draws.

This policy applies to loading saves. Developer memo seeking, live edits, scene
skip and rollback keep their existing policies. Validation occurs during replay,
not during structural JSON parsing: visible choices may depend on saved state.
The interactive runtime reports replay failure through its existing error channel;
this does not yet provide transactional rollback to the previously running game
when loading fails. Save slots are not rewritten by replay. Screen continuation
serialization and typed screen arguments remain unimplemented.


## Plain screen invocation slice

Plain authored lists now implement the declaration, pure binding, isolated entry
scope and nested open/close contracts above. See the
[executable stock and receipt example](../development/event-arguments.md#reusing-plain-screens).
Session and play share event-level binding and entry validation; the presentation
adapter owns labels and input, while session owns non-rendering requests.

This slice intentionally retains the screen-save rejection described above.
Parameterized planning surfaces and their implicit destinations are rejected.
Their rendering and navigation scopes must be implemented before accepting such
declarations. Frozen entry arguments also work with the existing named-root replay
path; entry replay alone is not a screen continuation descriptor.


## Random snapshot validation before continuation loading

Save parsing validates every nonempty envelope `rand` as a PCG snapshot before
returning a state to session, play or simulation. Truncated, extended and
wrong-generator bytes are rejected instead of allowing a frontend to choose a
fresh seed. This also applies to a cursor's event-start snapshot. Validation uses
an independent source and consumes no gameplay randomness. Saves without `rand`
retain the legacy seed-based behavior; this change does not require a new format.

For example, reloading a workshop save must preserve the next resource draw.
Selecting a damaged slot reports a load error while retaining the current game,
its random stream and the original slot bytes. The shared parser owns this check,
as it already owns save identity and cursor validation. Cached parses may reuse
only snapshots that passed the same validation. The existing deterministic battle
reload test and the new damaged-slot test cover the successful and rejected paths.

This prerequisite does not implement screen continuation descriptors or make
other replay failures transactional. Screen saves remain rejected until the
suspended opener and active entry can both be reconstructed.


## Slot replay validation before replacing live play

The slot loader now checks a saved cursor on a private copy of state, definitions
and random source before offering it for confirmation or replacing the live game.
`event.Runner.ReplayTo` owns the non-rendering dialogue/answer traversal shared
by session restoration and this probe. Session's previous traversal is removed.
Play still reconstructs presentation through its existing interactive replay after
the probe succeeds. The probe does not render, play sound, write saves or deliver
presentation effects. Legacy saves without random bytes get one frozen fallback
source for both validation and installation; missing legacy roots keep the existing
restart-at-location behavior.

For example, a workshop receipt saved after choosing `Collect` must have exactly
one valid answer. Missing, out-of-range or extra answers and unreachable lines
make the slot unavailable. If a displayed slot changes before selection, selection
reports a recoverable load error. The current resource count, dialogue runner,
next random draw and slot bytes remain unchanged. A valid receipt can still be
loaded and saved again without collecting twice.

The preflight is scoped to slots read by `load`, including the load menu and newest
save selection. It does not make arbitrary developer replay, live editing or direct
internal `loadSavedGame` calls transactional. It replays the saved prefix once more
than before; latency has not been benchmarked. Screen continuation descriptors,
open-screen saves and target-based candidate screens remain separate pending work.
