# Work overview cards and burnup

The pottery workshop's **Studio overview** opens a light `islands` surface. Click a resident to work or rest; the plan and calendar links are read-only.

`view.burnup` shows scope, planned work, observed work, today's position and the calendar end. Labels are authored strings. `planned` optionally evaluates a pure expression with `selected.date`; otherwise the budget interpolates working days between each activity's start and current deadline. Cancelled activities are excluded. The plan reflects the current schedule, while observed history retains earlier scope and reported work.

Successful `plan: {op: observe}` operations record the latest aggregate public sample for that date when a burnup view exists. Rendering never observes or advances the simulation. Samples survive save/load and checkpoint restoration. Earlier dates in old saves remain unknown; the chart never reconstructs progress from the true internal value. Observations may decrease after rework. Neither the chart nor cards read hidden attributes.

Resident cards center a circular portrait above the full authored name. Up to two concise fields work best for a large roster. `maximum_key` binds a public per-resident gauge limit, `warning_key` binds an amber threshold, and `critical_ratio` marks proximity to that limit in red. `text_key` provides a formatted public label while `key` remains numeric. Zero or missing limits disable the gauge, allowing text such as “Unavailable”. Limits and thresholds have no effect on simulation rules.

Use `view.light: true` for paper-colored surroundings and white cards. This setting applies to the surface and its contextual actions; it does not change other screens or dialogue themes.


## Frosted panels

The optional UI material applies to planning panels, meeting transcripts and
message windows. Omit it to retain the classic style. This neutral configuration
can be used with the workshop example:

```yaml
ui:
  material: {kind: glass, blur: 14, opacity: 0.78, border: 0.45, shadow: 0.65}
```

Set `kind: opaque` to eliminate background transmission. `blur` is a logical
pixel footprint (0–64); the other values are strengths (0–1), including zero.
A fullscreen planning view may set `background: pictures/studio.png` to an
author-supplied picture. No image or private game content is bundled here.
The background is dimmed and reduced once, then reused until the picture,
output size or blur changes. Moving dialogue stages refresh the reduced buffer
before UI drawing. All panels share that buffer; rounded edges are antialiased
at the output resolution. Tint follows the existing light/dark theme.

Burnup charts show today's authored planned and observed labels with an optional
`unit` suffix. A missing historical observation is shown as a dash, never zero.

## Automated policies and observations

A planning policy reads the same public entity fields through `entity.ID` and
`entities`. Activity queries through `activity.ID` and `activities` expose
reported `progress`, budget, assignee, deadline and cancellation, but omit true
completion. Collections are ordered by ID. For example, a workshop policy can
use `sum(activities, it.progress)` to react to reported work. Hidden resident
attributes remain unavailable even inside `filter`, `count` or `sum`.

Policies do not refresh observations. An observation can remain stale while
rules change internal work. Querying a policy never modifies saved planning
state, including when a private-field query fails.

Rule expressions also expose `activity.ID.observed` (and `it.observed` inside
`activities`) separately from true `progress`. Use `observed` when deriving
public remaining-work labels in entity observations. For example,
`sum(activities, !it.cancelled ? max(0, it.effort-it.observed) : 0)` sums reported
remaining work. A policy exposes that same reported value as both `progress`
and `observed`; it never receives true progress under either name.
