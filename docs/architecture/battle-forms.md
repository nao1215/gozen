# Battle forms, HP pictures, and motion

Set `schema_version: 7` in game.yaml before using these fields.

An enemy can change into another enemy with a self-scoped skill. Put that skill in an AI rule with an HP condition to make a second boss phase:

```yaml
skills:
  open_shell:
    name: Open Shell
    scope: self
    effects: [{transform: guardian_open, transform_hp: ratio, transform_clear: true}]
enemies:
  guardian:
    ai:
      - {use: open_shell, when: "self.hp_rate < 0.5", once: true, priority: 5}
      - {use: attack}
```

`transform_hp` accepts `ratio` (default), `full`, or an expression such as `b.hp + 20`. The expression is evaluated before the change; `a` is the user and `b` the target. A target other than the user must be a living enemy. The slot and queued turn stay in place. The final form alone provides battle rewards.

For an immediate phase change after damage, put `hp_triggers` on the enemy instead:

```yaml
enemies:
  guardian:
    hp_triggers:
      - {below: 0.5, transform: guardian_open, hp: ratio, clear: true}
```

Thresholds descend from high to low. A crossing fires before the enemy falls, including when one hit reaches zero HP. Only the first crossed threshold fires for that hit. `hp` uses the same choices as `transform_hp`.

To combine enemies, name the resulting enemy and the eligible source IDs. The user must be eligible:

```yaml
skills:
  gather:
    name: Gather
    scope: self
    effects: [{merge: {into: large_mote, from: [mote], count: 2, hp: sum}}]
enemies:
  mote:
    ai:
      - {use: gather, when: "allies.kind_mote >= 2", priority: 5}
      - {use: attack}
```

`hp` accepts `sum` (default), `ratio` (average member ratio), or `full`. The new form appears at the middle selected slot. The engine picks the user and then eligible allies in troop order. If fewer than `count` eligible foes live, a manual action reports failure and AI skips the rule. `gozen check` reports a merge that no troop containing its AI user can meet. Only the final form pays rewards.

An enemy may select alternate picture files at HP thresholds; actors use face names in their `faces` directory. Write thresholds from high to low. At exactly the threshold, the earlier picture still applies.

Temporary hit and action reactions take priority over HP pictures. An actor's explicit `face_rules` take priority over its HP picture list.

```yaml
enemies:
  mote:
    image: enemies/mote.png
    images:
      - {below: 0.5, file: enemies/mote_hurt.png}
      - {below: 0.2, file: enemies/mote_critical.png}
    motion: breathe
actors:
  scout:
    faces: scout
    face: normal
    images: [{below: 0.3, face: tired}]
```

Set `image_height` on an enemy to size its battle picture as a fraction of screen height, independent of the source picture pixels. For example, use `0.65` for a person, `0.28` for a small monster, and `0.55` for a large foe. The troop layout shrinks all pictures together if their combined widths exceed the available space. When omitted, the earlier pixel-based sizing and `scale` still apply.

Enemy `motion` is `none` by default. `breathe`, `sway`, and `bob` are subtle idle movements; all foes lunge when acting and shake when hit. An optional enemy `puppet` points to the same puppet YAML format used by actors. Both can define `soft_sway` regions in that file: a rectangle or ellipse of the base picture moves gently with breathing and trails action and hit reactions. The battle screen crossfades picture changes and flashes on form changes.

The Go Zen sample uses an author-generated King Slime picture for the merge result, the existing Tollkeeper file for both forms, a Drip Slime alternate picture below half HP, and Maren's worried face below 35% HP. The Succubus uses a puppet for blinking and motion. The existing Tollkeeper and Drip Slime alternate pictures can be replaced with distinct art when available.
