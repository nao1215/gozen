package gamedata_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

// lineOf is the line (from 1) of the first line of src holding s.
func lineOf(src, s string) int {
	i := strings.Index(src, s)
	if i < 0 {
		return 0
	}
	return strings.Count(src[:i], "\n") + 1
}

func scenesWith(old, new string) (string, map[string]string) {
	src := strings.Replace(testgame.ScenesData, old, new, 1)
	return src, testgame.ScenesFiles(map[string]string{"data/town.yaml": src})
}

func TestScenesLoad(t *testing.T) {
	db := testgame.LoadScenes(t, nil)
	s := db.Locations["plaza"].Stage
	if got := strings.Join(s.SpotIDs(), ","); got != "gate,north,bench" {
		t.Fatalf("spots in written order: %s", got)
	}
	if s.StartSpot() != "gate" {
		t.Fatalf("start spot %s", s.StartSpot())
	}
	if got := strings.Join(db.Locations["town"].Map.PointIDs(), ","); got != "square,shop,fork,sign,gate,out" {
		t.Fatalf("points in written order: %s", got)
	}
	if p := db.Locations["plaza"].People[0]; len(p.Bubbles) != 2 || p.Bubbles[0].Text != "Fine day." || !p.Bubbles[1].When.Set() {
		t.Fatalf("bubbles %+v", p.Bubbles)
	}
	if w := db.SceneWarnings(); len(w) != 0 {
		t.Fatalf("warnings for the town: %v", w)
	}
}

