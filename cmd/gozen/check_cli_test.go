package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/diag"
)

type problem struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Col      int    `json:"col"`
	Path     string `json:"path"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
	Code     string `json:"code"`
}

func checkJSONOut(t *testing.T, want int, args ...string) []problem {
	t.Helper()
	out, _ := invoke(t, want, append([]string{"check", "--format", "json"}, args...)...)
	var ps []problem
	if err := json.Unmarshal(resultData(t, out, "check"), &ps); err != nil {
		t.Fatalf("%v: %q", err, out)
	}
	return ps
}

// addWorld appends a YAML file to the fixture's data.
func addWorld(t *testing.T, dir, name, body string) {
	t.Helper()
	write(t, filepath.Join(dir, "data", name), body)
}

func longLine(n int) string { return strings.Repeat("Wide words keep coming, ", n) }

func TestCheckJSONReportsEveryProblemWithItsCode(t *testing.T) {
	dir := fixture(t)
	if ps := checkJSONOut(t, 0, dir); len(ps) != 0 {
		t.Fatalf("a clean game has no problems: %+v", ps)
	}
	if out, _ := invoke(t, 0, "check", "--format", "json", dir); string(resultData(t, out, "check")) != "[]" {
		t.Fatalf("an empty report is []: %q", out)
	}
	// A broken reference is an error with its own code, at its file and line.
	p := filepath.Join(dir, "data", "world.yaml")
	world := string(read(t, p))
	write(t, p, strings.Replace(world, "troop: lesson", "troop: lessn", 1))
	ps := checkJSONOut(t, 1, dir)
	if len(ps) != 1 || ps[0].Severity != "error" || ps[0].Code != diag.UnknownReference.String() || !strings.Contains(ps[0].Message, `unknown troop "lessn"`) || ps[0].File != "data/world.yaml" || ps[0].Line == 0 {
		t.Fatalf("%+v", ps)
	}
	// --only structure and --only layout split the work; both stop at an error of the data.
	if got := checkJSONOut(t, 1, "--only", "structure", dir); len(got) != 1 {
		t.Fatalf("structure: %+v", got)
	}
	if got := checkJSONOut(t, 1, "--only", "layout", dir); len(got) != 1 {
		t.Fatalf("layout: %+v", got)
	}
	write(t, p, world)
	// A directory that is not a game.
	ps = checkJSONOut(t, 1, filepath.Join(t.TempDir(), "absent"))
	if len(ps) != 1 || ps[0].Code != diag.GameUnreadable.String() {
		t.Fatalf("%+v", ps)
	}
}

func TestCheckReportsOversizedDungeon(t *testing.T) {
	dir := fixture(t)
	addWorld(t, dir, "large.yaml", "locations:\n  maze:\n    dungeon:\n      generate: {width: 9223372036854775807, height: 11, rooms: 2, start: S}\n      tiles: {S: {start: true}}\n")
	for _, p := range checkJSONOut(t, 1, "--only", "structure", dir) {
		if p.Code == diag.OutOfRange.String() && strings.Contains(p.Path, "generate") && strings.Contains(p.Message, "at most 255 cells") {
			return
		}
	}
	t.Fatal("gozen check did not report the dungeon size limit")
}

func TestCheckLayoutWarnsOfTextThatOverflowsTheWindow(t *testing.T) {
	dir := fixture(t)
	var b strings.Builder
	b.WriteString("locations:\n  hall:\n    name: Hall\n    exits: [{to: camp}]\n    people:\n")
	for i := range 24 {
		b.WriteString("      - name: P" + strings.Repeat("x", i) + "\n        talk:\n          - say: \"" + longLine(40+i) + "\"\n")
	}
	b.WriteString("      - name: Q\n        talk:\n          - say: \"" + longLine(2) + "{var.gold} fits\"\n")
	addWorld(t, dir, "hall.yaml", b.String())
	write(t, filepath.Join(dir, "data", "world.yaml"), strings.Replace(string(read(t, filepath.Join(dir, "data", "world.yaml"))), "exits: [{to: grotto, label: Explore}]", "exits: [{to: grotto, label: Explore}, {to: hall}]", 1))

	_, stderr := invoke(t, 0, "check", "--only", "layout", dir)
	if !strings.Contains(stderr, "warning: ") || !strings.Contains(stderr, diag.TextOverflow.String()) {
		t.Fatalf("stderr: %q", stderr)
	}
	if !strings.Contains(stderr, "data/hall.yaml") {
		t.Errorf("the warning must name the file: %q", stderr)
	}
	// Only the first 20 are listed; the rest are counted.
	if !strings.Contains(stderr, "ほか") && !strings.Contains(stderr, "more") {
		t.Errorf("the list is cut after 20 lines and says so: %q", stderr)
	}
	ps := checkJSONOut(t, 0, "--only", "layout", dir)
	if len(ps) < 24 {
		t.Fatalf("%d warnings: %+v", len(ps), ps)
	}
	for _, q := range ps {
		if q.Severity != "warning" || q.Code != diag.TextOverflow.String() || q.File != "data/hall.yaml" || q.Line == 0 {
			t.Fatalf("%+v", q)
		}
		if !strings.Contains(q.Message, "does not fit the message window") {
			t.Fatalf("%+v", q)
		}
	}
	// A warning never fails the check.
	invoke(t, 0, "check", dir)
}

func TestCheckLayoutCutsAPromptAboveItsChoices(t *testing.T) {
	dir := fixture(t)
	addWorld(t, dir, "ask.yaml", "events:\n  ask:\n    do:\n      - prompt: \""+longLine(30)+"\"\n        choice:\n          - {label: One, do: [{say: a}]}\n          - {label: Two, do: [{say: b}]}\n          - {label: Three, do: [{say: c}]}\n")
	_, stderr := invoke(t, 0, "check", "--only", "layout", dir)
	if !strings.Contains(stderr, "data/ask.yaml") {
		t.Fatalf("an over-long prompt is reported: %q", stderr)
	}
}

func TestCheckLayoutFontProblemsAreErrors(t *testing.T) {
	dir := fixture(t)
	write(t, filepath.Join(dir, "assets", "regular.ttf"), "not a font")
	_, stderr := invoke(t, 1, "check", dir)
	if !strings.Contains(stderr, diag.FontUnreadable.String()) || !strings.Contains(stderr, "game.yaml:") {
		t.Fatalf("stderr: %q", stderr)
	}
	ps := checkJSONOut(t, 1, dir)
	if len(ps) != 1 || ps[0].Code != diag.FontUnreadable.String() || ps[0].File != "game.yaml" || ps[0].Line == 0 || ps[0].Severity != "error" {
		t.Fatalf("%+v", ps)
	}
	// A font file that is absent is reported by the load, not the layout.
	if err := os.Remove(filepath.Join(dir, "assets", "regular.ttf")); err != nil {
		t.Fatal(err)
	}
	if ps := checkJSONOut(t, 1, dir); len(ps) != 1 || !strings.Contains(ps[0].Message, "regular.ttf") {
		t.Fatalf("%+v", ps)
	}
}

func TestCheckWarnsOfCharactersTheBuiltInFontCannotDraw(t *testing.T) {
	dir := fixture(t)
	game := string(read(t, filepath.Join(dir, "game.yaml")))
	write(t, filepath.Join(dir, "game.yaml"), strings.Replace(game, "fonts: {regular: regular.ttf}\n", "", 1))
	// Latin text needs no warning.
	if _, stderr := invoke(t, 0, "check", dir); strings.Contains(stderr, "fonts.regular") {
		t.Fatalf("stderr: %q", stderr)
	}
	addWorld(t, dir, "ja.yaml", "events:\n  jp:\n    do: [{say: 森の道は静かだ。森の道は静かだ。}]\n")
	_, stderr := invoke(t, 0, "check", dir)
	if !strings.Contains(stderr, "warning: game.yaml: fonts.regular is not set") || !strings.Contains(stderr, "no glyph for") || !strings.Contains(stderr, "森") {
		t.Fatalf("stderr: %q", stderr)
	}
	ps := checkJSONOut(t, 0, dir)
	if len(ps) != 1 || ps[0].Path != "fonts.regular" || ps[0].Severity != "warning" || !strings.Contains(ps[0].Message, "no glyph for") {
		t.Fatalf("%+v", ps)
	}
	// More than 20 missing characters are counted, not all listed.
	var many strings.Builder
	for r := '一'; r < '一'+40; r++ {
		many.WriteRune(r)
	}
	addWorld(t, dir, "many.yaml", "events:\n  many:\n    do: [{say: "+many.String()+"}]\n")
	_, stderr = invoke(t, 0, "check", dir)
	if !strings.Contains(stderr, " and ") || !strings.Contains(stderr, " more;") {
		t.Fatalf("stderr: %q", stderr)
	}
	// A game in Japanese without a font cannot draw the engine's screens, even if its own text is Latin.
	dir = fixture(t)
	game = string(read(t, filepath.Join(dir, "game.yaml")))
	write(t, filepath.Join(dir, "game.yaml"), strings.Replace(strings.Replace(game, "fonts: {regular: regular.ttf}\n", "language: ja\n", 1), "title: CLI expedition", "title: CLI", 1))
	_, stderr = invoke(t, 0, "check", dir)
	if !strings.Contains(stderr, "language ja") {
		t.Fatalf("stderr: %q", stderr)
	}
}
