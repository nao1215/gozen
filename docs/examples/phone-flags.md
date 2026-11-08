# A phone built from authored information and flags

Use this pattern when the story unlocks contacts, messages and social posts. All text is written in advance. The game saves its ordinary flags and variables; it does not record a second conversation log or accept free text.

The office sample has a complete example:

```sh
gozen run examples/office/game --dev --event phone_archive
```

Confirm the opening line to open the phone. Its home opens Contacts, Team chat and Updates. Closing an app returns to its parent phone. Closing home returns to the story. The test game at `e2e/atago/fixtures/input-readiness` supplies a separate studio example.

Use schema 11 for the compact event syntax below:

<!-- verify: load as=game.yaml -->
```yaml
schema_version: 11
```

## Define what becomes visible

`when` on a message, post or row uses the usual expression syntax. An omitted condition means visible. Conditions are evaluated when the phone opens; reopening it refreshes the view after flags change. Entries keep authored order.

<!-- verify: load -->
```yaml
events:
  workshop_chat:
    - phone:
        app: chat
        title: Guide
        history: true
        messages:
          - {day: Yesterday, from: Guide, text: Welcome to the workshop.}
          - {when: flag.met_guide, from: Guide, text: Your workbench is ready.}
          - {when: flag.sent_tools, mine: true, text: I will bring the tools.}
        replies:
          - button: Send - I will bring the tools.
            id: tools
            when: 'flag.met_guide and !flag.sent_tools'
            set: {flag.sent_tools: true}
```

`history: true` shows the visible messages together and allows scrolling back and forward. Up/Down, the wheel and dragging scroll. Right or Tab focuses a visible reply; OK sends the focused reply. Pointer users press its button. Cancel closes without sending. The `set` runs through the ordinary phone result only when that reply is selected. On reopening, `flag.sent_tools` reveals the predefined sent message and hides its reply button.

Without replies, this is a read-only history: no result or extra save field is required. Without `history`, the existing message-by-message chat behavior stays available. A normal feed already scrolls through posts; add `when` to reveal predefined posts and `browse: true` to keep it read-only.

## Add a home and a contact directory

A home uses `rows` of buttons; an optional `item.icon` supplies its icon. A contact directory uses item rows, with an optional button to make a row selectable. Static contact rows can display a number without accepting input.

<!-- verify: load -->
```yaml
events:
  phone_home:
    - phone:
        app: home
        title: Workshop phone
        rows:
          - {button: Contacts, event: contacts, item: {title: Contacts}}
          - {button: Chat, event: team_chat, when: flag.met_guide, item: {title: Chat}}
  contacts:
    - phone:
        app: contacts
        title: Contacts
        rows:
          - {item: {title: Front desk, value: '0100'}}
          - button: Guide
            event: team_chat
            when: flag.met_guide
            item: {title: Guide, text: Workshop guide, value: '0101'}
  team_chat:
    - phone:
        app: chat
        title: Guide
        history: true
        messages:
          - {from: Guide, text: Welcome to the workshop.}
```

Each `event` target must exist and declare no parameters. The existing event runner calls it and shows the parent phone again when it returns. Do not make the child event call home recursively. A row may also use `id`, `set` and the phone's `var` for ordinary result handling. Hidden rows cannot run their assignments or events.

Right/Tab focuses the first visible home or contact button and OK opens it. On home, Up/Down move between grid rows. Cancel returns from the current app; Cancel on the outer home closes the phone.

## Saving and authoring limits

Save flags and variables through the game's normal save flow. Reopening builds the history from those values, so viewed content does not execute its previous reply again. This feature does not add mid-phone saving, restoration of the current app, scroll position or a pending timer. Existing save restrictions and phone outcome replay still apply.

Displaying a message does not mark it as read. If opening a conversation means read in your story, set its read flag in that conversation's event. Use stable reply IDs and mutually exclusive conditions for mutually exclusive responses. The displayed history follows definition order, regardless of the order in which the player unlocked messages. Keep time labels authored too.

Read-only games can omit `when`, `history`, `replies` and navigation events. Visibility is not reevaluated every frame. History layout is cached for the open phone, and drawing visits the visible portion rather than formatting the entire conversation on every frame.
