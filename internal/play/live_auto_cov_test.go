package play

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The site (Edit and play loop): with --dir the game watches game.yaml, data/ and assets/, and
// "pictures and sounds are reloaded on their own"; the definitions wait for F5.
func TestWldLiveReloadsPicturesOnItsOwn(t *testing.T) {
	old := liveEvery
	liveEvery = 5 * time.Millisecond
	t.Cleanup(func() { liveEvery = old })
	dir := wldDisk(t, "", nil)
	g := wldLive(t, dir, "camp")
	if b := g.pics.get("pics/flat.png").Bounds(); b.Dx() != 16 {
		t.Fatalf("the picture is %v", b)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "pics", "flat.png"), wldPNG(t, 24, 24, color.NRGBA{R: 1}), 0o600); err != nil {
		t.Fatal(err)
	}
	screen := wldScreen(g)
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		wldStep(t, g, screen)
		if g.pics.get("pics/flat.png").Bounds().Dx() == 24 {
			break
		}
	}
	if b := g.pics.get("pics/flat.png").Bounds(); b.Dx() != 24 {
		t.Fatalf("the changed picture was not reloaded on its own: %v", b)
	}
	if g.live.pending || g.db.Locations["camp"].Name != "Camp" {
		t.Fatal("a picture changed the definitions")
	}
}

// A change to the definitions is not taken without F5.
func TestWldLiveLeavesDefinitionsToF5(t *testing.T) {
	old := liveEvery
	liveEvery = 5 * time.Millisecond
	t.Cleanup(func() { liveEvery = old })
	dir := wldDisk(t, "", nil)
	g := wldLive(t, dir, "camp")
	wldEdit(t, dir, "data/main.yaml", "name: Camp\n", "name: Camp site\n")
	screen := wldScreen(g)
	for deadline := time.Now().Add(200 * time.Millisecond); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		wldStep(t, g, screen)
	}
	if g.db.Locations["camp"].Name != "Camp" {
		t.Fatalf("the definitions were swapped without F5: %q", g.db.Locations["camp"].Name)
	}
}
