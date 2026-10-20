# ADR 0020: Moving portraits (puppet)

Status: Accepted (2026-10-19). Home: `puppet` (a layer 5 public package), `internal/layer5/puppet` (the shared implementation), `internal/layer2/puppetdef` (YAML types and validation), and `gozen puppet` (checking and cutting out differences).

## Users and problem

Make a single-image character move like a streamer's avatar. The author's decision (2026-10-19) is "Make the character move, with reasonably good animation" and "as a general-purpose Gozen feature".

| User | Usage |
| --- | --- |
| A streamer game (the first user) | Always show the streamer's transparent PNG from the chest up in a layer between the background and the UI. It hops, nods or changes expression in response to comments. The mouth moves during lines |
| An RPG conversation portrait (the next user) | The portrait of the speaking character blinks and moves its mouth while a line is displayed. The expression changes by the conversation's expression setting |

The assets that can be drawn by hand are limited to a base picture plus a closed-eye picture, mouth shape pictures and pictures of other expressions. Pictures split into parts, and manual work on bones and weights (rigging), are not required.

## Decision

Overlay small difference pictures on one base picture, and draw the result warped by a mesh (a triangle grid).

1. Compositing: every frame (only when something changed), overlay the base picture, the expression difference, the closed eyes and the mouth shape onto an image of the same size as the base. An expression switch blends the previous and next differences by opacity.
2. Deformation: turn the composed image into a grid of about 48 divisions on the long side and draw it with `DrawTriangles`. A "head weight" and a "hair weight" are computed beforehand for each vertex. The head weight is 1 inside the head circle, falls to 0 at 0.35 radius outside the circle, and falls to 0 at 0.4 radius before the base of the neck. This lets the head turn or shift around the base of the neck while the neck joins smoothly. The hair area (a rectangle or polygon, moving more toward the bottom) is shifted by the lag of a spring (stiffness and damping) that follows the motion of the head center. Breathing stretches the whole body vertically with the bottom edge as the reference, and a reaction adds to the translation and scaling of the whole and the angle and shift of the head.
3. Motion: idle sway is a sum of sines with different periods, a head tilt is an event with jitter in its interval, a blink has jittered intervals and an occasional double blink, and the mouth changes between two or three shapes irregularly only while speaking. Random numbers come from `Config.Rand` (nil means a fixed seed), so the same seed and the same number of Updates give the same motion.

| Type or function | Who uses it and for what |
| --- | --- |
| `Load(fsys, path) (*Rig, error)` | Read and validate the YAML and PNGs. For a difference picture of the full size, only the differing part is cut out with blurred edges (`area` can narrow the search range). A difference picture with no transparency uses the transparency of the picture below |
| `(*Rig).Size()` | Use the base picture's size for placement calculations |
| `Config{Rand, TPS, Face}`, `New(rig, cfg) (*Puppet, error)` | Create one body to show on screen. Any number can be made from the same Rig |
| `Update`, `Draw(dst, op)` | Called from the host's Update and Draw. Uses the GeoM, ColorScale and Blend of `op`; interpolation is always linear |
| `Speak`, `Stop` | Move the mouth only while a line is displayed |
| `React(name)` | Play once a hop, shrink, shake, nod, tilt, or a variant of different strength named in YAML |
| `SetFace(name)` | Change the expression softly (0.25 s by default) |
| `Close` | Release the GPU images |

In RPG conversations, writing a puppet YAML in the companion definition `Actor.Puppet` (one public field of `data`) turns the conversation window's portrait into a puppet (`internal/play/puppets.go`). It calls `SetFace` with the expression name of the line and moves the mouth only while characters are being displayed.

`puppet` exposes 17 declarations (3 types, 2 functions, 8 methods, 3 fields, and the package). Compositing, deformation and motion are kept internal, and strength values are changed only in YAML. The YAML format is described in [Moving portraits](../architecture/puppet.md).

### Layers

`puppet` uses Ebitengine, so it is a layer 5 public package, and an independent component is added to `.go-arch-lint.yml` (same permissions as `gfx` and the like; it does not refer to the sideways `gfx` or `play`). The implementation lives in `internal/layer5/puppet` so that it creates no sideways dependency when `internal/play` later uses it for conversation portraits. The YAML types and structure validation know nothing about images, so they live in `internal/layer2/puppetdef` (layer 2, where YAML use is allowed). No exception that allows YAML in layer 5 shared implementations is added.

`gozen puppet` checks with the same `Load` and prints the position of each difference. With `--out`, it writes the picture of differences cut out small and a YAML with `at` into a new directory (to make the distribution smaller).

### Swaying parts (addendum 2026-10-19)

The author's request (2026-10-19): in the sample game, show a character whose ribbon, hem of clothing, sleeves and so on, and not only the hair, sway with a lag behind the motion. Since it is a sample for general audiences, the feature is a general "swaying part" that carries no name of a specific part.

