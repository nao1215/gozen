# Hand-designability review, 2026-11-06

A person can design the sample games by hand. The limiting factor is how many
places must change together when a rule changes, not the number of YAML lines.
The largest generated input reviewed separately is not a reasonable manual
editing surface in its expanded form. Its underlying rules can mostly be
expressed with existing events, typed arguments, collections and planning rules.
Do that consolidation before adding another authoring language.

This is a review, not an engine change or a migration of any game. The external
input was read only. Its identifiers, text, paths and inspection records are
excluded here; all examples below are original neutral examples. Passing a
complexity budget does not establish human usability, and a warning does not
prove redundant behavior or unreachable content.

## Samples

Counts include `game.yaml` and `data/**/*.yaml`, including comments and blank
lines, but exclude translations and assets. The judgments come from reading
those definitions, not from a user study.

| Sample | Files / lines | Manual design judgment and evidence |
| --- | --- | --- |
| Minimal | 2 / 17 | Yes: one place, one actor and two skills. `examples/minimal/data/world.yaml:10` keeps the whole interaction local. |
| Novel | 3 / 282 | Yes: eight events carry authored dialogue and choices. Dialogue is content, not duplication to eliminate. `examples/novel/game/data/story.yaml:1` and `examples/novel/game/game.yaml:18` separate story and presentation. |
| Campaign | 6 / 241 | Yes: campaign orders already replace a hand-written turn dispatcher. `examples/campaign/game/game.yaml:24` and `examples/campaign/game/data/story.yaml:15` separate war rules from story. |
| Go Zen | 13 / 2,647 | Yes, incrementally: 29 events and separate files for classes, skills, encounters and story. Start with one area; do not ask an author to hold the entire game in mind. `examples/go-zen/game/data/story_dungeon.yaml:1` and `examples/go-zen/game/data/monsters.yaml:1` expose different editing tasks. |
| Workshop | 2 / 136 | Yes: two residents, two activities, five screens and one shared recovery rule. `examples/workshop/game/game.yaml:34` and `examples/workshop/game/data/workshop.yaml:18` already demonstrate reuse. |
| Office | 2 / 229 | Yes at this size, with friction: nine screens repeatedly spell out six people's names, pictures and card bounds. Lines reach 240 bytes; compact flow maps conceal editing work. See `examples/office/game/game.yaml:49` and `:63`. |
| Space | 2 / 101 | Yes with graph paper: aligned level grids and two stair links. `examples/space/game/data/world.yaml:25` makes geometry visible; alignment and floor numbering need rendered checks. |
| Space town | 2 / 71 | Yes: a small building and a few interactive tiles. `examples/space-town/game/data/world.yaml:11` keeps rules beside geometry. |
| Space buildings | 2 / 104 | Yes, but coordinate-heavy: material regions and roofs must agree with level grids. `examples/space-buildings/game/data/world.yaml:46` names shared materials; another macro language would not solve spatial mistakes. |
| Puppet asset example | Not a game directory | Hand-editable after measuring the image: `examples/puppet/assets/streamer/puppet.yaml:1`. Coordinates benefit from preview and `gozen puppet`, not event abstractions. |

The generated-input review found repeated event shapes, per-person/per-date state
names, long formulas, unread flag writes and a screen containing many mutations.
Existing argument-bearing calls and repeated rows were already present; their
presence alone does not eliminate copied formulas or state conventions. These
are different problems. A longer authored conversation is not equivalent
to twenty copies of one rule, and a generated name is not automatically a defect.

## Existing mechanisms, with before and after examples

The load-marked snippets below share this small context. The documentation test
loads them together; that checks syntax, types and references, not the rendered
appearance or equivalence of every possible saved game.

<!-- verify: load as=game.yaml -->
```yaml
schema_version: 11
planning:
  calendar: {start: '2030-04-01', end: '2030-04-03', phases: [morning]}
  attributes:
    output: {type: number, initial: 1}
    ready: {type: bool, initial: true}
    reviewed: {type: bool, initial: false}
    vigor: {type: number, initial: 60, min: 0, max: 100}
  entities:
    oak: {name: Oak, tags: [team], data: {greeting: Welcome back.}}
    willow: {name: Willow, tags: [team], data: {greeting: Good morning.}}
    reed: {name: Reed, tags: [team]}
  rules:
    recover:
      params: [amount]
      do:
        - {op: set, tag: team, attribute: vigor, value: 'min(100, self.vigor + arg.amount)'}
        - {op: observe}
```

