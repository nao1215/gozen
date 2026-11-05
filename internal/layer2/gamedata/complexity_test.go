package gamedata

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func complexitySource(t *testing.T, source string) *DB {
	t.Helper()
	db := &DB{FS: fstest.MapFS{"game.yaml": {Data: []byte(source)}}, sourceFiles: map[string]*sourceDocument{}}
	if _, err := db.readSource([]byte(source), "game.yaml"); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestComplexityBudgets(t *testing.T) {
	source := `screens:
  plot_1:
    title: Garden
    entries:
      - label: Water
        do:
          - set: {var.water: 'max(1, (2 + 3))'}
          - plan: {op: advance}
  plot_2:
    title: Orchard
    entries:
      - label: Water
        do:
          - set: {var.water: 'max(2, (3 + 4))'}
          - plan: {op: advance}
`
	db := complexitySource(t, source)
	cfg := ComplexityConfig{FileLines: 10, BlockLines: 3, Repetitions: 2, MinBlockNodes: 5, ExpressionLength: 8, ExpressionDepth: 1, ScreenOperations: 1, IndexedNames: 2, UnusedDefinitions: 2}
	got := errors.Join(complexityWarningsForTest(t, db, cfg)...).Error()
	for _, want := range []string{"game.yaml:1:1", "file lines = 15", "block lines = 7", "same-shaped screens blocks = 2", "expression characters = 15", "expression nesting = 2", "screen set/plan commands = 2", `indexed names "plot_#" = 2`, "possibly unreferenced screens definitions = 2"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if again := errors.Join(complexityWarningsForTest(t, db, cfg)...).Error(); again != got {
		t.Fatal("nondeterministic diagnostics")
	}
	if warnings := complexityWarningsForTest(t, db, ComplexityConfig{}); len(warnings) != 0 {
		t.Fatalf("small source: %v", warnings)
	}
	disabled := ComplexityConfig{-1, -1, -1, -1, -1, -1, -1, -1, -1}
	if warnings := complexityWarningsForTest(t, db, disabled); len(warnings) != 0 {
		t.Fatalf("disabled: %v", warnings)
	}
}

func TestComplexityBoundariesAndProse(t *testing.T) {
	db := complexitySource(t, "title: 'A very long title ((((( that is not an expression'\nscreens:\n  room:\n    title: Room\n    entries:\n      - {label: Wait, do: [{set: {var.water: '1 + 2'}}]}\n")
	cfg := ComplexityConfig{FileLines: 6, ExpressionLength: 5, ExpressionDepth: 1, UnusedDefinitions: -1}
	if got := complexityWarningsForTest(t, db, cfg); len(got) != 0 {
		t.Fatalf("equal budget or prose: %v", got)
	}
	cfg.ExpressionLength = 4
	if got := complexityWarningsForTest(t, db, cfg); len(got) != 1 {
		t.Fatalf("over budget: %v", got)
	}
	if got := expressionNesting(`max(1, len("((["))`); got != 2 {
		t.Fatalf("quoted delimiters: %d", got)
	}
}

func TestComplexityShapeAndAliases(t *testing.T) {
	db := complexitySource(t, "screens:\n  one: &base {title: One, entries: [{label: Open, open: one}]}\n  two: {entries: [{open: one, label: Other}], title: Two}\n  three: *base\n")
	cfg := ComplexityConfig{Repetitions: 2, MinBlockNodes: 1, UnusedDefinitions: 1}
	got := errors.Join(complexityWarningsForTest(t, db, cfg)...).Error()
	// Anchors retain their authored structure; aliases are not expanded copies.
	if strings.Contains(got, "same-shaped") {
		t.Fatalf("alias expanded: %s", got)
	}
	if !strings.Contains(got, "two, three") || strings.Contains(got, ": one,") {
		t.Fatalf("reference accounting: %s", got)
	}
	db = complexitySource(t, "screens:\n  one: {title: One, entries: [{label: Open, open: one}]}\n  two: {entries: [{open: one, label: Other}], title: Two}\n")
	if got := errors.Join(complexityWarningsForTest(t, db, cfg)...).Error(); !strings.Contains(got, "same-shaped screens blocks = 2") {
		t.Fatalf("map order changed shape: %s", got)
	}
}

// Every checked-in game is measured in make test, including nested modules.
// New budget overruns require inspecting the source instead of silently growing it.
func TestComplexityExamples(t *testing.T) {
	root := "../../../examples"
	count := 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "game.yaml" {
			return nil
		}
		count++
		t.Run(filepath.ToSlash(p), func(t *testing.T) {
			db, err := LoadDir(os.DirFS(filepath.Dir(p)), ".")
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range complexityWarningsForTest(t, db, ComplexityConfig{}) {
				t.Error(w)
			}
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("no example games measured")
	}
	t.Logf("measured %d example games", count)
}

func TestComplexityMultiline(t *testing.T) {
	db := complexitySource(t, "screens:\n  room:\n    entries:\n      - do:\n          - set:\n              var.water: >-\n                max(1,\n                  (2 + 3))\n")
	got := errors.Join(complexityWarningsForTest(t, db, ComplexityConfig{BlockLines: 6, ExpressionLength: 10, ExpressionDepth: 1})...).Error()
	for _, want := range []string{"block lines = 7", "expression characters", "expression nesting = 2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s: %s", want, got)
		}
	}
}

func complexityWarningsForTest(t *testing.T, db *DB, cfg ComplexityConfig) []error {
	t.Helper()
	warnings, err := db.ComplexityWarnings(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return warnings
}

func TestComplexitySourceFailure(t *testing.T) {
	db := complexitySource(t, "title: Garden\n")
	db.FS = fstest.MapFS{}
	if _, err := db.ComplexityWarnings(ComplexityConfig{}); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing source: %v", err)
	}
	db.FS = fstest.MapFS{"game.yaml": {Data: []byte("title: [\n")}}
	if _, err := db.ComplexityWarnings(ComplexityConfig{}); err == nil {
		t.Fatal("invalid replacement source accepted")
	}
}

func TestComplexityIndexedAssignmentTarget(t *testing.T) {
	db := complexitySource(t, `screens:
  room:
    entries:
    - do:
      - set: {'var["stock_" + str(max(1,2))]': '1'}
`)
	got := complexityWarningsForTest(t, db, ComplexityConfig{ExpressionLength: 8, ExpressionDepth: 1, UnusedDefinitions: -1})
	if len(got) != 2 {
		t.Fatalf("target complexity: %v", got)
	}
	text := errors.Join(got...).Error()
	for _, want := range []string{"game.yaml:5:", "expression characters", "expression nesting = 2"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s: %s", want, text)
		}
	}
}

