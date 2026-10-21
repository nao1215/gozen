package gamedata_test

import (
	"fmt"
	"maps"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

// themeFiles is a complete theme folder of the test game.
func themeFiles(name, colors string) map[string]string {
	m := map[string]string{}
	for _, f := range data.ThemeFiles {
		m["assets/dungeon/themes/"+name+"/"+f] = "x"
	}
	m["assets/dungeon/themes/"+name+"/theme.yaml"] = colors
	return m
}

func themed(dungeon string, extra map[string]string) map[string]string {
	m := map[string]string{"data/z.yaml": "locations:\n  deep:\n    name: Deep\n    dungeon:\n      map: \"###\\n#S#\\n###\"\n      tiles: {S: {start: true}}\n" + dungeon}
	maps.Copy(m, extra)
	return m
}

func TestThemeFillsWhatIsNotWritten(t *testing.T) {
	files := themed("      theme: cave\n      wall: mine.png\n      ceiling: \"#112233\"\n      looks:\n        - {when: \"true\", theme: ice, door: own.png}\n        - {when: \"false\"}\n",
		themeFiles("cave", "floor: \"#445566\"\nceiling: \"#778899\"\n"))
	maps.Copy(files, themeFiles("ice", "floor: \"#aabbcc\"\nceiling: \"#ddeeff\"\n"))
	files["assets/mine.png"], files["assets/own.png"] = "x", "x"
	d := testgame.Load(t, files).Locations["deep"].Dungeon

	if d.Wall.File != "mine.png" {
		t.Errorf("a written wall must win, got %q", d.Wall.File)
	}
	if d.Door.File != "dungeon/themes/cave/door.jpg" || d.FloorTex.File != "dungeon/themes/cave/floor.jpg" || d.CeilingTex.File != "dungeon/themes/cave/ceiling.jpg" {
		t.Errorf("the theme must fill door and textures: %q %q %q", d.Door.File, d.FloorTex.File, d.CeilingTex.File)
	}
	if got := fmt.Sprintf("%02x%02x%02x", d.Floor.R, d.Floor.G, d.Floor.B); got != "445566" || !d.Floor.Valid {
		t.Errorf("floor color from the theme, got %s", got)
	}
	if got := fmt.Sprintf("%02x%02x%02x", d.Ceiling.R, d.Ceiling.G, d.Ceiling.B); got != "112233" {
		t.Errorf("a written ceiling color must win, got %s", got)
	}
	lk := d.Looks[0]
	if lk.Wall.File != "dungeon/themes/ice/wall.jpg" || lk.Door.File != "own.png" || lk.Floor.R != 0xaa {
		t.Errorf("a look's theme fills what the look does not write: %+v", lk)
	}
	if l := d.Looks[1]; l.Wall.File != "" || l.Floor.Valid {
		t.Errorf("a look without a theme stays empty: %+v", l)
	}
}

func TestThemeIsReportedByCheck(t *testing.T) {
	tests := []struct {
		name, dungeon string
		extra         map[string]string
		want          string
	}{
		{"no folder", "      theme: cave\n", nil, "run `gozen theme add cave <game-dir>`"},
		{"no folder for a look", "      looks: [{when: \"true\", theme: ice}]\n", nil, "looks[0].theme: no assets/dungeon/themes/ice/wall.jpg"},
		{"a file is missing", "      theme: cave\n", map[string]string{"assets/dungeon/themes/cave/wall.jpg": "x"}, "no assets/dungeon/themes/cave/floor.jpg"},
		{"bad colors", "      theme: cave\n", themeFiles("cave", "floor: nope\n"), "theme.yaml cannot be read"},
		{"unknown key", "      theme: cave\n", themeFiles("cave", "walls: \"#000000\"\n"), "theme.yaml cannot be read"},
		{"bad name", "      theme: ../x\n", nil, "is not a theme name"},
		{"upper case", "      theme: Cave\n", nil, "is not a theme name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := testgame.TryLoad(themed(tt.dungeon, tt.extra))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("want an error with %q, got %v", tt.want, err)
			}
		})
	}
	_, err := testgame.TryLoad(themed("      theme: cave\n", nil))
	if err == nil || !strings.Contains(err.Error(), "GZN1111") {
		t.Errorf("a missing theme must carry GZN1111, got %v", err)
	}
}

func TestThemeName(t *testing.T) {
	for s, want := range map[string]bool{"cave": true, "deep-sea_2": true, "": false, "2cave": false, "Cave": false, "a/b": false, "a.b": false} {
		if got := data.ThemeName(s); got != want {
			t.Errorf("ThemeName(%q) = %v", s, got)
		}
	}
}