### Near-identical events: typed calls and bounded repetition

Before, each identity has its own copy of a counter update:

<!-- verify: load -->
```yaml
events:
  visit_oak: [{set: {var.oak_visits: 'var.oak_visits + 1'}}]
  visit_willow: [{set: {var.willow_visits: 'var.willow_visits + 1'}}]
  visit_before: [{call: visit_oak}, {call: visit_willow}]
```

After, one body and an explicit ordered selection preserve those same counter
names. Adding a third caller does not copy the update:

<!-- verify: load -->
```yaml
events:
  visit_one:
    params: {resident: entity}
    do:
      - set: {'var[arg.resident + "_visits"]': 'var[arg.resident + "_visits"] + 1'}
  visit_after:
    - call: {event: visit_one, each: {param: resident, items: [oak, willow]}}
```

Use `each.in: 'filter(entities, it.ready)'` for membership determined by state.
Membership freezes before the calls; other arguments evaluate before each call.
Order, pauses and partial failure still matter. This is bounded iteration, not
one transaction. Dynamic state destinations are incompatible with strict literal
state declarations; typed entity attributes are preferable for a new design.
Source: `internal/model/gamedata/arguments.go:91`,
`internal/rules/event/event.go:1240`, and
[the argument contract](../development/event-arguments.md#repeating-a-call-over-a-collection).

Text interpolation and `say.text_expr` can select per-entity definition data
instead of duplicating a conversation to change one greeting. They are text
facilities, not a general YAML template engine. Selected source text affects
translation extraction and scene-read analysis; preserve authored source strings
when consolidating an existing story. See `internal/model/gamedata/command.go:249`
and `internal/model/gamedata/planning.go:76`.

### Long expressions: collections instead of enumerating identities

Before, adding a person requires editing every total and eligibility predicate:

<!-- verify: load -->
```yaml
events:
  summarize_before:
    - set:
        var.output: 'entity.oak.output + entity.willow.output + entity.reed.output'
        flag.all_ready: 'entity.oak.ready && entity.willow.ready && entity.reed.ready'
```

After, the collection defines the population once:

<!-- verify: load -->
```yaml
events:
  summarize_after:
    - set:
        var.output: 'sum(entities, it.output)'
        flag.all_ready: 'count(entities, it.ready) == len(entities)'
```

This is equivalent for the three declared entities. Filter the collection when
visitors or inactive entities should not count. An empty population satisfies the
all-ready comparison; add an explicit nonempty condition if that is undesirable.
Do not replace formulas with cached variables unless the refresh points are
clear. Observations deliberately refresh through `plan: {op: observe}` and may
be stale between observations; they are not live computed properties.
Executable collection examples are at `internal/model/rpg/planning_test.go:100`.

### Indexed names: model the entity or date, not its spelling

Before, one flag per person makes the state schema grow with the cast:

<!-- verify: load -->
```yaml
events:
  review_before:
    - set: {flag.checked_oak: true, flag.checked_willow: true, flag.checked_reed: true}
```

After, one typed attribute belongs to each entity:

<!-- verify: load -->
```yaml
events:
  review_after:
    - plan: {op: set, tag: team, attribute: reviewed, value: true}
```

Read `entity.oak.reviewed` or summarize the collection. For names such as
`p01` through `p20`, first decide whether they identify real distinct content,
people, dates or merely copied control flow. Keep distinct story IDs. Use calendar
reservations for dated appointments, activities for work, and ledgers for period
history. Replacing indexed flag names with string concatenation only hides their
number; it does not model ownership or dates.

This changes the saved state representation. An existing game needs an explicit
old-key-to-attribute migration and save/reload checks; gozen does not
do that mapping (and gozen has no YAML upgrader before v1.0.0). `state.names` gives types, initial values and strict name
checking for deliberately retained flags; it does not compress twenty flags into
one fact. See `internal/model/gamedata/state_names.go:20`,
`internal/model/gamedata/planning.go:109` and `:174`.

### Flags written but never read: remove bookkeeping or name its consumer

Before, a button records an implementation detail with no future reader:

<!-- verify: load -->
```yaml
events:
  receipt_before:
    - set: {flag.receipt_button_pressed: true}
    - say: Receipt ready.
```

After, omit the unused write:

<!-- verify: load -->
```yaml
events:
  receipt_after: [{say: Receipt ready.}]
```

First inspect expressions, gallery conditions, carried state and external scripts.
If the fact controls a later scene, retain it, declare it and write the actual
consumer. A write-only warning is a question for the designer, not permission to
delete a save field. Static checks cannot establish every external use. The flag
usage contracts are tested in `internal/model/gamedata/flags_test.go:1`.

### Mutation-heavy screens: screens select intentions, rules own mutations

Before, each action repeats the update and observation ordering. After, buttons
call one rule with different amounts. Both screens below use the same entities
and recovery rule declared above:

<!-- verify: load as=game.yaml -->
```yaml
screens:
  recovery_before:
    title: Rest
    entries:
      - label: Short rest
        do:
          - plan: {op: set, tag: team, attribute: vigor, value: 'min(100, self.vigor + 5)'}
          - plan: {op: observe}
      - label: Long rest
        do:
          - plan: {op: set, tag: team, attribute: vigor, value: 'min(100, self.vigor + 20)'}
          - plan: {op: observe}
  recovery_after:
    title: Rest
    entries:
      - {label: Short rest, do: [{plan: {op: rule, rule: recover, args: {amount: 5}}}]}
      - {label: Long rest, do: [{plan: {op: rule, rule: recover, args: {amount: 20}}}]}
```

Use a typed event call when the action also talks, opens a screen or branches;
a planning rule contains planning operations, not arbitrary dialogue commands.
Plain screen parameters and `entries[].each` already remove repeated lists and
per-person detail screens. A planning rule's error handling differs from a
sequence of independent event commands: preserve successful behavior and review
failure atomicity before migrating. See `internal/model/gamedata/planning.go:193`
and `:578`, `internal/rules/planning/planning.go:18`, and
`internal/rules/event/event.go:355`.

Positioned workspace cards are a remaining problem: repeating an entry repeats
its fixed bounds too (`internal/model/gamedata/types.go:2004`). Collection
iteration alone does not lay out a grid.
For a small deliberately composed screen, explicit coordinates remain useful.
YAML anchors can reuse a static mapping within one file, but do not provide
arguments, cross-file imports or calculated bounds.

## Ranked proposals: two, neither required for the refactors above

| Rank | Proposal and benefit | Added contract and cost |
| --- | --- | --- |
| 1 | Bounded layout for repeated workspace entries. Let a designer specify one card and a grid origin, column count, cell size and gap; expand existing `each` rows into nonoverlapping bounds. | One optional `layout` mapping with `origin`, `columns`, `cell`, `gap`; no separate template language. Specify maximum rows, overflow, scrolling, focus, hit testing and saved row identity. Add schema, examples, diagnostics and rendered tests. Keep explicit bounds unchanged; migration of a regular grid can be opt-in. Irregular cards stay authored. |
| 2 | Named pure derived expressions, only if collection refactoring leaves repeated long predicates. Give a recurring rule one visible name without introducing stale cached state. | One optional `computed` map with declared type and expression, plus a read namespace. Define scope, dependency cycles, purity and evaluation cost; forbid RNG, mutations and hidden-state leaks. Add schema, diagnostic and author-guide coverage. No saved values; migration replaces repeated expressions only after equivalence tests. This is a larger semantic cost than a text alias, so defer it until concrete remaining formulas justify it. |

Do not add generic YAML macros, a second generator language or an unrestricted
script escape hatch. They would move the difficult reasoning into expansion and
make source diagnostics harder to follow. First consolidate one representative
repeated family, compare decisions and state on fixed inputs, save and resume
through its pauses, and ask a person to add one entity and one exception by hand.
That editing exercise is the missing usability evidence; line reduction alone
cannot supply it.
