# Pottery workshop

`examples/workshop/game` is an original neutral planning fixture. Run
`gozen check examples/workshop/game` and `gozen run examples/workshop/game`.
Work or rest through July 31 to August 4, reassign bowls, inspect the calendar and
participant cards, and save through the ordinary game menu.

Definitions live in `game.yaml.planning`; event commands live in
`data/workshop.yaml`. This example demonstrates a hidden vigor attribute, a
visible condition refreshed by `plan: {op: observe}`, typed assignment, a reusable
recovery rule with an `amount` argument, activity dependencies, a month close, `period_value(ledger, entity, month_offset)` history lookup and
a scheduled assembly. A `steady` policy selects rest or work from observations.

For reproducible headless inspection, use `gozen sim script` with `--start workshop`
and a script containing these lines:

```text
do plan: {op: observe}
until idle
do plan: {op: policy, policy: steady}
until idle
snapshot
quit
```

A snapshot is a full development record, including hidden state. It must not be
shown as a player-facing report. Repeat policy invocations with a bounded script
to measure a complete run. See [ADR 0053](../decisions/0053-planning-components.md)
for the current limits. The fixture contains no third-party artwork.

Planning screens can opt into `view.fullscreen: true`. The screen then fills the
play surface with a date and status strip, its planning view, and a footer.
`header` and `footer` accept ordinary text interpolation. `date_format` uses Go
time layout tokens; `{weekday}` uses the seven `weekdays` labels, starting with
Sunday. `phases` maps calendar IDs to display labels and `event_label` replaces
the calendar count suffix. Omitted labels retain English defaults.

In this mode, an entry's `bounds: [x, y, width, height]` places a direct action in
logical screen coordinates. An entry's `target: potter` instead puts that action
in the potter's seat menu. Its usual `when` condition still controls visibility.
Up and down select direct actions and seats; confirm opens or executes the
selection, and cancel dismisses a seat menu before closing the screen. Mouse
users select the same rectangles. A timeline includes the full planned activity
range, even when an activity finishes after the playable calendar ends.

Fullscreen peers can declare the same ordered `view.tabs: [workshop, team, plan]`
on each screen. Use two to five existing fullscreen screen IDs, including the
current screen. Titles supply the tab labels. Click a bottom tab or use left/right
to switch without consuming time or stacking return screens. Cancel closes the
surface and resumes its original opener. An open seat menu must be dismissed
before switching. Nested detail screens still use ordinary `open` and cancel.
Reserve the bottom-right 152 logical pixels per tab, from height minus 72 to
height minus 24, for the controls; place authored action bounds elsewhere.

Plain authored screens (with no `kind` and no `fullscreen`) can present a
candidate list by setting `view.portrait` and binding each entry's `target` to a
planning entity. `view.fields` supplies the public values shown below each
entry label. Rows use 18-pixel labels, 15-pixel summaries and a public portrait;
long lists scroll with keyboard selection. The selected entry's `text` remains
in the detail pane. Include full explanations there when a row summary is long.
Hidden attributes are never used as a fallback for missing observations.
Selecting an entry runs its ordinary `do`; authors can use a `choice` there to
confirm a costly action. Browsing the list itself does not advance planning.

Opening a candidate list from a grouped timeline lane keeps the timeline on
the left and places the list in a 360-logical-pixel pane on the right. The whole
selected calendar range remains visible. The selected activity description stays below
the timeline; candidate text uses six lines per page above the scrolling list. Left/Right,
the arrow buttons, or the wheel over the explanation read every page at
15 logical pixels. The page counter shows when more text is available. Changing
candidates resets to the first page; paging does not select or confirm a candidate. The candidate screen's `view.footer` supplies a short navigation hint.
Escape returns to the same timeline selection and expansion. Standalone lists
retain their original layout; no new schema field is needed.

Grouped timeline `view.rows[].items[].text` adds one interpolated explanation
below the activity name and public assignment/report summary when its group is
expanded. For example, `text: "Planned {activity.clay.effort} units"` explains
an activity's scale in the author's own units. Rows with this field reserve 64
logical pixels per activity instead of 44; the collapsed overview is unchanged.
The extra line uses at least 15-pixel text and truncates overflow. Hover or
keyboard focus includes the full explanation in the timeline description.
Use public values or saved observations for player-facing explanations.

A calendar marker may set `screen: day_detail` to open an authored screen when
its date is clicked, both in the timeline band and in the month calendar.
Define `day_detail` in `screens` with the day's information and any actions.
Markers sharing a date must agree on the destination. Omitting `screen` keeps
the date read-only. Opening or closing a date detail does not advance time;
only commands explicitly authored in its entries can change the game state.
For a date link without an annotation tick or label, omit the marker label.

Grouped timelines support a presentation-only zoom: press Plus/Equals to show
21 days centered on the current date, or Minus to restore the authored calendar.
The small plus/minus buttons at the axis's upper right provide the same controls.
At calendar boundaries the focused window shifts inward; shorter calendars stay
complete. Day numbers appear daily when at least 40 logical pixels are available
per day, otherwise weekly. Bars are clipped to the viewport without rescaling
their progress, expected position or historical marks. The date band opens the
date under the pointer in either mode. Candidate browsing and screen refresh
retain the zoom; reopening the timeline starts with the full calendar. This
does not change time, assignments, observations or saves. Z retains its existing
confirm binding.
