package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestThemeCommands(t *testing.T) {
	out, _ := invoke(t, 0, "theme", "list")
	for _, n := range []string{"castle", "cave", "crystal", "dream", "forest", "ice", "keep", "mine", "ruins", "sewer", "temple", "tomb", "vine"} {
		if !strings.Contains(out, n+" ") {
			t.Errorf("theme list lacks %s: %q", n, out)
		}
	}
	invoke(t, 2, "theme", "list", "extra")
	invoke(t, 2, "theme", "add", "cave")
	invoke(t, 2, "theme", "add", "lava", fixture(t))

	game := fixture(t)
	out, _ = invoke(t, 0, "theme", "add", "castle", game)
	if !strings.Contains(out, "wrote assets/dungeon/themes/castle/wall.jpg") || !strings.Contains(out, "theme: castle") {
		t.Errorf("first add: %q", out)
	}
	if _, err := os.Stat(filepath.Join(game, "assets", "dungeon", "themes", "castle", "theme.yaml")); err != nil {
		t.Error(err)
	}
	out, _ = invoke(t, 0, "theme", "add", "castle", game)
	if !strings.Contains(out, "kept assets/dungeon/themes/castle/wall.jpg") {
		t.Errorf("second add: %q", out)
	}
	out, _ = invoke(t, 0, "theme", "add", "castle", game, "--force")
	if !strings.Contains(out, "wrote assets/dungeon/themes/castle/wall.jpg") {
		t.Errorf("forced add: %q", out)
	}
	invoke(t, 1, "theme", "add", "castle", t.TempDir())
}
