package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestRuntimePathSelectsWhatShips(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		dir  bool
		want bool
	}{
		{"game.yaml", false, true},
		{"LICENSE", false, true},
		{"NOTICE.txt", false, true},
		{"ASSETS.md", false, true},
		{"data", true, true},
		{"data/world.yaml", false, true},
		{"data/sub/more.yml", false, true},
		{"lang/ja.yaml", false, true},
		{"streams/host/001.yaml", false, true},
		{"LICENSES/font.txt", false, true},
		{"LICENSES/anything.bin", false, true},
		{"assets/mote.png", false, true},
		{"assets/sub/NOTICE", false, true},
		{"assets/LICENSE-font", false, true},
		{"assets/faces.json", false, true},
		{"assets/credits.md", false, true},
		{"assets/font.TTF", false, true},
		{"asset-policy.yaml", false, false},
		{"trial.yaml", false, false},
		{"main.go", false, false},
		{"saves", true, false},
		{"data/debug.go", false, false},
		{"lang/notes.txt", false, false},
		{"assets/workflow.json", false, false},
		{"assets/source.psd", false, false},
		{"assets/.hidden.png", false, false},
		{".git", true, false},
		{"data/.cache/x.yaml", false, false},
		{"sub/game.yaml", false, false},
		{"game.yaml", true, false},
	} {
		if got := RuntimePath(tc.name, tc.dir); got != tc.want {
			t.Errorf("RuntimePath(%q, %v) = %v", tc.name, tc.dir, got)
		}
	}
}

func TestWalkRuntimeVisitsOnlyWhatShips(t *testing.T) {
	t.Parallel()
	tree := fstest.MapFS{}
	for _, name := range []string{"game.yaml", "data/world.yaml", "data/debug.go", "assets/a.png", "assets/raw/a.psd", "saves/1.json", ".git/config", "lang/ja.yaml"} {
		tree[name] = &fstest.MapFile{Data: []byte("x")}
	}
	var got []string
	if err := WalkRuntime(tree, func(name string, d fs.DirEntry) error {
		if d.IsDir() {
			name += "/"
		}
		got = append(got, name)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"assets/", "assets/a.png", "assets/raw/", "data/", "data/world.yaml", "game.yaml", "lang/", "lang/ja.yaml"}; !slices.Equal(got, want) {
		t.Errorf("visited %v, want %v", got, want)
	}
	tree["assets/bad?.png"] = &fstest.MapFile{Data: []byte("x")}
	if err := WalkRuntime(tree, func(string, fs.DirEntry) error { return nil }); err == nil || !strings.Contains(err.Error(), "non-portable game path: assets/bad?.png") {
		t.Errorf("a name that is not portable: %v", err)
	}
}

// A link is refused wherever it is in the tree, selected or not, and whatever it
// points at; so is a selected file that is not a regular one.
func TestWalkRuntimeAndCopyFileRefuseLinks(t *testing.T) {
	t.Parallel()
	root, outside := t.TempDir(), t.TempDir()
	if err := WriteNew(filepath.Join(outside, "secret.txt"), []byte("outside")); err != nil {
		t.Fatal(err)
	}
	if err := WriteNew(filepath.Join(root, "assets", "a.png"), []byte("picture")); err != nil {
		t.Fatal(err)
	}
	none := func(string, fs.DirEntry) error { return nil }
	for _, link := range []struct{ name, target string }{
		{"assets/leak.txt", filepath.Join(outside, "secret.txt")},
		{"lang", outside},
		{"shots", outside},
	} {
		p := filepath.Join(root, filepath.FromSlash(link.name))
		if err := os.Symlink(link.target, p); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if err := WalkRuntime(os.DirFS(root), none); err == nil || !strings.Contains(err.Error(), "symlinks cannot be exported: "+link.name) {
			t.Errorf("%s: %v", link.name, err)
		}
		dst := filepath.Join(t.TempDir(), "copy")
		if err := CopyFile(os.DirFS(root), link.name, dst, 0o600); err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Errorf("copying %s: %v", link.name, err)
		}
		if _, err := os.Lstat(dst); err == nil {
			t.Errorf("copying %s wrote a file", link.name)
		}
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := WalkRuntime(os.DirFS(root), none); err != nil {
		t.Errorf("a tree without links: %v", err)
	}
}

func TestCopyFileCreatesANewFileOnly(t *testing.T) {
	t.Parallel()
	tree := fstest.MapFS{"bin/player": {Data: []byte("player")}}
	dst := filepath.Join(t.TempDir(), "deep", "player")
	if err := CopyFile(tree, "bin/player", dst, 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dst) //nolint:gosec // Test-owned path under t.TempDir.
	if err != nil || string(b) != "player" {
		t.Fatalf("the copy: %q, %v", b, err)
	}
	if err := CopyFile(tree, "bin/player", dst, 0o755); !errors.Is(err, fs.ErrExist) {
		t.Errorf("a second copy onto the first: %v", err)
	}
	if err := CopyFile(tree, "bin", filepath.Join(t.TempDir(), "dir"), 0o600); err == nil {
		t.Error("a directory was copied as a file")
	}
	if err := CopyFile(tree, "absent", filepath.Join(t.TempDir(), "absent"), 0o600); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a missing source: %v", err)
	}
}
