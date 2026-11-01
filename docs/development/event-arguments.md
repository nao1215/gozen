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

## Sharing a cast-dependent event body

The example's `inspection_demo` calls `inspect` for two residents. An `entity`
parameter selects the planning target and reads `entity[arg.resident].vigor`;
a separate string supplies its display name. The same choice and effects serve
both calls. Arguments stay bound while the choice waits, and the caller chooses
the participants before entering the shared body. This also works for trainees,
farm workers and other planning entities.

Keep literal-only commands in small conditional branches when necessary. In
`inspect`, the speaker and the flag write have separate branches, while the
planning operation and choice remain shared. `say.who` and flag destinations
still require literals. Variable destinations can use the indexed syntax below;
`{arg.resident}` interpolation is not destination syntax. Avoid copying a whole
event to accommodate literal-only commands. For substantial per-person state,
prefer planning attributes and expression-form entity targets.

The planning atago suite runs both calls and checks their dialogue, flags and
resulting values. It uses the ordinary CLI and does not expand a template into
per-resident event definitions.

When migrating existing dialogue, preserve its read identity as well as its
rendered text. Dialogue keys use the authored speaker and text, and choice keys
use the authored prompt and label. Replacing distinct lines with one interpolated
line deliberately gives them a shared read identity. Keep literal lines or labels
in guarded branches when that would change the game's read/skip behavior. Several
guarded labels can call one parameterized choice-effect event, without duplicating
those effects.

## Share calculations instead of copying expressions

The example's `progress_demo` passes two different budgets to `progress_report`.
The caller supplies a pure expression (`3 + 9` or `0`); the event receives its
number once, computes a ratio, and returns. This keeps the formula in one place
without introducing a temporary saved variable. For date-specific reports, keep
the small date inputs at the call sites and share the calculation body.

A shared event also preserves the order of side effects. If a repeated effect
draws random values, put those draws in its body and call it once per effect.
Do not move a draw into an argument (arguments must be pure), cache a draw across
calls, or sort the calls: those changes alter the random sequence. Compare the
state and the next random value before and after replacing expanded commands.

The `shift_demo` example shares an ordered work sequence between residents.
An unavailable resident leaves state unchanged; later calls see the effects of
previous calls. Each call freezes its own `available` and `amount` values, and
`shift` passes only the arguments needed by `apply_shift`. This also fits farm
chores or training sessions. Keep eligibility checks before random draws and
preserve participant order when replacing copied command lists: reordering them
can change both shared resources and the random stream.

An invariant eligibility check can surround mutually exclusive branches rather
than being copied into each branch. This is safe only when earlier branches
cannot change eligibility for another matching branch. Do not hoist a check
across mutations merely because its expression text is repeated.

## Sharing ordered adjustments

The workshop's `adjustment_demo` calls one `adjust_started` event with a numeric
amount. Each task checks its current completion and progress before applying the
adjustment. A completed task stays complete, an unstarted task stays at zero,
and a started task receives the amount. The CLI scenario checks all three cases.
This pattern also fits crop growth and training exercises.

Freeze an amount only when its inputs stay unchanged throughout the original
sequence. A value computed from progress that an earlier operation changes must
be evaluated at that operation instead. Preserve task order, eligibility tests,
and random draws when replacing copies with calls.

Consecutive selection candidates can share an outer condition when none of their
checks or effects changes its inputs. Keep candidate-specific cooldowns and
priority checks inside, and keep source order: regrouping nonconsecutive
candidates can change which one wins or consumes the next random draw.

## Sharing priority selection

The workshop's `priority_demo` passes a score and plain title to one
`consider_task` event. It retains the first candidate on a tie, ignores lower
scores, and replaces the winner for a higher score. The same pattern supports
farm chores, training recommendations, and a fixed number of notification slots.
For several slots, shift lower-ranked entries from last to first before inserting
the new candidate. Preserve caller order when that order breaks ties.

Arguments capture inputs at the call. Freeze a score only when the shared body
cannot change its inputs before the original comparison. A title stored in a
variable should already be plain text; embedding another template does not
request a second interpolation. Keep period-specific bookkeeping outside a
shared scan if the scan depends on the current ledger and participant order.

## Reading calendar ranges

`calendar_days(first, last)` returns the signed number of days from `first` to
`last`; equal dates give zero. `calendar_workdays(first, last)` counts both
endpoints, excluding `planning.calendar.weekdays_off` and `closed`. A reversed
range gives zero working days. A date closed by both settings is counted once.
Both functions require a planning calendar and two ISO date strings in years
0001 through 9999. Invalid dates, non-string values and wrong argument counts
fail at execution. Dates may be outside the playable range; its closure rules
still apply. Neither function advances time, draws randomness or writes state.

