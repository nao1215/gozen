package gamedata_test

import (
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestMoreSceneMistakes: each mistake in a painted scene is reported with a plain message.
func TestMoreSceneMistakes(t *testing.T) {
	for _, c := range []struct{ old, new, msg string }{
		{"doll: {hair: bun,", "doll: {hair: spiky,", "want one of"},
		{"doll: {hair: bun,", "doll: {hair: bun, face_at: [0.5, 0.2],", "face_at is [middle, top, width]"},
		{"doll: {hair: bun,", "doll: {hair: bun, face_at: [0.5, 0.2, 3],", "face_at"},
		{"depth: bg/plaza_depth.png", "depth: bg/plaza_depth.png\n      camera: 120", "want degrees in [0, 90)"},
		{"depth: bg/plaza_depth.png", "depth: bg/plaza_depth.png\n      depth_bias: 1.5", "want a fraction in [0, 1)"},
		{"scale: [{at: [48, 50], height: 12}", "scale: [{at: [48, 50], height: 0}", "want a height above 0"},
		{"base: 25}", "base: 0}", "base is the ground's y"},
		{"base: 25}", "base: 999}", "base is the ground's y"},
		{"bench: {at: [80, 40], name: Sit on the bench, do: [{say: \"A warm bench.\"}], once: true}", "bench: {at: [80, 40], once: true}", "once is for a spot with do"},
		{"        - {id: walker, path: [[60, 30], [80, 30], [80, 45]], speed: 0.9}", "        - {id: walker, path: [[60, 30]], speed: 0.9}", "a path needs two points or more"},
		{"        - {id: walker, path: [[60, 30], [80, 30], [80, 45]], speed: 0.9}", "        - {id: walker, path: [[60, 30], [80, 30]], speed: -1}", "want a speed of 0 or more"},
		{"        - {id: walker, path: [[60, 30], [80, 30], [80, 45]], speed: 0.9}", "        - {id: keeper, at: [60, 30]}", "placed twice"},
		{"        - {id: walker, path: [[60, 30], [80, 30], [80, 45]], speed: 0.9}", "        - {at: [60, 30]}", "id is missing"},
		{"        - {id: board, name: Notice board,", "        - {id: board, name: Notice board, trigger: poke,", "want examine or step"},
		{"        - {id: board, name: Notice board, area: [[10, 30], [16, 30], [16, 36], [10, 36]], do: [{say: \"Water, five the bucket.\"}], once: true}", "        - {name: Notice board, area: [[10, 30], [16, 30], [16, 36], [10, 36]], do: [{say: hi}]}", "a thing needs an id"},
		{"        - {id: board, name: Notice board, area: [[10, 30], [16, 30], [16, 36], [10, 36]], do: [{say: \"Water, five the bucket.\"}], once: true}", "        - {id: board, area: [[10, 30], [16, 30], [16, 36], [10, 36]], do: [{say: hi}]}", "a thing needs a name"},
		{"        - {id: board, name: Notice board, area: [[10, 30], [16, 30], [16, 36], [10, 36]], do: [{say: \"Water, five the bucket.\"}], once: true}", "        - {id: board, name: Notice board, area: [[10, 30], [16, 30], [16, 36], [10, 36]]}", "a thing needs do"},
		{"        - {id: board, name: Notice board, area: [[10, 30], [16, 30], [16, 36], [10, 36]], do: [{say: \"Water, five the bucket.\"}], once: true}", "        - {id: board, name: B, area: [[10, 30], [16, 30], [16, 36], [10, 36]], do: [{say: hi}]}\n        - {id: board, name: C, area: [[20, 30], [26, 30], [26, 36], [20, 36]], do: [{say: hi}]}", "duplicate thing id"},
	} {
		src, files := scenesWith(c.old, c.new)
		if src == testgame.ScenesData {
			t.Fatalf("%q was not found in the fixture", c.old)
		}
		_, err := data.Load(testgame.ScenesFS(files))
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s: want an error with %q, got %v", c.new, c.msg, err)
		}
	}
}

const staticDungeon = `
locations:
  crypt:
    name: Crypt
    dungeon:
      map: |
        #####
        #S.>#
        #####
      tiles:
        S: {start: true}
        ">": {name: Stairs, solid: "true", do: [{say: down}]}
`

