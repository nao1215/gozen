package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/diag"
)

func TestDestinationReportsTheCodeOfWhatIsWrong(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	if err := os.Mkdir(src, 0o750); err != nil {
		t.Fatal(err)
	}
	taken := filepath.Join(root, "taken")
	if err := os.Mkdir(taken, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := Destination(taken); diag.Of(err) != diag.OutputExists || !strings.Contains(err.Error(), taken) {
		t.Errorf("an existing output: %v", err)
	}
	if err := Destination(filepath.Join(src, "inside"), src); diag.Of(err) != diag.OutputOverlaps {
		t.Errorf("inside the input: %v", err)
	}
	if err := Destination(filepath.Join(root, "ancestor-of"), filepath.Join(root, "ancestor-of", "deeper")); diag.Of(err) != diag.OutputOverlaps {
		t.Errorf("an output that contains an input: %v", err)
	}
	if err := Destination(filepath.Join(root, "fresh"), src, "", filepath.Join(root, "other")); err != nil {
		t.Errorf("an unrelated output, with an empty protected path skipped: %v", err)
	}
	// A parent that is a file is not "not found": the failure is reported as it is.
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// Windows reports a path below a file as not found, so the output is accepted there.
	if err := Destination(filepath.Join(file, "child")); err == nil && runtime.GOOS != "windows" {
		t.Error("an output below a file was accepted")
	}
}

func TestCanonicalResolvesTheExistingPrefix(t *testing.T) {
	if _, err := Canonical(""); err == nil {
		t.Error("an empty path was resolved")
	}
	root := t.TempDir()
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Canonical(filepath.Join(root, "a", "b", "c"))
	if err != nil || got != filepath.Join(want, "a", "b", "c") {
		t.Errorf("a path that does not exist keeps its tail: %q %v", got, err)
	}
	if runtime.GOOS != "windows" {
		target := filepath.Join(root, "target")
		if err := os.Mkdir(target, 0o750); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(root, "link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		a, errA := Canonical(filepath.Join(link, "new"))
		b, errB := Canonical(filepath.Join(target, "new"))
		if errA != nil || errB != nil {
			t.Fatal(errA, errB)
		}
		if a != b {
			t.Errorf("a symlink and its target differ: %q vs %q", a, b)
		}
		// A loop of links cannot be resolved, and says so.
		loop := filepath.Join(root, "loop")
		if err := os.Symlink(loop, loop); err != nil {
			t.Fatal(err)
		}
		if _, err := Canonical(filepath.Join(loop, "x")); err == nil {
			t.Error("a symlink loop was resolved")
		}
	}
	rel, err := Canonical("some/relative")
	if err != nil || !filepath.IsAbs(rel) {
		t.Errorf("a relative path becomes absolute: %q %v", rel, err)
	}
}

func TestPublishFailsCleanlyWhenTheParentIsNotADirectory(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	err := Publish(filepath.Join(file, "out"), nil, func(string) error { called = true; return nil })
	if err == nil || called {
		t.Errorf("an output below a file: err %v, built %v", err, called)
	}
	// A destination in a new parent is created together with it, and the staging directory does not stay.
	out := filepath.Join(root, "new", "parent", "out")
	if err := Publish(out, nil, func(stage string) error { return WriteNew(filepath.Join(stage, "sub", "f.txt"), []byte("x")) }); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "sub", "f.txt")) //nolint:gosec // Test-owned path under t.TempDir.
	if err != nil || string(b) != "x" {
		t.Errorf("%q %v", b, err)
	}
	ents, err := os.ReadDir(filepath.Dir(out))
	if err != nil || len(ents) != 1 || ents[0].Name() != "out" {
		t.Errorf("only the output is left in its parent: %v %v", ents, err)
	}
}

