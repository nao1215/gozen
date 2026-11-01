# Reusing events with typed arguments

Authors can reuse one event for several residents without storing a global
selection or copying the body. Schema version 10 adds `params` and the mapping
form of `call`. The original `call: name` remains valid for parameterless events.

The runnable [workshop example](../examples/event-arguments/game.yaml) calls
`encourage` twice from [one entry event](../examples/event-arguments/data/story.yaml):

```yaml
- call: {event: encourage, args: {resident: '"potter"', amount: '2'}}
- call: {event: encourage, args: {resident: '"weaver"', amount: '3'}}
```

The shared event declares `params: {resident: entity, amount: number}`. Its
planning operation uses `where: 'self.id == arg.resident'` and
`value: 'self.vigor + arg.amount'`. Its nested `report` receives the resident
explicitly and retains it while a choice waits. On return, the parent's amount
and resident remain intact. This model also supports crop care and training.

Every declared parameter is required. Types are finite `number`, `bool`, `string`,
`entity`, `activity`, `actor` and ISO `date` (`YYYY-MM-DD`). References must exist
in their declared namespace. `args` values are expressions: quote string literals
inside the YAML string. Arguments are evaluated once in sorted parameter-name
order before any callee effects; random calls are forbidden. Missing names,
extra names, undeclared `arg` fields and known type errors produce source-located
`gozen check` diagnostics. Dynamic values are checked again at execution.

Inline branches inherit arguments; named calls receive only explicit arguments.
Planning rules shadow event arguments with their own parameters. Triggered events,
schedules, policies, checkpoints and other literal-only entry points must use a
parameterless wrapper. A single mapped call supplied to a session can also be a
saved root; its evaluated values are saved, rather than reevaluated on resume.

Save format 3 records frozen root arguments and a fingerprint of a named root
and its reachable calls. Replay reconstructs nested scopes from the event-start
snapshot and RNG. Editing a reachable command or declaration rejects that cursor;
reordering YAML mapping keys does not. Both slot selection and direct player
installation validate saved answers and reachability on private state before
replacing the current game. A rejected replay preserves the current dialogue,
state and random stream. Older save formats remain readable for legacy games. Anonymous scripts have no named replay root. Schema-10 named roots can also
retain plain screen continuations, as described below.

Run the example's rules and save/resume checks with:

```sh
go test ./internal/session -run 'Test(EventArgumentsResume|BoundEventRootAndChangedSave)'
```

This is the event-call slice of [ADR 0054](../decisions/0054-typed-event-and-screen-arguments.md).
Expression-form `say.who` remains pending. The sections below describe implemented
planning targets and screen continuations.
The event-call example uses `where` filtering for entity selection. It does not expand event
bodies or add a generator. The session contract tests load these same example
files; the CLI E2E runs both residents and checks their resulting attributes.

## Reusing plain screens

Schema 10 also supports typed arguments on plain authored lists. The same example
now contains a stock screen opened with `open: {screen: stock, args: {amount: '2'}}`.
Its `params: {amount: number}` binds `arg.amount` in the title, entry labels,
entry text, visibility conditions and entry commands. The stock example confirms
or cancels a quantity, opens a nested receipt with a different amount, and returns
to its original scope. No selected value is stored in a global variable.

Bindings are frozen at open and retained on refresh. Entry commands inherit them;
a named event or nested screen receives only explicitly passed values. Closing
resumes the original caller once. A malformed dynamic argument logs an execution
error and skips opening the screen before any entry effect or random draw.

Parameterized planning surfaces, implicit tab destinations and
expression-form screen entry target IDs are not supported yet. Use a parameterless wrapper
screen with an explicit `open` action when a literal-only destination is required.
Plain portrait lists can still use fixed entity targets. Parameters do not expand
screens or generate YAML.

The player and session can save these plain screens, including confirmation and
entry dialogue, when opened from a schema-10 named root. Unsupported boundaries,
older schemas, anonymous roots and native planning surfaces still refuse saving
while a screen is open. The current slot stays intact on refusal.

The rules and input tests load the documented example:

```sh
go test ./internal/session -run '^TestScreenArguments$'
go test ./internal/play -run '^TestAuthorScreenArguments$'
atago run --ci --parallel 1 --skip-tag graphics,web e2e/atago/planning.atago.yaml
```

The CLI scenarios check cancellation, nested close, and save/resume in a second
process using simulated player input. They do not verify rendered screenshots.

## Assigning different activities with one screen

The [workshop example](../examples/event-arguments/game.yaml) defines a single
`candidates` screen with `task: activity` and `resident: entity` parameters.
Call `assign_visit` with `task: '"prepare"'` or `task: '"shape"'` and
`resident: '"weaver"'` to reuse its confirmation flow. Its confirmed action is:

```yaml
- plan:
    op: assign
    activity: {expr: arg.task}
    entity: {expr: arg.resident}
```

Literal IDs continue to work. Expression targets require schema 10 and must
return existing IDs of the correct kind without randomness. Targets are resolved
once before mutation, outside entity iteration; `self` is not available there.
Invalid targets roll back the entire planning operation or rule. Cancel leaves
both activities unchanged. The plain candidate list can be saved while its
confirmation is waiting, under the named-root contract below.


