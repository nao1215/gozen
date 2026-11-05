# ADR 0053: Composable planning state and operations

Status: In development.

## Purpose and ownership

Authors of workshop, farming and training games need typed residents, a calendar,
work dependencies and period totals without defining combat actors. Optional
`planning` data belongs to gamedata (layer 2), mutable progress to the existing
RPG state, deterministic operations to layer 3, and views to play. The existing
architecture policy permits these edges; no exception or public Go API is added.

`plan` is an immediate event command. Existing dialogue, choices, save cursors and
headless scripts remain the execution path. Entity IDs are independent of actor
IDs; an optional actor reference supplies presentation. Rules never infer the
meaning of health, productivity, work places or a successful outcome.

## Contracts

Attributes are finite typed scalars. An invalid assignment fails instead of
clamping. Operations select stable entity IDs in ascending order. A failed batch
restores the complete planning state. Observations and rule arguments also require finite scalars: numbers cannot be
NaN or infinity; strings, booleans and nil keep their existing meanings. An
overflow fails even when a rule does not use that argument. Validation happens
before accepting the candidate state, so saving and advancing still work after
a rejected refresh or rule call. Reusable rules bind scalar arguments and
contain at most 256 immediate operations; nested rule and policy calls are rejected.
Random draws belong to preceding ordinary event commands, not planning formulas.

Observations refresh explicitly. Hidden attributes are absent from player views;
this is not encryption. Policy conditions see observations, while event rules
can read full state. Diagnostic snapshots intentionally contain full state.

For example, a workshop resident can expose a numeric estimate without changing
the underlying attribute:

```yaml
entities:
  potter:
    name: Potter
    observations:
      estimated_output: 'self.vigor * 2'
```

Here `vigor` is a declared numeric attribute. If an authored formula overflows,
the whole `observe` operation fails; the previous report remains available.
The same validation applies to rule arguments even if their body ignores them.

### Numeric operation inputs

Progress, effort, ledger records and activity observations require finite numbers.
Boolean, nil and string results are rejected without implicit numeric conversion.
The checker rejects statically known nonnumeric results, including a nonnumeric
conditional branch. Dynamic references are validated at execution; a rejected
operation rolls back the entire rule, including earlier steps. Entity observations
remain finite scalars and may still be boolean, string or nil.
Authors can explicitly convert a boolean with `int(value)` when that is intended.
For example, a workshop can record one inspection for an active resident:

```yaml
- plan: {op: record, ledger: inspections, value: 'int(self.active)'}
- plan: {op: progress, activity: prepare, value: '2'}
```

The first command requires the declared boolean attribute `active` and the
`inspections` ledger. Direct `value: 'self.active'` is invalid at execution.
This contract does not change conversions inside ordinary arithmetic expressions.

### Rule argument names

Rule bodies can read only their own declared parameters through `arg`.
The checker validates dotted names and literal string indexes against `params`,
including references inside predicates. For example, a workshop can reuse:

```yaml
rules:
  work:
    params: [amount]
    do:
    - {op: progress, activity: prepare, value: 'arg.amount'}
```

Call it with `{plan: {op: rule, rule: work, args: {amount: '2'}}}`.
A misspelling such as `arg.amonut` is a located check error listing the
declared names. Outside a rule body, including expressions supplying call
arguments, `arg` is unavailable. A rule with no parameters exposes no argument
fields. Values remain finite scalars checked at execution; computed indexes
such as `arg[var.key]` require runtime lookup and are not statically resolved.
This does not change parameter evaluation order, rule results or saved state.

### Scoped observation contract

- Bare `observe` retains the existing global refresh of activities and entities.
- `entity`, `tag`, or `where` selects entities only. These filters intersect;
  no matching entity means no change. Assignment does not implicitly select work.
- `activity: ID` selects exactly that activity and no entities. An optional
  `where` binds `self` to that activity's progress, effort, assignee and cancelled
  fields. False means no change. Combining `activity` with `entity` or `tag`
  is an error, as is an unknown target.
