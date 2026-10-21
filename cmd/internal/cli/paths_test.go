package cli

import (
	"errors"
	"io"
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
	if err := Destination(filepath.Join(file, "child")); err == nil {
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

type failingWriter struct{ n int }

func (w *failingWriter) Write(b []byte) (int, error) {
	w.n--
	if w.n < 0 {
		return 0, io.ErrClosedPipe
	}
	return len(b), nil
}

func TestOutputKeepsTheFirstFailureAndStopsWriting(t *testing.T) {
	w := &failingWriter{n: 1}
	out := &Output{Writer: w}
	out.Print("a")
	if out.Err != nil {
		t.Fatalf("the first write succeeds: %v", out.Err)
	}
	out.Printf("%d", 2)
	if !errors.Is(out.Err, io.ErrClosedPipe) {
		t.Fatalf("the second write fails: %v", out.Err)
	}
	first := out.Err
	out.Println("x")
	out.Print("y")
	out.Printf("z")
	if !errors.Is(out.Err, first) || w.n != -1 {
		t.Errorf("after a failure nothing more is written: err %v, calls left %d", out.Err, w.n)
	}
	if n, err := out.Write([]byte("w")); n != 0 || !errors.Is(err, first) {
		t.Errorf("Write after a failure: %d %v", n, err)
	}
}

func TestUsageErrorsCarryTheirCodes(t *testing.T) {
	err := Invalid(diag.Operands, "expected %d args", 2)
	var u *UsageError
	if !errors.As(err, &u) || u.Code != diag.Operands || u.Message != "expected 2 args" {
		t.Fatalf("%#v", err)
	}
	if got := err.Error(); got != "expected 2 args ["+diag.Operands.String()+"]" {
		t.Errorf("the message ends with the code: %q", got)
	}
	coded := Parse(diag.UnknownOption.Errorf("unknown option --zzz"))
	if !errors.As(coded, &u) || u.Code != diag.UnknownOption || !strings.Contains(u.Message, "--zzz") || strings.Contains(u.Message, "GZN") {
		t.Errorf("a coded parse error keeps its code and its bare text: %#v", coded)
	}
	plain := Parse(errors.New("something else"))
	if !errors.As(plain, &u) || u.Code != diag.BadValue || u.Message != "something else" {
		t.Errorf("an error with no code is a bad value: %#v", plain)
	}
}
