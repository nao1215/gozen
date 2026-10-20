package testgame

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"maps"
	"strings"
	"testing"
	"testing/fstest"

	data "github.com/nao1215/gozen/schema"
)

// ScenesGame is the expedition at schema_version 3, starting on a map of the plain.
var ScenesGame = "schema_version: 3\nid: scenes-test\n" + strings.Replace(Game, "location: camp", "location: world", 1)

// ScenesData is a small town of maps and a stage: a map of the plain leading into the
// town's map, whose points lead into a painted plaza (a stage), a shop and a gate.
const ScenesData = `
locations:
  world:
    name: The Plain
    map:
      picture: maps/world.png
      points:
        town: {at: [20, 20], to: town, start: true}
        ruins: {at: [50, 10], to: ruins, when: "flag.heard", locked: "The road is washed out."}
      roads:
        - {between: [town, ruins], steps: 4, encounters: {rate: "1", troops: [{troop: one}]}}
  ruins:
    name: Ruins
    exits: [{to: world, label: Back}]
  town:
    name: Town
    map:
      picture: maps/town.png
      points:
        square: {at: [32, 30], to: plaza}
        shop: {at: [10, 10], to: shop}
        fork: {at: [32, 15]}
        sign: {at: [40, 12], label: Signpost, do: [{say: "A signpost."}, {set: {flag.read_sign: true}}], once: true}
        gate: {at: [60, 10], to: gate, when: "item.key > 0", locked: "The gate is shut."}
        out: {at: [2, 34], to: world, label: Leave town}
      roads:
        - {between: [square, shop], via: [[20, 20]], steps: 2}
        - {between: [square, fork]}
        - {between: [fork, sign]}
        - {between: [fork, gate], steps: 2}
        - {between: [square, out]}
  plaza:
    name: Plaza
    background: bg/plaza.png
    stage:
      depth: bg/plaza_depth.png
      scale: [{at: [48, 50], height: 12}, {at: [48, 10], height: 5}]
      walk: [[[4, 8], [92, 8], [92, 54], [4, 54]]]
      blocks: [[[40, 20], [56, 20], [56, 26], [40, 26]]]
      fronts: [{area: [[42, 4], [54, 4], [54, 26], [42, 26]], base: 25}]
      spots:
        gate: {at: [48, 50], facing: north}
        north: {at: [48, 12], facing: south}
        bench: {at: [80, 40], name: Sit on the bench, do: [{say: "A warm bench."}], once: true}
      exits:
        - {to: shop, area: [[44, 8], [52, 8], [52, 10], [44, 10]]}
        - {to: town, area: [[4, 52], [92, 52], [92, 54], [4, 54]]}
      people:
        - {id: keeper, at: [30, 40], facing: east, doll: {hair: bun, parts: [{kind: apron, color: "#2f6fb8"}]}}
        - {id: walker, path: [[60, 30], [80, 30], [80, 45]], speed: 0.9}
      things:
        - {id: board, name: Notice board, area: [[10, 30], [16, 30], [16, 36], [10, 36]], do: [{say: "Water, five the bucket."}], once: true}
        - {action: rest, area: [[70, 12], [80, 12], [80, 18], [70, 18]]}
    people:
      - {id: keeper, name: Keeper, image: x.png, talk: [{say: "Welcome."}], bubbles: ["Fine day.", {text: "Have you heard?", when: "!flag.heard"}]}
      - {id: walker, name: Walker, image: x.png, talk: [{say: "Busy, busy."}, {set: {flag.heard: true}}]}
    actions:
      - {id: rest, label: Rest, do: [{heal: all}]}
    exits:
      - {to: shop, label: Shop}
      - {to: town, label: Back to town}
  shop:
    name: Shop
    exits: [{to: town, label: Back to the street}]
  gate:
    name: Gate
    exits: [{to: town, label: Back}]
`

// ScenesFaces is the face boxes of the town's pictures (as gozen art faces writes them): the
// keeper's picture is the expedition's file sentinel.
var ScenesFaces = `{"version": 1, "model": "test", "pictures": {"x.png": {"box": [2, 3, 12, 15], "size": [40, 100], "sha256": "` + sentinelSHA() + `"}}}`

func sentinelSHA() string {
	sum := sha256.Sum256([]byte(sentinel))
	return hex.EncodeToString(sum[:])
}

// Picture is a PNG of w x h pixels, a soft gradient (a stand-in background or map).
func Picture(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, color.RGBA{uint8(40 + 160*x/max(1, w)), uint8(60 + 120*y/max(1, h)), 140, 255}) //nolint:gosec // in 0..255
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		panic(err)
	}
	return b.Bytes()
}

// Depth is a 16-bit gray PNG of w x h pixels: far at the top, near at the bottom.
func Depth(w, h int) []byte {
	img := image.NewGray16(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetGray16(x, y, color.Gray16{Y: uint16(65535 * y / max(1, h-1))}) //nolint:gosec // in 0..65535
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		panic(err)
	}
	return b.Bytes()
}

// ScenesFiles are the files of the scenes game beside the expedition's data: game.yaml,
// the town and its pictures. extra replaces or adds files.
func ScenesFiles(extra map[string]string) map[string]string {
	files := map[string]string{
		"game.yaml":                 ScenesGame,
		"data/town.yaml":            ScenesData,
		"assets/maps/world.png":     string(Picture(64, 36)),
		"assets/maps/town.png":      string(Picture(64, 36)),
		"assets/bg/plaza.png":       string(Picture(96, 54)),
		"assets/bg/plaza_depth.png": string(Depth(96, 54)),
		"assets/faces.json":         ScenesFaces,
	}
	maps.Copy(files, extra)
	return files
}

// ScenesFS is the scenes game as a file system (see ScenesFiles).
func ScenesFS(extra map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{"data/main.yaml": {Data: []byte(Data)}, "assets/x.png": {Data: []byte(sentinel)}}
	for n, s := range ScenesFiles(extra) {
		fsys[n] = &fstest.MapFile{Data: []byte(s)}
	}
	return fsys
}

// LoadScenes loads the scenes game (see ScenesFiles).
func LoadScenes(t testing.TB, extra map[string]string) *data.DB {
	t.Helper()
	return Load(t, ScenesFiles(extra))
}