- Each selected collection is visited in ascending ID order. Global refresh
  visits activities before entities, preserving existing formula visibility.
- Any expression or target failure restores the entire planning state, including
  earlier observations. Saving and advancing remain possible after failure.
  Refresh never changes true progress or assignment. Save/resume retains both
  stale observations and their subsequent explicit refresh behavior.

For a neutral workshop, the author syntax is:

```yaml
# Existing syntax: refresh both collections.
- plan: {op: observe}
# Scoped semantics: inspect one resident, leaving work reports stale.
- plan: {op: observe, entity: potter}
# Activity selection: inspect just the clay preparation report.
- plan: {op: observe, activity: prepare, where: 'self.progress > 0'}
```

The checker and executor reject mixed entity/activity selectors and unknown targets.
The observation selection tests cover stale reports, filtering and save/resume;
failure tests verify rollback and subsequent advancement and saving.

Calendars use ISO dates, inclusive endpoints, explicit phases, weekday closures and
exception dates. Closed days consume one phase. Advancement and reservation
dispatch are separate so authors can settle pending decisions before dispatch.
Reservations retain their dispatch keys across saves. Events must finish before
the author advances again.

Progress clamps to zero and effort, and rework reopens completed work. A dependency
ratio defaults to one and may be specified per activity. Explicit operation order
decides whether work completed earlier in a phase enables subsequent work.

The `effort` operation sets a finite, positive activity budget with `activity`
and `value`, for example `{plan: {op: effort, activity: prepare, value: 'activity.prepare.effort * 1.5'}}`.
It preserves completed work units, clamps progress and observations to a reduced
budget, and reopens work when the new budget exceeds progress. Dependencies,
observations, policies and views use the current budget. The override is saved;
older saves without it retain the authored budget. Invalid values fail atomically.

The `cancel` operation sets an activity's saved exclusion flag with a boolean
`value`. Cancelled activities retain assignment, effort, progress and observations,
but progress operations leave them unchanged and dependents ignore them. Setting
`value: 'false'` restores the dependency requirement. The `activity.ID.cancelled`
expression lets authored workload, quality and outcome rules exclude the same work.
Timeline and resident detail views omit cancelled activities. Old saves default to
false. Cancellation is distinct from completing work or deleting its history.

The `deadline` operation sets an activity's end date using an ISO date string in
`value`. The date must be on or after its authored start. For example,
`{plan: {op: deadline, activity: prepare, value: '"2030-08-01"'}}` moves its
planned finish without changing effort or progress. Expressions read the effective
date as `activity.ID.deadline`; timelines and saves retain the same override.
Old saves without an override use the authored end date. Invalid dates fail atomically.

Monthly ledgers accept explicit historical periods until closed. Closing twice or
writing a closed month fails. Retention removes old totals but retains closed
period markers so pruned periods cannot be reopened. Closure is explicit, allowing
pending declarations to settle first.

The additions are optional fields in schema 10 (planning, the screen kinds and views, the workspace entries, `say.ensemble`, `say.instant` and `ui.material` are all refused below 10, so a game that says 9 still loads in v0.15) and optional state in save format 3.
Old games retain fixed rendering and their existing state. A save made before a release that added or removed
entities, activities, ledgers or attributes still loads: missing records take
the initial values of a new game, records whose definition is gone are dropped,
a date past a shortened calendar end moves to the end, and every repair is
listed in the load report. Values that no definition change explains (NaN,
out of range, a phase past the calendar on an unchanged date) are still refused. The native screen
option renders a 16:9 inner framebuffer at device resolution; Ebitengine owns
letterboxing and pointer conversion. Scripted captures with an explicit window size include opaque black outer bars,
centered around the native framebuffer without resampling its pixels.

## Authored retry points

Schema 10 events may use `checkpoint: resume_event` to replace one retry point.
`restore_checkpoint: true` abandons the current event and suspended screen actions,
restores the state and PCG stream, and starts that event. Both the interactive
runtime and non-rendering sessions implement this contract. Missing or damaged
checkpoints produce an error without replacing the current state.

