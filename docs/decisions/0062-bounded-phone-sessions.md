# ADR 0062: Authored phone information unlocked by flags

Status: Accepted and implemented. Supersedes the unimplemented persistent phone-session proposal. ADR 0057's close-result save contract remains unchanged.

## Consumers and decision

Authors of contemporary narrative games need a contact directory, a home screen, past conversations and a social feed. They define that information in YAML and reveal it as the story advances. The phone displays authored data according to existing game state. It does not record a conversation log.

Messages, posts and rows use existing expressions in `when`. Omitted conditions mean visible. Conditions are evaluated once when the phone opens, not on every update or draw. False conditions hide their entries without renumbering their authored positions or result indices. No hidden entry can be selected through keyboard, pointer or scripted input.

`history: true` on a chat displays its visible messages as a scrollable history in authored order. Ordinary chats retain their existing sequential presentation. `replies` contains authored buttons with `id`, `set` and optional `when`. Selecting a reply explicitly sends that authored response through the existing phone result mechanism. Closing or browsing the history does not send anything. Authors reveal a matching owner message using the flag set by the reply. There is no free-text entry or dynamically appended transcript.

`app: home` presents a grid of row buttons. `app: contacts` presents item rows, optionally selectable through a button. Both retain the ordinary `var` and `set` result contract. A row may name a parameterless `event`. The existing event runner invokes it and redisplays the parent phone when it returns. Returning from a child app therefore shows home again; closing home continues the story. Repeated visits do not recursively stack copies of the home event. The phone view does not own an event interpreter or launch a second runtime.

## Ownership and compatibility

Definitions and validation remain in `internal/model/gamedata`; the event runner evaluates visibility and applies the selected result. `internal/play` owns layout, scrolling and input. It never writes gameplay flags directly. The existing `internal/rpg` flag and variable save format remains sufficient.

Loading restores flags and variables. Reopening the phone reconstructs the visible contacts, messages and posts from those values. This change does not save the active app, scroll position or progress through an open phone. Existing save restrictions stay in place, and old result indices retain their meaning.

No conditions, replies or history mode are required for read-only use. Existing phone cues keep their behavior and default appearance. Only phones that use history or navigation need their layout and input handling; hidden media is not prepared for display.

## Limits and alternatives

The order is authored, not the chronological order in which the player unlocked entries. Mutually exclusive replies need mutually exclusive conditions if both must not remain available. A sent reply can be hidden with `when: '!flag.sent'`. Reading an app may set an authored read flag in its surrounding event; drawing an individual message does not infer a read receipt.

A persistent phone session would add an action cursor, timers and open-app save state. Those are unnecessary for the agreed authored-data model. The phone continues to return one ordinary result when a cue ends; game-specific resource, schedule and relationship rules remain in authored events.

## Acceptance

- Hidden contacts, messages, posts and replies are absent from layout and input.
- A reply after a hidden reply keeps its original result and assignments.
- Dismissing a chat never sends a reply; reopening after a sent flag is restored shows the authored sent message without executing its assignment again.
- History scrolls both ways; replies remain operable with keyboard and pointer.
- Home and contact rows use authored IDs and existing event results.
- Default phone behavior and result/save replay tests continue to pass.
- Schema, documentation, CLI E2E and two neutral games exercise the feature; real graphics are verified separately from headless state tests.
