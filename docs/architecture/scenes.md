# Maps and painted scenes

A location (`locations`) can be shown in four ways: a menu (the default), a dungeon (`dungeon:`), a map (`map:`) and a painted scene (`stage:`). Maps and scenes can be written with `schema_version: 3`. The decision is recorded in [ADR 0022](../decisions/0022-maps-and-painted-scenes.md), and the item list is in the [schema](https://nao1215.github.io/gozen/schema/) under `Atlas`, `MapPoint`, `Road`, `Stage`, `Spot`, `StagePerson`, `Thing` and `Doll`.

## Rules know points, looks are pictures

The only position the rules (headless, `Explore`, `gozen sim battle`, `gozen check`) know is one standing point.

- Map: on the picture, a small marker per point (`marker`; if absent the screen draws a small pin, replaceable per point with `marker`) and a name plate (text size is `label_size`; placed so as not to overlap other plates, markers or the token). Roads are assumed to be painted into the picture; the polyline of a road in the YAML is the line the token walks, and by default it is not drawn (`draw_roads: true` draws it). A visited point has a name plate, an unvisited point follows `unknown` (`hidden` is not shown until an open road reaches it, `dim` is faint, `mark` is a "?" plate, `named` has a name plate like a visited point), and a closed point has a gray marker. The token (`token.picture`, or the round face of the first party member if absent; `token: doll` is a walking doll) stands on the current point and slides along roads. The arrow keys pick the point closest to a road's direction and confirm, a point can be clicked or tapped (the hit area is finger size), and the menu key or a long press opens a list. Choosing makes the token walk the road, and each time it reaches the end of a road, that road's steps, encounters and `do` run. A closed point says only its `locked` line. In a script, `walk <point>`.
- Scene: the current standing point (an id of `stage.spots`).

The state is `spot` (the name in expressions, `spot` in the save). The coordinates of free walking inside a scene, and of the token partway along a road on a map, are held only by the screen and are not saved. After returning from a save, the party stands on the `spot` point. A `spot` that does not exist in the current location is emptied (meaning the entrance) at load and shown in `repairs`. `set: {spot: ID}` is only for `--dev` and tests.

## Map

For the shape of a map (`picture`, `points`, `roads`, `to`, `when`, `locked`, `do`, `once`, `encounters`), see [Maps and painted scenes](https://nao1215.github.io/gozen/writing-a-game/#maps-and-painted-scenes) in the guide for game authors. This page adds the following.

- `unknown` takes `hidden`, `dim`, `mark` or `named`, and decides how an unvisited point looks.
- Items of a point: of the points reachable from the current point through the roads and points whose `when` holds, one with `to` is "-> name" (the same as an exit), and one with only `do` is its name (`label`, or the id if absent). A point with `once` does not appear again after it has run once. A point whose `when` does not hold becomes an item that says its `locked` line once it is next to a reachable point (it does not move).
- When you take a point, the party passes along the shortest roads (the sum of `steps`) in order. For each road, `steps` is added to the step count, `encounters.rate` is rolled `steps` times to stack battles (not rolled for `calm`), and the road's `do` runs. After the arrived point's `do` (`once` is recorded as `<location>/<point>`), `move` runs if there is a `to`.
- The point on arriving at a map: the point of `at` of `move` (`exits[].at`), otherwise the point that leads back to the location it came from (its `to` is the original location), otherwise the point of `start` (the first point if there is none). Visited points stay in `Mapped[<map location>]`.
- A map inside a map: if a point's `to` is a map, `at_point` can name a point over there.

## Painted scene

For the shape of a scene (`background`, `stage` with `depth`, `scale`, `walk`, `blocks`, `fronts`, `spots`, `exits`, `people`, `things`), see the same section of the [guide for game authors](https://nao1215.github.io/gozen/writing-a-game/#maps-and-painted-scenes). This page adds the following.

- Items: people (`talk:`), actions, `things` (their own `do`; one with `action` only gives a place to an action, and `when`, `trigger` and `ask` are written on the action's side or on the thing itself), `spots` with `do`, and exits. Taking a person's or thing's item makes the party stand at the spot closest to that person or thing. Taking a spot's item stands at that spot and runs its `do`.
- `camera` is the look-down angle in degrees (the angle the dolls are baked at), `depth_bias` is the difference counted as in front of the feet, `fronts[].base` hides a person whose feet are farther than `base`, and `depth` is a 16-bit gray (0 far to 65535 near, the same size as the background) and may be omitted.
- An exit's `to` and a thing's `action` give a place to an exit or an action (`exits` of `stage` and `actions` of the location).
- The spot on arriving at a scene: the spot of `at` of `move`, otherwise the spot of `start` (the first spot if there is none).
- Look of a doll (`doll`): hair (`short`, `long`, `bald`, `bun`; if omitted, it is inferred from the portrait), parts (`apron`, `cape`, `robe`, `skirt`, `dress`, `coat`, `shield`, `staff`, `glasses`, `hat`, `hood`, `bag`, `goggles`, `muffler`, `shawl`, `ponytail`, `lashes`, with `#rrggbb`), and the face crop `face_at: [center, top, width]`. Colors are picked from the portrait. The doll of the protagonist (the first party member) is `actors.<id>.doll`.
- Speech balloons (`people[].bubbles`): a list of strings or `{text, when}`. They exist only on screen and do not appear in the rules. The size is `ui.balloon` of game.yaml (the height of the text as a ratio of the screen height; the default is 0.037; the face is a circle 3 times the text with an outline). All have the same size, and only people farther than 8 m from the protagonist are one step smaller (0.85 times, never smaller than 3% of the screen height); the frame width follows the text while keeping a common minimum width. A balloon is placed just above the doll's head and points at the head with a short tail. It does not run off the screen edge, overlapping balloons are shifted with the farther one up (down if impossible), and the tail keeps pointing at its own head.

## Screen

- Map: open roads (white) and closed roads (faint) are shown over the picture, together with the points. A visited point has a name, an unvisited point follows `unknown` (`hidden`, `dim`, `mark` = "?"), and a closed point is dark with diagonal lines. The token (`token.picture`, or the protagonist's doll if absent) stands on the current point. The arrow keys pick the point closest to a road's direction and confirm, points can be clicked or tapped, and the menu key opens a list. Choosing makes the token walk the road, and each time it reaches the end of a road, that road's steps, encounters and `do` run. A closed point says only its `locked` line. In a script, `walk <point>`.
- Scene: the protagonist (the first party member) is a doll and walks with the arrow keys (1.6 m/s; a person's height is taken as 1.7 m and measured from `scale`). Clicking the floor walks there. Confirm takes the item of a person or thing within reach (2 m) or of a spot (0.6 m), whichever is faced first. When the feet enter an exit's area, the exit's item is taken. The menu key or a right click overlays a list of the items of the current place, and choosing in the list walks to that place and then takes it.
- Dolls pick colors from the portrait and are baked into 8 directions by 5 frames (a separate goroutine per person; scripts and recordings wait for the baking to finish). They are hidden where the depth is nearer than the feet, and under a `fronts` whose `base` is behind the feet.
- Clicks and taps in a scene walk a route around the blocks (A*). Clicking or tapping a person or thing walks near it and talks or examines. For touch, a tap is a click, a long press (0.5 s) opens the list on maps and scenes (nothing happens inside the list, and releasing chooses that item), and two fingers are a right click (back, in lists and menus). It works the same in a browser (`GOOS=js`).
- How to try: after `--start <location>`, use `walk <spot>`, `face <direction>` and `click x y` (logical coordinates, in units of height 720) in `--script`. The trace shows `spot` (the point stood on). The feet coordinates are kept even when F5 reloads.

## The face box (`gozen art faces`)

The round face in a balloon is centered on the face box of the portrait and cut round at 1.4 times the box (including a little hair). The box is found at the author's stage by `gozen art faces <game-dir>` through the face detection of `gozen art fetch face` (deepghs anime_face_detection), and written to `assets/faces.json` (picture path -> box, size, SHA-256). The game only reads this file and does not run the model, so the face is the same on WASM. The priority is the doll's `icon` (`doll: {icon: icons/ox.png}`; a square face picture drawn for the balloon, whose middle is cut round; `gozen art faces` skips this person, and `gozen check` makes a missing picture an error and a non-square one a warning), then `face_at`, then `faces.json` (when the picture's size is the same), and if none, an estimate from the top of the picture. `gozen check` warns when a scene's portrait has no box, when the picture changed, and when no face was found. In a gozen without cgo, `gozen art faces` says the reason and fails.

The reason for one file as the place is not to scatter the products written by authoring tools beside the pictures, and to have the check tell a swapped picture by SHA-256 (what is written by hand is YAML, what a tool writes is JSON).

## Checks

`gozen check` prints errors as `file:line:column: logical path: text`, and `--format json` is the same. Warnings (which do not stop loading) are a point not reachable from the start, a destination with no exit back, an exit or thing that does not touch the walkable area, and a self-intersecting polygon.

```text
data/town.yaml:42:16: locations.plaza.stage.spots.gate.at: [48, 22] is not where one can walk (outside walk or in blocks)
data/town.yaml:33:14: locations.town.map.points.gate.locked: locked is said when the point is closed: it needs when
```
