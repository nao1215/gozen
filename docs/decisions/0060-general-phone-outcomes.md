# ADR 0060: General phone outcomes and colors

Status: Accepted.

## Context

Game authors need to continue when a phone screen receives no answer, operate
notifications with a keyboard, and fit phone pages and chat bubbles to their
UI palette. An alarm-specific mode would couple the framework to one story.

## Decision

Lock, page and banner cues share `timeout`, `timeout_result` and `var`. Expiry
uses the ordinary recorded phone result, as choices record their muted result.
Existing incoming-call `ring` and `missed` keep their timing, assignments and
result order. A banner with `var` waits for open, dismiss or timeout; Enter and
click open it, and Cancel dismisses it. Existing asynchronous banners remain.

A timeout requires `var`. Untimed pages with `var` still require a button,
and untimed locks with `var` require a call. Page dismissal returns `dismiss`
with or without a timeout; completion without a button returns `open`. New
close outcomes are appended without changing existing button, call or expiry
indices. In a mixed lock, declining a call advances to the next notice; only
declining the final call closes with `decline`. Ordinary notices can close
with `open` or `dismiss`. Auto, Skip and Ctrl never choose a kept outcome;
authored timeouts and call rings still expire. Input wins on the deadline tick.

Phone page and owner-bubble colors belong to the existing game-wide `ui`
settings, with the current white and blue defaults. No per-page theme flag or
background-picture feature is needed. Keyboard operability and configurable
colors are general fixes.

## Consequences and alternatives

There is no alarm mode, alarm sound key or new vibration effect. Authors compose
ring-again and snooze behavior using ordinary event conditions, waits and calls.
A specialized alarm layout or banner-only timeout vocabulary would duplicate
state and increase the schema for one application. Shared outcomes retain the
existing event/save/replay ownership, and omitted colors retain default visuals.
