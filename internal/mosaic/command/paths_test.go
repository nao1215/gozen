package command

import (
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"

	"github.com/nao1215/gozen/internal/mosaic"
	"github.com/nao1215/gozen/internal/mosaic/detect"
)

func TestWithinAndOverlap(t *testing.T) {
	sep := string(filepath.Separator)
	root := sep + "a" + sep + "b"
	cases := []struct {
		p    string
		want bool
	}{
		{root, true},
		{root + sep + "c", true},
		{root + sep + "c" + sep + "d", true},
		{sep + "a", false},
		{sep + "a" + sep + "bb", false},
		{sep + "a" + sep + "b" + "..", false},
		{sep + "x", false},
	}
	for _, c := range cases {
		if got := within(c.p, root); got != c.want {
			t.Errorf("within(%q, %q) = %v, want %v", c.p, root, got, c.want)
		}
	}
	if !overlap(root, root+sep+"c") || !overlap(root+sep+"c", root) || overlap(root, sep+"a"+sep+"bb") {
		t.Fatal("overlap is containment in either direction")
	}
}

func TestCanonicalResolvesLinksAndAbsentTails(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real")
	if err := os.Mkdir(target, 0o750); err != nil {
		t.Fatal(err)
	}
	got, err := canonical(filepath.Join(target, "not", "yet", "there"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(want, "not", "yet", "there") {
		t.Fatalf("canonical %q", got)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symbolic links are not available")
	}
	got, err = canonical(filepath.Join(link, "child"))
	if err != nil || got != filepath.Join(want, "child") {
		t.Fatalf("through a link: %q %v", got, err)
	}
	// A relative path is made absolute first.
	t.Chdir(dir)
	got, err = canonical("real")
	if err != nil || got != want {
		t.Fatalf("relative: %q %v", got, err)
	}
}

func TestProtectPaths(t *testing.T) {
	game, out := fixture(t)
	root := filepath.Dir(game)
	fixFile := filepath.Join(root, "fix.yaml")
	manualFile := filepath.Join(root, "manual.yaml")
	for _, name := range []string{fixFile, manualFile} {
		if err := os.WriteFile(name, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := protectPaths(game, out, fixFile, manualFile, ""); err != nil {
		t.Fatalf("a safe set: %v", err)
	}
	hard := filepath.Join(root, "manual-hard.yaml")
	if err := os.Link(manualFile, hard); err != nil {
		t.Skip("hard links are not available")
	}
	cases := []struct {
		name  string
		out   string
		files []string
		want  string
	}{
		{"output is assets", filepath.Join(game, "assets"), nil, "separate from source"},
		{"output below assets", filepath.Join(game, "assets", "x"), nil, "separate from source"},
		{"output contains the game", root, nil, "output"},
		{"fix inside assets", out, []string{filepath.Join(game, "assets", "fix.yaml")}, "outside assets and output"},
		{"fix inside the output", out, []string{filepath.Join(out, "fix.yaml")}, "outside assets and output"},
		{"same file twice", out, []string{fixFile, fixFile}, "separate files"},
		{"hard link alias", out, []string{manualFile, hard}, "alias each other"},
	}
	for _, c := range cases {
		err := protectPaths(game, c.out, c.files...)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", c.name, err, c.want)
		}
	}
}

func TestValidPictureRules(t *testing.T) {
	game, out := fixture(t)
	c := config{gameDir: game, out: out}
	for _, ok := range []string{"pattern.png", "dir/sub/x.png", "not-there.png"} {
		if err := validPicture(c, ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", ".", "../x.png", "/abs.png", "a/../../x.png", "dir\\x.png", "c:x.png"} {
		if err := validPicture(c, bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	// A picture whose output would be the picture itself.
	same := config{gameDir: game, out: filepath.Join(game, "assets")}
	if err := validPicture(same, "pattern.png"); err == nil {
		t.Fatal("output aliasing the original accepted")
	}
	// A link in assets that leads out of them.
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.png"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(game, "assets", "escape")); err != nil {
		t.Skip("symbolic links are not available")
	}
	if err := validPicture(c, "escape/secret.png"); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("a link out of the assets: %v", err)
	}
	// A link inside the output directory that leads out of it.
	if err := os.MkdirAll(out, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(out, "sub")); err != nil {
		t.Fatal(err)
	}
	if err := validPicture(c, "sub/x.png"); err == nil {
		t.Fatal("an output path through a link out of the output was accepted")
	}
}

func TestValidateReport(t *testing.T) {
	game, out := fixture(t)
	c := config{gameDir: game, out: out}
	if err := validateReport(c); err != nil {
		t.Fatalf("no output yet: %v", err)
	}
	if err := os.MkdirAll(out, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := validateReport(c); err != nil {
		t.Fatalf("an empty output directory: %v", err)
	}
	write := func(s string) {
		if err := os.WriteFile(filepath.Join(out, "report.json"), []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(`[{"file":"pattern.png","output":"pattern.png"}]`)
	if err := validateReport(c); err != nil {
		t.Fatalf("a good report: %v", err)
	}
	for name, s := range map[string]string{
		"broken":           "{",
		"file escapes":     `[{"file":"../x.png"}]`,
		"output escapes":   `[{"file":"pattern.png","output":"../x.png"}]`,
		"output backslash": `[{"file":"pattern.png","output":"a\\b.png"}]`,
		"no file":          `[{"output":"x.png"}]`,
	} {
		write(s)
		if err := validateReport(c); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	// A symbolic link anywhere in the output tree is refused.
	write("[]")
	if err := os.Symlink(game, filepath.Join(out, "link")); err != nil {
		t.Skip("symbolic links are not available")
	}
	if err := validateReport(c); err == nil || !strings.Contains(err.Error(), "symlinks") {
		t.Fatalf("a link in the output: %v", err)
	}
}

func TestOptionsValidate(t *testing.T) {
	good := func() *options {
		fs := pflag.NewFlagSet("t", pflag.ContinueOnError)
		return (&cli{}).commonFlags(fs)
	}
	if err := good().validate(); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*options){
		"block":      func(o *options) { o.block = 257 },
		"block <0":   func(o *options) { o.block = -1 },
		"pad":        func(o *options) { o.padBlocks = 257 },
		"min":        func(o *options) { o.minBlocks = -1 },
		"dilate":     func(o *options) { o.dilate = -2 },
		"dilate up":  func(o *options) { o.dilate = 257 },
		"threads":    func(o *options) { o.threads = 0 },
		"threads up": func(o *options) { o.threads = 1025 },
		"quality 0":  func(o *options) { o.quality = 0 },
		"quality up": func(o *options) { o.quality = 101 },
		"threshold":  func(o *options) { o.minScore = 1.5 },
		"sure":       func(o *options) { o.sure = -0.1 },
		"margin":     func(o *options) { o.margin = math.NaN() },
		"inf":        func(o *options) { o.margin = math.Inf(1) },
	}
	for name, mutate := range mutations {
		o := good()
		mutate(o)
		if err := o.validate(); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	// The edges are allowed.
	o := good()
	o.block, o.padBlocks, o.minBlocks, o.dilate, o.threads, o.quality = 256, 256, 256, 256, 1024, 100
	o.minScore, o.sure, o.margin = 0, 1, 1
	if err := o.validate(); err != nil {
		t.Fatalf("edge values: %v", err)
	}
}

func TestParseClasses(t *testing.T) {
	if got, err := parseClasses(" "); err != nil || len(got) != 3 {
		t.Fatalf("default: %v %v", got, err)
	}
	got, err := parseClasses(" a , b ")
	if err != nil || len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("trimmed: %v %v", got, err)
	}
	if _, err := parseClasses("a,b,a"); err == nil || !strings.Contains(err.Error(), "--classes") {
		t.Fatalf("duplicate: %v", err)
	}
	// A copy of the default order, not the shared slice.
	d1, err := parseClasses("")
	if err != nil {
		t.Fatal(err)
	}
	d1[0] = "changed"
	d2, err := parseClasses("")
	if err != nil || d2[0] == "changed" {
		t.Fatalf("the default classes were shared: %v", err)
	}
}

func TestEnvDefaults(t *testing.T) {
	t.Setenv("MOSAIC_TEST_X", "set")
	if env("MOSAIC_TEST_X", "d") != "set" || env("MOSAIC_TEST_UNSET_X", "d") != "d" {
		t.Fatal("env")
	}
	t.Setenv("MOSAIC_TEST_X", "")
	if env("MOSAIC_TEST_X", "d") != "d" {
		t.Fatal("an empty variable is not set")
	}
	// The environment feeds the flags' defaults.
	t.Setenv("MOSAIC_SHAPE", "mask")
	t.Setenv("MOSAIC_PASSES", "native")
	fs := pflag.NewFlagSet("t", pflag.ContinueOnError)
	o := (&cli{}).commonFlags(fs)
	if o.shape != "mask" || o.passes != "native" {
		t.Fatalf("shape %q passes %q", o.shape, o.passes)
	}
}

func TestPlanGeometryRules(t *testing.T) {
	b := image.Rect(0, 0, 100, 100)
	p := defaultParams
	p.margin, p.padBlocks, p.minBlocks = 0.25, 1, 3
	box := penis(40, 40, 50, 50, 0.9)
	boxes := []detect.Box{box}
	// pad of 1 block (4) or a quarter of the side (3, rounded up) whichever is more: 4 each side;
	// then at least 3 blocks (12) per side, which the 18 pixel box already satisfies.
	_, _, regions, reasons, areas := plan(b, boxes, nil, mosaic.Fix{}, nil, 4, p)
	if len(regions) != 1 || regions[0] != image.Rect(36, 36, 54, 54) || len(reasons) != 0 || len(areas) != 1 || areas[0].Box {
		t.Fatalf("regions %v reasons %v", regions, reasons)
	}
	// A fix's own margin replaces the command's.
	_, _, regions, _, _ = plan(b, boxes, nil, mosaic.Fix{Margin: 1}, nil, 4, p)
	if regions[0] != image.Rect(30, 30, 60, 60) {
		t.Fatalf("fix margin: %v", regions)
	}
	// A box too small is grown to the minimum number of blocks, around its middle.
	p.margin, p.padBlocks, p.minBlocks = 0, 0, 5
	_, _, regions, _, _ = plan(b, boxes, nil, mosaic.Fix{}, nil, 4, p)
	if regions[0] != image.Rect(35, 35, 55, 55) {
		t.Fatalf("minimum size: %v", regions)
	}
	// A grow box widens by its amounts and marks the area.
	_, _, regions, _, areas = plan(b, boxes, nil, mosaic.Fix{Grow: [][8]int{{40, 40, 10, 10, 1, 2, 3, 4}}}, nil, 4, p)
	if regions[0] != image.Rect(34, 33, 58, 59) || !areas[0].Box {
		t.Fatalf("grow: %v", regions)
	}
	// Tag evidence without any detection asks for a look; a checked picture is never asked.
	_, _, _, reasons, _ = plan(b, nil, nil, mosaic.Fix{}, map[string]float32{"penis": 0.9}, 4, p)
	if len(reasons) != 1 || !strings.Contains(reasons[0], "tag evidence") {
		t.Fatalf("reasons %v", reasons)
	}
	_, _, _, reasons, _ = plan(b, nil, nil, mosaic.Fix{Checked: true}, map[string]float32{"penis": 0.9}, 4, p)
	if len(reasons) != 0 {
		t.Fatalf("a checked picture has reasons %v", reasons)
	}
	_, _, _, reasons, _ = plan(b, nil, nil, mosaic.Fix{Skip: true}, map[string]float32{"penis": 0.9}, 4, p)
	if len(reasons) != 0 {
		t.Fatalf("a skipped picture has reasons %v", reasons)
	}
	// Regions that touch are merged and ordered top to bottom, left to right.
	p.minBlocks = 0
	two := []detect.Box{pussy(60, 10, 70, 20, 0.9), penis(10, 60, 20, 70, 0.9), penis(10, 10, 20, 20, 0.9), penis(20, 10, 30, 20, 0.9)}
	_, _, regions, _, _ = plan(b, two, nil, mosaic.Fix{}, nil, 4, p)
	if len(regions) != 3 || regions[0] != image.Rect(10, 10, 30, 20) {
		t.Fatalf("the two touching boxes should be one region: %v", regions)
	}
	for i := 1; i < len(regions); i++ {
		a, c := regions[i-1], regions[i]
		if c.Min.Y < a.Min.Y || c.Min.Y == a.Min.Y && c.Min.X < a.Min.X {
			t.Fatalf("regions out of order: %v", regions)
		}
	}
	// A detection that lies wholly outside the picture is dropped.
	_, _, regions, _, _ = plan(image.Rect(0, 0, 10, 10), []detect.Box{penis(50, 50, 60, 60, 0.9)}, nil, mosaic.Fix{}, nil, 4, p)
	if len(regions) != 0 {
		t.Fatalf("regions %v", regions)
	}
}