// Directories that cannot be entered or written are reported as they are, and nothing is left behind.
func TestPublishAndDestinationReportUnwritableDirectories(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions do not stop this user")
	}
	root := t.TempDir()
	readOnly := filepath.Join(root, "readonly")
	if err := os.Mkdir(readOnly, 0o500); err != nil {
		t.Fatal(err)
	}
	sealed := filepath.Join(root, "sealed")
	if err := os.Mkdir(sealed, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(sealed, 0o700); err != nil { //nolint:gosec // The test directory is made usable again so it can be removed.
			t.Error(err)
		}
		if err := os.Chmod(readOnly, 0o700); err != nil { //nolint:gosec // The test directory is made usable again so it can be removed.
			t.Error(err)
		}
	})
	built := false
	build := func(string) error { built = true; return nil }
	// The parent exists but takes no new entry: the staging directory cannot be made.
	if err := Publish(filepath.Join(readOnly, "out"), nil, build); err == nil || built {
		t.Errorf("a read-only parent: err %v, built %v", err, built)
	}
	// The parent itself would have to be made.
	if err := Publish(filepath.Join(readOnly, "sub", "out"), nil, build); err == nil || built {
		t.Errorf("a parent that cannot be made: err %v, built %v", err, built)
	}
	// A protected path that cannot be resolved is an error, not "no overlap".
	if err := Destination(filepath.Join(root, "fresh"), filepath.Join(sealed, "inside")); err == nil || diag.Of(err) == diag.OutputOverlaps {
		t.Errorf("an unreadable protected path: %v", err)
	}
	if err := Destination(filepath.Join(sealed, "inside", "out")); err == nil {
		t.Error("an output below a directory that cannot be entered was accepted")
	}
	if _, err := os.Lstat(filepath.Join(root, "fresh")); err == nil {
		t.Error("a refused destination was created")
	}
}

func TestWriteNewCreatesParentsAndRefusesBadPlaces(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "a", "b", "c.txt")
	if err := WriteNew(p, []byte("hi")); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteNew(filepath.Join(file, "x"), []byte("hi")); err == nil {
		t.Error("a file below a file was written")
	}
	if err := WriteNew(root, []byte("hi")); err == nil {
		t.Error("a directory was written as a file")
	}
}

func TestPortablePaths(t *testing.T) {
	for _, p := range []string{"../file", "/file", "C:/file", `a\file`, "a//b", "a/CON.txt", "a/nul", "x.", "x ", "a\x00b", "COM1", "a:b"} {
		if Relative(p) {
			t.Errorf("accepted %q", p)
		}
	}
	for _, p := range []string{".", "a/b.yaml", "camp 日本語/file.png"} {
		if !Relative(p) {
			t.Errorf("rejected %q", p)
		}
	}
	if ID("a/b") || ID(".") || !ID("camp-1") {
		t.Fatal("invalid file ID policy")
	}
}

func TestPublishPreservesExistingAndRejectsOverlaps(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	if err := os.Mkdir(src, 0o750); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "output")
	build := func(stage string) error { return os.WriteFile(filepath.Join(stage, "marker"), []byte("kept"), 0o600) }
	if err := Publish(output, []string{src}, build); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{output, filepath.Join(src, "inside"), root} {
		if err := Publish(p, []string{src}, build); err == nil {
			t.Fatalf("accepted %s", p)
		}
	}
	b, err := os.ReadFile(filepath.Join(output, "marker")) //nolint:gosec // Test-owned path under t.TempDir.
	if err != nil || string(b) != "kept" {
		t.Fatalf("%s %v", b, err)
	}
	failure := errors.New("build failed")
	failed := filepath.Join(root, "failed")
	if err := Publish(failed, nil, func(stage string) error {
		if err := build(stage); err != nil {
			return err
		}
		return failure
	}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if _, err := os.Lstat(failed); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed build leaked destination")
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(root, "link")
		if err := os.Symlink(src, link); err != nil {
			t.Fatal(err)
		}
		if err := Destination(filepath.Join(link, "new"), src); err == nil {
			t.Fatal("symlink bypassed overlap check")
		}
	}
}

func TestWriteNew(t *testing.T) {
	p := filepath.Join(t.TempDir(), "new.txt")
	if err := WriteNew(p, []byte("keep")); err != nil {
		t.Fatal(err)
	}
	if err := WriteNew(p, []byte("replace")); !errors.Is(err, os.ErrExist) {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p) //nolint:gosec // Test-owned path under t.TempDir.
	if err != nil || !bytes.Equal(b, []byte("keep")) {
		t.Fatalf("%s %v", b, err)
	}
}