func TestComplexityStateKeyFamilies(t *testing.T) {
	source := "screens:\n  room:\n    entries:\n    - do:\n"
	ids := []string{"oak", "pine", "birch", "elm", "ash", "cedar", "maple", "willow"}
	for i, id := range ids {
		source += fmt.Sprintf("      - set: {var.sample_%s: 'var.sample_%s + 1', var.count_%d: 'var[\"booking_\" + arg.worker + \"_%d\"]'}\n", id, id, i, i)
	}
	db := complexitySource(t, source)
	db.Game.Planning = &Planning{Entities: map[string]Entity{}}
	for _, id := range ids {
		db.Game.Planning.Entities[id] = Entity{}
	}
	got := fmt.Sprint(errors.Join(complexityWarningsForTest(t, db, ComplexityConfig{})...))
	for _, want := range []string{`indexed state keys "var.sample_*" = 8`, `indexed state keys "var.count_#" = 8`, `indexed state keys "var.booking_*_#" = 8`, "game.yaml:5:"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "= 16") {
		t.Fatal("read/write or repeated key double counted", got)
	}
	if got := complexityWarningsForTest(t, db, ComplexityConfig{IndexedNames: -1}); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestComplexitySmallCopiedEvents(t *testing.T) {
	db := complexitySource(t, "title: Neutral\n")
	source := "events:\n"
	for i := range 8 {
		source += fmt.Sprintf("  inspect_%d: {do: [{call: inspect}]}\n", i)
	}
	db.FS = fstest.MapFS{"story.yaml": {Data: []byte(source)}}
	db.sourceFiles = map[string]*sourceDocument{"story.yaml": {}}
	got := fmt.Sprint(errors.Join(complexityWarningsForTest(t, db, ComplexityConfig{})...))
	if !strings.Contains(got, "same-shaped events blocks = 8") {
		t.Fatal(got)
	}
	if got := complexityWarningsForTest(t, db, ComplexityConfig{MinBlockNodes: -1, IndexedNames: -1, UnusedDefinitions: -1}); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestComplexitySmallIDCopiesAndFalseFriends(t *testing.T) {
	ids := []string{"fir_tree", "fir", "ash", "birch", "cedar", "maple", "elm", "willow"}
	source := "events:\n"
	for _, id := range ids {
		source += fmt.Sprintf("  inspect_%s: {do: [{set: {var.checked_%s: '1'}}]}\n", id, id)
	}
	db := complexitySource(t, "title: Neutral\n")
	db.Game.Planning = &Planning{Activities: map[string]Activity{}}
	for _, id := range ids {
		db.Game.Planning.Activities[id] = Activity{}
	}
	db.FS = fstest.MapFS{"story.yaml": {Data: []byte(source)}}
	db.sourceFiles = map[string]*sourceDocument{"story.yaml": {}}
	got := fmt.Sprint(errors.Join(complexityWarningsForTest(t, db, ComplexityConfig{})...))
	for _, want := range []string{"same-shaped events blocks = 8", `indexed state keys "var.checked_*" = 8`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
	// Names without a complete ID token do not become a collection family.
	db = complexitySource(t, `screens:
  room:
    entries:
    - label: 'var.checked_1 var.checked_2'
      do:
      - set: {var.first: 'var.first + var.ashes + var.maples', var.total: 'var["checked"] + var["checked"]'}
`)
	db.Game.Planning = &Planning{Entities: map[string]Entity{"fir": {}, "ash": {}, "maple": {}}}
	if got := complexityWarningsForTest(t, db, ComplexityConfig{IndexedNames: 2}); len(got) != 0 {
		t.Fatal(got)
	}
}
