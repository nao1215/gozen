---
title: Planning screens and narrative checks
description: Choose a planning surface, declare scene prerequisites, and check a character's voice.
weight: 38
toc: true
---

Planning is optional in schema 10. Start with a plain authored list for choices.
Use `entities` for a roster, `timeline` for work, `calendar` for dates, `islands`
for seats, `ensemble` for a conversation, and `workspace` only when controls need
explicit positions. The neutral workshop under `examples/workshop/game` contains
working definitions. The [workspace recipes](../workspaces/) explain
the larger layouts, and [typed arguments and repeated calls](https://github.com/nao1215/gozen/blob/main/docs/development/event-arguments.md) the event arguments.

## Start with a list

A plain screen needs a title and entries. A label with `do` is an action; a label
without an action is explanatory content. `when` hides a row. On a bounded
action (an entry with `bounds` and `do` or `destination`), `unavailable`
keeps it visible but refuses activation, and `reason` explains why. These two
fields are not supported on plain list rows. Mouse,
keyboard, scripts, headless sessions and saved-screen replay share the same
activation check. An evaluation error refuses activation.

<!-- verify: parse illustrative screen fragment; requires an existing game -->
```yaml
screens:
  workshop:
    title: Workshop
    entries:
      - label: Inspect kiln
        do: [{say: The kiln is cooling.}]
      - label: Work can resume after inspection.
```

Add a planning calendar, entities and activities only when the game needs these
rules. An activity's `after` refers to other activities and gates work. A
reservation's `after` refers to other reservations and gates scene selection;
these are separate namespaces.

## Scene dependencies without bookkeeping flags

`planning.schedule` accepts optional `after`, `exclusive` and `when`. `after`
lists reservation IDs whose events must already have started, recorded by the
engine's existing saved `ran` state. A dependent is not selected merely because
its prerequisite was queued by the same dispatch. Call `plan: {op: dispatch}`
again after the prerequisite returns. Normal execution runs each selected event
in order. A recursive dispatch inside a prerequisite can see that event as
started; this is not a promise that all its dialogue was read.

`exclusive` admits at most one reservation from the group on a calendar date,
across phases and save/load. Authored order breaks ties. `when` is a pure
expression; use public entity attributes for presence or other eligibility.
False leaves a reservation pending. Expression errors roll back the whole
dispatch. Existing schedules without these fields keep their behavior.

<!-- verify: parse illustrative schedule fragment; requires the listed events -->
```yaml
planning:
  schedule:
    - {id: arrival, phase: morning, event: arrival_scene}
    - id: inspection
      phase: morning
      event: inspection_scene
      after: [arrival]
      exclusive: visitor_scene
      when: entity.potter.present
```

`gozen check` rejects missing reservation references, dependency cycles, fixed
dates outside the calendar, prerequisites fixed later than their dependent,
and same-day dependencies in one exclusive group. Existing flag diagnostics
still report flags never written or never read. Arbitrary conditions are not
solved: the checker cannot prove general reachability, presence, or the order
of dynamically indexed flag writes. Repeated dispatch plus a script is the
runtime check for those conditions. An exclusive group chooses one event;
it does not guarantee that every member eventually runs.

## Optional character voice checks

An actor may declare `speech` keyed by the game's language. Without a matching
language declaration, no dialogue is inspected. `first_person`, `addresses`
and `endings` name preferred forms and explicit alternatives to flag. They do
not rewrite lines. Address keys identify the author's intended recipient;
the check is lexical and cannot infer the addressee from narrative prose.

<!-- verify: parse illustrative actor fragment -->
```yaml
actors:
  potter:
    name: Potter
    speech:
      en:
        match: word
        first_person: {preferred: I, avoid: [we]}
        addresses:
          weaver: {preferred: Master, avoid: [buddy]}
        endings: {preferred: please, patterns: ['dude[.!]?$']}
```

`word` is the default and uses Unicode letter/number boundaries. `substring`
is useful for languages without word spaces. RE2 `patterns` allow different
language-specific rules, including endings anchored with `$`. Patterns match
discouraged forms, not required ones: absence of a pronoun is not an error.
GZN0123 is a warning. Quoted speech can legitimately match; review it manually.
Only literal source lines with a known actor are checked. Dynamic speakers,
`text_expr`, interpolated values and translated alternatives are not inferred.

## Appearance and compatibility

A `plate` presents information; a `button` acts; a `ticket` presents progress;
a `carousel` groups bounded controls. Use the existing theme before adding
individual colors. `ui.material` controls surface finish; `ui.light` controls a
light surface palette; `ui.tones` distinguishes authored emphasis; `ui.chrome`
controls decorative frames. Call and chat views require `kind: ensemble`.
Call views require at least 640 by 360 logical pixels. Larger fonts can require
more caption pages. Call control strings draw labels without implied icons;
use `{label: Leave, icon: leave}` to request an exit icon and color explicitly.
The unused `view.door` field was removed before release.

Material panels sample the current layer, including previously drawn panels
and content. Overlapping panels therefore blur what is already behind them. Wallpaper is regenerated when its colors or
strength change, and GPU resources are released on reload and close.

## Saves and automation

New saves record authored effort and calendar bounds. Values are validated
against the saved effort before definition changes are reconciled. Without
this metadata an old save must fit the current effort and calendar; out-of-range
values are rejected rather than silently repaired. Definition additions and
removals retain their existing reconciliation behavior. Checkpoint commands
require schema 10. Carry values are filtered through the current allowlist on
read, application and write, including when the allowlist becomes empty.

`sim script` retains every simulation tick and seed. Idle planning screens skip
input-only geometry work. Capture preparation advances in bounded bursts before
image sampling. These changes do not skip event waits or random draws.
Web export has a separate four-worker ceiling and a 2 GiB estimated image-work
budget, using 56 bytes per pixel. A source exceeding the budget remains in its
original format. This is an estimate, not an operating-system RSS limit.

For complete card, flow, call and guide examples, see [workspace recipes](../workspaces/).