The point contains an ordinary save envelope without an event cursor or another
checkpoint. Manual saves retain it; repeated captures replace it without growing
history. Read marks, gallery unlocks and settings remain player-owned. The
checkpoint command is a replay boundary, like opening a screen.

Authors place captures after one-time period accounting and before repeatable
scenes. The resume event must restore the desired screen and must not redo that
accounting. For example, a workshop can capture after morning deliveries and
resume its briefing before the player chooses an activity. Capturing a point
does not write a disk autosave; players still save their progress normally.

## Current limits

Views include explicit public-value columns with gauges, separate planned and
observed activity bars, a monthly calendar, participant tiles and island seating.
Authors bind portrait, fixed video-off icon, camera, backdrop, mask and seat badge
to public attributes or observations. Hidden attributes cannot be bound directly.
Omitted columns show names only, never a dump of available attributes. Seating
order and door labels belong to game data. Background masking affects only the
backdrop; portraits are drawn afterward. Missing backdrop assets use the geometric
room fallback. An optional public `detail` text binding enables entity selection
and a pane listing assigned, unfinished activities and observed notes. Timeline
quantities stay in the name column, clear of the date axis. Header expressions can
use `grouped(number)` to show comma-separated thousands without changing state.

An optional unsigned `seed_xor` chooses the second PCG seed for an explicit
nonzero run seed. Omitting it preserves the existing seed derivation. This does
not guarantee compatibility of random draw ordering.

Box-style dialogue may select an ensemble screen with `say.ensemble`; the actor
ID highlights its participant tile and replaces the standalone talking portrait.

Island seats render authored public `fields` below the name, including when a
badge replaces the face. Keep seat fields short (two fields fit the compact layout).
Ensemble tiles reserve at most the lower third of the content area for up to
two public summary fields. Camera images and fallback icons stay above that
strip; the participant name retains its separate bottom row. Authors choose the text and use an ordinary `say.ensemble`
to keep the summary within the existing dialogue and save flow. An ensemble's
public `badge` binding appears in a contrasting strip above the portrait; empty
badges add no strip. Fullscreen ensembles also retain the authored date and
header during dialogue. Header columns use normal-size text with ellipsis for
overflow, shared with the fullscreen planning surfaces.
The optional public boolean `highlight` binding outlines ensemble tiles even
when nobody is speaking. Only an exact true value enables the outline; missing
or non-boolean observations do not. A badge can explain the highlight in words.
Fullscreen entry `text` is visible below its label when its bounds are at least
72 logical pixels high. This supports an authored agenda or notice card without
adding another source of state. Target action menus have a resident heading,
show at most six rows and open beside the selected island seat. The menu and
confirmation preview have opaque backgrounds and stay left of the sidebar,
below the header and above the footer.
On fullscreen authored screens, `notice` results temporarily replace the footer,
one at a time, without blocking event execution. Queued results retain their
display time until shown; dialogue pauses notification time. Keep result text
short enough for the footer's two lines. Other screens retain floating notices.
Timeline rows and entity action menus scroll when they exceed the available height.
Reservation cancellation and full policy metrics are not implemented. Activity
splitting is described below. No external game's rule
compatibility is claimed by the neutral component tests.

Resident views may bind `visible` to a public boolean attribute or observation.
When set, only an exact true value displays the resident and its selection target;
a missing or non-boolean value hides it. Omission preserves all residents and
explicit order. Authors can reveal newly available residents using saved state
and explicit observation refresh, without exposing hidden rule attributes.

