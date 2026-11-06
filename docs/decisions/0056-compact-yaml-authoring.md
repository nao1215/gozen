# ADR 0056: Compact YAML authoring

Status: Accepted (2026-11-06). The migration command was dropped the same day.

## Authors and problem

The four main examples contain 49 events, 42 of which contain only `do`.
Go Zen writes `enabled: true` on 51 tactic rows and uses an item mapping for
14 single-item transfers. These repeated structural words add indentation and
editing work without describing additional game behavior.

## Decision

Schema 11 accepts an event as a command list. The full event mapping remains
available for `trigger`, `when`, `once`, `location` and `params`. An item ID is
accepted for `give` and `take`, meaning one item. Explicit quantity mappings
remain available, including a count of zero.

An omitted `enabled` on an authored tactic row means true in schema 11. Explicit
false remains false. Schemas 0 through 10 retain their original defaults.
Changing only the version number is therefore not an upgrade procedure.

The migration command (`gozen migrate`) first decided here was dropped on
2026-11-06 by the owner's rule: before v1.0.0 the YAML and save formats may
change between versions, so gozen ships no migration tools. Older schema
versions still load with their own defaults; a game moves to schema 11 by its
author changing the header and writing `enabled: false` where a tactic row
omitted it.

The schema loader retains canonical runtime command paths, so shortened events
have the same replay roots. Diagnostics retain the author's file and line.
Unknown definition fields and ordinary references receive a close-name hint
only when there is a unique candidate. A hint never changes data automatically.

## Alternatives and consequences

Removing the full forms would force special syntax for event metadata and
quantity expressions. Changing all historical defaults would silently enable
previously disabled tactics. Keeping the old default in schema 11 would preserve
51 redundant switches in the principal RPG example.

Actor-keyed dialogue shorthand was considered but not added: literal speaker
names and expression speakers already share `say.who`, and an unrestricted map
would make a misspelled field look like a new speaker. Repeated workspace cards
need a separate review of layout and data binding; source compression alone is
not a sufficient abstraction.

No package boundary changes or architecture exceptions are needed. The loader
owns the syntax. Old engines cannot read schema 11. Command traces hash source
bytes and must be recorded again after any YAML rewrite.
