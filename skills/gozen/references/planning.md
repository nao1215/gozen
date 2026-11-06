# Planning, workspaces and management games

Start with the author-facing guides: `website/content/planning.md` (calendar,
entities, activities, rules) and `website/content/workspaces.md` (screens).
Copy from `examples/office/game`, a small app team's sprint that uses every
workspace feature, or `examples/workshop/game` for planning state alone. Read
ADR 0053 for state, ADR 0054 for typed bindings and ADR 0055 for guides. Start new games at schema 11; planning features require at least schema 10.

- A day is a `flows.<name>` of full-screen steps (`hold` and `done` for a step
  that only time or an action finishes, `restart` for a new day).
- `kind: workspace` screens place bounded entries. `style: plate` is read-only,
  `style: button` acts; `gozen check` refuses a plate that acts. Decorate plates
  with `bust`, `meter`, `dot`, `sign` and `peek`; `ticket` draws a work card,
  `carousel` a rotating choice with `unavailable` and `reason`; `drag` and
  `drop` hand a value to `arg.<drop>`.
- `message` queues a decision in the notification corner. A video call is a
  `kind: ensemble` screen with `view.call`; `layout: full` with
  `captions: overlay` needs `ui.message: novel`, and `call_view` changes the
  camera or shares a picture without touching state. A `choice` with
  `muted: {seconds, do}` after a call's line strikes the mic and waits for
  Unmute; left muted, `do` runs (the player kept quiet). Give the call its people
  through planning attributes named by `view.portrait`, `icon`, `camera` and
  `backdrop`.
- `screens.<name>.guide` walks a first visit once; scripted plays skip it
  unless the script says `guides on`.
- A full-screen calendar or timeline over `view.background` stands on glass
  when `ui.material` is set.
- Prefer a plain list over positioned controls unless the interaction needs
  them. Use reservation dependencies for scene ordering and activity
  dependencies for work ordering. Voice checks are opt-in per actor and
  language; never infer rules for a private game's undeclared characters.
- Check a layout with `gozen sim script <game> --script - --save-dir DIR`:
  `look` lists the controls, `pick LABEL` presses one, and a trace shows
  `screen_drop`, `message_open` and `guide_done`.

For repeated behavior, use typed `call.args` and bounded `call.each`; use
`planning.rules` for shared planning operations and collection expressions for
totals and eligibility. Plain screen entries accept `each` and expression
targets. These bind existing definitions rather than generating YAML. Native
planning surfaces still have argument and save-continuation limits; read
[the binding guide](https://github.com/nao1215/gozen/blob/main/docs/development/event-arguments.md)
before replacing a screen family.