// TestSceneErrors reports each mistake of a map and a stage at its file, line and path.
func TestSceneErrors(t *testing.T) {
	for _, c := range []struct {
		old, new, line, path, msg string
	}{
		{"gate: {at: [48, 50], facing: north}", "gate: {at: [48, 22], facing: north}", "gate: {at: [48", "stage.spots.gate.at", "not where one can walk"},
		{"gate: {at: [48, 50], facing: north}", "gate: {at: [48, 50], facing: up-ish}", "gate: {at: [48", "stage.spots.gate.facing", "want north, east, south or west"},
		{"north: {at: [48, 12], facing: south}", "north: {at: [48, 12], facing: south, start: true}\n        far: {at: [20, 12], start: true}", "spots:", "stage.spots", "2 spots have start"},
		{"{at: [48, 50], height: 12}, {at: [48, 10], height: 5}", "{at: [48, 50], height: 12}, {at: [30, 50], height: 5}", "scale:", "stage.scale", "different heights"},
		{"{at: [48, 50], height: 12}, {at: [48, 10], height: 5}", "{at: [48, 50], height: 12}", "scale:", "stage.scale", "two marks or more"},
		{"walk: [[[4, 8], [92, 8], [92, 54], [4, 54]]]", "walk: [[[4, 8], [92, 8], [92, 54], [4, 54], [200, 54]]]", "walk:", "stage.walk[0][4]", "outside the picture (96x54)"},
		{"blocks: [[[40, 20], [56, 20], [56, 26], [40, 26]]]", "blocks: [[[40, 20], [56, 20]]]", "blocks:", "stage.blocks[0]", "three points or more"},
		{"- {to: shop, area: [[44, 8]", "- {to: inn, area: [[44, 8]", "- {to: inn", "stage.exits[0].to", `"inn" is not the to of one of the location's exits`},
		{"- {id: keeper, at: [30, 40]", "- {id: smith, at: [30, 40]", "- {id: smith", "stage.people[0].id", `"smith" is not the id of one of the location's people`},
		{"- {id: walker, path: [[60, 30], [80, 30], [80, 45]], speed: 0.9}", "- {id: walker, at: [60, 30], path: [[60, 30], [80, 30]]}", "- {id: walker, at", "stage.people[1]", "one of them"},
		{`parts: [{kind: apron, color: "#2f6fb8"}]`, `parts: [{kind: wings, color: "blue"}]`, "parts: [{kind: wings", "stage.people[0].doll.parts[0].kind", `"wings" is not a part`},
		{`parts: [{kind: apron, color: "#2f6fb8"}]`, `parts: [{kind: wings, color: "blue"}]`, "parts: [{kind: wings", "stage.people[0].doll.parts[0].color", "want #rrggbb"},
		{"- {action: rest, area:", "- {action: nap, area:", "- {action: nap", "stage.things[1].action", `"nap" is not the id of one of the location's actions`},
		{"depth: bg/plaza_depth.png", "depth: bg/missing.png", "depth: bg/missing", "stage.depth", "no file assets/bg/missing.png"},
		{"to: plaza}", "to: piazza}", "square: {at", "map.points.square.to", `unknown location "piazza"`},
		{"square: {at: [32, 30]", "square: {at: [320, 30]", "square: {at", "map.points.square.at", "outside the picture (64x36)"},
		{`gate: {at: [60, 10], to: gate, when: "item.key > 0", locked: "The gate is shut."}`, `gate: {at: [60, 10], to: gate, locked: "The gate is shut."}`, "gate: {at: [60, 10]", "map.points.gate.locked", "it needs when"},
		{"- {between: [fork, gate], steps: 2}", "- {between: [fork, gates], steps: 2}", "- {between: [fork, gates]", "map.roads[3].between[1]", `unknown point "gates"`},
		{"- {between: [square, out]}", "- {between: [square, out]}\n        - {between: [out, square]}", "- {between: [out, square]}", "map.roads[5].between", "a second road between out and square"},
		{"- {between: [square, out]}", "- {between: [square, fork]}", "out: {at", "map.points.out", "no road leads to the point"},
		{"troops: [{troop: one}]}}", "troops: [{troop: nine}]}}", "- {between: [town, ruins]", "map.roads[0].encounters", `unknown troop "nine"`},
		{"    exits: [{to: town, label: Back to the street}]", "    exits: [{to: town, label: Back to the street, at: nowhere}]", "exits: [{to: town, label: Back to the street, at", "shop.exits[0].at", `town has no point "nowhere"`},
		{"      - {to: shop, label: Shop}", "      - {to: shop, label: Shop, at: gate}", "- {to: shop, label: Shop, at", "plaza.exits[0].at", "shop is not a dungeon, a map or a stage"},
		{"  ruins:\n    name: Ruins\n", "  ruins:\n    name: Ruins\n    stage: {walk: []}\n    map: {picture: maps/world.png, points: {a: {at: [1, 1]}}}\n", "  ruins:\n    name: Ruins", "locations.ruins", "shown one way"},
		{"bubbles: [\"Fine day.\"", "bubbles: [\"\"", "bubbles: [\"\"", "people[0].bubbles[0]", "a balloon needs text"},
	} {
		t.Run(c.path+" "+c.msg, func(t *testing.T) {
			src, files := scenesWith(c.old, c.new)
			if src == testgame.ScenesData {
				t.Fatalf("the case does not change the data: %q", c.old)
			}
			_, err := testgame.TryLoad(files)
			if err == nil {
				t.Fatal("loaded")
			}
			at := fmt.Sprintf("data/town.yaml:%d:", lineOf(src, c.line))
			for _, line := range strings.Split(err.Error(), "\n") {
				if strings.Contains(line, c.path) && strings.Contains(line, c.msg) {
					if !strings.HasPrefix(line, at) {
						t.Errorf("at %s, want %s", line, at)
					}
					return
				}
			}
			t.Errorf("no %s: %s in\n%v", c.path, c.msg, err)
		})
	}
}

