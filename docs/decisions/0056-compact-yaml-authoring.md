# ADR 0056: Compact YAML authoring with a reversible upgrade

Status: Accepted (2026-11-06).

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

`gozen migrate GAME --out NEW` validates the input, copies it, transforms its
YAML and validates the result before publishing a new directory. It never
replaces the input or an existing destination. The migrator writes explicit
false for previously disabled rows, shortens metadata-free events and item-only
transfers, and preserves game identity, data version, text and command order.
Explicit item counts stay explicit, even one: existing replay fingerprints
distinguish an omitted count from a supplied value. Comments attached to a
wrapper can keep that wrapper. Assets and saves are copied without conversion.

Versioned games 1 through 11 can migrate. Unversioned/schema-0 games continue to
load but are refused by the automatic upgrade: their numeric zero semantics
require a separate migration. The failure leaves the source unchanged.

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
owns syntax and the migration transformation; the CLI owns directory publication.
Old engines cannot read schema 11. Command traces hash source bytes and must be
recorded again after any YAML rewrite; game state and command replay fingerprints
are checked separately for equivalence.
