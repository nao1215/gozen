# Novel games

A novel game (ADV) sets `schema_version: 4` and `ui.message: novel` in game.yaml; its places mostly run events. Start from `gozen new GAME --template novel`: speakers are classless and the party may be empty, so no combat rules are needed. The player can roll back, skip read lines, save in a scene and change the language without any YAML of yours.

- Give a `choice` an `id` to read the option taken as `chose.<id>`. Keep other facts in flags with `set`. `gozen check` warns of a flag set but never read (GZN0112) and read but never set (GZN0113).
- Stage a scene with `stand` (who stands where, how they come and go), `effect` (shake one portrait, fade through a color), `music` with `fade` and `wait` with `skip: false`. A line's `face` still changes the expression of a portrait standing.
- Define reusable steps instead of copying them. `verbs: {greet: greeting}` maps a verb name to an event, and `greet: true` runs it. `screens` are information pages with a title and `entries` (conditional text and pictures; an entry can run ordinary event commands). The older declaration-only forms `verbs: [name]` and `screens: [name]` are no longer accepted.
- Keep who each character is in `profiles` (data/): a title, fields with your own labels, an introduction and how they see the others. `--dev` opens it as Characters (F6), and `profile.<id>.unlocked` reads whether its `when` holds. Press Tab there for the relationship chart; set optional `affiliation` on profiles and `kind` (`like`, `love`, `rival`, `enemy`, `family`, `friend`, `other`) on relations. Run `gozen faces` to center the face icons.
- Reaction books (a speaker who remembers a greeting) keep their memory in saves; see the cookbook recipe "A speaker who remembers a greeting".
- `enemies.<id>.lore` plus `ui.bestiary: true` offers the monster book. `bestiary.<id>.seen` and `.defeated` are expressions; F4 opens every page under `--dev`.

Recipes: "Let the player roll back", "Stage portraits and effects" and "Offer the game in two languages" in the cookbook. A complete short sample is [examples/novel](https://github.com/nao1215/gozen/tree/main/examples/novel).

## Staging with stand

<!-- verify: parse a fragment of an event; the portraits are the author's pictures -->
```yaml
- stand: {who: Keeper, face: keeper.png, at: left}
- stand: {who: Guard, face: guard.png, at: right, enter: slide}
- effect: {name: shake, who: Guard, seconds: 0.4}
- stand: {who: Guard, leave: fade}
- stand: none
```

`at` is `left`, `center`, `right` or a fraction of the screen width. A name that is not an actor needs `face`, a picture under `assets/`. A `stand` with a map needs `schema_version: 4`.

## Frame tall portraits by their heads (schema version 8)

Static portraits with transparent padding are placed by the whole canvas unless you give `ui.stage_framing`. It is opt-in per image path; use one entry for each expression image whose canvas or head differs.

<!-- verify: parse requires the author's portrait image and its measured head coordinates -->
```yaml
ui:
  message: novel
  stage_framing:
    portraits/guide.png:
      head: [0.4, 0.1, 0.2]   # head center x, top y, head height, as fractions of the source image
      height: 0.18            # displayed head height, a fraction of the screen height (required)
      top: 0.08               # where the head top lands, a fraction of the screen height
```

The head's horizontal center follows `stand.at` or automatic placement. `stage_scale` multiplies size around the head; `stage_drop` shifts the head top. It does not apply to moving puppets, battle cut-ins or the `box` window.

## Picture limits

Pictures may be up to 8192 pixels per side, 33,554,432 pixels in all and 64 MiB encoded, so a 4096 by 6144 portrait works as it is. `gozen check` tests every actor face PNG against these limits, even one no profile uses. A decoded image takes up to 128 MiB of memory; export pictures near the size they are shown and test the target devices.

## Wording and languages

- `text:` in game.yaml replaces the engine's own wording (menus, the monster book, battle messages), keyed by the engine's Japanese source text; `language: en|ja` picks the default set and `terms` names money and stats. The English catalog lists every key (cookbook: "Change the language and the engine's wording").
- For a second language for your own lines, set `written_in` and `languages` and translate every text in `lang/<code>.yaml`. `gozen check --format json` lists each text not translated yet (GZN0111); fill them until the check is quiet. Map-point `advice` texts are part of the export.
- `ui.battle_ai_history: false` hides the battle AI history panel and its buttons (default true). It changes no decisions or traces.

## Exploring branches

`gozen sim routes GAME --format json --fail-unreachable` counts paths to endings and lists choice options never offered; tune it with `--paths`, `--depth` and `--timeout`. Then play the important branches with `gozen run --script`.
