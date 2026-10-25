# ADR 0022: Maps and painted scenes — rules are points, looks are pictures

Status: Accepted (2026-10-20). Stage 0 (rules, checks, headless), stage 1 (the scene screen), stage 2 (people, speech bubbles, things), stage 3 (the map screen) and stage 4 (A*, touch, WASM) are done. Stage 1 onward is under "Plan" below. Home: `internal/layer2/gamedata` (YAML types and checks), `internal/layer1/geom` (plane geometry), `internal/layer3/atlas` (rules for map roads and standing points), `internal/layer3/field` (the entries of a place).

## Users and problem

Let the player walk around a town on a picture instead of picking from a menu list. The author made two decisions (2026-10-20).

1. Map: move from point to point on a single picture, along roads. A point is an entrance to a place. Facilities stay as the current menu.
2. Scene: a box-shaped doll walks on a background painted for a fixed camera (one picture plus a depth picture), FF7 style. A voxel town is set aside (the conclusion of the voxel-spike prototype). Dungeons stay as the current first-person four-direction view.

The users are authors who write maps and scenes in YAML, and the authors' tests that run them through headless tests and `gozen design`. Neither writes Go code (principles 0 and 1 of [ADR 0021](0021-query-and-commands.md)).

## Decision

### Not a new kind of place

Both the map and the scene are a way of showing a `Location`. In the same form as `dungeon:`, which means "walk in first person instead of a menu", `Location` gains `map:` (map) and `stage:` (scene). One place has exactly one of `dungeon`, `map` and `stage`. People, actions and exits stay the current `people`, `actions` and `exits`; `stage:` adds only "where one stands" and `map:` adds only "which point is which place".

### Rules are points, looks are pictures

The only position that the rules (headless, `Explore`, `sim`, `check`) know is one standing point.

| Layer | Map | Scene |
| --- | --- | --- |
| Rules (state) | `location` is the map's place, `spot` is the id of a point | `location` is the scene's place, `spot` is the id of a standing point (`stage.spots`) |
| Screen only (not saved) | The coordinates of the piece partway along a road | The foot coordinates (picture pixels), direction and walking phase |

- The only addition to the state is `State.Spot` (JSON `spot,omitempty`). The save format version stays 1 (an old save has it empty, meaning the entrance). On reload, a `spot` that is not in the current place is cleared and reported as a repair (`repairs`).
- Add the expression name `spot` (usable in place conditions and event commands). The left side `spot` of `set` is for `--dev` and tests only (same treatment as `pos.x`).
- The points a map has visited are `State.Mapped[<map place>]` (one byte per point, in the order written).

### The source of truth for entries is the current menu entries

`field.Entries` (the entries of a place's menu) is also the source of truth for entries on maps and scenes. So `pick`, `headless.Driver.Do/GoTo`, `Explore`, auto-play and `gozen design` work unchanged in shape. The screen only changes "pick an entry" into "walk to it".

| Way of showing | Entries |
| --- | --- |
| Map | Among the points reachable from the current point over open roads, those with `to` (`→ 名前`, the engine text "→ name", handled like an exit) and those with `do`. A closed point (its `when` does not hold) becomes an entry that says the `locked` line when it is reachable to the neighbor |
| Scene | People (`話す：`, the engine text "Talk:"), actions, `things` (things to examine), and `spots` that have `do`. Exits are the current `exits` |

Taking an entry stands at that entry's standing point (`Entry.Spot`). People and things of a scene are rounded to the nearest `spots`. Taking a map point runs, per road, the step count (`State.Steps`), encounters (the road's `encounters`, drawn `steps` times; not drawn when `calm`) and the road's `do`, then the point's `do` (`once` is `Done` as `<place>/<point>`), and `move` if the point has `to`. No new command is added (`pick` and `move` suffice).

### Where one arrives

