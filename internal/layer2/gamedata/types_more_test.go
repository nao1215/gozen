package gamedata_test

import (
	"image/color"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/goccy/go-yaml"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestColorParsesTheTwoWrittenForms(t *testing.T) {
	cases := []struct {
		in   string
		want color.NRGBA
		ok   bool
	}{
		{"#ff8000", color.NRGBA{255, 128, 0, 255}, true},
		{"ff8000", color.NRGBA{255, 128, 0, 255}, true},
		{"#ff800040", color.NRGBA{255, 128, 0, 64}, true},
		{"#FFFFFF", color.NRGBA{255, 255, 255, 255}, true},
		{"#fff", color.NRGBA{}, false},
		{"#ff80000", color.NRGBA{}, false},
		{"#gg0000", color.NRGBA{}, false},
		{"", color.NRGBA{}, false},
		{"#+f0000", color.NRGBA{}, false},
		{"#ff００00", color.NRGBA{}, false}, // full-width digits are not hex
	}
	for _, c := range cases {
		var col data.Color
		err := col.UnmarshalText([]byte(c.in))
		if (err == nil) != c.ok {
			t.Errorf("%q: %v", c.in, err)
			continue
		}
		if c.ok && (col.NRGBA != c.want || !col.Valid) {
			t.Errorf("%q = %+v", c.in, col)
		}
		if !c.ok && col.Valid {
			t.Errorf("%q: a refused color must not be valid", c.in)
		}
	}
	var unset data.Color
	def := color.NRGBA{1, 2, 3, 4}
	if unset.Or(def) != def {
		t.Error("an unset color is the default")
	}
	var set data.Color
	_ = set.UnmarshalText([]byte("#102030"))
	if set.Or(def) != (color.NRGBA{16, 32, 48, 255}) {
		t.Error("a set color is itself")
	}
}

func TestColorFromYAMLNodes(t *testing.T) {
	var v struct {
		A data.Color `yaml:"a"`
		B data.Color `yaml:"b"`
	}
	if err := yaml.UnmarshalWithOptions([]byte("a: '#112233'\nb: 112233\n"), &v, yaml.Strict()); err != nil {
		t.Fatalf("a string, and a number read as text: %v", err)
	}
	if v.A.NRGBA != (color.NRGBA{0x11, 0x22, 0x33, 255}) || v.B.NRGBA != v.A.NRGBA {
		t.Errorf("%+v", v)
	}
	for _, bad := range []string{"a: [1, 2]\n", "a: {r: 1}\n", "a: '#12'\n", "a: true\n"} {
		if err := yaml.UnmarshalWithOptions([]byte(bad), &v, yaml.Strict()); err == nil {
			t.Errorf("%q was accepted as a color", bad)
		}
	}
}

func TestExprFromYAMLScalars(t *testing.T) {
	var v struct {
		A, B, C, D, E data.Expr
	}
	src := "a: 'gold > 1'\nb: 3\nc: true\nd: 2.5\ne: |\n  1 +\n  2\n"
	if err := yaml.UnmarshalWithOptions([]byte(src), &v, yaml.Strict()); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"a": "gold > 1", "b": "3", "c": "true", "d": "2.5"} {
		got := map[string]data.Expr{"a": v.A, "b": v.B, "c": v.C, "d": v.D}[name]
		if !got.Set() || got.String() != want {
			t.Errorf("%s = %q, want %q", name, got.String(), want)
		}
	}
	if b, err := v.A.MarshalText(); err != nil || string(b) != "gold > 1" {
		t.Errorf("marshal %q %v", b, err)
	}
	var none data.Expr
	if b, err := none.MarshalText(); err != nil || len(b) != 0 || none.Set() {
		t.Errorf("an unset expression marshals to nothing: %q %v", b, err)
	}
	for _, bad := range []string{"a: [1]\n", "a: {x: 1}\n", "a: '1 +'\n", "a: '('\n"} {
		if err := yaml.UnmarshalWithOptions([]byte(bad), &v, yaml.Strict()); err == nil {
			t.Errorf("%q was accepted as an expression", bad)
		}
	}
}

func TestVaryAndFxMultipliers(t *testing.T) {
	var none data.SfxVary
	if p, v := none.Values(); p != 0.03 || v != 0.1 {
		t.Errorf("defaults %v %v", p, v)
	}
	a, b := 0.5, 0.0
	set := data.SfxVary{Pitch: &a, Volume: &b}
	if p, v := set.Values(); p != 0.5 || v != 0 {
		t.Errorf("set values %v %v (0 turns it off)", p, v)
	}
	neg := -2.0
	one := 1.5
	if (data.BattleFx{}).Get(nil) != 1 || (data.BattleFx{}).Get(&neg) != 0 || (data.BattleFx{}).Get(&one) != 1.5 {
		t.Error("BattleFx.Get: nil is 1, negative is 0")
	}
	if (data.BattleFxAuto{}).Get(nil, 0.4) != 0.4 || (data.BattleFxAuto{}).Get(&neg, 0.4) != 0 || (data.BattleFxAuto{}).Get(&one, 0.4) != 1.5 {
		t.Error("BattleFxAuto.Get: nil is the default, negative is 0")
	}
}

