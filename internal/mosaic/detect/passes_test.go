package detect

import (
	"image"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func passNames(ps []Pass) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Name)
	}
	return out
}

func TestPassesByName(t *testing.T) {
	def := passNames(DefaultPasses)
	cases := []struct {
		in   string
		want []string
	}{
		{"default", def},
		{"all", append(slices.Clone(def), "fliptile2x")},
		{" full640 , flip640 ", []string{"full640", "flip640"}},
		{"full640,,tile2", []string{"full640", "tile2"}},
		{"fliptile2x", []string{"fliptile2x"}},
		{"native,default", append([]string{"native"}, def...)},
		{"", nil},
		{" , ", nil},
	}
	for _, c := range cases {
		got, err := PassesByName(c.in)
		if err != nil || !slices.Equal(passNames(got), c.want) {
			t.Errorf("PassesByName(%q) = %v, %v; want %v", c.in, passNames(got), err, c.want)
		}
	}
	if _, err := PassesByName("full640,warp"); err == nil || !strings.Contains(err.Error(), "warp") {
		t.Fatalf("an unknown name must be named in the error: %v", err)
	}
	// The documented passes exist with the documented meaning.
	byName := map[string]Pass{}
	for _, p := range append(slices.Clone(DefaultPasses), ExtraPasses...) {
		byName[p.Name] = p
	}
	if p := byName["flip640"]; !p.Flip || p.Size != 640 {
		t.Fatalf("flip640 %+v", p)
	}
	if p := byName["native"]; p.Size != 0 || p.Tiles != 0 {
		t.Fatalf("native %+v", p)
	}
	if p := byName["tile3x"]; p.Tiles != 3 || p.Size != 960 {
		t.Fatalf("tile3x %+v", p)
	}
}

func TestTilesCoverTheRegionWithOverlap(t *testing.T) {
	b := image.Rect(10, 20, 310, 220) // 300 x 200
	for _, n := range []int{2, 3} {
		rs := tiles(b, n, 1.0/3)
		if len(rs) != n*n {
			t.Fatalf("n=%d: %d tiles", n, len(rs))
		}
		var union image.Rectangle
		for _, r := range rs {
			if r.Empty() || !r.In(b) {
				t.Fatalf("tile %v outside %v", r, b)
			}
			union = union.Union(r)
		}
		if union != b {
			t.Fatalf("n=%d: tiles cover %v, want %v", n, union, b)
		}
		// Neighbours overlap, so a box on a seam is whole in one of them.
		if !rs[0].Overlaps(rs[1]) || !rs[0].Overlaps(rs[n]) {
			t.Fatalf("n=%d: first tile does not overlap its neighbours", n)
		}
	}
	// With no overlap, tiles meet edge to edge.
	rs := tiles(image.Rect(0, 0, 100, 100), 2, 0)
	if rs[0] != image.Rect(0, 0, 50, 50) || rs[3] != image.Rect(50, 50, 100, 100) {
		t.Fatalf("no overlap: %v", rs)
	}
}

func TestSameBoxRule(t *testing.T) {
	a := image.Rect(0, 0, 10, 10)
	cases := []struct {
		name string
		b    image.Rectangle
		want bool
	}{
		{"identical", a, true},
		{"disjoint", image.Rect(20, 20, 30, 30), false},
		{"touching only", image.Rect(10, 0, 20, 10), false},
		{"small overlap", image.Rect(8, 8, 18, 18), false},
		{"high iou", image.Rect(1, 0, 11, 10), true},
		{"contained", image.Rect(2, 2, 4, 4), true},
	}
	for _, c := range cases {
		if got := same(a, c.b, 0.5, 0.9); got != c.want {
			t.Errorf("%s: same = %v, want %v", c.name, got, c.want)
		}
	}
	// A contained box needs the containment threshold, not just any overlap.
	if same(a, image.Rect(2, 2, 4, 4), 0.5, 1.0) != true {
		t.Fatal("fully contained box with contain=1")
	}
	if same(a, image.Rect(5, 5, 15, 15), 0.9, 1.0) {
		t.Fatal("partial overlap joined at contain=1")
	}
}

