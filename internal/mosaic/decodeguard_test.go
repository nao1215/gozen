package mosaic

import (
	"os"
	"path/filepath"
	"strings"
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
		_, err := LoadFixes(path)
		if err == nil {
			t.Errorf("LoadFixes accepted %q", y)
			continue
		}
		// A tagged scalar under a list makes the decoder dereference nil. Recovering that
		// fault corrupted the heap on Windows, so it must be refused before decoding.
		if strings.Contains(y, "!x") && (strings.Contains(err.Error(), "nil pointer") || !strings.Contains(err.Error(), "tag")) {
			t.Errorf("LoadFixes(%q) reached the decoder's nil dereference: %v", y, err)
		}
	}
	for _, y := range []string{"a: [\n", "a: {b: \"unterminated\n"} {
		base := fstest.MapFS{"x.yaml": {Data: []byte(y)}}
		if _, _, err := references(base, "assets", []string{".png"}); err == nil {
			t.Errorf("references accepted %q", y)
		}
	}
	tagged := fstest.MapFS{"x.yaml": {Data: []byte("a: !x 0\n")}}
	if _, _, err := references(tagged, "assets", []string{".png"}); err == nil || !strings.Contains(err.Error(), "tag") {
		t.Errorf("references did not refuse a tagged scalar: %v", err)
	}
}
