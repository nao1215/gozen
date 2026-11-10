package themes

import (
	"bytes"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/model/gamedata"
)

func TestEveryThemeIsComplete(t *testing.T) {
	want := []string{"castle", "cave", "crystal", "dream", "forest", "ice", "keep", "mine", "ruins", "sewer", "temple", "tomb", "vine"}
	var got []string
	for _, th := range List() {
		got = append(got, th.Name)
		if th.Summary == "" {
			t.Errorf("%s has no summary", th.Name)
		}
		for _, f := range gamedata.ThemeFiles {
			b, err := assets.ReadFile("assets/" + th.Name + "/" + f)
			if err != nil {
				t.Fatalf("%s/%s: %v", th.Name, f, err)
			}
			if filepath.Ext(f) != ".jpg" {
				continue
			}
			img, err := jpeg.Decode(bytes.NewReader(b))
			if err != nil || img.Bounds().Dx() != 1024 || img.Bounds().Dy() != 1024 {
				t.Errorf("%s/%s must be a 1024x1024 JPEG: %v", th.Name, f, err)
			}
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("themes %v, want %v", got, want)
	}
	if len(summaries) != len(want) {
		t.Errorf("summaries has %d entries for %d themes", len(summaries), len(want))
	}
	if !Has("cave") || Has("lava") {
		t.Error("Has is wrong")
	}
}

func TestAddKeepsWhatIsThereUnlessForced(t *testing.T) {
	dir := t.TempDir()
	if _, err := Add("lava", dir, false); err == nil {
		t.Error("an unknown theme must fail")
	}
	r, err := Add("ice", dir, false)
	if err != nil || len(r.Written) != len(gamedata.ThemeFiles)+1 || len(r.Kept) != 0 {
		t.Fatalf("first add: %+v %v", r, err)
	}
	wall := filepath.Join(dir, "assets", "dungeon", "themes", "ice", "wall.jpg")
	if err := os.WriteFile(wall, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err = Add("ice", dir, false)
	if err != nil || len(r.Written) != 0 || len(r.Kept) != len(gamedata.ThemeFiles)+1 {
		t.Fatalf("second add: %+v %v", r, err)
	}
	if b, err := os.ReadFile(wall); err != nil || string(b) != "mine" { //nolint:gosec // Test-owned path under t.TempDir.
		t.Error("a file of the game was replaced without --force")
	}
	r, err = Add("ice", dir, true)
	if err != nil || len(r.Written) != len(gamedata.ThemeFiles)+1 {
		t.Fatalf("forced add: %+v %v", r, err)
	}
	if b, err := os.ReadFile(wall); err != nil || string(b) == "mine" { //nolint:gosec // Test-owned path under t.TempDir.
		t.Error("--force must replace the file")
	}
}

func TestAddFailsWhenTheDestinationIsBlocked(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "assets"), []byte("a file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Add("cave", dir, false); err == nil {
		t.Error("a file where assets/ should be must fail")
	}
	dir = t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets", "dungeon", "themes", "cave", "wall.jpg"), 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := Add("cave", dir, true); err == nil {
		t.Error("a directory where wall.jpg should be must fail")
	}
}

// TestRuntimeDoesNotCarryTheThemes keeps the pictures out of every game binary (ADR 0032):
// none of the runtime packages may depend on this one. go-arch-lint also forbids it,
// because play is not allowed to depend on the commands.
func TestRuntimeDoesNotCarryTheThemes(t *testing.T) {
	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", "github.com/nao1215/gozen/internal/drive/session", "github.com/nao1215/gozen/internal/screen/play", "github.com/nao1215/gozen/internal/screen/gfx", "github.com/nao1215/gozen/internal/model/gamedata").Output()
	if err != nil {
		t.Fatalf("go list failed: %v", err)
	}
	for dep := range strings.FieldsSeq(string(out)) {
		if strings.HasPrefix(dep, "github.com/nao1215/gozen/cmd/") {
			t.Errorf("the runtime depends on %s", dep)
		}
	}
}

// Add writes only inside the game: a theme folder that is a link out of the game
// takes none of the theme's files, and --force replaces a file that is such a
// link instead of writing to what it names.
func TestAddStaysInTheGame(t *testing.T) {
	game, outside := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(game, "assets", "dungeon"), 0o750); err != nil {
		t.Fatal(err)
	}
	themes := filepath.Join(game, "assets", "dungeon", "themes")
	if err := os.Symlink(outside, themes); err != nil {
		t.Skip(err) // links need a privilege on Windows
	}
	for _, force := range []bool{false, true} {
		r, err := Add("cave", game, force)
		if err == nil || len(r.Written) != 0 {
			t.Errorf("force=%v: Add through a link out of the game: %+v %v", force, r, err)
		}
		if ents, err := os.ReadDir(outside); err != nil || len(ents) > 0 {
			t.Fatalf("force=%v: Add wrote %d entries outside the game through a link (%v)", force, len(ents), err)
		}
	}
	if err := os.Remove(themes); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(outside, "target.jpg")
	if err := os.WriteFile(target, []byte("theirs"), 0o600); err != nil {
		t.Fatal(err)
	}
	wall := filepath.Join(themes, "cave", "wall.jpg")
	if err := os.MkdirAll(filepath.Dir(wall), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, wall); err != nil {
		t.Fatal(err)
	}
	if r, err := Add("cave", game, false); err != nil || !slices.Contains(r.Kept, "assets/dungeon/themes/cave/wall.jpg") {
		t.Fatalf("a link the game has is kept: %+v %v", r, err)
	}
	if _, err := Add("cave", game, true); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(target); err != nil || string(b) != "theirs" {
		t.Fatalf("--force wrote through a link to %q (%v)", b, err)
	}
	if info, err := os.Lstat(wall); err != nil || !info.Mode().IsRegular() || info.Size() < 1000 {
		t.Fatalf("--force did not replace the link by the theme's wall: %v", err)
	}
}
