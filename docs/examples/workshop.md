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
