# YAML traps

- In a flow map `{...}`, quote a value that holds `, ` or `: `, or starts with `[`, `{`, `*`, `&`, `!`, `%` or `@`: `{say: "Wait, what?"}`. Without quotes the line is cut, and `gozen check` reports GZN1104 or a parse error.
- Text with a `#` after a space starts a comment. Quote it.
- Start new games at `schema_version: 11`. Older minimum versions describe feature availability, not genre presets: schema 3 adds maps (`map:`), painted scenes (`stage:`), dolls and balloons need it, and an older version asks for it. A novel (`ui.message: novel`) needs 4, a campaign (`regions`, `factions`, `campaign`) needs 5, monster collection 6, battle forms and enemy puppets 7, and stacked spaces and `ui.stage_framing` 8; `gozen check` says when the version is too low.
- An ID is the key of a map (`locations: {village: ...}`). Every reference names that key: `start.location`, `exits[].to`, `troops.*.members`, `actors.*.class`, `battle: TROOP`. A typo is GZN1101 with the file and line.
- Expressions are strings: `mhp: "20 + level * 6"`, `when: flag.asked`. Quote one that starts with `!` or holds `: `.
- `data/` may hold any number of files; they are merged. The same ID in two files is an error.

## Compact authoring (schema 11)

<!-- verify: parse fragment; ticket and station belong to the author -->
```yaml
events:
  greeting:
    - say: Welcome to the station.
    - give: ticket
  first_visit:
    trigger: enter
    location: station
    once: true
    do: [{call: greeting}]
```

`give: ticket` is the same as `give: {item: ticket}`. For quantities, use
`give: {item: ticket, count: 2}`; zero remains zero. Tactic rows omit
`enabled: true`; write `enabled: false` to disable a row.

Before v1.0.0 there is no migration command; formats may change between
versions. To move a game to schema 11, change `schema_version`, write
`enabled: false` on every tactic row that omitted `enabled`, then check and play
it. Earlier schemas still load with their old defaults. Old engines cannot read
schema 11.

## Top-level sections

`game.KEY` belongs in `game.yaml`; `data.KEY` belongs in any file under `data/`.
Omit sections a game does not need. The [schema](https://nao1215.github.io/gozen/schema/)
provides the field contracts; these tables explain which section to open first.

| Section | Purpose |
| --- | --- |
| `game.phone` | Pin a catalog and declare bundled or optional games offered by phone apps. |
| `game.asides` | Events inserted after the lines of certain speakers, such as a moment of thought or a picture in the character's mind. |
| `game.battle` | The rules every battle shares. |
| `game.campaign` | Configure factions, turns, orders, generals and battles for a campaign. |
| `game.card_bubbles` | Small particles (such as poison bubbles) that rise from a party member's status card. |
| `game.carry` | Name state values retained for a new game after the `carry` command. |
| `game.cg_review` | Settings for the picture review debug mode: the tags to attach, extra pictures and picture groups. |
| `game.data_version` | The version of the game's content. 0 or omitted means unmanaged. It is separate from the save format version. |
| `game.equip_slots` | The equipment slots a character has, such as weapon or armor. |
| `game.flows` | Schema 10: named orders of full-screen surfaces gone through step by step; a step not reached opens the current step. |
| `game.fonts` | The font files the game uses. |
| `game.gallery` | What the title screen's gallery shows: pictures seen and scenes to replay. |
| `game.game_over` | What happens when the party falls. |
| `game.id` | The game's ID, which names its save directory. |
| `game.join_exp` | Set the minimum experience of a joining party member with an expression. |
| `game.language` | The language shown: the engine's built-in words, `en` (the default) or `ja`, and the game's own text, translated by `lang/<language>.yaml` when it is not the language written. |
| `game.languages` | List the languages offered in settings and selected from device preferences on first use. |
| `game.max_party` | The largest number of characters in the active party. |
| `game.memo` | Settings for the test player's notes (kinds of notes and numbers kept with each). |
| `game.menus` | Choose normal menus or `none` for immediate start/resume with autosave only. |
| `game.meters` | Gauges of the player's numbers shown on the field, in the dungeon and in the status menu. |
| `game.planning` | Optional typed entities, calendar, activities and period ledgers. |
| `game.reactions` | Map speakers to reaction books under `assets/`, with conditions and saved memory. |
| `game.scene_review` | Settings for the scene review debug page, such as the tags attached to scenes. |
| `game.schema_version` | The version of the YAML specification the game is written for. 0 or omitted means the legacy rules. |
| `game.screen` | The logical screen size. |
| `game.screens` | Named information screens, each with a title and ordered entries. Open with `open: NAME`. |
| `game.stand_ins` | A file under assets/ listing placeholder pictures, one path per line. A placeholder picture does not hide character portraits. |
| `game.start` | The state a new game begins in. |
| `game.state` | Declarations of the game's variables and whether unknown names are rejected. |
| `game.streams` | Point to the separate stream and episode definitions file. |
| `game.subtitle` | The subtitle shown on the title screen. |
| `game.terms` | The words the menus use, such as the currency name and level label. |
| `game.text` | Replaces the engine's built-in words on the screens, as a map from the engine's key to your text. |
| `game.title` | The game's title. |
| `game.title_art` | A picture drawn on the title screen, such as a logo or key art. |
| `game.title_background` | The background picture of the title screen. |
| `game.title_figures` | Layer title-screen figures in place of `title_art`. |
| `game.title_music` | The song and mood played on the title screen. |
| `game.title_text` | Set title and subtitle sizes, spacing and colors. |
| `game.ui` | The look of the game's menus and windows. |
| `game.verbs` | Maps reusable command names to event IDs. Invoke with `NAME: true`, or call the event directly with `call`. |
| `game.web` | Set browser-export address, description and social preview image. |
| `game.written_in` | Identify the source language translated by `lang/<code>.yaml`. |
| `data.actors` | Characters, keyed by actor ID. |
| `data.ai` | Named sets of battle AI rules, keyed by AI ID. |
| `data.breeding` | Monster breeding requirements, species pairs and family rules. Needs schema version 6. |
| `data.classes` | Character classes, keyed by class ID. |
| `data.elements` | Damage elements, mapping each element ID to its display name. |
| `data.enemies` | Enemy definitions, keyed by enemy ID. |
| `data.events` | Reusable sequences of commands, keyed by event ID. |
| `data.factions` | The sides of a campaign, keyed by faction ID. Needs schema version 5. |
| `data.fusions` | Recipes that combine items into another item. |
| `data.items` | Items, equipment and key items, keyed by item ID. |
| `data.locations` | Places the party can be, keyed by location ID. |
| `data.monster_families` | Monster family IDs and their display names. Needs schema version 6. |
| `data.music` | The songs and the rules that choose the mood. |
| `data.profiles` | Pages of the character book (the Characters screen), keyed by profile ID; a profile whose ID is an actor's shows that actor. |
| `data.regions` | The regions of a campaign, keyed by region ID. Needs schema version 5. |
| `data.shops` | Shops, keyed by shop ID. |
| `data.skills` | Skills, keyed by skill ID. |
| `data.states` | Status conditions such as poison or buffs, keyed by state ID. |
| `data.stone_sets` | Named combinations of stones that give a bonus, keyed by set ID. |
| `data.tactic_actions` | Actions the player can choose in tactics, keyed by ID. |
| `data.tactic_conditions` | Conditions the player can choose in tactics, keyed by ID. |
| `data.tactic_targets` | Targets the player can choose in tactics, keyed by ID. |
| `data.troops` | Enemy groups that appear in a battle, keyed by troop ID. |
