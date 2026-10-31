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

Monthly ledgers accept explicit historical periods until closed. Closing twice or
writing a closed month fails. Retention removes old totals but retains closed
period markers so pruned periods cannot be reopened. Closure is explicit, allowing
pending declarations to settle first.

The additions are optional fields in schema 9 and optional state in save format 2.
Old games retain fixed rendering and their existing state. The native screen
option renders a 16:9 inner framebuffer at device resolution; Ebitengine owns
letterboxing and pointer conversion. Scripted captures with an explicit window size include opaque black outer bars,
centered around the native framebuffer without resampling its pixels.

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

Scrolling of long planning views, reservation
cancellation, general activity splitting and full policy metrics are not implemented. No external game's rule
compatibility is claimed by the neutral component tests.
