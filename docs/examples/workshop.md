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
