package gamedata_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

// drawn is the made dungeon with a hand-drawn floor 2 (rows is the map as written, one string a row).
func drawn(rows []string, extra string) map[string]string {
	var b strings.Builder
	b.WriteString("rooms: 2\n        floors:\n          2:\n")
	b.WriteString(extra)
	b.WriteString("            map: |\n")
	for _, r := range rows {
		b.WriteString("              " + r + "\n")
	}
	return map[string]string{"data/z.yaml": strings.Replace(madeDungeon, "rooms: 2", strings.TrimRight(b.String(), "\n"), 1)}
}

func TestDrawnFloors(t *testing.T) {
	good := []string{
		"#######",
		"#S...>#",
		"#.....#",
		"#######",
	}
	if _, err := testgame.TryLoad(drawn(good, "")); err != nil {
		t.Fatalf("a sound drawn floor: %v", err)
	}
	cases := []struct {
		name  string
		rows  []string
		extra string
		want  string
	}{
		{"rows of unequal width", []string{"#######", "#S...>##", "#######"}, "", "the rows are of one width"},
		{"an open edge", []string{"#######", "S....>#", "#######"}, "", "the edge of a drawn floor is wall"},
		{"an open bottom edge", []string{"#######", "#S...>#", "#.....#"}, "", "the edge of a drawn floor is wall"},
		{"a character that is no tile", []string{"#######", "#S.Z.>#", "#######"}, "", `"Z" is neither # . D nor a tile`},
		{"no start", []string{"#######", "#....>#", "#######"}, "", "the start"},
		{"two starts", []string{"#######", "#S..S>#", "#######"}, "", "drawn once"},
		{"a seed with a map", good, "            seed: 5\n", "needs no seed"},
		{"a floor over the map limit", []string{strings.Repeat("#", 256)}, "", "map width and height must be at most 255 cells"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := testgame.TryLoad(drawn(c.rows, c.extra))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want an error with %q, got %v", c.want, err)
			}
		})
	}
}

func TestPlaceNearAndTileChains(t *testing.T) {
	tiles := func(extra string, place string) map[string]string {
		y := strings.Replace(madeDungeon, "S: {name: Up}", "S: {name: Up}\n        T: {name: Chest"+extra+"}\n        X: {name: Box}", 1)
		y = strings.Replace(y, `place: [{char: ">", far: true}]`, "place: [{char: \">\", far: true}"+place+"]", 1)
		return map[string]string{"data/z.yaml": y}
	}
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"near a tile that is not one", tiles("", `, {char: T, near: Q}`), "is not a tile character"},
		{"near a tile placed later", tiles("", `, {char: T, near: X}, {char: X}`), "near"},
		{"near with far", tiles("", `, {char: X}, {char: T, near: X, far: true}`), "near goes alone"},
		{"near with steps", tiles("", `, {char: X}, {char: T, near: X, steps: 2}`), "near goes alone"},
		{"near with a wall", tiles("", `, {char: X}, {char: T, near: X, wall: true}`), "near goes alone"},
		{"else chain that loops", tiles(", else: X", `, {char: T}`), "else"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.name == "else chain that loops" {
				c.files["data/z.yaml"] = strings.Replace(c.files["data/z.yaml"], "X: {name: Box}", "X: {name: Box, else: T}", 1)
			}
			_, err := testgame.TryLoad(c.files)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want an error with %q, got %v", c.want, err)
			}
		})
	}
	// a near placed after the tile it names is fine
	if _, err := testgame.TryLoad(tiles("", `, {char: X}, {char: T, near: X}`)); err != nil {
		t.Errorf("near an earlier tile: %v", err)
	}
}