## Saving a screen continuation

The player and internal session harness can save an open schema-10 plain screen when
its caller is a named event. The stock/receipt example above is also its neutral
save/resume example: confirm a collection, save on the nested receipt, restore,
close the receipt and stock, and observe the caller's `Returned` line. Cancelling
before saving does not collect stock; restoring a confirmation does not answer it.
Use the save action on an idle plain list or during dialogue/confirmation.
The input script uses `press save`; the save menu itself is an overlay and does
not become part of the saved continuation.

```sh
go test ./internal/session -run 'TestScreen(Arguments|Continuation)'
go test ./internal/rpg -run TestScreenReplayCacheOwnership
go test ./internal/play -run TestAuthorScreenContinuationAcrossFrontends
```

The save stores the root snapshot and random state with an input transcript, not
expanded YAML. Restoration rebuilds screens and frozen arguments without issuing
external callbacks. The transcript is limited to 20,000 inputs and screen nesting
to 64. Unsupported replay boundaries and older schemas still refuse mid-screen
saves. Finishing the event permits an ordinary state save again. Session and player
share reconstruction in the event layer. Validation runs on private state before
replacing the live game; a damaged transcript preserves its caller, state and RNG.
The player restores persistent images and standing portraits without replaying
sounds or transient effects. Keyboard focus, scroll position and elapsed display
ticks are presentation state and reset when loading.

The example's parameterless `stock_demo` root is suitable for CLI verification:

```sh
gozen sim script docs/examples/event-arguments --start workshop --event stock_demo --script input.txt --save-dir saves
```

Start `input.txt` with `wait 30`, `press z`, `wait 12`, `press save`, `press z`,
`wait 90`, and `quit`, each on its own line, to save the first stock preview.
The atago scenario also saves confirmation, loads it in another process, and
checks both returned caller lines and the final stock total.

## Bounded nested screens

Session and player permit at most 64 simultaneously open authored screens.
For example, a stock screen may open a receipt screen and return to stock after
closing the receipt. Reusing a screen recursively counts each open instance;
it does not create an unlimited stack merely because the definition is shared.
At the limit, another open reports `author screens nested too deep` and continues
the command after that open. Existing screens remain open, and closing one frees
a slot. Save menus and other built-in overlays do not consume authored slots or
hide their count. The same depth bound applies during saved reconstruction.

## Definition reload during an authored screen

Development-mode definition reload waits until all authored screens close and the
calling event finishes. For example, requesting reload while the stock entry shows
a receipt keeps the current action and its caller alive; closing the screen still
executes the caller's `Returned` line once. The queued reload applies at idle.
This also protects a screen beneath a save menu. Picture and sound reload remains
available through the existing asset-only path. Definition reload and save
restoration remain separate operations.


### Visible entry identity

A screen answer selects a visible row. Hidden entries do not consume an answer
position, but execution still uses the original authored entry index. For example,
a stock list with a hidden `Collect` entry followed by `Inspect` must execute
`Inspect` for answer zero in both the player and a non-rendering session. Screen
arguments remain frozen while visibility is evaluated against current state.
The shared event-layer selection iterator preserves definition order and label
rendering. Frontends retain their existing presentation and diagnostic policies;
entry guards are checked again before execution. Transcript answers use this
same visible-row contract during restoration.

## Calendar date destinations

Schema 10 calendar and timeline date markers can pass `args` to one ordinary
screen. The workshop example uses two dates with one `day_detail` definition:

```yaml
markers:
- date: '2030-01-01'
  screen: day_detail
  args: {day: '"2030-01-01"'}
- date: '2030-01-02'
  screen: day_detail
  args: {day: '"2030-01-02"'}
```

Declare `params: {day: date}` on the destination and use `{arg.day}` in its
text. Marker arguments follow the existing pure expression and type checks;
missing, extra and invalid literal arguments fail `check`. Dynamic values bind
at click time. A failed binding reports an error without opening or changing
state. Multiple links on the same date must agree on both screen and arguments.
Browsing does not advance the planning calendar. Native navigation still cannot
be saved while open; this does not extend the named-event replay contract.


## Timeline lane destinations and candidate columns

The workshop `tasks` timeline above supplies `screen: candidates` and an `args`
map on each lane. Clicking or confirming either activity opens the same list
with its own task and resident. Bindings use the same pure, typed validation as
explicit `open`; a runtime binding error leaves the current screen and state
unchanged. The candidate pane retains the selected timeline activity on return.
Implicit navigation still does not establish a saveable named event root.

Schema-10 plain portrait lists may interpolate `view.fields[].key` and `label`.
For example, a reusable workshop picker can declare `column: string` and
`caption: string`, then use `fields: [{key: '{arg.column}', label: '{arg.caption}'}]`.
Pass `column: '"vigor"'` and `caption: '"Vigor"'` to display that public value.
The lookup reads the same public observations as literal columns: a missing key
or a hidden attribute is omitted. Literal keys retain reference validation;
dynamic keys are checked as templates and resolved on display. Native planning
surfaces still require literal column keys.