The `at` of `move` (`exits[].at`) is widened: if the destination is a dungeon it is a character (as before), for a scene it is the id of a `spots`, and for a map it is the id of a point. When omitted, a scene uses the `start` spot (or the first spot if none), and a map uses the point that returns to the place one came out of (the point whose `to` is the original place), or the `start` point if none.

### Checks

`gozen check` reports errors of maps and scenes with line numbers: a missing picture or depth picture, a depth size different from the background, a point or polygon outside the picture, a polygon with fewer than 3 points, a `spot`, person or road point outside the walkable area or inside a hit area, two `start`s, a direction word, a `stage.exits[].to` that is not in `exits`, a `stage.people[].id` that is not in `people` or appears twice, both or neither of `at` and `path`, the doll's parts and colors, a `things[].action` that is not in `actions`, fewer than 2 `scale` marks or marks at the same height, a map's `to`, `at_point` and `between` points, duplicate roads, a point with no road, a `locked` without `when`, the points and spots of `exits[].at` and `move.at`, and duplicates of `map`, `stage` and `dungeon`. Warnings (they do not stop loading; `DB.SceneWarnings`): points not reachable from the start point, a destination with no exit back, exits or things that do not touch the walkable area, and self-intersecting polygons. The new words (`map`, `stage`, `doll`, `bubbles`, and `spot` in expressions and `set`) require `schema_version: 3`. YAML of version 2 or earlier is read as it is.

### No new public API

`api/public.txt` stays at 197. The user's entrance is YAML (`map:`, `stage:`, `doll`, `bubbles`) and the expression `spot`. `play.Config`, `Screen`, `Request` and `Effect` do not change (bubbles are screen only, and movement is `move`). `api/schema.txt` gains `Location.Map`, `Location.Stage`, `Person.Bubbles` and `Actor.Doll`, plus `DB.SceneWarnings`, `DB.ScenePictures` and `Location.Spotted` used by checks and repairs, and the internal commands `Command.Cross` and `Command.Reach` (stacked per road when a map point is taken; they cannot be written in YAML). The new definition types (`Stage`, `Atlas` and so on) get no alias in `schema` (they are reachable from the fields; add one if a user appears who uses them by name).

### Layers

| Home | Content | Component |
| --- | --- | --- |
| `internal/layer1/geom` | Points, polygons, inside/outside, distance, self-intersection, polylines (`Polyline` is used in stage 3 for the map piece to advance along a road) | private1 |
| `internal/layer2/gamedata` (`stage.go`, `atlas.go`) | Types, checks, the order written, `SceneWarnings` | private2 (aliased from `schema`) |
| `internal/layer3/atlas` | `Here`, `Routes` (shortest by steps), `Choices`, `Go`, `Cross`, `Place`, `Visit` | private3 |
| `internal/layer3/field` | The expansion of `Entries`, `Entry.Spot`, `Step` (stepping on a spot) | private3 |
| `internal/rpg` | `State.Spot`, the expression `spot`, `set spot`, save repair | rpg |
| `internal/headless` | `route` goes through map points | headless |
| `internal/layer5/doll`, `internal/layer5/stage` (stages 1 and 2) | Baking the box doll, drawing the scene (a shader that hides by depth and fronts) | private5 |
| `internal/play` (stages 1 and 2) | The scene screen, walking, bubbles, the safety net of the list | play |