// TestSceneWarnings are the problems that do not stop the game: a point no road from the
// start reaches, an exit no one can walk into, a polygon crossing itself.
func TestSceneWarnings(t *testing.T) {
	for _, c := range []struct{ old, new, path, msg string }{
		{"- {between: [fork, gate], steps: 2}", "- {between: [sign, gate], steps: 2}\n        - {between: [shop, fork]}", "", ""},
		{"- {to: town, area: [[4, 52], [92, 52], [92, 54], [4, 54]]}", "- {to: town, area: [[93, 52], [95, 52], [95, 54], [93, 54]]}", "stage.exits[1].area", "does not touch walk"},
		{"blocks: [[[40, 20], [56, 20], [56, 26], [40, 26]]]", "blocks: [[[40, 20], [56, 26], [56, 20], [40, 26]]]", "stage.blocks[0]", "crosses itself"},
	} {
		_, files := scenesWith(c.old, c.new)
		db, err := testgame.TryLoad(files)
		if err != nil {
			t.Fatal(err)
		}
		w := errors.Join(db.SceneWarnings()...)
		if c.path == "" {
			if w != nil {
				t.Errorf("warned %v", w)
			}
			continue
		}
		if w == nil || !strings.Contains(w.Error(), c.path) || !strings.Contains(w.Error(), c.msg) || !strings.Contains(w.Error(), "data/town.yaml:") {
			t.Errorf("want %s: %s, got %v", c.path, c.msg, w)
		}
	}
	// a point cut off from the start (its only road is to another cut-off point)
	src := strings.Replace(testgame.ScenesData, "- {between: [square, out]}", "- {between: [square, out]}\n        - {between: [gate, out]}", 1)
	src = strings.Replace(src, "- {between: [fork, gate], steps: 2}", "", 1)
	src = strings.Replace(src, "- {between: [square, out]}", "", 1)
	db, err := testgame.TryLoad(testgame.ScenesFiles(map[string]string{"data/town.yaml": src}))
	if err != nil {
		t.Fatal(err)
	}
	w := errors.Join(db.SceneWarnings()...)
	if w == nil || !strings.Contains(w.Error(), "map.points.gate") || !strings.Contains(w.Error(), "cannot be reached from") {
		t.Errorf("a cut-off point: %v", w)
	}
}

// TestBalloonSize: game.yaml ui.balloon is a fraction of the screen's height, at version 3.
func TestBalloonSize(t *testing.T) {
	for _, c := range []struct{ head, want string }{
		{"schema_version: 3\nid: b\nui: {balloon: 0.05}\n", ""},
		{"schema_version: 3\nid: b\nui: {balloon: 0.5}\n", "ui.balloon: want a fraction"},
		{"schema_version: 2\nid: b\nui: {balloon: 0.05}\n", "ui.balloon needs schema_version: 3"},
	} {
		_, err := testgame.TryLoad(map[string]string{"game.yaml": c.head + testgame.Game})
		if c.want == "" && err != nil || c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%q: %v", c.head, err)
		}
	}
}

// TestFaceWarnings warns of the scenes' pictures without a usable face box.
func TestFaceWarnings(t *testing.T) {
	for _, c := range []struct{ faces, want string }{
		{"", "no such file"},
		{`{"version": 1, "pictures": {}}`, "x.png: no face box"},
		{`{"version": 1, "pictures": {"x.png": {"box": [1, 1, 5, 5], "sha256": "00"}}}`, "the picture changed"},
		{`{"version": 1, "pictures": {"x.png": {"sha256": "` + strings.Split(strings.Split(testgame.ScenesFaces, `"sha256": "`)[1], `"`)[0] + `"}}}`, "no face was found"},
	} {
		files := testgame.ScenesFiles(nil)
		if c.faces == "" {
			delete(files, "assets/faces.json")
		} else {
			files["assets/faces.json"] = c.faces
		}
		db, err := testgame.TryLoad(files)
		if err != nil {
			t.Fatal(err)
		}
		w := errors.Join(db.SceneWarnings()...)
		if w == nil || !strings.Contains(w.Error(), c.want) {
			t.Errorf("%s: want %q, got %v", c.faces, c.want, w)
		}
	}
}

