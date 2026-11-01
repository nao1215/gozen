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
restores the complete planning state. Reusable rules bind scalar arguments and
contain at most 256 immediate operations; nested rule and policy calls are rejected.
Random draws belong to preceding ordinary event commands, not planning formulas.

Observations refresh explicitly. Hidden attributes are absent from player views;
this is not encryption. Policy conditions see observations, while event rules
can read full state. Diagnostic snapshots intentionally contain full state.

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

The additions are optional fields in schema 9 and optional state in save format 2.
Old games retain fixed rendering and their existing state. The native screen
option renders a 16:9 inner framebuffer at device resolution; Ebitengine owns
letterboxing and pointer conversion. Scripted captures with an explicit window size include opaque black outer bars,
centered around the native framebuffer without resampling its pixels.

## Authored retry points

Schema 9 events may use `checkpoint: resume_event` to replace one retry point.
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
Ensemble tiles can show up to two public fields over the lower portrait area for
briefing summaries. Authors choose the text and use an ordinary `say.ensemble`
to keep the summary within the existing dialogue and save flow. An ensemble's
public `badge` binding appears in a contrasting strip above the portrait; empty
badges add no strip. Fullscreen ensembles also retain the authored date and
header during dialogue. Header columns use normal-size text with ellipsis for
overflow, shared with the fullscreen planning surfaces.
Fullscreen entry `text` is visible below its label when its bounds are at least
72 logical pixels high. This supports an authored agenda or notice card without
adding another source of state. Target action menus have a resident heading,
show at most six rows and use the auxiliary column on island screens.
On fullscreen authored screens, `notice` results temporarily replace the footer,
one at a time, without blocking event execution. Queued results retain their
display time until shown; dialogue pauses notification time. Keep result text
short enough for the footer's two lines. Other screens retain floating notices.
Timeline rows and entity action menus scroll when they exceed the available height.
Reservation cancellation, general activity splitting and full policy metrics are not implemented. No external game's rule
compatibility is claimed by the neutral component tests.

Resident views may bind `visible` to a public boolean attribute or observation.
When set, only an exact true value displays the resident and its selection target;
a missing or non-boolean value hides it. Omission preserves all residents and
explicit order. Authors can reveal newly available residents using saved state
and explicit observation refresh, without exposing hidden rule attributes.

Grouped timelines may specify `view.rows`, each with a `label` and `items` of
`activity`, short `label`, and optional authored `screen`. Items occupy separate
lanes within each row. Unlisted activities appear in fallback rows; duplicates
and unknown references are rejected. The axis stays at the inclusive calendar
range, clipping extended deadlines. Month and Monday ticks, closed-day shading,
planned outlines, reported fills, expected-position ticks and shortfall hatching
share that axis. Cancelled lanes remain hatched in this grouped view so exclusions
do not silently change the row order. The legacy ungrouped view still omits them.

`view.markers` adds ISO `date`, `label` and optional `milestone` annotations.
Pointing at the calendar band reads that day's names; milestones have dashed
vertical lines. Fullscreen grouped views allow pointer selection or up/down and
Enter to open a lane's authored screen. The footer shows the selected activity,
assignee, reported percentage and dates. No hidden progress, inferred forecast or
verification history is exposed. At the standard logical viewport twelve groups
with three lanes fit without scrolling; larger tables scroll whole groups.

Targeted actions on fullscreen islands use their existing entry `text` as an
optional confirmation preview. Selecting an action with text shows that text
below the menu without running commands; selecting the same row again or
pressing Enter confirms it. Moving to another row clears the pending
confirmation. Escape or clicking outside the menu cancels it. Authors should
include the effect, cost and confirmation instruction in the text. Entries
without text retain immediate activation. The preview uses 18 logical pixels
and at most five lines; authors must keep the explanation within that space.
