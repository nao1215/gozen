# ADR 0054: Typed arguments for reusable events and screens

Status: Accepted for phased implementation (2026-11-01). Event calls implemented; screen calls and expression-form target fields remain planned.

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
