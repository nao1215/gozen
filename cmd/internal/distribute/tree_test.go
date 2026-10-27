package distribute

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/cmd/internal/starter"
)

func TestCopyGameSelectsRuntimeContent(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	if err := starter.Write(src, starter.Config{Template: "rpg", ID: "export-test", Title: "Export test"}); err != nil {
		t.Fatal(err)
	}
	keep := []string{"LICENSE", "LICENSES/pictures.txt", "streams/host/001.yaml", "assets/faces.json", "assets/sub/NOTICE", "assets/credits.md"}
	drop := []string{"main.go", "go.mod", ".git/config", "saves/save1.json", "shots/test.png", "data/debug.go", "streams/notes.go", "assets/model.onnx", "assets/source.psd", "assets/inspection.jsonl", "assets/workflow.json"}
	for _, name := range append(slices.Clone(keep), drop...) {
		if err := cli.WriteNew(filepath.Join(src, filepath.FromSlash(name)), []byte("neutral fixture")); err != nil {
			t.Fatal(err)
		}
	}
	files, err := CopyGame(src, dst)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range append(keep, "game.yaml", "data/world.yaml", "data/rules.yaml", "assets/mote.png") {
		if !slices.Contains(files, name) {
			t.Errorf("missing %s", name)
		}
	}
	for _, name := range drop {
		if _, err := os.Stat(filepath.Join(dst, filepath.FromSlash(name))); !os.IsNotExist(err) {
			t.Errorf("exported author file %s: %v", name, err)
		}
	}
	if !slices.IsSorted(files) {
		t.Fatalf("unsorted manifest: %v", files)
	}
}

func TestCopyGameRejectsPolicyAndSymlink(t *testing.T) {
	src := t.TempDir()
	policy := filepath.Join(src, "asset-policy.yaml")
	if err := os.WriteFile(policy, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CopyGame(src, t.TempDir()); err == nil || !strings.Contains(err.Error(), "release") {
		t.Fatalf("policy: %v", err)
	}
	if err := os.Remove(policy); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(src, "data")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := CopyGame(src, t.TempDir()); err == nil || !strings.Contains(err.Error(), "symlinks") {
		t.Fatalf("symlink: %v", err)
	}
}
