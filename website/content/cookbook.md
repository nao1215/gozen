---
title: Cookbook
description: Short YAML recipes for common tasks - shops, inns, companions, talk and choices, battles, bosses, dungeons and their themes, maps, campaigns, painted scenes, portraits, pictures, music, language, test play, measuring and releases.
weight: 35
toc: true
---

Copy a recipe's YAML into a file under `data/` (or into `game.yaml` where it says so) and change the names. The tests load every block over a small game, so each works as written. Recipes link to the [Schema](/schema/) and, where it has the feature, the [Go Zen sample](https://github.com/nao1215/gozen/tree/main/examples/go-zen/game/data); the commands they use are at the end, in [Commands used by the recipes](#commands-used-by-the-recipes).

The recipes use a few IDs they do not define: the actor and class `walker`, the class `mender`, the skills `attack` and `guard`, the item `key`, the enemy `mote`, the troop `wolves` and the locations `field`, `ridge`, `shop` and `gate`. Use your own instead. Rosters, work, scene dependencies, materials and character voice checks are in [Planning screens and narrative checks](/planning/).

## By task

| You want to | Uses |
|:--|:--|
| [Start a new game](#start-a-new-game) | `start` in game.yaml, an opening event |
| [Open a shop](#open-a-shop) | `shops`, items with a `price`, the `shop` command |
| [Run an inn and a place to save](#run-an-inn-and-a-place-to-save) | actions, `if` on `gold`, `heal: all`, `save: true` |
| [Write talk with choices and branches](#write-talk-with-choices-and-branches) | `people`, `say`, `choice`, `if` |
| [Remember things with flags and conditions](#remember-things-with-flags-and-conditions) | `set`, `flag.*` and `var.*`, `when`, `trigger: auto` |
| [Let the player roll back and save in a scene](#let-the-player-roll-back-and-save-in-a-scene) | `ui.rollback`, `ui.saves`, a choice's `id`, `chose.*` |
| [Stage portraits and effects](#stage-portraits-and-effects) | `stand`, `effect`, `music` fades, `wait` and `sfx` that hold |
| [Set the weather and the tone](#set-the-weather-and-the-tone) | `weather` (rain, snow, petals, embers), `tone` (gray, sepia, night, dusk, or its parts) |
| [Add voices](#add-voices) | `say.voice`, actor `voices`, `ui.voice`, the backlog's ▶ |
| [Add a companion who joins](#add-a-companion-who-joins) | `actors`, `join`, `leave` |
| [Grow by level and learn skills](#grow-by-level-and-learn-skills) | class `stats` formulas, `exp_for`, `skills` by level |
| [Make items and equipment](#make-items-and-equipment) | item `use`, `kind`, `slot`, `stats`, class `equip` |
| [Add a status effect](#add-a-status-effect) | `states`, `add_state`, `remove_state` |
| [Set up troops and enemy AI](#set-up-troops-and-enemy-ai) | `enemies`, `troops`, `encounters`, AI rules |
| [Give the party tactics](#give-the-party-tactics) | actor `ai`, shared `ai` sets, tactic catalogs and presets |
| [Stage a boss fight](#stage-a-boss-fight) | `boss: true`, troop `events`, `battle` with `on_win` |
| [Send the party home instead of game over](#send-the-party-home-instead-of-game-over) | `game_over.event`, `battle.retry` |
| [Draw a dungeon by hand](#draw-a-dungeon-by-hand) | `dungeon.map`, `tiles` |
| [Put a chest in a dungeon](#put-a-chest-in-a-dungeon) | `once` and `after` tiles |
| [Lock a door with a key](#lock-a-door-with-a-key) | a `solid` tile with `when`, `take` |
| [Make a one-way passage](#make-a-one-way-passage) | `slide` tiles |
| [Generate dungeon floors](#generate-dungeon-floors) | `generate`, `place`, `dive` |
| [Let strong foes roam a floor](#let-strong-foes-roam-a-floor) | `roamers` |
| [Change a dungeon's look](#change-a-dungeons-look) | `looks` by floor, textures, `darkness` |
| [Show a map with points and roads](#show-a-map-with-points-and-roads) | `map`, `points`, `roads` |
| [Make a campaign](#make-a-campaign) | `regions`, `factions`, `campaign`, `orders`, `clash`, `gozen sim campaign` |
| [Mix a campaign with a dungeon](#mix-a-campaign-with-a-dungeon) | an order with `move`, a dungeon's way out, `a.atk` in `clash` |
| [Paint a scene with people and balloons](#paint-a-scene-with-people-and-balloons) | `stage`, `spots`, dolls, `bubbles` |
| [Give a character faces and a moving portrait](#give-a-character-faces-and-a-moving-portrait) | `faces`, `say.face`, `puppet` |
| [Set the face icons](#set-the-face-icons) | `face_box`, `face_shade`, a doll's `icon` |
| [Show event pictures and fill the gallery](#show-event-pictures-and-fill-the-gallery) | `image`, `gallery.cgs`, `gallery.scenes`, `gallery.endings` |
| [Keep a character book](#keep-a-character-book) | `profiles`, `ui.profiles`, `open: profiles` |
| [Keep a monster book](#keep-a-monster-book) | `enemies.<id>.lore`, `ui.bestiary`, `bestiary.<id>.seen` |
| [Collect, raise and breed monsters](#collect-raise-and-breed-monsters) | `recruit`, `monster_families`, `breeding`, `open: ranch` |
| [Play music for places, battles and moods](#play-music-for-places-battles-and-moods) | `music.songs`, `music`, `battle_music`, `mood` rules |
| [Change the language and the engine's wording](#change-the-language-and-the-engines-wording) | `language`, `text`, `terms` |
| [Offer the game in two languages](#offer-the-game-in-two-languages) | `written_in`, `languages`, `lang/<code>.yaml`, ruby `|base《reading》` |
| [Test-play with notes, reload and quick starts](#test-play-with-notes-reload-and-quick-starts) | `memo`, `--dev`, F5, F9, `--start` |
| [Measure battles with sim battle and sim dive](#measure-battles-with-sim-battle-and-sim-dive) | `gozen sim battle`, `gozen sim dive` |
| [Check and measure in CI](#check-and-measure-in-ci) | `setup-gozen`, `gozen check`, `gozen sim` in GitHub Actions |
| [Cut a release and a trial](#cut-a-release-and-a-trial) | `trial.yaml`, `gozen release`, `gozen trial` |
| [Wear stone sets and fuse stones](#wear-stone-sets-and-fuse-stones) | `stone_sets`, `fusions`, `equip_stones`, `stone_suggest`, `open: fusion` |
| [Set a retry point with a checkpoint](#set-a-retry-point-with-a-checkpoint) | `checkpoint`, `restore_checkpoint` |
| [Insert a thought after a speaker's lines](#insert-a-thought-after-a-speakers-lines) | `asides` |
| [Dress the title screen](#dress-the-title-screen) | `ui.title`: `background`, `picture`, `pictures` |
| [Review pictures and scenes while writing](#review-pictures-and-scenes-while-writing) | `cg_review`, `scene_review`, `--review-log` |
| [Tune a place, the windows and the battle pace](#tune-a-place-the-windows-and-the-battle-pace) | `no_menu`, `outfit`, `resume_to`, `talk_on_enter`, `ui.hide_gold`, `battle.tempo`, `battle.passives` |
| [Change states, duels and skill points from an event](#change-states-duels-and-skill-points-from-an-event) | `affect`, `fighters`, `reset_tree`, `sp_suggest`, `open_wall` |

## Start a new game

`start` in `game.yaml` says where a new game begins, with whom, and what they carry; `start.event` runs once before the player gets control.

<!-- verify: load group=start as=game.yaml -->
```yaml
# game.yaml
title: Lantern Trail
start:
  location: village
  party: [walker]
  gold: 50
  items: {key: 1}
  event: opening
```

<!-- verify: load group=start -->
```yaml
locations:
  village: {name: Village, background: village.jpg, exits: [{to: field, label: Out to the field}]}
events:
  opening:
    do:
      - say: The lantern by the gate has gone out again.
```

Reference: [Start](/schema/#start), [Game](/schema/#game).

## Open a shop

A shop is a list of goods with prices. Open it from an action, an exit or anyone's talk with the `shop` command; selling to it pays half the price.

<!-- verify: load group=shop -->
```yaml
items:
  tonic: {name: Tonic, price: 8, desc: Restores 30 HP., use: {scope: one_ally, effects: [{heal: 30}]}}
  lamp_oil: {name: Lamp oil, price: 3}
shops:
  general:
    name: General store
    greeting: Take a look around.
    keeper: keeper.png
    goods: [tonic, lamp_oil]
locations:
  market:
    name: Market
    actions:
      - {label: Shop, do: [{shop: general}]}
    exits: [{to: field, label: Out to the field}]
```

`sell: false` makes a shop that only sells, and `spent: var.NAME` adds every purchase to a variable. Reference: [Shop](/schema/#shop), [Item](/schema/#item).

## Run an inn and a place to save

An inn is an action that takes gold and heals everyone; a save point is an action with `save: true`. `safe: true` also lets the player save from the menu here.

<!-- verify: load group=inn -->
```yaml
locations:
  inn:
    name: Inn
    safe: true
    actions:
      - label: Stay the night (10G)
        do:
          - if: gold >= 10
            then: [{gold: -10}, {heal: all}, {say: You wake up rested.}]
            else: [{say: You cannot pay for a room.}]
      - {label: Save, do: [{save: true}]}
    exits: [{to: field, label: Leave}]
```

Reference: [Location](/schema/#location), [Action](/schema/#action). Go Zen's inn, The Dry Cup, has a rest and a save point.

## Write talk with choices and branches

A person's `talk` is a list of commands. `say` shows a line, `choice` asks a question with its own commands for each answer, and `if` branches on a condition.

<!-- verify: load group=talk -->
```yaml
items:
  letter: {name: Letter, kind: key}
locations:
  well:
    name: Old well
    people:
      - name: Keeper
        image: keeper.png
        talk:
          - say: {who: Keeper, text: Will you carry a letter to the ridge?}
          - choice:
              - label: I will
                do:
                  - give: {item: letter}
                  - say: {who: Keeper, text: Thank you. Mind the wolves.}
              - label: Not now
                do: [{say: {who: Keeper, text: Come back if you change your mind.}}]
      - name: Guard
        talk:
          - if: item.letter > 0
            then: [{say: {who: Guard, text: A letter for the ridge? Go on.}}]
            else: [{say: {who: Guard, text: Nobody passes without business.}}]
```

A `choice` answer can have its own `when`, and `prompt` puts a line above the answers. Reference: [Person](/schema/#person), [Say](/schema/#say), [ChoiceOption](/schema/#choiceoption), [Command](/schema/#command).

## Remember things with flags and conditions

`set` writes flags (`flag.NAME`, true or false) and variables (`var.NAME`, numbers); anything with `when` appears only while its condition holds. An event with `trigger: auto` runs as soon as its `when` holds.

<!-- verify: load group=flags -->
```yaml
locations:
  crossing:
    name: Crossing
    people:
      - name: Ferryman
        when: "!flag.ferry_gone"
        talk:
          - set: {var.asked: "var.asked + 1"}
          - say: {who: Ferryman, text: Ask me again tomorrow.}
    exits:
      - {to: ridge, label: Wade across, when: flag.ferry_gone}
events:
  ferry_leaves:
    trigger: auto
    when: "var.asked >= 3 && !flag.ferry_gone"
    once: true
    do:
      - say: The ferry has left for the season. The river is low enough to wade.
      - set: {flag.ferry_gone: true}
```

To catch typos in names, declare the variables in `game.yaml` and turn on `strict`:

<!-- verify: load group=flags as=game.yaml -->
```yaml
# game.yaml
state:
  strict: true
  names:
    var.asked: {type: number, initial: 0}
    flag.ferry_gone: {type: bool, initial: false}
```

Reference: [Command](/schema/#command) (`set`, `if`), [Event](/schema/#event), [StateNames](/schema/#statenames), and the expressions on [Writing a game](/writing-a-game/#expressions). Go Zen opens its dungeon gate with flags.

## Let the player roll back and save in a scene

In the novel window, "Back", Q or the mouse wheel steps back a line, and flags, variables, items and answers go back with it, so a choice can be answered again. Read lines, the gallery and the settings stay. Rolling back stops at a battle, a shop, a save command, a screen, a move or the end, and reaches up to 8 earlier events; `ui.rollback` changes that number, and 0 turns it off. "Save" and "Load" on the band work mid-scene: loading replays the event from its start to the line, with the same answers and random numbers.

![Back goes over two lines and a choice, with the state](/images/novel-back.mp4)
![the option taken again, then Skip stops at the first line not read](/images/novel-skip.mp4)

<!-- verify: load group=rollback as=game.yaml -->
```yaml
# game.yaml
schema_version: 11
ui:
  message: novel
  rollback: 16
```

A choice with an `id` records the option taken in `chose.<id>` (the option's `id`, or its number from 0), which rolls back and is saved with the state.

<!-- verify: load group=rollback -->
```yaml
events:
  dinner:
    do:
      - say: Soup or bread?
      - choice:
          - {id: soup, label: Soup}
          - {id: bread, label: Bread}
        id: dinner
      - if: chose.dinner == "soup"
        then: [{say: Warm.}]
        else: [{say: Crisp.}]
```

Use a separate `--save-dir` when testing save and load through `gozen run`. Reference: [UIStyle](/schema/#uistyle), [Command](/schema/#command) (`id`).

Each save row on the save and load screens shows a small picture of the scene. `ui.saves.slots` sets the number of slots (3 by default, up to 99; extra rows scroll), and `ui.saves.quick` adds "Q.Save" and "Q.Load" to the band: Q.Save writes the quick slot at once, and Q.Load asks once and goes back to it. The load screen lists the quick slot after the autosave, and a script presses them with `press quick_save` and `press quick_load`.

![Save writes slot 1 with a picture of the scene, Q.Save keeps a later line, Q.Load asks once and goes back to it, and the load screen shows each save with its picture](/images/novel-saves.mp4)

<!-- verify: load group=saves as=game.yaml -->
```yaml
# game.yaml
schema_version: 11
ui:
  message: novel
  saves: {slots: 9, quick: true}
```

Reference: [SaveStyle](/schema/#savestyle).

## Stage portraits and effects

In the novel window, portraits come and go with the speakers. `stand` places them as written instead: who stands where, how they enter (`fade` or `slide`) and when they leave, and a line's `face` still changes a standing portrait's expression. `effect` shakes a portrait or the screen, plays a moving portrait's reaction, or fades the screen through a color. `music` fades in and out, a `wait` with `skip: false` holds even in skip mode, and a `sfx` with `wait: true` holds until the sound ends.

![Brenna slides in and her portrait shakes](/images/novel-stage.mp4)

<!-- verify: load group=staging as=game.yaml -->
```yaml
# game.yaml
schema_version: 11
ui:
  message: novel
```

<!-- verify: load group=staging -->
```yaml
music:
  songs:
    rain: {file: rain.ogg}
events:
  meeting:
    do:
      - stand: {who: Keeper, face: keeper.png, at: left}
      - say: {who: Keeper, face: keeper.png, text: You came back.}
      - stand: {who: Guard, face: guard.png, at: right, enter: slide}
      - effect: {name: shake, who: Guard, seconds: 0.4}
      - say: {who: Guard, face: guard.png, text: Stand where you are.}
      - music: {song: rain, fade: 2}
      - wait: {seconds: 0.8, skip: false}
      - sfx: {name: door, wait: true}
      - stand: {who: Guard, leave: fade}
      - effect: {name: fade, color: "#000000", seconds: 1, hold: 0.5}
      - stand: none
      - music: {stop: true, fade: 3}
```

`at` is `left`, `center`, `right` or a fraction of the screen width. A name that is not an actor needs `face`, a picture under `assets/`; for an actor, `face` is one of its faces. The headless session reports `stand` as an effect (`Effect.Kind` `stand`, with the actor), so a test can check who stands in a scene. The map forms need `schema_version: 4`; the old string forms still work. Reference: [Stand](/schema/#stand), [EffectCue](/schema/#effectcue), [WaitCue](/schema/#waitcue), [SFXCue](/schema/#sfxcue), [MusicCue](/schema/#musiccue).

## Set the weather and the tone

`weather` makes rain, snow, petals or embers fall over the scene, and `tone` colors it with a preset (`gray`, `sepia`, `night`, `dusk`) or its parts: `color` (multiplied), `gray`, `bright` and `vignette` (dark edges). Both last over every place and line until the next `weather` or `tone`, saves keep them, and `none` removes each. A change eases in over `seconds` (1 by default; 0 is at once).

![the painted plaza in rain at night, then snow, petals in sepia and embers at dusk](/images/go-zen-weather.mp4)

| Weather | Tone of a memory |
|:--|:--|
| ![snow falls over the hill above the village, leaning with the wind](/images/novel-weather.mp4) | ![Brenna starts her memory and the tavern eases to a cold gray with dark edges, then the colors come back](/images/novel-tone.mp4) |

<!-- verify: load group=look as=game.yaml -->
```yaml
# game.yaml
schema_version: 12
ui:
  message: novel
```

<!-- verify: load group=look -->
```yaml
events:
  arrival:
    do:
      - weather: {kind: snow, power: 0.55, wind: 0.35, seconds: 0}
      - say: Snow squeaks under every step.
      - weather: {kind: none, seconds: 0.5}
      - say: Warmth rolls over you.
  memory:
    do:
      - tone: {gray: 0.7, color: "#d8e4ff", bright: -0.1, vignette: 0.5, seconds: 1.5}
      - say: Fourth day. The lead wagon started to slide.
      - tone: none
  storm:
    do:
      - weather: rain
      - tone: night
```

Both are drawn over the place, its pictures and standing portraits, and under the windows; battles have neither. The drops fall the same way in every capture of the same play. The headless session reports each command as an effect (`Effect.Kind` `visual`, named `weather` or `tone`), and the state's `look` holds what is falling and the tone. Both need `schema_version: 12`. Reference: [WeatherCue](/schema/#weathercue), [ToneCue](/schema/#tonecue).

## Add voices

A line's `voice` names its recording: for an actor with `voices` (a folder under `assets/`), a name in that folder, extension (`.ogg` or `.wav`) optional; for anyone else, a path under `assets/`. Voices and sound effects last at most 60 seconds, and a longer file is skipped with a line in the log.

<!-- verify: load group=voices as=game.yaml -->
```yaml
# game.yaml
schema_version: 11
ui:
  message: novel
  voice:
    cut: true        # the next line stops a voice still playing (the default)
    auto_wait: true  # auto waits for the voice to end (the default)
```

<!-- verify: load group=voices -->
```yaml
actors:
  keeper: {name: Keeper, class: walker, voices: keeper/voice}
events:
  welcome:
    do:
      # plays assets/keeper/voice/welcome_001.ogg
      - say: {who: keeper, voice: welcome_001, text: Come in out of the cold.}
      - say: {who: Traveler, voice: voice/traveler_001.ogg, text: Thank you.}
```

A voice plays when its line opens and stops at the next line. In the backlog a voiced line has a ▶ that plays it again without closing the log. Skip mode, Ctrl, scene skip and the replay of rolling back stay silent over the lines they pass, and the line they land on speaks. The settings have a voice volume and one for each actor with `voices`. `gozen check` reports a missing voice file and counts a voiced actor's lines without one (GZN0109). The headless session reports each voice as an effect (`Effect.Kind` `voice`, with the file), and a trace has a `voice` line for each voice played. Reference: [Say](/schema/#say) (`voice`), [Actor](/schema/#actor) (`voices`), [VoiceStyle](/schema/#voicestyle).

## Add a companion who joins

A companion is an actor. `join` adds them to the party and `leave` takes them out; a flag keeps them from offering twice.

<!-- verify: load group=companion -->
```yaml
actors:
  sela:
    name: Sela
    class: mender
    faces: sela              # assets/sela/normal.png
    back: true               # starts in the back row
locations:
  hut:
    name: Hut
    people:
      - name: Sela
        image: sela/normal.png
        when: "!flag.sela_joined"
        talk:
          - say: {who: sela, text: You are going to the ridge? I will walk with you.}
          - join: sela
          - set: {flag.sela_joined: true}
```

`join_exp` in `game.yaml` raises a newcomer's experience to the party's. Reference: [Actor](/schema/#actor), [Command](/schema/#command). Go Zen's Ox, Tilly and Kes join this way.

## Grow by level and learn skills

A class gives each stat as a formula of `level`, the experience each level needs, and the skills learned on the way.

<!-- verify: load group=growth -->
```yaml
skills:
  spark: {name: Spark, mp: 3, effects: [{damage: a.mat * 2 - b.mdf}]}
  mend: {name: Mend, mp: 4, scope: one_ally, anim: heal, effects: [{heal: 10 + a.mat}]}
classes:
  scholar:
    name: Scholar
    stats: {mhp: "18 + level * 5", mmp: "6 + level * 2", atk: 3, def: 2, mat: "4 + level * 2", mdf: 4, agi: 5, luk: 2}
    exp_for: level * 25
    skills:
      - {level: 1, skill: spark}
      - {level: 4, skill: mend}
actors:
  ione: {name: Ione, class: scholar}
```

Experience comes from enemies' `exp` and the `exp` command; `learn` teaches a skill at any time. A class `tree` adds a skill tree paid with SP.

`reset_actor` starts one actor over, for a new game plus where only some companions begin again. It resets level, experience, learnt skills, skill tree and SP, restores HP and MP and clears states, and keeps equipment, name, row and whether the actor has joined. `level` picks the starting level (1 to the class's max); otherwise the level in the actors file is used. An unknown actor ID is a `gozen check` error, and an actor who has not joined is already at the start.

<!-- verify: load group=growth -->
```yaml
events:
  new_game_plus:
    do:
      - reset_actor: ione
      - reset_actor: ione
        level: 5
```

Reference: [Class](/schema/#class), [Learning](/schema/#learning), [Skill](/schema/#skill), [Command](/schema/#command).

## Make items and equipment

An item with `use` can be used from the menu or in battle. Equipment has a `kind`, the `slot` it goes into and the `stats` it adds; a class lists the kinds it can wear.

<!-- verify: load group=items -->
```yaml
items:
  tonic:
    name: Tonic
    price: 8
    desc: Restores 30 HP.
    use: {scope: one_ally, effects: [{heal: 30}]}
  bronze_blade: {name: Bronze blade, kind: sword, slot: weapon, price: 60, stats: {atk: 4}}
  leather_vest: {name: Leather vest, kind: vest, slot: armor, price: 40, stats: {def: 3}}
classes:
  fencer:
    name: Fencer
    equip: [sword, vest]
    stats: {mhp: "20 + level * 6", mmp: 2, atk: "5 + level", def: 3, mat: 1, mdf: 2, agi: 6, luk: 2}
    exp_for: level * 20
actors:
  fen: {name: Fen, class: fencer, equip: {weapon: bronze_blade}}
```

The slots are `weapon`, `armor` and `accessory` unless `equip_slots` in `game.yaml` says otherwise. Reference: [Item](/schema/#item), [Usage](/schema/#usage), [Slot](/schema/#slot).

## Add a status effect

A state is put on with `add_state` and taken off with `remove_state` (or `bad` for every bad state). `on_turn_end` runs its effects at the end of each turn.

<!-- verify: load group=state -->
```yaml
states:
  poison:
    name: Poison
    short: PSN
    bad: true
    turns: 5
    on_turn_end: [{damage: 3}]
    messages: {add: "{target} is poisoned!", remove: "{target} feels better."}
skills:
  sting: {name: Sting, effects: [{damage: a.atk}, {add_state: poison, chance: 0.4}]}
items:
  antidote: {name: Antidote, price: 6, use: {scope: one_ally, effects: [{remove_state: poison}]}}
```

`restrict: skip` makes a state cost turns (sleep), `stats` changes the holder's stats (a buff), and `persist: true` keeps it after the battle. Reference: [State](/schema/#state), [Effect](/schema/#effect).

## Set up troops and enemy AI

An enemy needs `mhp` and its AI, rules tried by `priority`, each with an optional condition and target; other stats default to 0 and the picture to `assets/enemies/<id>.png`. A troop is a group met together, and `encounters` sets how often troops are met here.

<!-- verify: load group=troops -->
```yaml
skills:
  bite: {name: Bite, effects: [{damage: a.atk + 2}]}
enemies:
  bat:
    name: Bat
    image: bat.png
    stats: {mhp: 14, mmp: 0, atk: 4, def: 1, mat: 0, mdf: 1, agi: 7, luk: 2}
    exp: 4
    gold: 3
    drops: [{item: key, chance: 0.1}]
    ai:
      - {use: bite, when: "self.hp_rate < 0.5", priority: 1}
      - {use: attack, target: weakest}
troops:
  bats: {members: [bat, bat]}
  mixed: {members: [bat, mote]}
locations:
  cave_mouth:
    name: Cave mouth
    encounters:
      rate: 0.25
      troops: [{troop: bats, weight: 3}, {troop: mixed}]
```

Reference: [Enemy](/schema/#enemy), [Rule](/schema/#rule), [Target](/schema/#target), [Troop](/schema/#troop), [Encounter](/schema/#encounter). To see how hard a troop is, [measure it](#measure-battles-with-sim-battle-and-sim-dive).

## Give the party tactics

Party members on Auto follow their `ai`, written inline or as a shared set under `ai`. The player changes them on the tactics screen, which offers the conditions, targets and actions of the catalogs.

<!-- verify: load group=tactics -->
```yaml
skills:
  mend: {name: Mend, mp: 3, scope: one_ally, anim: heal, effects: [{heal: 12}]}
ai:
  healer:
    - {use: mend, when: "allies.hp_rate < 0.7", target: ally_lowest_hp_rate, priority: 1}
    - {use: attack}
actors:
  sela: {name: Sela, class: mender, ai: healer}
tactic_conditions:
  always: {name: Always, kind: always}
  hurt: {name: An ally is hurt, kind: hp_below, subject: allies}
tactic_targets:
  foe: {name: A foe, scope: one_enemy}
  weakest: {name: The most hurt ally, scope: one_ally, selector: {pick: min, by: t.hp_rate}}
tactic_actions:
  strike: {name: Attack, use: attack, target: foe}
  heal: {name: Mend, use: mend, target: weakest}
```

<!-- verify: load group=tactics as=game.yaml -->
```yaml
# game.yaml
battle:
  tactic_presets:
    careful:
      name: Careful
      description: Heal first, then attack.
      rows:
        healer:                # the AI the rows replace
          - {enabled: true, condition: hurt, percent: 60, action: heal}
          - {enabled: true, condition: always, action: strike}
```

The rows are keyed by the AI they replace. In an event, `set: {actor.sela.tactics: careful}` picks a preset and `auto` or `manual` switches the mode. Reference: [Actor](/schema/#actor), [Rule](/schema/#rule), [TacticPreset](/schema/#tacticpreset), [TacticCondition](/schema/#tacticcondition), [BattleCfg](/schema/#battlecfg). Go Zen's party has AI rules and three presets.

## Stage a boss fight

A boss is an enemy with `boss: true`. Its troop can forbid running away, say a line at the start, and run commands mid-battle; the `battle` command decides what winning and losing do.

![Ox's Shield Rush cut-in](/images/go-zen-cutin.mp4)

<!-- verify: load group=boss -->
```yaml
skills:
  quake: {name: Quake, scope: all_enemies, effects: [{damage: 8}]}
enemies:
  warden:
    name: Stone warden
    title: Keeper of the gate
    image: warden.png
    boss: true
    stats: {mhp: 120, mmp: 0, atk: 9, def: 4, mat: 0, mdf: 3, agi: 3, luk: 1}
    exp: 60
    gold: 40
    ai:
      - {use: quake, when: "turn % 3 == 0", priority: 1}
      - {use: attack}
troops:
  warden:
    members: [warden]
    can_escape: false
    intro: The statue in the gate opens its eyes.
    events:
      - {when: "turn == 4", do: [{say: Cracks run across the warden's shell.}]}
locations:
  gate_hall:
    name: Gate hall
    actions:
      - label: Face the warden
        when: "!flag.warden_beaten"
        do:
          - battle: warden
            on_win: [{set: {flag.warden_beaten: true}}, {say: The way north is open.}]
    exits:
      - {to: gate, label: Go north, when: flag.warden_beaten}
```

`level_scale` on the troop keeps an over-leveled party from crushing it, and `music` gives it its own song. Reference: [Enemy](/schema/#enemy), [Troop](/schema/#troop), [TroopEvent](/schema/#troopevent), [LevelScale](/schema/#levelscale).

## Send the party home instead of game over

`game_over.event` replaces the return to the title: after the game over screen, the next press recovers the party and runs the event. `battle.retry` offers to fight a lost battle again.

<!-- verify: load group=gameover as=game.yaml -->
```yaml
# game.yaml
game_over: {event: carried_home}
battle: {retry: true}
```

<!-- verify: load group=gameover -->
```yaml
locations:
  home: {name: Home, safe: true, exits: [{to: field, label: Out to the field}]}
events:
  carried_home:
    do:
      - say: A passing carter brings you home. Half your gold is gone.
      - gold: -floor(gold / 2)
      - heal: all
      - move: home
```

Reference: [GameOver](/schema/#gameover), [BattleCfg](/schema/#battlecfg).

## Draw a dungeon by hand

A location with `dungeon` is walked in first person. Each character of `map` is a cell: `#` a wall, `.` a floor, `D` a door, and any character you define in `tiles`.

<!-- verify: load group=dungeon -->
```yaml
locations:
  crypt:
    name: Crypt
    battle_background: crypt.jpg
    encounters: {rate: 0.08, troops: [{troop: wolves}]}
    dungeon:
      wall: wall.jpg
      door: door.jpg
      floor: "#8a8f99"
      ceiling: "#2a2d33"
      map: |
        #########
        U.S.#...#
        ###.D.#.#
        #.....#.#
        #########
      tiles:
        S: {start: true, facing: east}
        U: {name: Stairs up, look: door, symbol: U, do: [{move: field}]}
```

Lead into it with an exit like any place (`exits: [{to: crypt}]`). Reference: [Dungeon](/schema/#dungeon), [Tile](/schema/#tile). Go Zen's first two floors are hand-drawn mazes.

## Put a chest in a dungeon

A tile with `once: true` runs its commands a single time and then becomes its `after` character, here an open chest.

<!-- verify: load group=chest -->
```yaml
locations:
  storeroom:
    name: Storeroom
    dungeon:
      map: |
        #######
        #S.U.T#
        #######
      tiles:
        S: {start: true, facing: east}
        T: &chest
          name: Old chest
          picture: chest.png
          solid: "true"
          once: true
          after: t
          mark: "#edbe70"
          do: [{gold: 20}, {say: Inside the chest are 20 gold coins.}]
        U:
          <<: *chest
          do: [{say: The chest is empty but for dust.}]
        t: {name: Open chest, picture: chest_open.png, solid: "true"}
```

The second chest copies the first with a YAML anchor: `&chest` names the first tile, `<<: *chest` takes its keys, and keys written after it (`do` here) replace the copied ones. Write `<<` first, since a key before it is overwritten by the copy. Anchors work anywhere within one file.

Reference: [Tile](/schema/#tile). Go Zen's floors have the same chests.

## Lock a door with a key

A tile is there only while its `when` holds; otherwise it becomes its `else` character, a floor by default. Walking into a solid tile runs its commands.

<!-- verify: load group=door -->
```yaml
locations:
  vault:
    name: Vault
    dungeon:
      map: |
        #########
        #S..L..E#
        #########
      tiles:
        S: {start: true, facing: east}
        L:
          name: Locked door
          look: door
          solid: "true"
          when: "!flag.vault_open"
          do:
            - if: item.key > 0
              then: [{take: {item: key}}, {set: {flag.vault_open: true}}, {say: The key turns in the lock.}]
              else: [{say: The door is locked.}]
        E: {name: Way out, look: door, do: [{move: gate}]}
```

The key itself can come from a [chest](#put-a-chest-in-a-dungeon), a shop or a person. Reference: [Tile](/schema/#tile). Go Zen's Cistern Hall has three brass gates and three brass keys.

## Make a one-way passage

A `slide` tile carries the party on in one direction, so the passage can be crossed only that way.

<!-- verify: load group=oneway -->
```yaml
locations:
  river_cave:
    name: River cave
    dungeon:
      map: |
        #########
        #S.>>>..#
        #########
      tiles:
        S: {start: true, facing: east}
        ">": {name: Fast water, slide: east, hint: (one way), mark: "#6fa8dc", symbol: ">"}
```

`slide: facing` makes ice, and `spin` turns the party around when it stops. Reference: [Tile](/schema/#tile).

## Generate dungeon floors

With `generate` instead of `map`, each floor is built anew as a maze of rooms and corridors holding the characters of `place`. `dive` goes down, and `var.floor` is the floor number. Inside `generate` and `roamers` (a `count`, say) plain `floor` is that number too, but in a tile's commands `floor` is the `floor()` function, so write `var.floor` there. The `size` of `generate` and the `count` of `place` may read only `floor` (no `var`, `flag`, `rand` or `chance`), because a loaded save rebuilds the floor from its seed; a roamer's `count` may read anything.

<!-- verify: load group=generate -->
```yaml
locations:
  depths:
    name: The depths
    encounters: {rate: 0.1, troops: [{troop: wolves}]}
    dungeon:
      wall: wall.jpg
      generate:
        width: 15
        height: 15
        rooms: 3
        start: S
        place:
          - {char: ">", far: true}
          - {char: T, count: "floor < 5 ? 2 : 1"}
      tiles:
        S: {start: true}
        ">": {name: Stairs down, symbol: v, do: [{dive: depths, floor: var.floor + 1}]}
        T: {name: Chest, picture: chest.png, solid: "true", once: true, after: ".", do: [{gold: var.floor * 5}]}
```

Enter it with a `dive: depths` command. `seed` gives every save the same maps, and `floors` sets a seed or a hand-drawn map for one floor, such as a boss floor. Reference: [Generate](/schema/#generate), [Put](/schema/#put), [FloorPlan](/schema/#floorplan). [Look at the floors and play a dive](#check-and-measure) before you tune them.

## Let strong foes roam a floor

A roamer walks a generated floor and starts its battle when it touches the party; once beaten it is gone.

<!-- verify: load group=roamers -->
```yaml
locations:
  catacombs:
    name: Catacombs
    dungeon:
      generate: {width: 13, height: 13, rooms: 2, start: S, place: [{char: ">", far: true}]}
      tiles:
        S: {start: true}
        ">": {name: Stairs down, do: [{dive: catacombs, floor: var.floor + 1}]}
      roamers:
        - name: Ghoul
          picture: ghoul.png
          move: chase
          sight: 5
          count: "floor >= 3 ? 1 : 0"
          troops: [{troop: wolves}]
```

`move` is `chase`, `wander`, `guard` or `hunt`, and `meet` runs commands instead of a battle. Reference: [Roamer](/schema/#roamer).

## Change a dungeon's look

`looks` replace the walls, colors, darkness, music and the area name while their `when` holds, for example by floor (`var.floor`). The first that holds wins.

<!-- verify: load group=looks -->
```yaml
locations:
  tower:
    name: Tower
    dungeon:
      wall: brick.jpg
      floor: "#9c8f7a"
      floor_texture: stone.jpg
      darkness: 0.4
      generate: {width: 11, height: 11, start: S, place: [{char: ">", far: true}]}
      tiles:
        S: {start: true}
        ">": {name: Stairs up, do: [{dive: tower, floor: var.floor + 1}]}
      looks:
        - {when: "var.floor >= 6", area: Upper floors, wall: marble.jpg, floor: "#c8c8d0", darkness: 0.2}
        - {when: "var.floor >= 3", area: Middle floors, wall: mossy.jpg, ceiling_texture: vines.jpg}
```

A look can also set `battle_background`, `music`, `battle_music` and `no_map`. Reference: [DungeonLook](/schema/#dungeonlook), [Dungeon](/schema/#dungeon).

## Use a dungeon theme

A theme is a wall, floor, ceiling and door picture with matching colors. `gozen theme list` names the thirteen built into `gozen` (`cave`, `ice`, `ruins`, `castle`, `crystal`, `temple`, `sewer`, `forest`, `tomb`, `keep`, `dream`, `mine`, `vine`), and `gozen theme add` copies one into the game without network access.

<!-- verify: each fixture=cli -->
```shell
gozen theme add game --name cave
gozen theme add game --name ice
```

Then name it in the dungeon, and in a look for deeper floors. Keys written in the YAML (`wall`, `door`, `floor`, `ceiling`, `floor_texture`, `ceiling_texture`) win over the theme, and a look's theme replaces the dungeon's for whatever the look does not write.

<!-- verify: parse the theme folders come from gozen theme add, which the snippet above runs on its own copy -->
```yaml
locations:
  tower:
    name: Tower
    dungeon:
      theme: cave
      generate: {width: 11, height: 11, start: S, place: [{char: ">", far: true}]}
      tiles:
        S: {start: true}
        ">": {name: Stairs up, do: [{dive: tower, floor: floor + 1}]}
      looks:
        - {when: "var.floor >= 3", area: Deeper floors, theme: ice, door: gate.jpg}
```

The thirteen themes in the same corridor, in the order listed above:

![The thirteen dungeon themes seen in one corridor](/images/dungeon-themes.jpg)

`gozen check` fails with `GZN1111` when a named theme is not in the game. Reference: [Dungeon](/schema/#dungeon), [DungeonLook](/schema/#dungeonlook).

## Show a map with points and roads

A location with `map` is a picture with points (`schema_version: 3`), and the party walks the open roads between them. A point with `to` leads into that location; one with only `do` runs its commands.

![the party's face walks the road to the well](/images/go-zen-map.mp4)

<!-- verify: load group=map -->
```yaml
locations:
  valley:
    name: Valley
    map:
      picture: maps/valley.jpg        # point positions are in this picture's pixels
      unknown: dim
      points:
        home:   {at: [300, 620], to: field, start: true}
        store:  {at: [620, 480], to: shop}
        shrine: {at: [880, 600], label: Shrine, do: [{heal: all}, {say: You feel refreshed.}]}
        pass:   {at: [1100, 260], to: ridge, when: flag.pass_open, locked: Snow blocks the pass.}
      roads:
        - {between: [home, store], steps: 2}
        - {between: [store, shrine], via: [[760, 520]]}
        - {between: [shrine, pass], steps: 3, encounters: {rate: "0.2", troops: [{troop: wolves}]}}
```

Paint the roads into the picture; the YAML lines, which the token walks, are drawn only with `draw_roads: true`. Reference: [Atlas](/schema/#atlas), [MapPoint](/schema/#mappoint), [Road](/schema/#road), and [Maps and painted scenes](/writing-a-game/#maps-and-painted-scenes).

## Make a campaign

A campaign (`schema_version: 5`) is a war of regions over turns. The player gives a few orders a turn with the generals (actors of the game), and the other factions move by their `ai` rules when the turn ends. Its place is a map whose points are named like the regions.

![Five Banners, the campaign sample: the war map, the sortie screen, a clash of two armies, and the region in the Crown's colour](/images/campaign.mp4)

<!-- verify: load as=game.yaml group=campaign -->
```yaml
schema_version: 11
campaign:
  location: war
  player: crown
  turns: 10
  calendar: week                  # the band says "Year 1, week 1"
  count: orders                   # each order once a turn, then End turn
  generals:
    walker: {troops: 500, might: 3, tactic: guard, skills: [volley]}
  clash:
    units: 2                      # at most two units a side (1: a duel)
    per_soldier: 30               # one soldier in the ranks for 30 troops (the default is 50)
    # or, instead of the ranks, an army by kind: poses of one soldier, lined up by the engine
    # (the kind follows the leader: generals.<id>.army, enemies.<id>.army)
    # armies: {men: [soldiers/men_1.png, soldiers/men_2.png], monsters: [soldiers/orc_1.png]}
    # max_soldiers: 4            # most soldiers one unit of an army shows
    # layout: front               # or below (the default): large cards of the leaders along the bottom
    backgrounds: {field: bg/field.jpg, castle: bg/castle.jpg}
    skills:
      volley: {label: Volley, charge: 3, all: true, power: "a.troops * 0.12"}
  orders:
    attack: {label: Attack, general: ready, target: border, march: true, fight: clash_view, generals: 2,
             on_win: [{say: {who: walker, text: The ford is ours.}}]}
    draft: {label: Draft, per_turn: 1, when: "gold >= 50", general: any, do: [{gold: -50}, {set: {general.troops: "general.troops + 150"}}]}
```

<!-- verify: load group=campaign -->
```yaml
factions:
  crown: {name: Crown, color: "#3060c0", capital: vale}
  raven:
    name: Raven
    color: "#c03030"
    capital: well
    ai:
      - {draft: "150"}
      - {march: true, keep: 800, where: "s.troops * 0.6 > t.troops * 1.25", pick: min, by: t.troops}
regions:
  vale: {name: Ashvale, next: [ford], owner: crown, troops: 600, terrain: castle, area: [[0, 0], [600, 0], [600, 1080], [0, 1080]]}
  ford: {name: Brookford, next: [vale, well], troops: 350, terrain: field, defend: wolves, area: [[600, 0], [1300, 0], [1300, 1080], [600, 1080]]}
  well: {name: Deepwell, next: [ford], owner: raven, troops: 1500, terrain: castle, defend: wolves, area: [[1300, 0], [1920, 0], [1920, 1080], [1300, 1080]]}
locations:
  war:
    name: The war
    map:
      picture: maps/land.jpg          # the areas are in this picture's pixels
      points:
        vale: {at: [300, 540], start: true}
        ford: {at: [950, 540]}
        well: {at: [1600, 540]}
      roads:
        - {between: [vale, ford]}
        - {between: [ford, well]}
```

Attack asks for the target and opens the sortie screen, where the generals take the front and back slots, each with a tactic (the engine's `front`, `weakest`, `strongest`, `leader` and `guard`, or your `clash.tactics`). `fight: clash_view` plays the march as two armies on the terrain's picture: the soldiers in the ranks are the troops, a skill fires when charged, and a table shows the losses. Without `fight` the numbers of `clash` decide (`power` against `defense`); `fight: battle` fights the region's `defend` troop as a battle of the game. A region without an `area` shows as a flag. On a map of one picture only the chosen region gets its roads and outline, unless the map sets `region_roads: always` and `region_borders: always`. A region may also stand an optional cut-out `landmark` (its city or fort) on its point.

`gozen sim campaign game` plays 200 campaigns and prints the win rate, the turns and the regions each side held turn by turn; `--tactic guard` marches every general with one tactic, to compare tactics. Reference: [Campaign](/schema/#campaign), [Region](/schema/#region), [Faction](/schema/#faction), [Clash](/schema/#clash), [Order](/schema/#order), and [Campaigns](/writing-a-game/#campaigns). [Five Banners](https://github.com/nao1215/gozen/tree/main/examples/campaign) is a complete one ([play it in the browser](/play/campaign/)).

## Mix a campaign with a dungeon

The generals are the party's actors, and `move` goes between the war map and any other place. Here an order sends the party into a mine whose way out leads back to the war, and the clash reads the general's attack, so levels won in the mine make the marches stronger.

<!-- verify: load as=game.yaml group=mixed -->
```yaml
schema_version: 11
campaign:
  location: war
  player: crown
  turns: 8
  generals:
    walker: {troops: 400}
  clash:
    power: "a.troops * (1 + a.atk / 20) * (0.8 + rand() * 0.4)"   # the actor's attack counts
  orders:
    attack: {label: Attack, general: ready, target: border, march: true}
    delve: {label: Delve into the mine, cost: 0, do: [{move: mine}]}
```

<!-- verify: load group=mixed -->
```yaml
factions:
  crown: {name: Crown, capital: home}
  raven: {name: Raven, capital: keep}
regions:
  home: {name: Home, next: [keep], owner: crown, troops: 300}
  keep: {name: Keep, next: [home], owner: raven, troops: 600}
locations:
  war:
    name: The war
    map:
      picture: maps/land.jpg
      points:
        home: {at: [400, 540], start: true}
        keep: {at: [1500, 540]}
      roads: [{between: [home, keep]}]
  mine:
    name: Old mine
    dungeon:
      wall: walls/rock.png
      floor: "#6a6258"
      map: |
        #####
        #S.E#
        #####
      tiles:
        S: {start: true, facing: east}
        E: {name: Way out, look: door, do: [{move: war}]}
    encounters: {rate: "0.2", troops: [{troop: wolves}]}
```

Gold, items, flags, experience and HP are one state, so the mine's finds count on the map and `generals.walker.troops` can be read in the mine. Reference: [Campaigns](/writing-a-game/#campaigns) and [Make a campaign](#make-a-campaign).

## Paint a scene with people and balloons

A location with `stage` (`schema_version: 3`) is a painting the party walks as a small doll. The stage says where one can walk, how tall people are at two depths, where the people stand and what can be looked at; `bubbles` are lines said now and then in balloons.

![Maren walks the plaza and Ox says a line in a balloon](/images/go-zen-plaza.mp4)

<!-- verify: load group=stage -->
```yaml
locations:
  square:
    name: Square
    background: bg/square.jpg          # painted from a fixed camera
    stage:
      scale: [{at: [640, 700], height: 160}, {at: [640, 430], height: 70}]
      walk: [[[200, 420], [1080, 420], [1260, 715], [20, 715]]]
      spots:
        start: {at: [640, 680], facing: north}
      exits: [{to: field, area: [[560, 420], [720, 420], [720, 440], [560, 440]]}]
      people:
        - {id: baker, at: [420, 600], facing: east, doll: {hair: bun, parts: [{kind: apron, color: "#c08040"}]}}
        - {id: runner, path: [[800, 500], [1000, 520], [900, 640]], speed: 0.9}
      things:
        - {id: well, name: Well, area: [[900, 600], [960, 600], [960, 650], [900, 650]], do: [{say: The water is cold and clear.}], once: true}
    people:
      - id: baker
        name: Baker
        image: baker.png
        talk: [{say: Fresh bread every morning.}]
        bubbles: [Bread, fresh bread!, {text: Back so soon?, when: flag.met_baker}]
      - {id: runner, name: Runner, image: runner.png, talk: [{say: Cannot stop now!}]}
    exits:
      - {to: field, label: Out to the field}
```

`fronts` hide people behind pillars and stalls, `depth` does the same from a depth picture, and `ui.balloon` in `game.yaml` sizes the balloons. Reference: [Stage](/schema/#stage), [Spot](/schema/#spot), [StagePerson](/schema/#stageperson), [Thing](/schema/#thing), [Doll](/schema/#doll), [Bubble](/schema/#bubble).

## Give a character faces and a moving portrait

`faces` is a directory with one picture per expression, and a line picks one with `face`. A `puppet` makes the talk portrait blink, sway and move its mouth while the text runs, with faces of the same names.

![Maren talks, a moving portrait that blinks and changes its face](/images/go-zen-face.mp4)

<!-- verify: load group=puppet -->
```yaml
actors:
  guide:
    name: Guide
    class: walker
    faces: walker             # assets/walker/normal.png, assets/walker/smile.png
    puppet: walker/puppet.yaml
events:
  greeting:
    do:
      - say: {who: guide, text: The trail starts here.}
      - say: {who: guide, face: smile, text: Ready when you are.}
```

The puppet, `assets/walker/puppet.yaml` (picture paths are relative to it):

<!-- verify: load group=puppet as=assets/walker/puppet.yaml -->
```yaml
base: normal.png
face: normal               # the first face
faces:
  normal: {}
  smile: {image: smile.png}
eyes:
  closed: {image: eyes_closed.png, at: [166, 56]}
mouth:
  frames: [face, {image: mouth_open.png, at: [180, 92]}]
head: {center: [205, 85], radius: 75, neck: [205, 175]}
hair:
  - rect: [130, 70, 50, 90]
```

[Check where each piece lies](#moving-portraits) with `gozen puppet`. Reference: [Actor](/schema/#actor), [Say](/schema/#say), [FaceRule](/schema/#facerule), and [the puppet format](https://github.com/nao1215/gozen/blob/main/docs/architecture/puppet.md). [Go Zen's Maren](https://github.com/nao1215/gozen/blob/main/examples/go-zen/game/assets/maren/puppet.yaml) has a puppet. In a novel game, `ui.stage_puppets: true` lets the standing portraits move too ([Move the standing portraits](/writing-a-game/#move-the-standing-portraits)).

## Set the face icons

Menus and battle cards show a round face icon cut from the standing picture. `face_box` places the face (the center as fractions of width and height, then the size as a fraction of width), and `face_shade` darkens the icon.

<!-- verify: load group=faceicons -->
```yaml
actors:
  ione:
    name: Ione
    class: mender
    faces: ione                  # assets/ione/normal.png
    face_box: [0.5, 0.17, 0.32]
    face_shade: 0.15
locations:
  square:
    name: Square
    background: bg/square.jpg
    stage:
      scale: [{at: [640, 700], height: 160}, {at: [640, 430], height: 70}]
      walk: [[[200, 420], [1080, 420], [1260, 715], [20, 715]]]
      spots: {start: {at: [640, 680]}}
      people:
        - {id: baker, at: [420, 600], doll: {icon: icons/baker.png}}   # a square face for balloons
    people:
      - {id: baker, name: Baker, image: baker.png, bubbles: [Fresh bread!]}
```

In a painted scene, a balloon's face comes from the doll's `icon`, then `face_at`, then the face boxes that [gozen faces](#check-and-measure) finds. Reference: [Actor](/schema/#actor), [Doll](/schema/#doll).

## Show event pictures and fill the gallery

`image` shows a full-screen picture under the following lines, and `image: ""` removes it. The title screen's gallery opens a picture once it has been shown, and a scene once its flag is set.

<!-- verify: load group=gallery -->
```yaml
events:
  sunrise:
    do:
      - image: {file: cg/sunrise.jpg, fade: 1}
      - say: The sun comes up over the ridge.
      - image: ""
      - set: {flag.saw_sunrise: true}
```

<!-- verify: load group=gallery as=game.yaml -->
```yaml
# game.yaml
gallery:
  cgs:
    - {title: Sunrise, file: cg/sunrise.jpg}
  scenes:
    - {title: Sunrise on the ridge, event: sunrise, flag: saw_sunrise, location: ridge}
```

`files` shows several pictures in turn, and `look: dream` blurs the edges for a memory.

`gallery.endings` adds an Endings tab after Scenes. An ending is reached once its `flag` is set or its `when` holds, and stays reached across saves and new games; the tab counts them. An unreached ending shows ??? and its `hint`, a reached one its title and `picture`. Set the flag on the ending's last lines, and it is recorded before the game returns to the title.

![Brenna's ending runs out to THE END; the gallery's Endings tab lists it with its picture, the other two as ??? with a hint](/images/novel-endings.mp4)

<!-- verify: load group=endings -->
```yaml
events:
  farewell:
    do:
      - say: The road home is long, and you walk it together.
      - set: {flag.end_together: true}
      - clear: true
```

<!-- verify: load group=endings as=game.yaml -->
```yaml
# game.yaml
gallery:
  endings:
    - {id: together, title: Together, flag: end_together, hint: Stay with her after the festival.}
    - {id: alone, title: The Long Road, when: "flag.left_early", hint: Leave before the festival.}
```

`gozen check` reports an ending without an id, a title or a way to reach it, and two endings with the same id. Reference: [CG](/schema/#cg), [Gallery](/schema/#gallery), [GalleryCG](/schema/#gallerycg), [GalleryScene](/schema/#galleryscene), [GalleryEnding](/schema/#galleryending).

## Keep a character book

`profiles` gives each character a page on the Characters screen: the standing picture with its faces, your own labeled fields, an introduction and how they see the others. A profile whose ID is an actor's shows that actor; `picture` shows anyone else. Under `--dev` the title and menu offer Characters, F6 opens it anywhere and every page is open, so you can check characters while writing. `gozen faces` centers the list's face icons on the faces. Every text is translated through `lang/` like the lines.

`met: true` (schema 10) opens a page the first time a line of the actor is shown, and it stays open in later plays. A face with `seen: true` is offered once a line shows the actor with it, and `label` names its button. `ui.profile_book: {title: Who's who, layout: grid}` names the book and lays it out as a grid of face cards grouped by `affiliation`.

<!-- verify: load group=profiles as=game.yaml -->
```yaml
# game.yaml
schema_version: 11
ui:
  profiles: true    # Characters in the title, the menu and the novel bar for players too
```

<!-- verify: load group=profiles -->
```yaml
actors:
  mira:
    name: Mira
    class: walker
    faces: mira             # assets/mira/normal.png, assets/mira/smile.png
profiles:
  mira:
    title: The courier of the pass
    affiliation: Pass couriers
    faces: [normal, smile]  # the faces the screen switches between (Z)
    when: flag.met_mira     # a shadow named ??? until then; once open, it stays open
    fields:
      - {label: Role, value: Courier}
      - {label: Likes, value: Hot cider}
    text: Mira has carried letters over the pass for {var.mira_years} winters.
    relations:
      - {who: walker, kind: friend, calls: Boss, text: Trusts them with the map.}
      - {who: you, kind: other, calls: Stranger}
  keeper:
    name: The keeper
    picture: people/keeper.png
events:
  meet_mira:
    do:
      - set: {flag.met_mira: true}
      - open: profiles
```

`open: profiles` opens the book from an event, and a test reads `profile.mira.unlocked`. Reference: [Profile](/schema/#profile), [ProfileField](/schema/#profilefield), [ProfileRelation](/schema/#profilerelation), [UIStyle](/schema/#uistyle).

Tab in the book shows the relationship chart. Select a face with the arrows and press Z: everyone connected to it in either direction gathers around it, and other faces and lines fade. Z again reads its page, X goes back to the full chart, and the wheel zooms. A relation's `kind` is `like`, `love`, `rival`, `enemy`, `family`, `friend` or `other`; the colours mean only what you give them, and `love` has no engine rule. `affiliation` keeps a group's members together. Locked characters show as `?`, and their lines stay hidden until they open. The chart's face crops come from `assets/faces.json`: run `gozen faces <game-dir>` to make it, then `gozen check <game-dir>` to catch unknown targets and kinds.

## Keep a monster book

Add `lore` to each enemy met in encounters and set `ui.bestiary: true`. Meeting an enemy in battle reveals its name, picture and lore; beating it reveals its record and counts up `bestiary.<id>.defeated`, which expressions read along with `bestiary.<id>.seen`. Under `--dev`, F4 opens every page.

<!-- verify: parse the game setting shown by the following enemy example -->
```yaml
# game.yaml
schema_version: 11
ui:
  bestiary: true
```

<!-- verify: parse the compact enemy definition focuses on the fields used by the book -->
```yaml
# data/enemies.yaml
enemies:
  mote:
    name: Mote
    image: enemies/mote.png
    lore: "Motes drift through the old stone tunnels and gather near quiet water."
    stats: {mhp: 42, mmp: 0, atk: 9, def: 2, mat: 0, mdf: 1, agi: 5, luk: 1}
```

Use `open: bestiary` to open the book from an event. `gozen check` warns with GZN0115 when an enemy named by a troop has no lore, and `gozen book` prints the enemy pages. Reference: [Enemy](/schema/#enemy), [UIStyle](/schema/#uistyle).

## Collect, raise and breed monsters

Use schema version 6. Give each species a family, a positive rank and a class for its growth and level skills; `monster_class` defaults to `monster_<family>` when the game has that class. Add `recruit` only to species that may ask to join after a won battle. The data file below assumes `attack`, `guard` and the two pictures under `assets/`.

![the party wins, and a Drip Slime rises and asks to join, with Yes and No](/images/go-zen-recruit.mp4)

<!-- verify: parse a complete data fragment for a game with the pictured assets -->
```yaml
monster_families: {slime: Slime}
classes:
  slime:
    name: Slime
    exp_for: level * level * 10
    stats: {mhp: "30 + level * 6", mmp: "4 + level", atk: "5 + level * 2", def: "2 + level", mat: "3 + level", mdf: "2 + level", agi: "5 + level", luk: 4}
    skills: [{level: 1, skill: attack}, {level: 5, skill: guard}]
enemies:
  dewdrop:
    name: Dewdrop
    image: enemies/dewdrop.png
    family: slime
    rank: 1
    monster_class: slime
    sex: any
    sex_ratio: 0.5
    recruit: {chance: 0.2, level: 1}
    stats: {mhp: 30, mmp: 0, atk: 6, def: 2, mat: 1, mdf: 2, agi: 5, luk: 4}
  silverdrop:
    name: Silverdrop
    image: enemies/silverdrop.png
    family: slime
    rank: 2
    monster_class: slime
    stats: {mhp: 60, mmp: 5, atk: 12, def: 5, mat: 6, mdf: 5, agi: 7, luk: 4}
breeding:
  min_level: 5
  skill_slots: 2
  staging: {rare_rank: 4, legend_rank: 7}
  keeper: {name: Mara, text: "Bring me two monsters and I will join them."}
  pairs: [{parents: [dewdrop, dewdrop], child: silverdrop}]
  families: [{parents: [slime, slime], children: [dewdrop, silverdrop]}]
```

At a safe location, add an action such as `- {label: Monster ranch, do: [{open: ranch}]}`. The ranch shows active and reserve members, swaps them, asks before a release, previews a child (`?` until seen), lets the player choose inherited skills and stages the fusion. Each monster has a saved sex (♂ or ♀), and only a male and a female can breed; a species sets `sex: male`, `sex: female`, or `any` (the default) with an optional male `sex_ratio`. YAML events and development scripts can run `reserve: {in: monster_2, out: monster_1}`, `release: monster_2` and `breed: {parents: [monster_1, monster_2], skills: [attack]}`, whose IDs are saved instances, not species. Expressions read `reserve.size`, `monster.monster_1.family`, `monster.monster_1.plus`, `monster.monster_1.sex` and `bestiary.dewdrop.caught`.

The Yes/No request after a battle also works in Session scripts and traces. A full party sends a new recruit to the reserve, unless the species has `recruit: {chance: 0.2, level: 1, admission: party_only}` (the default is `admission: reserve`). Capacity is checked when each offer is accepted, also after loading a pending choice, and a full party gets a rejection message with no new instance, caught count or recruitment reward. Breeding still replaces its parents normally.

An event can recruit a specific species directly and react to a full party:

<!-- verify: parse event fragment using the recruitable species defined above -->
```yaml
- capture: dewdrop
- if: 'var.capture_result == "party_full"'
  then:
    - say: Make room in the active party before returning.
```

`capture` needs a non-boss species with `recruit` and schema version 6 or later, and follows `admission` too. It tries admission directly, without the post-battle chance, and stores `party`, `reserve`, `party_full` or `unavailable` in `var.capture_result`, replacing the previous result.

While tuning, run `gozen check game` (GZN1112 for invalid rules, GZN0116 for non-boss species no capturable monster can lead to), `gozen sim breed game --format json` for reachable and unreachable species, `gozen book game` for the rules, and battle and dive simulations. A specific pair wins over a family rule, and family offspring are chosen by the mean parent rank.

The monster book shows the shortest recipe first as a tree of up to seven pictures: the species, its two parents and four grandparents. Captured leaves show their locations, specific recipes both species, and family rules the families and an example pair. Arrows move between pictures, Z puts the chosen species at the top, X goes back along the breadcrumb, Tab changes the top species' recipe, and Q opens a known species' page. A parent with a deeper recipe has a `more` mark, an unseen species shows as `?`, and `--dev` reveals all. `gozen book game --format json` gives the same tree as data. In Go Zen, Yorn at the Breeding Altar opens the ranch: Whisper Imp comes from a male Incense Bloom and Pale Sylph, Vampire from Night Bat and Ghoul, and those two can produce Succubus, the two-generation path `gozen sim breed examples/go-zen/game` reports.

![Go Zen's two-generation family tree](/images/go-zen-lineage.jpg)
![Breeding at the altar: Yorn in the center, the parents, a silhouette reveal and the child](/images/go-zen-breeding.mp4)
![The author's breeding illustration](/images/breeding.jpg)

## Transform an enemy

Use an HP trigger for a change that happens immediately after damage. The resulting enemy keeps the slot and its share of HP.

![The Tollkeeper changes into its second form](/images/go-zen-transform.mp4)

<!-- verify: parse The fragment uses example enemy IDs and is completed by the game's other definitions. -->
```yaml
enemies:
  guardian:
    image: enemies/guardian.png
    hp_triggers: [{below: 0.5, transform: guardian_open, hp: ratio, clear: true}]
  guardian_open:
    image: enemies/guardian_open.png
```

Use `schema_version: 7`. The Tollkeeper in Go Zen uses this form change.

## Merge enemies

Give a self-scoped skill a merge effect. Its `from` list names the eligible sources; `count` includes the user. The new enemy occupies their middle slot.

![Drip Slimes gather into King Slime](/images/go-zen-merge.mp4)

<!-- verify: parse The fragment uses example enemy IDs and is completed by the game's other definitions. -->
```yaml
skills:
  gather: {name: Gather, scope: self, effects: [{merge: {into: king_slime, from: [drip_slime], count: 2, hp: sum}}]}
enemies:
  drip_slime:
    ai: [{use: gather, when: "allies.kind_drip_slime >= 2"}]
  king_slime:
    name: King Slime
    image: enemies/king_slime.png
```

## Change pictures by HP

List picture thresholds from high to low. An enemy names image files; an actor names faces in its face directory. Temporary hit reactions still take priority.

![Maren and the Drip Slime drop to low HP and their pictures switch](/images/go-zen-hp-pictures.mp4)

<!-- verify: parse The fragment names pictures supplied by the game. -->
```yaml
enemies:
  mote:
    image: enemies/mote.png
    images: [{below: 0.5, file: enemies/mote_hurt.png}]
actors:
  scout:
    faces: scout
    images: [{below: 0.35, face: worried}]
```

## Move a portrait with soft sway

Set `puppet` on an enemy or actor and define its motion in the puppet YAML. The same `soft_sway` region accepts `rect: [x, y, width, height]` or `ellipse: [x, y, width, height]`. `follow: body` makes it trail breathing and action or hit reactions. Coordinates are pixels of the base picture.

![The Succubus blinks, a soft region sways, and she reacts to a hit](/images/go-zen-motion.mp4)

<!-- verify: parse The fragment names pictures supplied by the game. -->
```yaml
enemies:
  sprite:
    image: enemies/sprite.png
    puppet: sprite/puppet.yaml
```

In `assets/sprite/puppet.yaml`:

<!-- verify: parse The puppet file is checked by gozen puppet when used in a game. -->
```yaml
base: base.png
eyes:
  closed: {image: eyes-closed.png, at: [40, 30]}
head: {center: [55, 40], radius: 25, neck: [55, 70]}
soft_sway:
  - {ellipse: [20, 75, 70, 60], root: top, follow: body, axis: y, amount: 1.2, limit: 6}
motion:
  breath: {depth: 0.012, period: 4}
```

Run `gozen puppet assets/sprite/puppet.yaml` to inspect the picture parts.

## Play music for places, battles and moods

A song is a file, or a modular song with one module per mood. A location plays its `music`, its ordinary battles play `battle_music`, and a troop's `music` comes first. Mood rules switch a modular song's mood as the moment changes.

<!-- verify: load group=music -->
```yaml
music:
  songs:
    town: {file: music/town.ogg, loop_start: 4.2}
    battle: {file: music/battle.ogg}
    boss: {file: music/boss.ogg}
    cave:                          # assets/music/cave/calm.ogg, assets/music/cave/tense.ogg
      dir: music/cave
      bpm: 96
      moods: {calm: calm, tense: tense}
  rules:
    field:
      - {song: cave, when: "party.min_hp_rate < 0.3", mood: tense}
locations:
  plaza: {name: Plaza, music: town}
  caves: {name: Caves, music: cave/calm, battle_music: battle}
troops:
  champion: {members: [mote], music: boss}
events:
  ambush:
    do:
      - mood: tense
      - battle: wolves
```

<!-- verify: load group=music as=game.yaml -->
```yaml
# game.yaml
ui: {title: {music: town}}
battle: {music: battle, boss_win_music: town}
```

A song named `battle` is the battle music by default. `music: town` in an event changes the song. Reference: [Music](/schema/#music), [Song](/schema/#song), [MusicCue](/schema/#musiccue), [MoodRule](/schema/#moodrule).

## Change the language and the engine's wording

`language` picks the engine's own words (menus, battle messages, settings, saves): `en` (the default) or `ja`. `text` replaces any of them, keyed by the engine's Japanese source text; a format keeps its `%d` and `%s`, and the value may reorder them (`%[2]d`). `terms` names the money and the stats.

<!-- verify: load group=language as=game.yaml -->
```yaml
# game.yaml
language: en
terms: {currency: coins, level: Lv., hp: HP, mp: MP, exp: EXP}
text:
  はじめから: New journey
```

[The English catalog](https://github.com/nao1215/gozen/blob/main/internal/base/text/catalog/english.go) lists every key. `gozen check` warns when the regular font lacks letters of the chosen words, and when `language` is unset but the game is written in Japanese. Reference: [Game](/schema/#game), [Terms](/schema/#terms).

## Offer the game in two languages

Write `data/` in one language and translate it in `lang/<code>.yaml`, a map `text` from each text as written to its translation, `{expressions}` kept. `languages` lets the player choose in the settings, which keep the choice.

![the settings switch to Japanese, the line on the screen turns Japanese at once with a reading over a word, and so does the next](/images/novel-language.mp4)

<!-- verify: load group=languages as=game.yaml -->
```yaml
# game.yaml
schema_version: 11
language: en          # shown at first
written_in: en        # the language of data/
languages: [en, ja]   # the settings offer both
```

<!-- verify: load group=languages -->
```yaml
events:
  dinner:
    do:
      - say: {who: Keeper, text: "Soup or bread? You have {gold} gold."}
      - choice: [{label: Soup}, {label: Bread}]
```

<!-- verify: parse a file of lang/, which the load examples do not place; the translations are written in romaji here, and in Japanese letters in a game -->
```yaml
# lang/ja.yaml
text:
  "Soup or bread? You have {gold} gold.": "Supu to pan, dochira ni? Shojikin wa {gold} G."
  "Keeper": "Yado no shujin"
  "Soup": "Supu"
  "Bread": "Pan"
```

Lines, speakers, choices, prompts and notices are translated as they are shown. The names and descriptions of places, people, actions, exits, items, skills, states, enemies, classes and shops, the title, and the gallery's pictures, scenes and endings change when the language does.

`|base《reading》` puts a reading in small type above a word, as web novels do; the bar may be left out when the base is a run of kanji (`乾杯《かんぱい》`), as in the Japanese lines of the video. The backlog shows the base alone, `gozen check` measures a line without its marks, and a `《` with no base before it or no `》` after it is shown as written. Read lines are kept by the text as written, so they stay read in every language. One text has one translation, so words that need two translations must be written differently. The same map may also replace the engine's own words in that language, as `text` in `game.yaml` does.

`gozen check` reports a translation that drops or changes an `{expression}`, warns of a key that is no text of the game (GZN0110) and of each untranslated text (GZN0111, with its path; `--format json` lists them for a script or an agent to fill in), and lays out the lines in each language. A test of the Japanese game loads the same files with `game.yaml` set to `language: ja` (an internal integration test overlay), and its requests are in Japanese. Reference: [Game](/schema/#game) (`written_in`, `languages`).

## Test-play with notes, reload and quick starts

With `--dev`, F5 reloads the YAML without losing your place, F9 writes a note (a screenshot, the state and your comment), and `--start` and related options begin anywhere. `memo` in `game.yaml` sets the kinds of note and the values kept with each.

<!-- verify: load group=memo as=game.yaml -->
```yaml
# game.yaml
memo:
  kinds:
    - {id: text, label: Wording}
    - {id: balance, label: Too hard or too easy}
    - {id: bug, label: Bug}
  vars:
    - {name: gold, value: gold}
    - {name: steps, value: steps}
```

The commands are in [Test play](#test-play). Reference: [MemoCfg](/schema/#memocfg), and [The edit–play loop](/edit-play-loop/) for every key and option.

## Measure battles with sim battle and sim dive

`gozen sim battle` fights a troop many times on AUTO and prints the win rate; `gozen sim dive` takes the party down a generated dungeon floor by floor and prints levels, gold and boss win rates. Measure again after each change to a stat, an AI rule or a troop. The F7 panel shows the same numbers while you play.

The commands are in [Check and measure](#check-and-measure). Reference: [Commands](/commands/#sim).

## Check and measure in CI

In GitHub Actions, [setup-gozen](https://github.com/nao1215/setup-gozen) installs the release binary of the version in your `go.mod`, checked against the release checksums, so every push can run the check and the simulations:

<!-- verify: skip runs in GitHub Actions -->
```yaml
# .github/workflows/game.yml
on: [push, pull_request]
jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: nao1215/setup-gozen@1c378fc446f62ecb29a7bcd40aea6dff8565d698 # v0.1.1
      - run: gozen check game
      - run: gozen sim battle game --troop boss --format json   # boss: a troop id of your game
      - run: gozen sim campaign game --format json      # for a campaign
```

`gozen check` exits 1 on any error, so a broken reference fails the job. Without the action, `go run github.com/nao1215/gozen/cmd/gozen check game` builds the version in `go.mod`. Reference: [Install](/install/#in-github-actions), [Commands](/commands/#check).

## Cut a release and a trial

`gozen release` copies the game with only approved pictures. `gozen trial` builds a demo from the part of the game inside the range of `trial.yaml`, removing the definitions and assets it can prove are outside; `-n` counts what would be kept, and `--list` writes what was kept and dropped. The analysis is conservative, so unresolved branches and kept files can still hold later content: it is no encryption or secrecy guarantee, and you should inspect the output before you ship ([Check the contents of a trial](#check-the-contents-of-a-trial)). This demo stops at the grotto's third floor and leaves out the ending event:

<!-- verify: same internal/dev/docs/testdata/fixtures/cookbook/game/trial.yaml -->
```yaml
# game/trial.yaml
dive_max: {grotto: 3}      # dives below the third floor run blocked instead
cut: [finish]              # the ending; calls to it run blocked instead
blocked:
  - say: This is as far as the trial goes. Thank you for playing!
```

The release reads which pictures are approved from the game's `asset-policy.yaml` and the inspection records:

<!-- verify: same internal/dev/docs/testdata/fixtures/cli/game/asset-policy.yaml -->
```yaml
# game/asset-policy.yaml
overlay: ../overlay
report: approvals.json
extensions: [.png]
inspection: {file: file, source: source, required: required, approved: approved}
records: {file: file, source: source, required: required, approved: approved, output: output}
```

A trial built without `--overlay` keeps the game's `asset-policy.yaml`, so `gozen export` refuses it until its assets are approved; with `--overlay` and `--inspect` it is an approved tree. Either way it keeps all the game's translations and its license notices.

The commands are in [Release and trial](#release-and-trial). Reference: [Commands](/commands/#release), [Commands](/commands/#trial).

## Commands used by the recipes

The examples use `game` as the game directory, `lesson` as a troop and `grotto` as a generated dungeon. [Commands](/commands/) has every option.

### Check and measure

<!-- verify: each fixture=cookbook -->
```shell
gozen check game                                       # references, expressions, pictures, text layout
gozen sim battle game --troop lesson -n 200 --level 5          # win rate of the party at level 5
gozen sim battle game --troop lesson --party scout --seed 7 -v # the messages of one battle
gozen sim floors game --location grotto --from 1 --to 3           # the generated maps
gozen sim dive game --location grotto --floors 5 --boss 5:lesson  # a whole dive: levels, gold and wins per floor
gozen book game > book.md                              # the whole game as one Markdown file
```

`gozen faces` finds the faces of the standing pictures for balloons; it needs a detection model:

<!-- verify: skip downloads a model and needs a cgo build with ONNX Runtime -->
```shell
gozen models fetch face
gozen faces game
```

### Test play

Use the game directory and enable the author tools:

<!-- verify: skip opens a game window and waits for input -->
```shell
gozen run game --dev --memos notes           # F5 reloads game/, F9 writes notes/
gozen run game --dev --start cave_mouth --level 5 --battle bats
gozen run game --dev --start village --flags sela_joined,var.asked=3 --gold 500
gozen run game --dev --event greeting --line 2
gozen run game --dev --from-memo notes/2026-01-01/001.json
```

Collect the notes into one HTML page:

<!-- verify: each fixture=cli -->
```shell
gozen notes notes -o notes-html
```

### Moving portraits

<!-- verify: each fixture=puppet -->
```shell
gozen puppet streamer/puppet.yaml                        # check it and show where each piece lies
gozen puppet streamer/puppet.yaml --out streamer-packed  # write it with small difference pictures
```

### Release and trial

<!-- verify: each fixture=cookbook -->
```shell
gozen release game --out release --overlay overlay --inspect inspect.jsonl
gozen trial game -n                               # count what the trial keeps
gozen trial game --out trial --list trial.txt           # build it, and list what was kept and dropped
```

## A speaker who remembers a greeting

Create a novel with `gozen new my-story --template novel`, add `reactions: {rowan: rowan.yaml}` to `game.yaml`, and save this reaction book as `assets/rowan.yaml`:

<!-- verify: parse reaction book loaded by the narrative resume integration test -->
```yaml
people:
  - id: rowan
    rules:
      - {id: repeated, when: tag.repeat > 0, say: [Welcome back.]}
      - {id: welcome, say: [Hello.]}
```

With `- respond: {who: rowan, text: Hello}` in a location's `on_enter` or an event's `do`, the first greeting gets "Hello." and later ones "Welcome back.", and saves keep that memory. Keep ordinary lines as `say` and use `respond` where rule-based variation helps. The speaker needs no battle class or stats.

## Check the contents of a trial

A condition the trial analysis cannot prove false keeps its branch, so a successful build does not mean the later story is gone. `gozen trial game -n` counts the unresolved conditions, and `--list trial.txt` lists their paths with the kept definitions and assets.

`forbid` in `trial.yaml` fails the build when chosen definitions or assets remain:

<!-- verify: parse trial configuration fragment -->
```yaml
cut: [ending]
forbid: [events.ending, assets/ending/*]
```

`cut` replaces calls to the named event with the `blocked` commands. `forbid` checks kept IDs in `events`, `troops`, `enemies`, `locations`, `skills`, `ai`, `shops` and `songs`, and asset paths, with the same glob syntax as the other patterns, and reports an error before writing anything. Still play the trial's routes before you ship it.

## Record filled stream lines

For a stream game (schema version 9), set `voices: voice` in the streams file and, optionally, `voice_fallback: Thank you.` on each channel, with the translated phrase recorded as Ogg Vorbis in the channel's voice folder. A dynamic line plays the recording of its filled text, else of its translated unfilled template, else the channel fallback, else none; player tip quotations also use the fallback, and static lines work as usual. The subtitle shows the full filled line. Without a recording, the browser speaks the line with speech synthesis by default.

`gozen book GAME --section voices --format json` exports what to record. The result's `data` has `version: 1` and `lines`, each with `channel`, `language`, `text` and `file`; record each exact text as Ogg Vorbis at its game-relative `file`, once per path, and set `voices` before exporting. Records with `fields` are templates whose `file` hashes the translated text with placeholders intact, so record them without speaking the name or other runtime values.

The inventory expands NPC names for their question options, and bot-tip reactions from crowd names and tip lines. Player names and input, visit counts, reaction mood, matched slots and remembered values stay runtime values, and reaction candidates may include rules that do not run for a given tip. Translations are applied before filling and hashing, and recordings named after translated unfilled templates still work. `gozen check` checks template and expanded keys, missing fallback recordings (GZN0119) and audio format (GZN0120). The browser checks its file manifest before loading a voice, so a missing recording costs no network request.

`missing_voice: captions` beside `voices` (in the file that `game.yaml`'s `streams` names) keeps unvoiced lines silent. The field takes `speech` (the default) or `captions`, and anything else fails `gozen check` with GZN1104. It covers missing recordings, decode errors and fetch timeouts, an unavailable channel fallback included. Captions mode keeps the desktop caption duration and mouth timing, the loading deadline and the question-answer hold, and does not change recording playback or timeout logs. The speaker ON/OFF button controls recorded audio when `voices` is set and is hidden otherwise; the captions button is separate.

## Wear stone sets and fuse stones

An item of slot `stone` fits the equip slots `stone1`, `stone2` and so on. A `stone_sets` entry names stones worn together: while one member wears all of them, the member has the set's `stats` and `skill`. `for` lists the members a set suits, and `only: true` limits it to them.

<!-- verify: load group=stones as=game.yaml -->
```yaml
# game.yaml
schema_version: 11
equip_slots:
  - {id: weapon, name: Weapon}
  - {id: stone1, name: Stone 1, manual: true}
  - {id: stone2, name: Stone 2, manual: true}
```

<!-- verify: load group=stones -->
```yaml
classes:
  gemwright:
    name: Gemwright
    stats: {mhp: 20, mmp: 4, atk: 4, def: 2, mat: 2, mdf: 2, agi: 4, luk: 1}
    exp_for: level * 20
    equip: [stone]
actors:
  sage: {name: Sage, class: gemwright}
items:
  amber: {name: Amber, kind: stone, slot: stone, stats: {def: 2}}
  jet: {name: Jet, kind: stone, slot: stone, stats: {mat: 3}}
  flint: {name: Flint, kind: stone, slot: stone, price: 6, stats: {atk: 1}}
  spark: {name: Spark, kind: stone, slot: stone, stats: {atk: 4}}
stone_sets:
  dusk: {name: Dusk, desc: Amber and jet together., stones: [amber, jet], stats: {mdf: 4}, for: [sage]}
fusions:
  - {from: [flint, flint], to: spark, price: 10}
locations:
  workshop:
    name: Workshop
    safe: true
    actions:
      - {label: Fuse stones, do: [{open: fusion}]}
      - {label: Wear Dusk, do: [{equip_stones: {actor: '"sage"', set: '"dusk"'}}]}
      - label: Ask for advice
        do:
          - stone_suggest: tip
          - if: var.tip > 0
            then:
              - say: "{var.tip_page1}"
              - stone_apply: tip
            else:
              - say: Your stones are already at their best.
    exits: [{to: field, label: Out to the field}]
```

`manual: true` keeps the menu's best-equipment choice and its offers away from a slot, which suits stones, since a set is worth more than its stones' numbers. `equip_stones` takes the stones from the bag or from members not using them; `actor` and `set` are expressions, so a fixed ID is quoted. `stone_suggest: tip` finds the strongest way to wear every owned stone and writes `var.tip`, the number of members whose stones would change (0 when the party already wears the best), with a text per member in `var.tip_page1`, `var.tip_page2` and so on; `stone_apply: tip` re-equips everyone that way. `stone_choose`, `stone_wearer` and `stone_decline` let the player pick a set and its wearer, or refuse the advice until the owned stones change.

`open: fusion` opens the `fusions` screen, where a recipe takes the stones of `from` and the `price` and gives the stone `to`. Reference: [StoneSet](/schema/#stoneset), [Fusion](/schema/#fusion), [EquipStones](/schema/#equipstones), [Slot](/schema/#slot).

## Set a retry point with a checkpoint

`checkpoint: EVENT` (schema version 9) keeps the state and random numbers of that moment as the retry point, replacing the previous one. `restore_checkpoint: true` drops what happened since, returns to that state and runs the named event. Saves hold the retry point, so restoring works after loading too.

<!-- verify: load group=checkpoint as=game.yaml -->
```yaml
# game.yaml
schema_version: 11
```

<!-- verify: load group=checkpoint -->
```yaml
locations:
  trailhead:
    name: Trailhead
    on_enter:
      - checkpoint: back_at_the_trailhead
    actions:
      - {label: Go back to the trailhead, do: [{restore_checkpoint: true}]}
    exits: [{to: field, label: Up the trail}]
events:
  back_at_the_trailhead:
    do:
      - say: You are back where the trail begins.
```

Everything changed after the checkpoint is undone, flags and variables included, so set what must survive a retry (a count of attempts, say) in the checkpoint's event. Reference: [Command](/schema/#command).

## Insert a thought after a speaker's lines

`asides` in `game.yaml` run an event right after a given speaker's line, while `when` holds, and the conversation resumes when it ends. Name the speaker by `actor`, by the line's `name`, or by a pattern for its `face`.

<!-- verify: load group=asides as=game.yaml -->
```yaml
# game.yaml
asides:
  - {actor: walker, when: "!flag.walker_wondered", call: walker_wonders, skip: ["finale*"]}
```

<!-- verify: load group=asides -->
```yaml
events:
  walker_wonders:
    do:
      - say: (Why does the trail feel shorter today?)
      - set: {flag.walker_wondered: true}
```

No aside runs during a battle, inside another aside, or while an event matching `skip` (or one it calls) runs. The engine does not count asides, so limit how often one comes with a flag or variable set in the event and tested in `when`. Reference: [Aside](/schema/#aside).

## Dress the title screen

In `ui.title`, `background` is the picture behind the title and `picture` a logo or key art over it. `pictures` stand figures on the screen instead, later ones in front, and then `picture` is not drawn. `text` letters the title and subtitle, and `music` plays.

<!-- verify: load group=title as=game.yaml -->
```yaml
# game.yaml
ui:
  title:
    background: title/sky.jpg
    pictures:
      - {file: title/guide.png, x: 0.3}
      - {file: title/walker.png, x: 0.62, height: 0.8, bottom: 0.98}
```

`x` is the horizontal center as a fraction of the screen width; `height` (default 0.95) and `bottom` (default 1, the bottom edge) are fractions of the screen height. Reference: [TitleScreen](/schema/#titlescreen), [TitleFigure](/schema/#titlefigure).

## Review pictures and scenes while writing

`cg_review` and `scene_review` in `game.yaml` set the tags of two review tools. With `--review-log FILE` and `cg_review`, pictures can be tagged; adding `--gallery-all` gives the gallery a column listing the `gallery.scenes` to tag. `gozen run game --dev --review --review-log review.jsonl` starts by showing every event's pictures one by one.

<!-- verify: load group=review as=game.yaml -->
```yaml
# game.yaml
gallery:
  cgs:
    - {title: Sunrise, file: cg/sunrise.jpg}
cg_review:
  tags:
    - {id: ok, label: OK, ok: true}
    - {id: redraw, label: Redraw}
  extra: [cg/draft_*.jpg]
  groups:
    - {label: Sketches, files: [cg/sketch_*.jpg]}
scene_review:
  title: Scene check
  tags:
    - {id: ok, label: OK, ok: true}
    - {id: pacing, label: Pacing}
```

The number keys attach the first ten tags. A tag with `ok: true` marks a picture done, and the filter for finished pictures hides it. `extra` adds pictures the gallery does not list, and a `groups` entry is a set left out unless chosen before the review. Picture tags go to the review log as JSON Lines, scene tags to `scene-review.jsonl` beside it. Reference: [CGReviewCfg](/schema/#cgreviewcfg), [SceneReviewCfg](/schema/#scenereviewcfg).

## Tune a place, the windows and the battle pace

<!-- verify: load group=tuning as=game.yaml -->
```yaml
# game.yaml
schema_version: 11
battle:
  tempo: 1.5          # battle messages and moves go by half as fast again
  back_weight: 0.5    # the weight of one back-row member as a target (default 1)
  passives:
    - {actor: walker, state: steady, when: flag.trained}
ui:
  battle_message: fixed    # always three lines tall; fit (the default) is as tall as its lines
  choice_position: center  # the novel window's choices in the middle of the screen
  field_notices: false     # no small notices on the field
  hide_gold: [field, map]  # no money window in these kinds of place
```

<!-- verify: load group=tuning -->
```yaml
states:
  steady: {name: Steady, stats: {def: 1.2}}
locations:
  snow_town:
    name: Snow town
    outfit: winter          # actors talking here wear <faces>/winter/<face>.png when there is one
    no_menu: true           # the menu cannot be opened here
    talk_on_enter: true     # one person's talk plays on each entry instead of a list of people
    people:
      - {name: Porter, talk: [{say: Mind the ice.}]}
    exits: [{to: snow_inn, label: The inn}]
  snow_inn:
    name: Snow inn
    safe: true
    resume_to: snow_town    # a save made here starts from the town when it is loaded
    actions:
      - {label: Train, do: [{set: {flag.trained: true}}]}
    exits: [{to: snow_town, label: Back outside}]
```

A `passives` state is put on the member at the start of each battle while its `when` holds, and ends with the battle. Enemies aim at a front-row member with weight 4 and a back-row member with weight 1 unless `front_weight` and `back_weight` say otherwise. `hide_gold` takes `field`, `dungeon`, `map` and `stage`. With `talk_on_enter`, people who have something new to say come first. Reference: [BattleCfg](/schema/#battlecfg), [Passive](/schema/#passive), [UIStyle](/schema/#uistyle), [Location](/schema/#location).

## Change states, duels and skill points from an event

<!-- verify: load group=eventverbs -->
```yaml
states:
  chilled: {name: Chilled, turns: 3, stats: {agi: 0.8}}
actors:
  guide: {name: Guide, class: mender}
locations:
  pass:
    name: Mountain pass
    on_enter:
      - affect: {who: all, add: chilled, resist: true}   # each member's state resistance may keep it off
    actions:
      - {label: Warm up, do: [{affect: {who: all, remove: chilled}}]}
      - label: Duel the mote alone
        do:
          - battle: wolves
            fighters: [walker]     # only these actors fight, not the whole party
            on_win: [{say: The mote backs away.}]
            on_lose: [{say: You step back to the fire.}]
      - {label: Unlearn, do: [{reset_tree: all}]}
      - label: Ask how to spend skill points
        do:
          - sp_suggest: plan
          - if: var.plan > 0
            then: [{say: "{var.plan_page1}"}, {sp_apply: plan}]
    exits: [{to: field, label: Down to the field}]
```

`affect` puts a state on party members outside a battle (`who` is an actor's ID or `all`) for the state's `turns` unless `turns` is given, or takes one off with `remove` (`remove: bad` takes off every bad state). A battle with `on_lose` runs those commands after a defeat instead of the game over. `reset_tree` refunds the SP spent in the skill tree of one actor or `all`. `sp_suggest: plan` plans how each member's spare SP would be spent, without spending it, and writes `var.plan` (the number of members with a plan) and a text per member in `var.plan_page1` and so on; `sp_apply: plan` spends it.

In a dungeon, `open_wall: H` turns the wall beside the party's cell (the one it faces first) into the tile `H` and turns the party to it, and the floor and saves keep it. Under the dungeon's `complete`, it makes a stair appear once every cell has been seen:

<!-- verify: load group=eventverbs -->
```yaml
locations:
  cellar:
    name: Cellar
    dungeon:
      map: |
        #####
        #S..#
        #####
      tiles:
        S: {start: true, facing: east}
        H: {name: Hidden stair, look: door, solid: "true", do: [{move: field}]}
      complete:
        - open_wall: H
```

Reference: [Affect](/schema/#affect), [Command](/schema/#command), [Dungeon](/schema/#dungeon).
