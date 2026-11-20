---
title: Planning screens and narrative checks
description: Choose a planning surface, declare scene prerequisites, and check a character's voice.
weight: 38
toc: true
---

Planning is optional in schema 10. Start with a plain list of choices. Use `entities` for a roster, `timeline` for work, `calendar` for dates, `islands` for seats, `ensemble` for a conversation, and `workspace` only when controls need fixed positions. `examples/workshop/game` has working definitions; the [workspace recipes](../workspaces/) cover the larger layouts and [typed arguments and repeated calls](https://github.com/nao1215/gozen/blob/main/docs/development/event-arguments.md) the event arguments.

## Start with a list

A plain screen needs a title and entries. A label with `do` is an action and a label without one is text. `when` hides a row. On a bounded action (an entry with `bounds` and `do` or `destination`), `unavailable` keeps it visible but refuses it and `reason` says why; plain list rows do not take these two fields. Mouse, keyboard, scripts, headless sessions and saved-screen replay share the same check, and an evaluation error refuses the action.

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

Add a planning calendar, entities and activities only when the game needs those rules. An activity's `after` names other activities and holds back work; a reservation's `after` names other reservations and holds back scenes. The two sets of IDs are separate.

## Scene dependencies without bookkeeping flags

`planning.schedule` takes optional `after`, `exclusive` and `when`. `after` lists reservations whose events must already have started, as the saved `ran` state records. A prerequisite queued by the same dispatch does not count, so call `plan: {op: dispatch}` again after it returns. Selected events run in order. A dispatch nested inside a prerequisite sees that event as started, even if not all of its dialogue was read.

`exclusive` lets at most one reservation of the group run on a calendar date, across phases and save/load, with authored order breaking ties. `when` is a pure expression (use public entity attributes for presence and the like); false leaves the reservation pending, and an expression error rolls back the whole dispatch.

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

`gozen check` rejects missing reservation references, dependency cycles, fixed dates outside the calendar, prerequisites fixed later than their dependent, and same-day dependencies in one exclusive group; flags never written or never read are reported as usual. It does not solve arbitrary conditions such as reachability, presence or the order of flag writes by computed index; test those with repeated dispatches in a script. An exclusive group picks one event and does not promise that every member runs eventually.

## Optional character voice checks

An actor may declare `speech` keyed by the game's language; without an entry for the game's language nothing is checked. `first_person`, `addresses` and `endings` name the preferred forms and the alternatives to flag, without rewriting lines. Address keys name the recipient the author means, since the check reads words and cannot tell the addressee from the prose.

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

`word`, the default, matches at Unicode letter and number boundaries; `substring` suits languages without spaces between words. RE2 `patterns` add rules of a language, such as endings anchored with `$`. Patterns match the forms to avoid, so a missing pronoun is not an error. The finding is the warning GZN0123; quoted speech can match on purpose, so review it by hand. Only literal lines with a known actor are checked, not dynamic speakers, `text: {expr}`, interpolated values or translations.

## Appearance and compatibility

A `plate` shows information, a `button` acts, a `ticket` shows progress, and a `carousel` groups bounded controls. Try the theme before setting colors one by one. `ui.material` sets the surface finish, `ui.light` a light surface palette, `ui.tones` the authored emphasis and `ui.chrome` the decorative frames. Call and chat views need `kind: ensemble`, and call views at least 640 by 360 logical pixels; larger fonts can need more caption pages. A call control given as a string draws a label without an icon; `{label: Leave, icon: leave}` asks for the exit icon and color.

Material panels sample the current layer, panels drawn before included, so overlapping panels blur what is behind them. Wallpaper is redrawn when its colors or strength change, and GPU resources are released on reload and close.

## Saves and automation

Saves record the authored effort and calendar bounds, and values are checked against the saved effort before changed definitions are applied. An older save without these records must fit the current effort and calendar; values out of range are rejected, not repaired. Checkpoint commands need schema 10. Carry values pass through the current allowlist when read, applied and written, even when the allowlist is empty.

`sim script` keeps every tick and seed. Idle planning screens skip geometry work that only input needs, and capture prepares in bounded bursts before sampling the image; neither skips event waits or random draws. Web export uses at most four workers and an estimated 2 GiB for image work at 56 bytes per pixel; a picture over that budget keeps its original format. The budget is an estimate, not an operating-system RSS limit.

For complete card, flow, call and guide examples, see [workspace recipes](../workspaces/).