func TestClusterJoinsChainsAndKeepsApart(t *testing.T) {
	box := func(x0, y0, x1, y1 int, score float32, pass string) Box {
		return Box{Rect: image.Rect(x0, y0, x1, y1), Label: "x", Score: score, Pass: pass}
	}
	boxes := []Box{
		box(0, 0, 10, 10, 0.6, "a"),
		box(1, 0, 11, 10, 0.9, "b"),
		box(50, 50, 60, 60, 0.7, "a"),
		box(0, 0, 10, 10, 0.8, "a"), // the same pass again: a hit but not another pass
	}
	gs := Cluster(boxes, 0.5, 1)
	if len(gs) != 2 {
		t.Fatalf("%d groups: %+v", len(gs), gs)
	}
	// Best score first; the box of a group is its best member's, the rect grows to the union.
	g := gs[0]
	if g.Score != 0.9 || g.Hits != 3 || g.Passes != 2 || g.Rect != image.Rect(0, 0, 11, 10) {
		t.Fatalf("group %+v", g)
	}
	if gs[1].Hits != 1 || gs[1].Passes != 1 || gs[1].Rect != image.Rect(50, 50, 60, 60) {
		t.Fatalf("second group %+v", gs[1])
	}
	if len(Cluster(nil, 0.5, 1)) != 0 {
		t.Fatal("no boxes, no groups")
	}
	// Groups that come to overlap through a later union are merged: A-B overlap, B-C overlap.
	chain := []Box{
		box(0, 0, 10, 10, 0.9, "a"),
		box(30, 0, 40, 10, 0.8, "b"),
		box(5, 0, 35, 10, 0.5, "c"), // bridges the two
	}
	gs = Cluster(chain, 0.1, 1)
	if len(gs) != 1 || gs[0].Hits != 3 || gs[0].Passes != 3 || gs[0].Rect != image.Rect(0, 0, 40, 10) {
		t.Fatalf("chain: %+v", gs)
	}
}

func TestLibraryPathAndResolveClassesEdges(t *testing.T) {
	t.Setenv("ONNXRUNTIME_LIB", "/opt/ort/libonnxruntime.so")
	if LibraryPath() != "/opt/ort/libonnxruntime.so" {
		t.Fatal("LibraryPath ignores ONNXRUNTIME_LIB")
	}
	t.Setenv("ONNXRUNTIME_LIB", "")
	if LibraryPath() != "" {
		t.Fatal("LibraryPath")
	}
	fallback := []string{"x", "y"}
	// No model, or a model that is not there: the fallback, as a copy.
	for _, model := range []string{"", filepath.Join(t.TempDir(), "gone.onnx")} {
		names, src, err := ResolveClasses(model, "lib.so", fallback)
		if err != nil || src != SourceDefault || !slices.Equal(names, fallback) {
			t.Fatalf("model %q: %v %s %v", model, names, src, err)
		}
		names[0] = "changed"
		if fallback[0] != "x" {
			t.Fatal("the fallback was returned, not a copy")
		}
	}
	// A labels.json that cannot be read for another reason is an error, not a fallback.
	dir := t.TempDir()
	model := filepath.Join(dir, "m.onnx")
	if err := os.WriteFile(model, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "labels.json"), 0o750); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ResolveClasses(model, "", fallback); err == nil {
		t.Fatal("an unreadable labels.json was ignored")
	}
}

func TestValidateClassesMessages(t *testing.T) {
	if err := ValidateClasses(nil); err == nil {
		t.Fatal("empty list accepted")
	}
	if err := ValidateClasses([]string{"a", " "}); err == nil || !strings.Contains(err.Error(), "position 2") {
		t.Fatalf("blank name: %v", err)
	}
	if err := ValidateClasses([]string{"a", "b", "a"}); err == nil || !strings.Contains(err.Error(), `"a"`) {
		t.Fatalf("duplicate: %v", err)
	}
	if err := ValidateClasses([]string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
}