Grouped timelines may specify `view.rows`, each with a `label` and `items` of
`activity`, short `label`, and optional authored `screen`. Items occupy separate
lanes within each row. Optional row `text` interpolates an authored summary
in place of the lane-label line, at a minimum of 15 logical pixels. It uses
the same saved-state environment as other authored text and does not change
activity state or timeline geometry. For example, a workshop can display
inspection counts while retaining its production schedule. Unlisted activities appear in fallback rows; duplicates
and unknown references are rejected. The axis stays at the inclusive calendar
range, clipping extended deadlines. Month and Monday ticks, closed-day shading,
planned outlines, reported fills, expected-position ticks and shortfall hatching
share that axis. Cancelled lanes remain hatched in this grouped view so exclusions
do not silently change the row order. The legacy ungrouped view still omits them.

`view.markers` adds ISO `date`, `label` and optional `milestone` annotations.
Pointing at the calendar band reads that day's names; milestones have dashed
vertical lines. Fullscreen grouped views allow pointer selection or up/down and
Enter to open a lane's authored screen. The footer shows the selected activity,
assignee, reported percentage and dates. No hidden progress or verification history is inferred. An optional lane `sample` expression supplies
a saved ratio from 0 to 1, with `sample_label` for its date and source. Valid
samples draw a downward wedge and a footer percentage; absent, negative or
invalid values hide it. Authors snapshot the ratio and provenance in ordinary
save variables at the observation event. Rendering never reads live progress
to populate or refresh a sample. Optional `sample_freshness` is a pure expression
from 0 (old) to 1 (fresh), blending the marker between the theme's dim and text
colors without removing it. Values are clamped; errors and non-finite or
non-numeric values use 0. Omission retains full brightness. Authors choose the
aging interval from saved observation times, and include the age in `sample_label`
so color is not the only signal. This does not move the historical marker.
At the standard logical viewport twelve groups
with three lanes fit without scrolling; larger tables scroll whole groups.

An optional lane `forecast_label` enables a linear finish extrapolation using
reported work and calendar days elapsed since the authored start. It is not a
working-day or staffing model. The author supplies a label explaining that basis.
Zero reports, dates at or before the start, completed reports and cancelled work
show no forecast. Dashes run from today to the predicted end boundary; the footer
gives the inclusive finish date. Dates beyond the calendar are clipped and marked
with `>` in the footer. Omission preserves the existing view. Neither live hidden
progress nor completion flags influence this forecast.

Targeted actions on fullscreen islands use their existing entry `text` as an
optional confirmation preview. Selecting an action with text shows that text
below the menu without running commands; selecting the same row again or
pressing Enter confirms it. Moving to another row clears the pending
confirmation. Escape or clicking outside the menu cancels it. Authors should
include the effect, cost and confirmation instruction in the text. Entries
without text retain immediate activation. The preview uses 18 logical pixels
and at most five lines; authors must keep the explanation within that space.

Monthly calendars reuse `view.markers` as named day entries and shade calendar
closures. `view.week_start` chooses the first column (0 Sunday by default,
1 Monday); the seven weekday labels remain in Sunday-first order. Named markers
replace the legacy reservation count on that day. Cells show up to three lines,
with a numeric overflow indicator; pointing at a day shows its names in the
footer. Fullscreen calendars offer left/right keys and arrow buttons to browse
within the authored start/end months. Browsing changes only transient view state,
never the simulation date, reservations or save. Reopening starts at today's month.

Grouped timelines may opt into an assignee selector with `view.filter_label`,
which labels the unfiltered state. The arrows cycle through that state and the
public residents selected by `order` and `visible`; the middle name/portrait
resets to all residents. `portrait` uses the same public binding as resident
cards. Other residents' lanes become faint but keep their positions and remain
selectable, including in authored quality summaries. Refresh preserves the
selection; reopening clears it, and a newly hidden resident clears the filter.
The selector changes no assignments, observations, date or saved state. Existing
left/right tab keys retain their meaning. Omission keeps the previous view.

Grouped rows with a `portrait` binding show distinct visible assignees beside the
row name, in lane order. Cancelled lanes do not contribute a resident. Up to three
24-pixel portraits fit in the name column; for larger groups the third position
shows the remaining count. Names and summaries truncate at the existing minimum
text size, while pointing at the name column reveals the full row label, resident
names and summary in the footer. The selected filter resident has an accent ring.
Portraits come only from the public binding, so authors can supply a neutral icon
for off-site residents. Omission retains text-only rows. This does not change the
calendar geometry, assignments or saved state.

