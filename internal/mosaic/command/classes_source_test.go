package command

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func TestClassesFromLabelsJSON(t *testing.T) {
	game, out := fixture(t)
	dir := t.TempDir()
	model := filepath.Join(dir, "m.onnx")
	if err := os.WriteFile(model, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	write := func(s string) {
		if err := os.WriteFile(filepath.Join(dir, "labels.json"), []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	setup := func(args ...string) (config, error) {
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		o := (&cli{}).commonFlags(fs)
		if err := fs.Parse(append(args, "--model", model, "-o", out)); err != nil {
			t.Fatal(err)
		}
		return o.setup(t.Context(), game)
	}
	write(`["face"]`)
	c, err := setup()
	if err != nil || c.classSource != "labels.json" || !slices.Equal(labelSet(c), []string{"face"}) {
		t.Fatalf("%v %v %v", c.classes, c.classSource, err)
	}
	c, err = setup("--classes", "hand")
	if err != nil || c.classSource != "--classes" || !slices.Equal(c.classes, []string{"hand"}) {
		t.Fatalf("--classes must win: %v %v", c.classes, err)
	}
	write(`["nipple_f","penis","pussy"]`)
	c, err = setup()
	if err != nil || !slices.Equal(labelSet(c), []string{"penis", "pussy"}) {
		t.Fatalf("default classes keep the default targets: %v %v", labelSet(c), err)
	}
	write(`["a","a"]`)
	if _, err = setup(); err == nil {
		t.Fatal("an invalid labels.json must fail")
	}
}

func TestDryRunShowsClasses(t *testing.T) {
	game, out := fixture(t)
	code, _, diag := invoke(t, "auto", "-n", "--classes", "face", "-o", out, game)
	if code != 0 || !strings.Contains(diag, "classes: face (--classes)") || !strings.Contains(diag, "pixelated: face") {
		t.Fatalf("code %d: %s", code, diag)
	}
}
