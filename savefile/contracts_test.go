package savefile_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/savefile"
)

func TestExplicitSaveRootAndInvalidIDs(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"", ".", "..", "../other", "a/b", `a\b`, "a:b", "a b", "CON", "nul", "Com1", "LPT9"} {
		if _, err := savefile.Prepare(id, savefile.Config{Root: root}); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("id %q: %v", id, err)
		}
	}
	a, err := savefile.Prepare("expedition", savefile.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	b, err := savefile.Prepare("puzzle", savefile.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if a == b || filepath.Dir(a) != root {
		t.Fatal("games share save directory")
	}
}
func TestWriteFailurePreservesDirectoryAndCleansTemporary(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "occupied")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := savefile.Write(target, []byte("must fail")); err == nil {
		t.Fatal("replaced a directory")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "occupied" || !entries[0].IsDir() {
		t.Fatalf("failed write leaked or modified entries: %v", entries)
	}
}
func TestLoadPreservesIOCause(t *testing.T) {
	_, err := savefile.Load(fstest.MapFS{}, "absent.json")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	if _, err = savefile.Load(fstest.MapFS{}, "../outside"); !errors.Is(err, fs.ErrInvalid) {
		t.Fatal(err)
	}
}
