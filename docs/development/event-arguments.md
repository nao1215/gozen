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
state and random stream. Older save formats remain readable for legacy games. As before, anonymous scripts have no named replay root and screen
opening is a replay boundary.

Run the example's rules and save/resume checks with:

```sh
go test ./internal/session -run 'Test(EventArgumentsResume|BoundEventRootAndChangedSave)'
```

This is the event-call slice of [ADR 0054](../decisions/0054-typed-event-and-screen-arguments.md).
Expression-form `say.who` and player screen continuation saves remain pending.
The sections below describe implemented planning targets and session continuations.
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

Parameterized planning surfaces, implicit tab/calendar/timeline destinations and
expression-form screen entry target IDs are not supported yet. Use a parameterless wrapper
screen with an explicit `open` action when a literal-only destination is required.
Plain portrait lists can still use fixed entity targets. Parameters do not expand
screens or generate YAML.

The player still rejects saving while a screen is open, including during
confirmation or entry dialogue; the current slot is preserved. Ordinary event
saves before opening and completed-root saves after closing remain supported.
The internal session harness supports these screen continuations as described
below; player restoration remains a separate implementation step.

The rules and input tests load the documented example:

```sh
go test ./internal/session -run '^TestScreenArguments$'
go test ./internal/play -run '^TestAuthorScreenArguments$'
atago run --ci --parallel 1 --skip-tag graphics,web e2e/atago/planning.atago.yaml
```

The CLI scenario confirms cancellation and nested close using simulated player
input; it is not a rendered screenshot or a screen-save restoration test.

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
both activities unchanged. Close the screen and finish its calling event before
saving in the player; its restoration of an open screen remains unsupported.


## Non-rendering session continuation

The internal session harness can now save an open schema-10 author screen when
its caller is a named event. The stock/receipt example above is also its neutral
save/resume example: confirm a collection, save on the nested receipt, restore,
close the receipt and stock, and observe the caller's `Returned` line. Cancelling
before saving does not collect stock; restoring a confirmation does not answer it.
This applies to the internal session harness, not the player save menu yet.

```sh
go test ./internal/session -run 'TestScreen(Arguments|Continuation)'
go test ./internal/rpg -run TestScreenReplayCacheOwnership
go test ./internal/play -run TestLoadRejectsSessionScreenReplay
```

The save stores the root snapshot and random state with an input transcript, not
expanded YAML. Restoration rebuilds screens and frozen arguments without issuing
external callbacks. The transcript is limited to 20,000 inputs and screen nesting
to 64. Unsupported replay boundaries and older schemas still refuse mid-screen
saves. Finishing the event permits an ordinary state save again. Player restoration
and CLI save/resume verification remain pending; the player rejects these session
saves rather than loading an incomplete continuation.

## Bounded nested screens

Session and player permit at most 64 simultaneously open authored screens.
For example, a stock screen may open a receipt screen and return to stock after
closing the receipt. Reusing a screen recursively counts each open instance;
it does not create an unlimited stack merely because the definition is shared.
At the limit, another open reports `author screens nested too deep` and continues
the command after that open. Existing screens remain open, and closing one frees
a slot. Save menus and other built-in overlays do not consume authored slots or
hide their count. This bounds live navigation; it does not enable player screen
save restoration, which remains unsupported.
