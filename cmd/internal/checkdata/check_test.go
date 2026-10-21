package checkdata

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/nao1215/gozen/internal/diag"
	"github.com/nao1215/gozen/internal/testgame"
)

// game is the test expedition with the figure of e2e/atago/fixtures/puppet worn by the scout.
func game(t *testing.T, extra map[string]string) fstest.MapFS {
	t.Helper()
	fsys := fstest.MapFS{
		"game.yaml":    {Data: []byte(testgame.Game)},
		"assets/x.png": {Data: []byte("synthetic file sentinel")},
	}
	// The scout wears a moving portrait.
	fsys["data/main.yaml"] = &fstest.MapFile{Data: []byte(strings.Replace(testgame.Data, "scout: {name: Scout, class: ranger, equip: {weapon: stick}}", "scout: {name: Scout, class: ranger, equip: {weapon: stick}, puppet: puppet.yaml, face: default}", 1))}
	for _, name := range []string{"puppet.yaml", "base.png", "closed.png", "mouth.png"} {
		b, err := os.ReadFile("../../../e2e/atago/fixtures/puppet/" + name)
		if err != nil {
			t.Fatal(err)
		}
		fsys["assets/"+name] = &fstest.MapFile{Data: b}
	}
	for n, s := range extra {
		fsys[n] = &fstest.MapFile{Data: []byte(s)}
	}
	return fsys
}

func TestLoadSummarisesAGameThatLoads(t *testing.T) {
	var out, warn bytes.Buffer
	db, err := Load(game(t, nil), &out, &warn)
	if err != nil {
		t.Fatal(err)
	}
	if db == nil || !strings.HasPrefix(out.String(), "ok: 3 locations, 2 events, 2 actors, 9 skills, 5 items, 2 enemies, 3 troops, 0 ai sets\n") {
		t.Fatalf("summary: %q", out.String())
	}
	// The test expedition sets one flag nothing reads and reads one nothing sets: two warnings, no error.
	if !strings.Contains(warn.String(), "flag.saw_two is set but nothing reads it") || !strings.Contains(warn.String(), "flag.ghost is read but nothing sets it") || strings.Count(warn.String(), "warning: ") != 2 {
		t.Errorf("warnings: %q", warn.String())
	}
	got, diags := Diagnose(game(t, nil))
	if got == nil || len(diags) != 2 || diags[0].Code != diag.UnusedFlag.String() || diags[1].Code != diag.UnsetFlag.String() {
		t.Errorf("the same two warnings, as diagnostics: %+v", diags)
	}
	for _, d := range diags {
		if d.Severity != "warning" || d.File != "data/main.yaml" || d.Line == 0 {
			t.Errorf("%+v", d)
		}
	}
}

func TestLoadAndDiagnoseNameABrokenPuppet(t *testing.T) {
	broken := game(t, map[string]string{"assets/mouth.png": "this is not a picture"})
	var out bytes.Buffer
	db, err := Load(broken, &out, io.Discard)
	if db != nil || err == nil || diag.Of(err) != diag.Shape || !strings.Contains(err.Error(), "actors.scout.puppet") {
		t.Fatalf("Load: %v %v", db, err)
	}
	if out.Len() != 0 {
		t.Errorf("a failing check never prints ok: %q", out.String())
	}
	db, diags := Diagnose(broken)
	if db != nil || len(diags) != 1 {
		t.Fatalf("Diagnose: %v %+v", db, diags)
	}
	d := diags[0]
	if d.Path != "actors.scout.puppet" || d.Severity != "error" || d.Code != diag.Shape.String() || !strings.Contains(d.Message, "mouth.png") {
		t.Errorf("%+v", d)
	}
}

func TestLoadReportsAMissingGameWithItsCode(t *testing.T) {
	empty := fstest.MapFS{}
	_, err := Load(empty, io.Discard, io.Discard)
	if diag.Of(err) != diag.GameUnreadable {
		t.Errorf("Load: %v", err)
	}
	db, diags := Diagnose(empty)
	if db != nil || len(diags) != 1 || diags[0].Code != diag.GameUnreadable.String() {
		t.Errorf("Diagnose: %v %+v", db, diags)
	}
	// A game.yaml that cannot be parsed is a problem of the definitions, not of the directory.
	_, err = Load(fstest.MapFS{"game.yaml": {Data: []byte("title: [")}}, io.Discard, io.Discard)
	if err == nil || diag.Of(err) == diag.GameUnreadable {
		t.Errorf("a broken game.yaml: %v", err)
	}
}

func TestLoadWarnsOfLanguageProblems(t *testing.T) {
	jp := game(t, map[string]string{
		"game.yaml":        testgame.Game + "language: ja\nfonts: {regular: latin.ttf}\n",
		"assets/latin.ttf": string(goregular.TTF),
	})
	var warn bytes.Buffer
	if _, err := Load(jp, io.Discard, &warn); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(warn.String(), "warning: game.yaml fonts.regular: latin.ttf has no glyph for") || !strings.Contains(warn.String(), diag.LanguageWords.String()) {
		t.Errorf("warnings: %q", warn.String())
	}
	_, diags := Diagnose(jp)
	found := false
	for _, d := range diags {
		if d.Code == diag.LanguageWords.String() && d.Severity == "warning" && d.File == "game.yaml" && d.Path == "language" {
			found = true
		}
	}
	if !found {
		t.Errorf("Diagnose lacks the language warning: %+v", diags)
	}
}

type limitedWriter struct{ after int }

func (w *limitedWriter) Write(b []byte) (int, error) {
	if w.after--; w.after < 0 {
		return 0, errors.New("disk full")
	}
	return len(b), nil
}

func TestLoadStopsWhenItsOutputFails(t *testing.T) {
	if _, err := Load(game(t, nil), &limitedWriter{}, io.Discard); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Errorf("the summary could not be written: %v", err)
	}
	jp := game(t, map[string]string{
		"game.yaml":        testgame.Game + "language: ja\nfonts: {regular: latin.ttf}\n",
		"assets/latin.ttf": string(goregular.TTF),
	})
	if _, err := Load(jp, io.Discard, &limitedWriter{}); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Errorf("the warning could not be written: %v", err)
	}
}

func TestLoadReportsWarningsOfTheDefinitionsWithoutFailing(t *testing.T) {
	// A flag that is set and never read is a warning, not an error.
	extra := map[string]string{"data/flags.yaml": "events:\n  peek: {do: [{set: {flag.lonely: true}}]}\n"}
	var out, warn bytes.Buffer
	if _, err := Load(game(t, extra), &out, &warn); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "ok: ") || !strings.Contains(warn.String(), "flag.lonely is set but nothing reads it") {
		t.Fatalf("stdout %q warnings %q", out.String(), warn.String())
	}
	db, diags := Diagnose(game(t, extra))
	lonely := 0
	for _, d := range diags {
		if d.Severity != "warning" {
			t.Errorf("%+v", d)
		}
		if strings.Contains(d.Message, "flag.lonely") {
			lonely++
		}
	}
	if db == nil || lonely != 1 || len(diags) != 3 {
		t.Fatalf("%v %+v", db, diags)
	}
	if _, err := Load(game(t, extra), io.Discard, &limitedWriter{}); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Errorf("a warning that cannot be written fails the check: %v", err)
	}
}
