package play

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"testing/fstest"
)

// wldPNG is a plain picture of the given size with a diagonal gradient (so that scaling and
// cropping show up as different pixels, and nothing is a single flat color).
func wldPNG(t testing.TB, w, h int, base color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			k := uint8((x + y) * 60 / (w + h))
			img.SetNRGBA(x, y, color.NRGBA{R: base.R + k, G: base.G + k, B: base.B + k, A: 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// wldDepthPNG is a 16-bit gray depth map: far at the top, near at the bottom.
func wldDepthPNG(t testing.TB, w, h int) []byte {
	t.Helper()
	img := image.NewGray16(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetGray16(x, y, color.Gray16{Y: uint16(y * 65535 / (h - 1))})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// wldMapYAML is a map location with the given way of showing an unvisited point and token.
func wldMapYAML(id, unknown, extra string) string {
	return fmt.Sprintf(`  %[1]s:
    name: Map %[1]s
    safe: true
    map:
      picture: pics/map.png
      unknown: %[2]s
      label_size: 0.04
%[3]s
      points:
        home: {at: [40, 40], start: true, label: Home, do: [{set: {flag.home_seen: true}}]}
        inn: {at: [160, 40], to: camp, label: Inn}
        shrine: {at: [160, 100], do: [{set: {flag.prayed: true}}], once: true, label: Shrine}
        gate: {at: [280, 100], when: "flag.gate_open", locked: "The gate is shut.", do: [{set: {flag.passed: true}}], label: Gate}
        cave: {at: [280, 160], label: Cave}
      roads:
        - {between: [home, inn], via: [[100, 60]]}
        - {between: [home, shrine], steps: 3}
        - {between: [shrine, gate]}
        - {between: [gate, cave]}
`, id, unknown, extra)
}

const wldWorldData = `locations:
  fx_maze:
    name: Test Maze
    area: West wing
    encounters: {rate: "0.05", gauge: 0.5, troops: [{troop: one}]}
    dungeon:
      wall: pics/wall.png
      door: pics/door.png
      floor: "#445566"
      ceiling: "#223344"
      floor_texture: pics/floor.png
      ceiling_texture: pics/floor.png
      depth: 5
      darkness: 0.3
      looks:
        - {when: "flag.deep", area: Deep end, wall: pics/wall2.png, floor: "#665544", darkness: 0.8}
      map: |
        ###########
        #S..f..a..#
        #.#########
        #.>..s..p.#
        #.#########
        #.P..W..K.#
        ###########
      tiles:
        S: {start: true, facing: east}
        f: {flat: true, picture: pics/flat.png, name: Rune, hint: Do not step, size: 0.8}
        a: {picture: pics/sprite.png, size: 0.6, name: Statue, solid: "true"}
        ">":
          name: Switch
          once: true
          trigger: step
          mark: "#ffcc00"
          symbol: "!"
          do: [{set: {flag.deep: true}}]
        s: {slide: east, mark: "#ff0000"}
        p: {spin: back}
        P: {look: door, picture: pics/portrait.png, name: Painted door, solid: "true"}
        W: {look: wall, picture: pics/decal.png, size: 0.5, name: Sign}
        K: {look: door, name: Plain door, solid: "true"}
  fx_hunt:
    name: Hunt Hall
    encounters: {gauge: 0.2, rate: "0.1", troops: [{troop: one}]}
    dungeon:
      wall: pics/wall.png
      floor: "#445566"
      ceiling: "#223344"
      generate:
        width: 15
        height: 15
        rooms: 3
        start: S
        place: [{char: E, far: true}]
      roamers:
        - {name: Wisp, mark: "#ff44ff", icon: pics/flat.png, picture: pics/sprite.png, move: hunt, appear: "true", troops: [{troop: one}]}
      tiles:
        S: {start: true, facing: north}
        E: {name: Exit, mark: "#00ff00", symbol: E}
  fx_nomap:
    name: Dark Hall
    dungeon:
      no_map: true
      map: |
        #####
        #S..#
        #####
      tiles:
        S: {start: true, facing: east}
  fx_yard:
    name: Test Yard
    background: pics/bg.png
    safe: true
    stage:
      depth: pics/depth.png
      camera: 30
      depth_bias: 0.01
      scale: [{at: [160, 170], height: 80}, {at: [160, 90], height: 40}]
      walk: [[[20, 175], [300, 175], [300, 90], [20, 90]]]
      blocks: [[[140, 120], [180, 120], [180, 140], [140, 140]]]
      fronts: [{area: [[200, 90], [260, 90], [260, 140], [200, 140]], base: 140}]
      spots:
        start: {at: [60, 160], facing: east}
        well: {at: [250, 160], facing: north, name: Well, do: [{set: {flag.at_well: true}}], once: true}
      exits:
        - {to: camp, area: [[20, 170], [60, 170], [60, 176], [20, 176]]}
      people:
        - {id: elder, at: [200, 150], facing: west, doll: {hair: bald, parts: [{kind: apron, color: "#2f6fb8"}]}}
        - {id: walker, path: [[80, 100], [260, 100]], speed: 1.0}
      things:
        - {id: sign, name: Sign, area: [[100, 100], [120, 100], [120, 115], [100, 115]], do: [{set: {flag.read_sign: true}}]}
        - {id: trap, name: Trap, trigger: step, area: [[60, 100], [90, 100], [90, 115], [60, 115]], do: [{set: {flag.trapped: true}}]}
    people:
      - id: elder
        name: Elder
        image: pics/person.png
        talk: [{set: {flag.met_elder: true}}]
        bubbles: ["Nice day.", {text: "The gate is open.", when: flag.gate_open}]
      - id: walker
        name: Walker
        image: pics/person.png
        talk: [{set: {flag.met_walker: true}}]
        bubbles: ["Hm."]
    actions: [{label: Rest, do: [{heal: all}]}]
    exits: [{to: camp}]
`

// wldWorldFiles is a neutral game with a dungeon, a map and a painted scene made of generated
// pictures. extra replaces or adds files (a name under the game root, e.g. "data/more.yaml").
func wldWorldFiles(t *testing.T, gameYAML string, extra map[string]string) fstest.MapFS {
	t.Helper()
	fsys := fstest.MapFS{}
	base := gameFiles(t)
	for k, v := range base {
		fsys[k] = v
	}
	g := string(base["game.yaml"].Data)
	fsys["game.yaml"] = &fstest.MapFile{Data: []byte("schema_version: 4\n" + gameYAML + g)}
	maps := wldMapYAML("fx_mark", "mark", "      draw_roads: true\n      token: {picture: pics/token.png, height: 0.12}\n      marker: {picture: pics/marker.png, height: 0.08}") +
		wldMapYAML("fx_hidden", "hidden", "      token: doll") +
		wldMapYAML("fx_dim", "dim", "") +
		wldMapYAML("fx_named", "named", "")
	fsys["data/world.yaml"] = &fstest.MapFile{Data: []byte(wldWorldData + maps)}
	pics := map[string][]byte{
		"wall.png":     wldPNG(t, 32, 32, color.NRGBA{R: 100, G: 80, B: 90}),
		"wall2.png":    wldPNG(t, 32, 32, color.NRGBA{R: 60, G: 90, B: 100}),
		"door.png":     wldPNG(t, 32, 32, color.NRGBA{R: 120, G: 80, B: 40}),
		"floor.png":    wldPNG(t, 16, 16, color.NRGBA{R: 90, G: 90, B: 90}),
		"flat.png":     wldPNG(t, 16, 16, color.NRGBA{R: 200, G: 60, B: 60}),
		"square.png":   wldPNG(t, 32, 32, color.NRGBA{R: 60, G: 160, B: 60}),
		"sprite.png":   wldPNG(t, 16, 32, color.NRGBA{R: 160, G: 160, B: 60}),
		"portrait.png": wldPNG(t, 16, 32, color.NRGBA{R: 160, G: 60, B: 160}),
		"decal.png":    wldPNG(t, 32, 16, color.NRGBA{R: 60, G: 160, B: 160}),
		"map.png":      wldPNG(t, 320, 180, color.NRGBA{R: 40, G: 100, B: 60}),
		"bg.png":       wldPNG(t, 320, 180, color.NRGBA{R: 90, G: 110, B: 150}),
		"token.png":    wldPNG(t, 16, 32, color.NRGBA{R: 220, G: 200, B: 40}),
		"marker.png":   wldPNG(t, 16, 16, color.NRGBA{R: 220, G: 40, B: 40}),
		"person.png":   wldPNG(t, 32, 64, color.NRGBA{R: 80, G: 60, B: 50}),
		"depth.png":    wldDepthPNG(t, 320, 180),
	}
	for n, b := range pics {
		fsys["assets/pics/"+n] = &fstest.MapFile{Data: b}
	}
	// the leader has a standing picture, so the maps and scenes can make its doll and face
	mainYAML := strings.Replace(string(base["data/main.yaml"].Data), "scout: {name: Scout, class: ranger,", "scout: {name: Scout, class: ranger, faces: pics/scout, face: normal,", 1)
	if mainYAML == string(base["data/main.yaml"].Data) {
		t.Fatal("the base game no longer has the scout actor this fixture expects")
	}
	fsys["data/main.yaml"] = &fstest.MapFile{Data: []byte(mainYAML)}
	fsys["assets/pics/scout/normal.png"] = &fstest.MapFile{Data: wldPNG(t, 32, 64, color.NRGBA{R: 70, G: 90, B: 60})}
	for n, s := range extra {
		fsys[n] = &fstest.MapFile{Data: []byte(s)}
	}
	return fsys
}

// wldMeters is a meter every location shows.
const wldMeters = "meters:\n  - {label: Debt, value: var.debt, max: 10, color: \"#d98a6a\", when: var.debt > 0}\n"

// wldWorld opens the neutral world game at the location.
func wldWorld(t *testing.T, loc string, opts Options) *Game {
	t.Helper()
	opts.FS = wldWorldFiles(t, wldMeters, nil)
	opts.Location = loc
	return wldOpenWith(t, opts)
}