Grouped timeline name headers toggle one inline group at a time. Expanded
lanes show activity names, public assignees and reported percentages at 16/15
logical pixels; their labels and bars open the same authored item screen.
The calendar axis remains fixed. Opening a group near the footer scrolls it
into view; collapsing restores the overview. Groups too tall to fit the
available detail area retain the compact layout. Expansion and focus survive
screen refresh but are presentation state, not save data. A twelve-group
overview still fits at the standard logical viewport when collapsed.

## Finite collection queries

Rules can read `activities` and `entities` as lists sorted by ID. The lists are
fresh views of the current state; reading them does not mutate state or draw
random numbers. Activity items expose `id`, authored `from` and `to`, current
`deadline`, authored `planned_effort`, current `effort`, `progress`, `assignee`, `completed` and `cancelled`. Entity
items expose the same rule values as `entity[id]`, including hidden attributes.
These lists are for rule expressions, not automatic public display bindings.

`sum(list, expression)`, `count(list, predicate)` and `filter(list, predicate)`
bind `it` to each item. Nested queries shadow `it` only inside their own body;
`self` and `arg` retain the caller's meaning. Filters preserve list order. Sums
accumulate left to right, require finite numeric terms and reject overflow.
Predicates must return booleans. Empty sums and counts return zero; an empty
filter returns an empty list. An unknown field is an error, not a zero value.

Each input list is limited to 4,096 items, and a whole expression is limited to
65,536 item visits, including nested queries. Exceeding either limit fails the
expression rather than returning a truncated result. The functions are syntax
forms, not first-class callback values. Existing ordinary functions and scalar
expressions retain their behavior. This does not yet introduce command iteration
or weighted selection.

For example, a workshop, farm or training schedule can report remaining effort
without enumerating activity IDs:

```yaml
- set:
    var.remaining: 'sum(filter(activities, !it.cancelled), it.effort-it.progress)'
    var.overdue: 'count(activities, !it.completed && planning.date > it.deadline)'
```

The executable neutral example is `collection_demo` in
[the argument workshop](../examples/event-arguments/data/story.yaml). Its CLI
scenario checks current cancellation state, entity counts and authored dates.
The expression tests cover empty lists, nested scopes, type failures and both
resource bounds. Floating-point results can differ in their last bits when an
authored expanded sum used an order other than ID order; authors migrating such
sums must verify their observable decisions and saved results.


`range(start, end)` constructs ascending integer offsets, including `start` and
excluding `end`. Bounds must be finite integers within the exact integer range
of binary64 (absolute value at most 9,007,199,254,740,991); reversed bounds and
more than 4,096 items are errors. Equal bounds produce an empty list.
`first(list, fallback)` returns its first item or the fallback for an empty list,
with the same list-size limit. Like ordinary functions, both arguments evaluate
eagerly; use pure fallbacks for searches. Neither function draws randomness.
Together with `filter`, these replace expanded date-search conditionals while
keeping an explicit bounded search horizon. The workshop collection example and
its CLI scenario search for the next open day and cover exhausted candidates.

### Activity-local attributes

`planning.activity_attributes` declares typed scalar defaults shared by activities.
It uses the same `type`, `initial`, bounds and explicit assignment rules as entity
attributes. Activity field names such as `progress`, `effort` and `id` are reserved.
An activity stores only assigned overrides; an absent override reads the declared
default. Saves reject unknown override keys and invalid values. Existing saves
without overrides continue to read defaults, without adding mutable global keys.

`plan: {op: set, activity: prepare, attribute: moisture, value: 'self.moisture + 1'}`
updates one activity. `activity: {expr: arg.task}` selects a typed dynamic target.
`where` and `value` bind `self` to the same full activity object exposed through
`activity[ID]` and `activities`. Entity/tag selection cannot accompany an activity
assignment. Failed operations and rules roll back their writes. Assigning work
to another resident leaves the activity's attributes unchanged.