The stage 1 screen: `internal/layer5/doll` (the prototype's box doll. Since it is a convex solid made only of planes, ray marching is dropped and rays are cast against the bounding sphere of each solid. A body of 8 directions by 5 frames at 96x168 takes about 0.8 s, each person is baked in a separate goroutine, and people appear as they finish. Scripts and recordings wait for the bake to finish), `internal/layer5/stage` (ground geometry and drawing. The background is centered to fit the screen, and the doll's shadow is not drawn behind fronts), and `internal/play/stage.go` (the scene screen. Arrow keys walk, confirm takes the nearby person, thing or spot, the menu key or right click overlays the list of the place's entries, and choosing in the list walks to the entry's place and then takes it. When the feet enter an exit's area, the exit entry is taken. A click walks to the floor (a straight line until the A* of stage 4, sliding when blocked). F5 keeps the foot coordinates).

The stage 2 screen: a person points to `people` by the id in `stage.people`, and either stands as a doll whose colors are picked from the portrait (`image`) or walks along `path` (when `when` changes they fade in or out. They wait when the protagonist blocks the road. The protagonist also stops when bumping into a person). Bubbles use the prototype's timing (one person within 4 m for 3.5 s, the same person rests 15 s, and every 5 s someone speaks for 2.8 s), and the face is cut round from the portrait (`doll.face_at`). Confirm takes first, among reachable things, the one being faced (2 m; a spot is 0.6 m). The name of a reachable thing is shown at the bottom of the screen. When one talks, the other turns toward the protagonist and stops, and the portrait in the conversation window is the same as the current `standTalker` (if a line's `face` is given a portrait path, the portrait stays even when the speaker's name is shown). A `trigger: step` thing and a spot with `do` run when the feet enter (not run again until they leave).

The bubble face (the author's remark of 2026-10-20, "the face icon is not centered"): `gozen faces <game-dir>` finds the face box of each portrait using the face detection of `internal/mosaic/detect` (the model of `gozen mosaic fetch face`) and writes it to `assets/faces.json`. At run time ONNX is not used; this file is read (the same on WASM). The priority is `doll.icon` (a square face picture drawn per person, approved by the author on 2026-10-20), `doll.face_at`, `faces.json`, then an estimate. `icon` is part of the doll's look, so it is placed in `doll`, which can be written in the same form for scene people (`stage.people[].doll`) and companions (`actors.<id>.doll`). The CLI (`commands`) may import the detection of `mosaic` (an existing permission in `.go-arch-lint.yml`), so no new component is added. Detection is also usable by general tools, so when a second user of inference appears with `gozen depth` (stage 5), the inference adapter will be moved to something like `internal/layer1/onnx`. The destination is one JSON, not small files beside the pictures, so that the generated artifacts of the tool are not scattered and the check can warn by SHA-256 when a picture is replaced.

Fixes from the review (2026-10-20): taking a map point stacks, per road, the internal command `cross` (after the road's steps, encounters and do, it passes the point at the end of the road) and `reach` (stand at the point, the visited mark, the point's do, move). When a road's battle is lost, one is at the last point reached (`Spot`, `Mapped` and `Done` are not written ahead). A resume back to a map with `resume_to` stands at the point of the place one came out of by `atlas.Place`. F5 with only pictures rebuilds the scene's textures. `gozen trial` keeps `assets/faces.json` (`release` copies the whole tree so it stays, but if pictures were replaced, run `gozen faces` again; the check warns on a SHA-256 difference). The warnings of maps and scenes are collected once by `Check` and held, and `SceneWarnings` does not redo the check. Open: the `pictures.decode` of the baking goroutine and the `FS.Forget` of F5 may run at the same time (assuming that an FS with `Forget` does not interfere with loads during a reload; the check with -race is done together with the WASM of stage 4). The matter of the background's `Unmanaged` images was solved in stage 4 by making them ordinary images.

Stage 4: clicks, taps and list walking on a scene use `stage.Scene.Path` (a grid of the walkable area: 16 pixels at 1920 width, about 100 cells for a small picture, at least 4 pixels; 8-direction A* that does not cut blocked corners; at the end the corners are dropped as far as a straight line can go). Pointing at an unwalkable place goes to the nearest walkable place. Touch is mapped to the mouse in `input` (a tap is a click where it is released, while pressing the finger position is the pointer, and two fingers are a right click). A long press (0.5 s) is received only by maps and scenes and opens the list of the place's entries; on other screens it does nothing, and releasing makes a click (so one can choose without the list closing even when hesitating over an item in it). The hit area of a map point is 40 units (the size of a finger; larger than the drawn marker). Clicking or tapping a person or thing in a scene walks near it and talks or examines (the same as choosing in the list). A* reuses the grid arrays and also drops corners on the grid (`BenchmarkPath`: about 1 ms per call with 40 boxes in a 1920x1080 scene, for a diagonal and a far outside click, with 5 allocations; before the review it was 14 ms to 300 ms). WASM: the depth and fronts images were changed from `Unmanaged` to ordinary images (Ebitengine restores the contents even if the browser loses the WebGL context. The shader reads with `imageSrcNUnsafeAtFromSrc0Pos`, so it is the same wherever it is placed in a texture. Confirmed by the check of pixels behind a pillar under Xvfb). The sample scene was built with `GOOS=js`, and a plaza was opened and captured in headless Chrome (swiftshader WebGL), showing the doll, bubbles and background. Touch on a real device is not confirmed.

`atlas` does not import `dungeon` (because `dungeon` uses `field` and `field` uses `atlas`). `move` calls `dungeon.Place` after `atlas.Place` as before. `.go-arch-lint.yml` does not change (the component `depth` is to be added with `gozen depth` in stage 5).

### What to use from the prototype and what to discard

| Prototype (voxel-spike) | Handling |
| --- | --- |
| The polygons, `Front`, linear height function, `Free`, `Move`, and the drawing and shader of `internal/prerender` | Use (`geom`, `gamedata`, `layer5/stage`) |
| The YAML loading (`fileScene`), own people, tour and view of `internal/prerender` | Rewrite (`Location.stage`, people refer to `people`, capture by `--script`) |
| The box doll of `internal/voxel` (`BakeLowPolyAt`, 17 kinds of parts, colors from the portrait, cutting out the face) | Use (`layer5/doll`). Discard the round doll |
| The nearby-person and bubble timing and round face of `examples/prerender-scene` | Use (`play`). Discard the own window, fonts and conversation, and use the current conversation window |
| The voxel town (town, render, shader, light, spots, wear) | Discard (the branch is kept) |

## Plan

| Stage | Content | Status |
| --- | --- | --- |
| 0 | This ADR, spec version 3, types and checks, `State.Spot` and the expression `spot`, `field.Entries`, `atlas` and `route`, `gozen book` | Done |
| 1 | The scene screen (PC keys): background, depth, fronts, the protagonist's doll, walking, exits, the safety net of the list, `--script walk <spot>` | Done |
| 2 | People: dolls (colors from portraits), `path`, bubbles, talking (the current conversation window), `things`, the `do` of a spot | Done |
| 3 | The map screen: picture, points, roads, piece, closed points, unvisited points, encounters on roads, a map within a map | Done |
| 4 | Mouse click walking (A*), touch, WASM | Done |
| 5 | Creator tools: polygon editing and writing back in `--dev`, `gozen depth` (ONNX with cgo) | Planned |
| 6 | Samples (examples) and README | Planned |

The stage 3 map screen (`internal/play/atlas.go`): fit the picture to the screen, and put a small marker per point (a `marker` picture, or a small pin the screen draws if none; replaceable per point with that point's `marker`) and a tag with a calm name. Roads are meant to be painted into the map picture, and the road polylines in YAML are used only for the piece's movement and hit areas and are not drawn by default (`draw_roads: true` draws them. After the author's remark of 2026-10-20, "the map is too shabby", the white roads and round points drawn in code were dropped). A visited point gets a name tag; an unvisited point follows `unknown` (`hidden` is not shown until an open road reaches it, `dim` is faint, `mark` is a "?" tag, and `named` is a name tag like a visited point, for maps such as towns where the places are known), and a closed point has a gray marker. The piece is the map's `token.picture`, or if none the round-cut face of the first companion (the same face as the bubbles; `token: doll` gives a doll). Arrow keys select the point closest to a road's direction and confirm; click or tap selects a point; the menu key shows the list. Choosing runs the commands of `atlas.Go` one at a time, and before `cross` the piece walks that road (0.12 of the picture width per second, with the town picture the author chose, a speed at which walking the street is visible. The piece's face is 9.5% of the screen height, with an accent-colored rim and a light shadow, a dark shadow at its feet and a flat ring for the current place. It glides along the road, has no walking animation, and bounces slightly only while moving (the author's instruction of 2026-10-20, "a face icon is fine for moving on the map; there is no need to force a doll to walk". The face is found by its rim color, so no arrow is put above the head). A `token: doll` doll is 17% of the screen height, with a dark shadow and a circle at its feet and a small swaying arrow above its head. The point where the piece stands draws no marker, only the tag. The name tag's text is 3% of the screen height (changeable with the map's `label_size`), on a dark ground with an outline and a light shadow, and the selected point uses the window color without changing label geometry. A tag goes below its point, and if blocked, above the marker, to the left or right, or below shifted, in that order, at a place that overlaps no other tag, marker or piece (if every place overlaps, the one with the least overlap). The marker pin is 6% of the screen height. This follows the author's remarks of 2026-10-20, "the tag text is small and buried in the picture" and "the piece is small and hard to find"). A road's battle occurs where the piece reaches the end of the road, and on a loss it stops there. The trace records `travel` (from, to, roads) and the `spot` stood on for each road. How the points and roads look, and the choices, are recomputed only when the state changes (after a command, on arrival, and every second). F5 while walking waits until the walk ends. The line of a closed point is not a `notice` but the current conversation window (`say`; the form of `atlas.Go` in stage 0). The script's `walk <point>` ends where it starts walking if the point has `to` (arrival is awaited).

## Navigable map camera

A large connected world can opt into a camera while retaining the same map rules:

```yaml
map:
  picture: world.png
  camera: {zoom: 3, max_zoom: 8}
  unknown: named
```

The snippet belongs to a location with authored points and roads. Omit `camera`
to retain the fitted overview. With `camera: {}`, the initial zoom is 2 and the
maximum is 8. Zoom is relative to the fitted picture; 1 always shows the entire
map. The camera clamps to the picture, centering an axis that is smaller than the
viewport. Marker text stays at `label_size` of the screen height; zoom does not
shrink it. Offscreen point labels are not moved onto the viewport edge.

| Control | Effect |
| --- | --- |
| Arrow keys / D-pad | Select the nearest point in that direction from the current selection; reveal offscreen selections |
| Tab | Cycle all available points |
| `+` / `-`, mouse wheel, right/left shoulder | Zoom in/out around the view center |
| Home / controller View | Fitted overview |
| End / left stick click | Focus current party position at the configured initial zoom |
| P / right stick click | Toggle pan mode; directions then pan without selecting or traveling |
| Onscreen arrows, `-`, `+`, All, Here | Pointer/touch equivalents for pan, zoom, overview and current position |

Confirm or clicking a marker/label retains ordinary travel. The camera follows the
token while traveling. Picture, roads, ownership areas, landmarks, markers, labels
and token use the same transform. Looking around does not change the party's spot,
road steps, encounters or resources.

Camera position, zoom and pan mode belong to the screen, not saves. Entering or
loading the map starts at the party's saved point and configured zoom. Opening and
closing a menu retains the view; reloading game definitions or leaving and
returning resets it. No camera data enters headless exploration or save migration.

## Alternatives

| Alternative | Why not chosen |
| --- | --- |
| Make maps and scenes a new kind of place (`kind: map`) | It would hold the rules for people, actions, exits, arrival and saving twice. As a way of showing, the current rules apply unchanged |
| Save screen coordinates in the state | The rules (headless, Explore) would have to know coordinates, and the branches of play would multiply by coordinate. Rounded to points, the branches are only the entries |
| Put the scene position in `pos` (the dungeon position) | It collides in meaning with the dungeon position (the repair of `pos.in`, the map record). A separate name `spot` reads better in expressions |
| `scene:` instead of `stage:` | It collides with the start option `--scene` and the `scenes` of recollections |