The [calendar example](../examples/event-arguments/data/story.yaml) passes typed
date arguments to one shared report. A farming or workshop game can use
`calendar_workdays(start, today) / max(1, calendar_workdays(start, finish))`
as a planned fraction instead of writing one expression per day. Authors decide
whether to cap this fraction and whether the start day contributes. This is an
explicit working-day model; it does not change timeline rendering or activity
progress automatically.

Long pure calculations can similarly pass intermediate results as numeric event
arguments, then call another event with the next result. This avoids adding
scratch variables to saves. Evaluate a result before mutation only when that
snapshot is intended; later calls otherwise see the current state.


`calendar_shift(date, days)` returns the ISO date a signed, integer number of
calendar days away. It does not advance the game clock or change saved state.
For example, `calendar_shift("2028-02-28", 1)` returns `"2028-02-29"` and
`calendar_shift("2030-12-31", 1)` returns `"2031-01-01"`. Weekends and configured
closures count as calendar days; use `calendar_workdays` to inspect whether the
result is open. Both the input and result must be in years 0001 through 9999;
invalid dates, fractional offsets, wrong types and out-of-range results are
runtime errors. The offset must be within -3652058 through 3652058.

The workshop's `calendar_demo` passes shifted dates to a typed event and shows
forward and backward shifts. A calendar card can calculate its next date and
weekday, then keep only authored exceptions such as festivals in conditional
rows. This avoids copying an entire card for every date. Preserve the original
range and integer-day guard when replacing a fixed date table, including its
behavior for inputs outside the authored calendar.

## Gate random scores inside a shared event

The workshop's `lottery_demo` calls `rank_resident` for two residents. Only the
resident with positive vigor draws a score. The event evaluates eligibility
first, writes the random score once, then passes that value to `consider_task`.
That helper retains the first candidate on a tie. Keep the original candidate
order when replacing copied branches with calls: both ties and later random
results depend on that order. Random expressions cannot be call arguments.

For a longer selection pass, pure totals and availability checks can be passed
once as typed arguments to the event that scans candidates. Freeze only inputs
that cannot change during that pass. Nested named events need those values
forwarded explicitly. Keep random predicates inside their original gates, and
keep score draws after eligibility checks. This pattern applies to choosing a
farm task, assigning a workshop resident, or selecting a training encounter;
it does not require scratch values in saved state for the frozen inputs.


## Indexed variable destinations

Schema 10 permits a computed variable name in `set`. The workshop's
`counter_demo` records stock by resident and slot with one shared event:

```yaml
params: {resident: entity, slot: number, amount: number}
do:
- set: {'var["stock_" + arg.resident + "_" + arg.slot]': 'var["stock_" + arg.resident + "_" + arg.slot] + arg.amount'}
```

The index must be a pure expression yielding an identifier: an ASCII letter or
underscore followed by letters, digits or underscores. Empty names, dots,
spaces, non-string results and random draws are rejected. A bad destination
logs an execution error before evaluating its value; that assignment changes
neither state nor randomness. Other assignments continue normally. Values use
the existing variable type and save checks, including declared types.

Assignments execute in lexical order of their **authored** keys. Each target
is resolved immediately before its value, so earlier writes are visible to
later targets and values. Split order-dependent operations into separate
commands. The saved key is the resolved name, exactly as with a literal write;
there is no new save representation. Only `var[...]` supports this syntax,
and strict state declarations still require literal destinations. The state
usage index records a computed write against the variable table and records
reads made by its index expression.

This is useful when replacing an existing family of saved scalar counters.
The complexity checker measures index expressions as well as values. This
does not turn those keys into a collection, and does not suppress complexity
warnings about pseudo-arrays. New entity state usually belongs in planning
attributes. The atago example checks separate counters and repeated updates;
the runtime tests also cover a save/load round trip and failed destinations.

When preserving literal dialogue identities in a shared event, put consecutive
lines and effects for one cast under one argument-only guard. Arguments cannot
change while the call runs, so that guard remains valid across dialogue waits.
Do not combine state-dependent guards this way: their conditions may change
between commands.

The workshop's `threshold_demo` uses one keyed rule for water and stock
warnings. It keeps the displayed level separate from the highest level already
shown. Acknowledging a warning clears only the displayed level; another call at
the same level leaves it cleared, and a higher level posts it again. These are
ordinary saved variables, so the shared rule needs no per-resource event body.
The atago scenario checks acknowledgement, escalation and independent keys.

When extracting a sequence that updates several variables, preserve its original
evaluation order. Literal `set` keys execute in lexical order, which may differ
from the order of computed keys. Use separate commands when later values depend
on earlier writes. Pass stable inputs as arguments, but keep changing values in
the shared body so each call reads the current state.
