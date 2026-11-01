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
reordering YAML mapping keys does not. Older save formats remain readable for
legacy games. As before, anonymous scripts have no named replay root and screen
opening is a replay boundary.

Run the example's rules and save/resume checks with:

```sh
go test ./internal/session -run 'Test(EventArgumentsResume|BoundEventRootAndChangedSave)'
```

This is the event-call slice of [ADR 0054](../decisions/0054-typed-event-and-screen-arguments.md).
Screen arguments, expression-form `say.who` and planning target fields, screen
continuations, and pending confirmation saves remain later slices. Use existing
`where` filtering for entity selection in this slice. It does not expand event
bodies or add a generator. The session contract tests load these same example
files; the CLI E2E runs both residents and checks their resulting attributes.