func TestPageNamesAndCounts(t *testing.T) {
	for in, want := range map[string]string{
		"cg/x.jpg":      "cg/x.jpg",
		"cg/x.002.jpg":  "cg/x.jpg",
		"cg/x.10.png":   "cg/x.png",
		"cg/x.a.jpg":    "cg/x.a.jpg",
		"cg/x..jpg":     "cg/x..jpg",
		"cg/x.2":        "cg/x.2",
		"x.001.002.jpg": "x.001.jpg",
		"":              "",
	} {
		if got := data.PageBase(in); got != want {
			t.Errorf("PageBase(%q) = %q, want %q", in, got, want)
		}
	}
	fsys := fstest.MapFS{
		"assets/cg/x.jpg": {}, "assets/cg/x.002.jpg": {}, "assets/cg/x.003.jpg": {}, "assets/cg/y.jpg": {},
	}
	if data.CountPages(fsys, "cg/x") != 3 || data.CountPages(fsys, "cg/y") != 1 || data.CountPages(fsys, "cg/none") != 0 {
		t.Errorf("counts %d %d %d", data.CountPages(fsys, "cg/x"), data.CountPages(fsys, "cg/y"), data.CountPages(fsys, "cg/none"))
	}
	if data.CountPages(fsys, "cg/*.jpg") != 4 || data.CountPages(nil, "cg/x") != 0 || data.CountPages(fsys, "") != 0 || data.CountPages(fsys, "cg/[") != 0 {
		t.Error("patterns count their matches; nothing, nowhere and a bad pattern count 0")
	}
	cg := data.CG{File: "one.png"}
	if got := cg.Pages(fsys); len(got) != 1 || got[0] != "one.png" {
		t.Errorf("no files: %v", got)
	}
	many := data.CG{Files: []string{"cg/y.jpg", "cg/x.*.jpg", "cg/none*.jpg", "cg/["}}
	if got := strings.Join(many.Pages(fsys), ","); got != "cg/y.jpg,cg/x.002.jpg,cg/x.003.jpg" {
		t.Errorf("pages %s", got)
	}
}

// TestPictureNumbersFollowTheSchemaVersion: a fade or brightness of 0 means "default" in the first spec and "none" or
// "black" from the newer one; omitted always means the default.
func TestPictureNumbersFollowTheSchemaVersion(t *testing.T) {
	load := func(version, image string) data.CG {
		t.Helper()
		header := ""
		if version != "" {
			header = "schema_version: " + version + "\n"
		}
		db := testgame.Load(t, map[string]string{
			"game.yaml":     header + "id: p\n" + testgame.Game,
			"data/img.yaml": "events: {pic: {do: [{image: " + image + "}]}}\n",
		})
		return *db.Events["pic"].Do[0].Image
	}
	omitted := load("4", "{file: x.png}")
	if omitted.FadeTicks() != 15 || omitted.Picture().Brightness() != 1 || omitted.HasFade() {
		t.Errorf("omitted: fade %d brightness %v has %v", omitted.FadeTicks(), omitted.Picture().Brightness(), omitted.HasFade())
	}
	zeroNew := load("4", "{file: x.png, fade: 0, brightness: 0}")
	if zeroNew.FadeTicks() != 0 || zeroNew.Picture().Brightness() != 0 || !zeroNew.HasFade() {
		t.Errorf("zero in the new spec: fade %d brightness %v has %v", zeroNew.FadeTicks(), zeroNew.Picture().Brightness(), zeroNew.HasFade())
	}
	zeroOld := load("", "{file: x.png, fade: 0, brightness: 0}")
	if zeroOld.FadeTicks() != 15 || zeroOld.Picture().Brightness() != 1 {
		t.Errorf("zero in a game without a schema version is the default: fade %d brightness %v", zeroOld.FadeTicks(), zeroOld.Picture().Brightness())
	}
	two := load("4", "{file: x.png, fade: 2, brightness: 0.5}")
	if two.FadeTicks() != 120 || two.Picture().Brightness() != 0.5 || !two.HasFade() {
		t.Errorf("fade 2: %d brightness %v", two.FadeTicks(), two.Picture().Brightness())
	}
	tiny := load("4", "{file: x.png, fade: 0.001}")
	if tiny.FadeTicks() != 1 {
		t.Errorf("a very short fade is at least one tick, got %d", tiny.FadeTicks())
	}
	if !two.Same(two) || two.Same(zeroNew) || omitted.Same(two) {
		t.Error("Same compares how a picture is shown")
	}
	plain := load("4", "x.png")
	if plain.File != "x.png" {
		t.Errorf("a picture can be written as its file name: %+v", plain)
	}
}
