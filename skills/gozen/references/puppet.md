# Moving portraits (puppet)

One picture plus small difference pictures make a portrait that blinks, moves its mouth while text runs, breathes, sways its head and hair, and changes expression. Put `puppet: PATH` (relative to `assets/`) on an actor or an enemy; the file's picture paths are relative to the puppet file. Enemy puppets need `schema_version: 7`.

- Name the puppet's `faces` like the actor's faces (`assets/<faces>/<face>.png`), because a line's `face` selects them. A line whose face the puppet lacks shows the face picture as before.
- The mouth moves only while text appears. In the `box` message window the speaker's portrait is the puppet (`ui.talk_portrait` places it); novel `effect` reactions can move a standing puppet. Portraits in menus and cut-ins stay still. Static `ui.stage_framing` does not apply to puppets.
- The randomness comes from the actor's ID, not the game's, so recordings replay the same. F5 rebuilds it.
- Run `gozen puppet FILE` to check where each piece lies; `--out DIR` writes the difference pictures cut out, which makes the distributed files smaller. Leave a margin above and at both sides of the picture: head, hair and swaying parts extend beyond it.

Cookbook: "Give a character faces and a moving portrait" and "Move a portrait with soft sway". Full format: docs/architecture/puppet.md in the Gozen repository.

## The pieces

<!-- verify: parse a puppet file, not game data; the pictures are the author's -->
```yaml
base: normal.png
face: normal                 # first face; required with 2 or more faces
faces:
  normal: {}
  smile: {image: smile.png}
eyes:
  closed: {image: eyes_closed.png, at: [166, 56]}
  every: [2, 5.5]            # seconds between blinks
mouth:
  frames: [face, {image: mouth_open.png, at: [180, 92]}]
head: {center: [205, 85], radius: 75, neck: [205, 175]}
hair:
  - rect: [130, 70, 50, 90]
motion:
  breath: {depth: 0.006, period: 3.8}
  sway: {angle: 2.5, shift: 10, period: 6.5}
```

A whole redrawn picture differs slightly everywhere, so use `area: [x, y, w, h]` to restrict where the difference is searched. Unknown keys are errors.

## Swaying parts for enemies and actors

`springs` and `soft_sway` list rect, ellipse or poly regions that lag behind what they follow. `follow: body` trails breathing and whole-body reactions, `follow: head` the head. Tune with `stiffness`, `damping`, `amount` and `limit`. `motion.hit: soft_only` makes a hit move only the `soft_sway` regions, which grow livelier as HP falls (`motion.sway.damage_boost`), instead of shaking the whole picture.

<!-- verify: parse a puppet file fragment, not game data -->
```yaml
soft_sway:
  - {ellipse: [490, 760, 240, 160], root: top, follow: body, axis: y, amount: 1.2, limit: 8}
motion:
  hit: soft_only
```

## Idle motion

Give a part `idle: {amplitude, period, phase}` to move it at full HP while the body is still. `amplitude` is pixels of the base picture, `period` seconds (default 2) and `phase` degrees. It pauses during a freeze reaction. Zero `motion.breath.depth` and `motion.sway` when you want a still body.

<!-- verify: parse a puppet file fragment, not game data -->
```yaml
soft_sway:
  - ellipse: [34, 52, 20, 36]
    follow: body
    axis: y
    limit: 4
    idle: {amplitude: 4, period: 2, phase: 0}
motion:
  hit: soft_only
  breath: {depth: 0}
  sway: {angle: 0, shift: 0}
```
