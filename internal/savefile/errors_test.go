package savefile_test

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/savefile"
)

func skipIfRoot(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file permissions do not apply here")
	}
}

// TestPrepareUsesTheConfigDirectoryByDefault: with no Root, saves go under <user config dir>/gozen/<id>, in a private directory.
func TestPrepareUsesTheConfigDirectoryByDefault(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("HOME", cfg)
	t.Setenv("AppData", cfg)
	dir, err := savefile.Prepare("my-game_2", savefile.Config{})
	if err != nil {
		t.Skipf("no user config directory here: %v", err)
	}
	if filepath.Base(dir) != "my-game_2" || filepath.Base(filepath.Dir(dir)) != "gozen" || !strings.HasPrefix(dir, cfg) {
		t.Fatalf("save directory %q is not under the config directory %q", dir, cfg)
	}
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		t.Fatalf("the directory was not made: %v", err)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0o700 {
		t.Errorf("save directory mode %v, want 0700", st.Mode().Perm())
	}
}

func TestPrepareRejectsUnportableIDs(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"日本語", "ミア", "é", "🌟", "a.b", "a*b", "a\x00b", "a\nb", strings.Repeat("x", 8) + "/"} {
		if _, err := savefile.Prepare(id, savefile.Config{Root: root}); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("id %q: %v", id, err)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("a refused id made %d entries", len(entries))
	}
	// the Windows device names are refused in any case, but COM10 is an ordinary name
	if _, err := savefile.Prepare("COM10", savefile.Config{Root: root}); err != nil {
		t.Errorf("COM10: %v", err)
	}
}

func TestPrepareFailsWhenTheRootIsAFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(root, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := savefile.Prepare("game", savefile.Config{Root: root}); err == nil {
		t.Fatal("a file where the save root should be a directory was accepted")
	}
}

func TestLoadReadsOnlyValidPaths(t *testing.T) {
	fsys := fstest.MapFS{"a/save.json": {Data: []byte("1")}}
	if b, err := savefile.Load(fsys, "a/save.json"); err != nil || string(b) != "1" {
		t.Fatalf("%q %v", b, err)
	}
	for _, p := range []string{"", "/a/save.json", "../a", "a/../a/save.json", "a//save.json"} {
		if _, err := savefile.Load(fsys, p); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("%q: %v", p, err)
		}
	}
	if _, err := savefile.Load(nil, "a"); !errors.Is(err, fs.ErrInvalid) {
		t.Error("a nil file system is invalid")
	}
	if _, err := savefile.Load(fsys, "a/none.json"); !errors.Is(err, fs.ErrNotExist) {
		t.Error("a missing file is fs.ErrNotExist")
	}
}

// TestWriteCreatesParentsAndReplaces: a write makes the parent directories, leaves no temporary file, and replaces the old bytes whole.
func TestWriteCreatesParentsAndReplaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "save.json")
	for _, content := range []string{strings.Repeat("long", 5000), "short", ""} {
		if err := savefile.Write(path, []byte(content)); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(path) //nolint:gosec // Path is isolated in t.TempDir.
		if err != nil || string(b) != content {
			t.Fatalf("the file holds %d bytes, want %d (%v)", len(b), len(content), err)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("a temporary file was left behind: %v %v", entries, err)
	}
}

func TestWriteFailsWhereItCannotWrite(t *testing.T) {
	skipIfRoot(t)
	dir := t.TempDir()
	// a parent that is a file
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := savefile.Write(filepath.Join(blocker, "save.json"), []byte("s")); err == nil {
		t.Error("a write below a file succeeded")
	}
	// a read-only directory: the temporary file cannot be made, the old file stays
	ro := filepath.Join(dir, "ro")
	if err := os.Mkdir(ro, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(ro, "save.json")
	if err := savefile.Write(target, []byte("old")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ro, 0o500); err != nil { //nolint:gosec // A test directory.
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(ro, 0o700); err != nil { //nolint:gosec // A test directory.
			t.Error(err)
		}
	})
	if err := savefile.Write(target, []byte("new")); err == nil {
		t.Error("a write into a read-only directory succeeded")
	}
	if b, err := os.ReadFile(target); err != nil || string(b) != "old" { //nolint:gosec // Path is isolated in t.TempDir.
		t.Errorf("a failed write changed the old file: %q", b)
	}
	// replacing a directory fails at the rename and cleans its temporary file
	occupied := filepath.Join(dir, "occupied")
	if err := os.MkdirAll(filepath.Join(occupied, "child"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := savefile.Write(occupied, []byte("s")); err == nil {
		t.Error("a write over a directory succeeded")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".save-") {
			t.Errorf("a temporary file was left: %s", e.Name())
		}
	}
}

func TestWritePreservingOfAMissingFileJustWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new", "save.json")
	backup, err := savefile.WritePreserving(path, []byte("first"))
	if err != nil || backup != "" {
		t.Fatalf("backup %q err %v: there is nothing to preserve", backup, err)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != "first" { //nolint:gosec // Path is isolated in t.TempDir.
		t.Errorf("%q", b)
	}
}

func TestWritePreservingFailures(t *testing.T) {
	dir := t.TempDir()
	// the original is a directory: it cannot be read
	d := filepath.Join(dir, "dir")
	if err := os.Mkdir(d, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := savefile.WritePreserving(d, []byte("x")); err == nil {
		t.Error("a directory cannot be preserved")
	}
	// the backup name is taken by a directory: its content cannot be checked
	path := filepath.Join(dir, "save.json")
	old := []byte("old bytes")
	if err := os.WriteFile(path, old, 0o600); err != nil {
		t.Fatal(err)
	}
	backup := fmt.Sprintf("%s.before-%x.bak", path, sha256.Sum256(old))
	if err := os.Mkdir(backup, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := savefile.WritePreserving(path, []byte("new")); err == nil {
		t.Error("an unreadable backup was accepted")
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != string(old) { //nolint:gosec // Path is isolated in t.TempDir.
		t.Errorf("the original changed: %q", b)
	}
	// the final write fails: the backup is reported and kept
	if err := os.Remove(backup); err != nil {
		t.Fatal(err)
	}
	skipIfRoot(t)
	if err := os.Chmod(dir, 0o500); err != nil { //nolint:gosec // A test directory.
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // A test directory.
			t.Error(err)
		}
	})
	if _, err := savefile.WritePreserving(path, []byte("new")); err == nil {
		t.Error("a write into a read-only directory succeeded")
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != string(old) { //nolint:gosec // Path is isolated in t.TempDir.
		t.Errorf("a failed write changed the original: %q", b)
	}
}
