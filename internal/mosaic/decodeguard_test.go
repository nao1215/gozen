package mosaic

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// TestHostileYAMLIsAnErrorNotAPanic: fix files and the YAML files that name pictures are
// untrusted input (SECURITY.md). The YAML decoder panics on some shapes; they must come
// back as errors. The first is a crash found by fuzzing: a rectangle of five numbers.
func TestHostileYAMLIsAnErrorNotAPanic(t *testing.T) {
	dir := t.TempDir()
	for _, y := range []string{
		"0: {add:[[0,0,0,0,0]]}",
		"a.png: {grow: [[0,0,1,1,2,2,2,2,2]]}",
		"a.png: {add: !x 0}",
		"a.png: !x 0",
	} {
		path := filepath.Join(dir, "f.yaml")
		if err := os.WriteFile(path, []byte(y), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadFixes(path); err == nil {
			t.Errorf("LoadFixes accepted %q", y)
		}
	}
	for _, y := range []string{"a: [\n", "a: {b: \"unterminated\n"} {
		base := fstest.MapFS{"x.yaml": {Data: []byte(y)}}
		if _, _, err := references(base, "assets", []string{".png"}); err == nil {
			t.Errorf("references accepted %q", y)
		}
	}
}