// TestDollIcon: an icon must exist and should be square; a person with one is not searched
// for a face.
func TestDollIcon(t *testing.T) {
	_, err := testgame.TryLoad(testgame.ScenesFiles(map[string]string{"data/town.yaml": strings.Replace(testgame.ScenesData, "doll: {hair: bun,", "doll: {icon: icons/none.png, hair: bun,", 1)}))
	if err == nil || !strings.Contains(err.Error(), "doll.icon: no file assets/icons/none.png") {
		t.Fatalf("a missing icon: %v", err)
	}
	src := strings.Replace(testgame.ScenesData, "doll: {hair: bun,", "doll: {icon: bg/plaza.png, hair: bun,", 1)
	src = strings.Replace(src, "speed: 0.9}", "speed: 0.9, doll: {icon: bg/plaza.png}}", 1)
	files := testgame.ScenesFiles(map[string]string{"data/town.yaml": src})
	delete(files, "assets/faces.json")
	db, err := testgame.TryLoad(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(db.ScenePictures()) != 0 {
		t.Fatalf("the keeper and the walker have icons, yet %v", db.ScenePictures())
	}
	w := errors.Join(db.SceneWarnings()...)
	if w == nil || !strings.Contains(w.Error(), "96x54, not square") || strings.Contains(w.Error(), "faces.json") {
		t.Fatalf("warnings %v", w)
	}
}

// TestSceneNeedsVersion3 rejects the new words at schema_version 2.
func TestSceneNeedsVersion3(t *testing.T) {
	files := testgame.ScenesFiles(map[string]string{"game.yaml": strings.Replace(testgame.ScenesGame, "schema_version: 3", "schema_version: 2", 1)})
	_, err := testgame.TryLoad(files)
	if err == nil {
		t.Fatal("loaded")
	}
	for _, want := range []string{"map needs schema_version: 3", "stage needs schema_version: 3", "bubbles needs schema_version: 3", "doll needs schema_version: 3"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("no %q in %v", want, err)
		}
	}
	_, err = testgame.TryLoad(map[string]string{"game.yaml": "schema_version: 2\nid: v2\n" + testgame.Game,
		"data/x.yaml": "events: {look: {do: [{if: \"spot == 'a'\", then: [{say: x}]}]}}"})
	if err == nil || !strings.Contains(err.Error(), "the name spot needs schema_version: 3") {
		t.Fatalf("spot at version 2: %v", err)
	}
	var d data.Diagnostics
	if !errors.As(err, &d) && err == nil {
		t.Fatal("not diagnostics")
	}
}

// TestSceneReviewChecks: a thing with action takes no when, trigger or ask; an exit has one
// area; a spot's name without do and a person without an image are warned of.
func TestSceneReviewChecks(t *testing.T) {
	for _, c := range []struct{ old, new, want string }{
		{"- {action: rest, area:", "- {action: rest, trigger: step, area:", "no id, name, do, once, when, trigger or ask"},
		{"- {to: shop, area: [[44, 8]", "- {to: town, area: [[44, 8]", "a second area for the exit to town"},
	} {
		_, files := scenesWith(c.old, c.new)
		if _, err := testgame.TryLoad(files); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", c.want, err)
		}
	}
	for _, c := range []struct{ old, new, want string }{
		{"north: {at: [48, 12], facing: south}", "north: {at: [48, 12], facing: south, name: North}", "a spot without do is no entry"},
		{"name: Walker, image: x.png,", "name: Walker,", "walker has no image"},
	} {
		_, files := scenesWith(c.old, c.new)
		db, err := testgame.TryLoad(files)
		if err != nil {
			t.Fatal(err)
		}
		if w := errors.Join(db.SceneWarnings()...); w == nil || !strings.Contains(w.Error(), c.want) {
			t.Errorf("%s: %v", c.want, w)
		}
	}
}

// TestMapUnknown: unknown is hidden, dim, mark or named.
func TestMapUnknown(t *testing.T) {
	_, files := scenesWith("      picture: maps/town.png\n", "      picture: maps/town.png\n      unknown: named\n")
	if _, err := testgame.TryLoad(files); err != nil {
		t.Fatalf("named: %v", err)
	}
	_, files = scenesWith("      picture: maps/town.png\n", "      picture: maps/town.png\n      label_size: 0.5\n")
	if _, err := testgame.TryLoad(files); err == nil || !strings.Contains(err.Error(), "label_size") {
		t.Fatalf("label_size: %v", err)
	}
	_, files = scenesWith("      picture: maps/town.png\n", "      picture: maps/town.png\n      token: doll\n")
	if g, err := testgame.TryLoad(files); err != nil || g.Locations["town"].Map.Token == nil || !g.Locations["town"].Map.Token.Doll {
		t.Fatalf("token: doll: %v", err)
	}
	_, files = scenesWith("      picture: maps/town.png\n", "      picture: maps/town.png\n      token: face\n")
	if _, err := testgame.TryLoad(files); err == nil || !strings.Contains(err.Error(), "doll") {
		t.Fatalf("token: face: %v", err)
	}
	_, files = scenesWith("      picture: maps/town.png\n", "      picture: maps/town.png\n      unknown: shown\n")
	if _, err := testgame.TryLoad(files); err == nil || !strings.Contains(err.Error(), "want hidden, dim, mark or named") {
		t.Fatalf("shown: %v", err)
	}
}