These attributes are rule state, not automatic player-facing observations. Use
an explicit activity observation or authored display expression to expose them.
For example, a workshop can declare `moisture: {type: number, initial: 0}` and
compute `sum(activities, it.moisture)` without `moisture_prepare` globals. The
[argument workshop](../examples/event-arguments/game.yaml) demonstrates this
with a bounded inspection counter and an argument-based update event.

### Decomposing work without increasing its budget

A root activity may declare two or more authored children using `parent`. Only
one level is supported. Their initial efforts must sum to the root's effort;
children start unassigned and dormant. For example:

```yaml
activities:
  prepare: {name: Prepare clay, effort: 2, assignee: potter}
  mix: {name: Mix clay, parent: prepare, effort: 0.5}
  knead: {name: Knead clay, parent: prepare, effort: 1.5}
```

`plan: {op: split, activity: prepare}` activates the children, clears the root
assignee and distributes existing actual and observed work by authored effort.
Repeating the operation is inert. The `active` expression field identifies the
current work units; `parent` and `split` support authored filters. A split root
aggregates child progress and observations. Observing that root refreshes its
children. Root effort, cancellation and deadline changes propagate to the
children. Root rework is distributed in proportion to existing child work;
positive adjustments use remaining budgets. Work history counts only roots.
Save/load preserves the decomposition. New authored child definitions require a
new game; there is no automatic migration of an older content schema.

Quality, difficulty, priorities and assignment drafts can be ordinary typed
activity attributes or event variables. They are not engine-specific concepts.
Authors decide how these quantities aggregate and how available time is spent.
Use `active` when selecting work and retain root IDs for dependency checks.

### Workspace controls and navigation

`kind: workspace` uses the planning window and authored bounded entries, with a
quiet opaque card surface. An entry's interpolated `icon` draws a portrait;
`muted` evaluates in that entry's bindings and adds an unavailable indicator.
The same entries remain keyboard accessible. A pointer drag from a portrait to
another action applies the source and destination actions in sequence, matching
two-click selection. Authors must keep those actions reversible until confirmation.

`destination` replaces the current authored screen after its `do` completes.
The destination must be parameterless. Unlike nested `open`, repeated navigation
does not accumulate screens; closing resumes the original opener. Pending choices
and destination changes survive session save/load. For example:

```yaml
screens:
  studio:
    kind: workspace
    title: Studio
    entries:
    - label: Supplies
      bounds: [32, 160, 240, 48]
      destination: supplies
  supplies:
    kind: workspace
    title: Supplies
    entries:
    - label: Studio
      bounds: [32, 160, 240, 48]
      destination: studio
```

## Review refinements: dependencies, presentation and saves

Reservation selection now accepts `after` reservation IDs, `exclusive` per-day
groups and a pure `when`. Dependencies use the existing saved event-start history;
selection alone does not count as a started event. Declaration order breaks
exclusive ties. Static checks reject cycles and fixed-date contradictions but
do not solve arbitrary conditions. This keeps flag-based games compatible and
avoids a second event interpreter or a genre-specific scheduler.

Save reconciliation records the authored effort/calendar basis, validates saved
progress against the old effort first, and only normalizes limits known to have
changed. Older saves without this evidence use current limits strictly.

Presentation keeps one backdrop per composition layer. Theme roles supply card
colors; call controls declare their own icons. Plain strings have no positional
meaning. The minimum call viewport is 640 by 360. Drawing and hit tests share
island geometry. Timeline, entity-table and calendar code have separate files
within play; they introduce no new dependency edges.

Optional actor speech declarations are keyed by language, with explicit lexical
alternatives and RE2 patterns. They produce warnings on literal source lines,
not runtime edits. There is no built-in Japanese pronoun list or automatic
addressee inference. See the planning author guide for limits and examples.