// TestDungeonMistakes: mistakes in drawn and made dungeons, tiles, looks and roamers.
func TestDungeonMistakes(t *testing.T) {
	made := func(old, repl string) map[string]string {
		return map[string]string{"data/z.yaml": strings.Replace(madeDungeon, old, repl, 1)}
	}
	drawn := func(old, repl string) map[string]string {
		return map[string]string{"data/z.yaml": strings.Replace(staticDungeon, old, repl, 1)}
	}
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"empty map", drawn("#####\n        #S.>#\n        #####", "   "), "the map is empty"},
		{"tile of two characters", drawn(">\": {name: Stairs", ">>\": {name: Stairs"), "a tile is one character"},
		{"tile that is built in", drawn("S: {start: true}", "S: {start: true}\n        '#': {name: Wall}"), "a tile is one character other than"},
		{"tile not used", drawn("S: {start: true}", "S: {start: true}\n        Z: {name: Ghost}"), "Z"},
		{"generate with a map", made("      generate:", "      map: |\n        ###\n      generate:"), "a dungeon with generate has no map"},
		{"generate too small", made("width: 11", "width: 5"), "width and height are 9 or more"},
		{"place of an unknown tile", made(`place: [{char: ">", far: true}]`, `place: [{char: "Q", far: true}]`), `"Q" is not a tile`},
		{"negative steps", made("far: true}", "steps: -2}"), "want 0 or more"},
		{"near an unplaced tile", made("far: true}", "near: S}"), "near"},
		{"two ways to place", made("far: true}", "far: true, steps: 3}"), "give one"},
		{"floor zero", made("rooms: 2", "rooms: 2\n        floors: {0: {seed: 1}}"), "a floor is 1 or more"},
		{"floor without seed or map", made("rooms: 2", "rooms: 2\n        floors: {2: {}}"), "give seed or map"},
		{"tile ask without a look trigger", drawn("S: {start: true}", "S: {start: true, ask: Open it}"), "ask is the answer of a trigger look tile"},
		{"after without once", drawn("S: {start: true}", "S: {start: true, after: S}"), "after is for a tile with once"},
		{"flat tile too big", drawn("S: {start: true}", "S: {start: true, flat: true, size: 2, picture: x.png}"), "a cell at most"},
		{"look without when", drawn("      tiles:", "      looks: [{darkness: 0.2}]\n      tiles:"), "a look needs when"},
		{"look with a bad song", drawn("      tiles:", "      looks: [{when: 'true', music: {song: nope}}]\n      tiles:"), `unknown song "nope"`},
		{"swap from nothing", drawn("      tiles:", "      pictures: {'': x.png}\n      tiles:"), "a swap is from a picture to a picture"},
		{"roamers on a drawn floor", drawn("      tiles:", "      roamers: [{picture: x.png, troops: [{troop: one}]}]\n      tiles:"), "roamers walk made floors only"},
		{"roamer with count and appear", made("      generate:", "      roamers: [{picture: x.png, count: '2', appear: 'true', troops: [{troop: one}]}]\n      generate:"), "no count"},
		{"roamer without a picture", made("      generate:", "      roamers: [{troops: [{troop: one}]}]\n      generate:"), "needs a picture or an icon"},
		{"roamer without a troop", made("      generate:", "      roamers: [{picture: x.png}]\n      generate:"), "needs a troop"},
		{"roamer that moves strangely", made("      generate:", "      roamers: [{picture: x.png, move: fly, troops: [{troop: one}]}]\n      generate:"), "want chase, wander, guard or hunt"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := testgame.TryLoad(c.files)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want an error with %q, got %v", c.want, err)
			}
		})
	}
	if _, err := testgame.TryLoad(map[string]string{"data/z.yaml": staticDungeon}); err != nil {
		t.Fatalf("the plain dungeon is sound: %v", err)
	}
}

// TestArrivalMistakes: moving into a place at a mark or a spot that is not there is reported.
func TestArrivalMistakes(t *testing.T) {
	for name, c := range map[string]struct{ y, want string }{
		"a facing that is not a direction":  {`events: {e: {do: [{move: crypt, at: S, facing: diagonal}]}}` + staticDungeon, "want north, east, south or west"},
		"a mark that is not in the dungeon": {`events: {e: {do: [{move: crypt, at: Z}]}}` + staticDungeon, "at"},
		"a spot of a place with no stage":   {`events: {e: {do: [{move: camp, at: somewhere}]}}`, "at"},
	} {
		_, err := testgame.TryLoad(map[string]string{"data/z.yaml": c.y})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