`springs:` (a list of parts) was added to the YAML. Each part gets a range (rectangle or polygon), a root (an edge of the range or a point), what it follows (`head` or `body`), a direction (`x`, `y` or `both`), the spring's stiffness and damping, an amount (`amount`) and a limit on the lag (`limit`). `body` follows where the root point moves under breathing and the whole-body reactions (hop, shrink, shake), so a part attached low on the body sways with a lag behind the vertical motion of the body. Where parts overlap, a later part in the list replaces the motion of an earlier one.

- No public API is added. Only YAML items; the Go types are in `internal/layer2/puppetdef`. `api/public.txt` does not change.
- `hair` goes through the same computation as the first part of the list (root on the top edge, following the head). Existing YAML is read as is, and the mesh vertices are bit-for-bit the same as before the change (`TestHairUnchanged`, `TestHairIsASpring`).
- The default strength is the same small value as hair (the lag is up to 15% of the head radius). There are at most 16 parts (including hair), with stiffness up to 10000 and damping up to 1000. With stiff springs or a small TPS, Update is split into sub-steps for calculation and does not diverge.
- The range is blurred inward, so vertices do not move outside the range. Through triangle interpolation, the influence on the picture extends at most one mesh cell beyond the range (`TestGraphicsSpringStaysInItsArea`).
- Alternatives: a separate Go type or public setting per part was not chosen because YAML alone suffices. Making a part a physical chain (several segments) was not chosen because the segment boundaries show easily on a single-image mesh and it is heavy for the amount of sway that one spring suffices for.

## Alternatives

- Live2D: the motion quality is high, but there is no Go runtime (the official SDKs are C++, C#, JS and Java), and it would bring in cgo and redistribution terms. The assets also need splitting into parts and manual rigging, which does not fit the aim of getting by with "a single picture and differences".
- Inochi2D: a part-based system with a free license. Its runtime is in D, so using it from Go needs a port or cgo. It also needs splitting into parts and rigging, as Live2D does.
- Splitting the picture finely into parts (layers): put hair, face and body in separate transparent PNGs and move them overlaid. It gives more freedom of motion, but cutting parts out of a generated single picture and painting in the hidden areas is a lot of work. Mesh deformation moves the head and hair while keeping one picture. If parts become wanted, it can be extended later by adding per-part weights to the same mesh.
- Deformation only, with no differences: eyes and mouth cannot be made by deformation (closed eyes need another picture). Differences are small pictures.
- Put it in `gfx`: `gfx.Load` uses font names, which conflicts with the `Load(fsys, path)` convention. The use is also different, so it is an independent package.

## Verification

- `internal/layer2/puppetdef/spec_test.go`: loading valid YAML, 26 kinds of errors (unknown keys, picture extension, range, strength, kind of reaction, hair without a head, and so on), and `FuzzParse`.
- `internal/layer5/puppet/spring_test.go`: the look of hair is unchanged (the sum of the mesh before the change), a default part equal to hair gives the same motion as hair, the same sway for the same seed, swaying with a lag behind the body's hop and settling, the direction limit and the cap, vertices outside the range do not move and the order of overlaps, stability with stiff springs and a low TPS, and the weight of a point root.
- `internal/layer5/puppet/puppet_test.go`: automatic cutting of differences (location, transparency of the edge, ignoring tiny dots, a picture with no difference), 8 kinds of loading errors, the same motion for the same seed and different motion for different seeds, the number of blinks and mouth shapes, expression blending, returning to the original when a reaction ends, the original mesh coordinates being inside the picture and the vertices never straying from the picture by more than the head radius over 3000 updates, the head weight (nothing below the base of the neck moves), and reading back the `--out` output to get the same differences.
- `puppet/puppet_test.go`: loading, creating, reacting, expressions and errors through the public API. `puppet/graphics_test.go` (`make graphics`, Xvfb): reads the real rendering and checks the base color, translucency, blinking, mouth, the colors midway and at the end of an expression, that the bottom edge lifts when hopping, that nothing is drawn after Close, and that the mesh draws nothing outside the shape.
- `internal/layer2/gamedata/check_test.go`: 4 kinds of errors of a companion's `puppet` (missing file, YAML error, missing picture, missing companion expression) and the valid form.
- `e2e/atago/puppet.atago.yaml`: the output of `gozen puppet`, writing and reading back with `--out`, an existing output destination, and the exit code on error.
- Not confirmed: rendering on WASM, Windows and macOS. Speed when many bodies of large pictures (4000 pixels or more on the long side) are shown.

## Review before Gozen goes public

1. Two users, the streamer game and the RPG conversation, actually use it. Do not keep an API that only suits one of them (for example `Config.Face`).
2. The default motion strengths look natural for pictures of different sizes (a large chest-up picture, and a full-body conversation portrait).
3. If a part-based request appears, check that it can be extended by holding mesh weights per part.
