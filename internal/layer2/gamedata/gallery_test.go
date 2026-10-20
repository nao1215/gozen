package gamedata

import (
	"maps"
	"slices"
	"testing"
	"testing/fstest"
)

// A gallery CG groups its variants with files, and opens under the same name (the first file) whichever stage is viewed
func TestGalleryCGPages(t *testing.T) {
	fsys := fstest.MapFS{
		"assets/cg/x.jpg":     {Data: []byte("x")},
		"assets/cg/x.002.jpg": {Data: []byte("x")},
		"assets/cg/x.003.jpg": {Data: []byte("x")},
		"assets/cg/x_y.jpg":   {Data: []byte("x")},
	}
	cg := GalleryCG{Files: []string{"cg/x.jpg", "cg/x.*.jpg"}, Title: "X"}
	if got, want := cg.Pages(fsys), []string{"cg/x.jpg", "cg/x.002.jpg", "cg/x.003.jpg"}; !slices.Equal(got, want) {
		t.Fatalf("pages %v, want %v", got, want)
	}
	if cg.Key() != "cg/x.jpg" {
		t.Fatalf("key %q", cg.Key())
	}
	for _, f := range []string{"cg/x.jpg", "cg/x.003.jpg"} {
		if !cg.Has(f) {
			t.Errorf("%s is a page of it", f)
		}
	}
	if cg.Has("cg/x_y.jpg") {
		t.Error("cg/x_y.jpg is another picture")
	}
	one := GalleryCG{File: "cg/x.jpg"}
	if one.Key() != "cg/x.jpg" || !one.Has("cg/x.jpg") || one.Has("cg/x.002.jpg") {
		t.Error("file alone is that one picture")
	}
}

// A flashback opened by a flag alone keeps the old name (scene:flag); those with when or seen get a per-scene name
func TestGallerySceneKey(t *testing.T) {
	if k := (GalleryScene{Event: "e", Flag: "f", Title: "T"}).Key(); k != "scene:f" {
		t.Errorf("flag alone: %q", k)
	}
	a := GalleryScene{Event: "e", Flag: "f", Title: "T", Seen: "cg/x.jpg"}.Key()
	b := GalleryScene{Event: "e", Flag: "f", Title: "T2", Seen: "cg/x.jpg"}.Key()
	if a == "scene:f" || a == b {
		t.Errorf("with seen: %q %q", a, b)
	}
	fixed := GalleryScene{ID: "memory-1", Event: "e", Flag: "f", Title: "旧題", Seen: "cg/x.jpg"}
	key := fixed.Key()
	fixed.Title, fixed.Event, fixed.Flag, fixed.Seen = "翻訳後の題名", "other", "other_flag", ""
	fixed.Pages = []GalleryPage{{Title: "途中", Line: 4}}
	if key != "scene:id:memory-1" || fixed.Key() != key {
		t.Fatal("表示名や再生条件の変更で固定キーが変わった")
	}
	fixed.ID = "memory-2"
	if fixed.Key() == key {
		t.Fatal("同じ題名の別 ID を区別できない")
	}
}

func TestGalleryKeyMigrationKeepsUnlocks(t *testing.T) {
	g := Gallery{Scenes: []GalleryScene{
		{ID: "first", LegacyKeys: []string{"scene:old/旧題", "scene:old/別の旧題"}},
		{ID: "second", LegacyKeys: []string{"scene:flag"}},
		{ID: "locked", LegacyKeys: []string{"scene:locked"}},
		{ID: "current", LegacyKeys: []string{"scene:false"}},
		{Event: "old", Flag: "unchanged"},
	}}
	unlocked := map[string]bool{"scene:old/別の旧題": true, "scene:flag": true, "scene:locked": false,
		"scene:false": false, "scene:id:current": true, "scene:unchanged": true,
		"scene:unknown": true, "cg:cg/a.jpg": true, "pic:cg/page.jpg": true}
	want := maps.Clone(unlocked)
	want["scene:id:first"], want["scene:id:second"] = true, true
	if !g.MigrateUnlocks(unlocked) || !maps.Equal(unlocked, want) {
		t.Fatalf("開放・未開放・未知記録の移行結果が違う。%v", unlocked)
	}
	if g.MigrateUnlocks(unlocked) || !maps.Equal(unlocked, want) {
		t.Fatal("繰り返しの移行で記録が変わった")
	}
}

// The expression pages('cg/x') is the number of pictures of that name (x.jpg, x.002.jpg ...; x_y.jpg is a different picture). For a pattern, the number that match
func TestCountPages(t *testing.T) {
	fsys := fstest.MapFS{
		"assets/cg/x.jpg":     {Data: []byte("x")},
		"assets/cg/x.002.jpg": {Data: []byte("x")},
		"assets/cg/x.003.jpg": {Data: []byte("x")},
		"assets/cg/x_y.jpg":   {Data: []byte("x")},
	}
	for name, want := range map[string]int{"cg/x": 3, "cg/x_y": 1, "cg/x.*.jpg": 2, "cg/none": 0, "": 0} {
		if got := CountPages(fsys, name); got != want {
			t.Errorf("CountPages(%q) = %d, want %d", name, got, want)
		}
	}
	if CountPages(nil, "cg/x") != 0 {
		t.Error("no files, no pages")
	}
}
