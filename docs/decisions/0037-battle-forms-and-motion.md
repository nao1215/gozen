# ADR 0037: Battle forms and HP pictures

Status: Accepted (2026-10-22).

## Context

The battle engine already supports summoned foes and face rules, but a change of form needs to keep a battle slot and queued turn intact. A merge must have a deterministic result in the screen, headless simulation, and replay. Actors already use named face pictures and optional puppet portraits.

## Decision

Schema version 7 introduces these words. An enemy's `hp_triggers` list can also change form immediately after damage crosses a descending ratio threshold, before a fatal fall. Each row uses `below`, `transform`, optional `hp` (the same modes as `transform_hp`) and optional `clear`. Only the first crossed row fires for one hit.

Skills use one `transform` effect or one `merge` effect. `transform` changes a living enemy target in place; a self-scoped skill changes its user. `transform_hp` is `ratio` by default, `full`, or an expression evaluated against the old form. The result is clamped to one through the new maximum. States and guard remain unless `transform_clear: true`. The changed unit keeps its slot and any queued action, while its AI rules are taken from the new form. An AI rule can trigger the skill with `self.hp_rate`.

`merge` selects the user and the first eligible living allies in troop order. `from` lists allowed enemy IDs, `count` is at least two, and `into` names the resulting enemy. The result occupies the middle selected slot. `hp` is `sum` by default (clamped to the new maximum), `ratio` (the average member ratio), or `full`. Insufficient members produce a battle message; AI does not select the skill. `allies.kind_<enemy id>` counts living allies of one kind. A merge is possible only if the user is in `from`.

Both changes mark the resulting form as seen in the bestiary. Rewards are paid once for the final form, not for each earlier form or merge member. A transformed enemy killed later counts as a kill of its final ID. A merged member does not count as a defeat. Form changes are deterministic and use the battle engine's steps, so simulation and screen execution share the rules.

An enemy's `images` list contains descending HP ratio thresholds and files; an actor's list contains thresholds and face names. The last matching threshold wins. The battle screen crossfades changes. Enemy `motion` chooses `none` (the default), `breathe`, `sway`, or `bob`; action and hit motions remain separate. An enemy can name a puppet definition under assets with `puppet`. The screen uses the puppet when it loads and falls back to the picture if it cannot. The default `none` keeps existing games and the sample's other enemies still.

## Consequences

These YAML fields add no new public Go entrance. The definition checker reports unknown form IDs, invalid merge size or HP mode, unavailable merge troops, missing HP picture files, and unsorted thresholds. Animations are presentation state; battle outcomes do not depend on frame timing. Save and restore through `Session` persist the state between battles; the current Session contract does not serialize an in-progress battle.
