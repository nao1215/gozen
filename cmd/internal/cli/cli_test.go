package cli

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseFlagsOperandsAndLiteralPaths(t *testing.T) {
	for _, args := range [][]string{{"sample dir", "-n", "2", "troop"}, {"-n=2", "sample dir", "troop"}} {
		f := flag.NewFlagSet("rpgtool sim", flag.ContinueOnError)
		n := f.Int("n", 1, "runs")
		out := &Output{Writer: io.Discard}
		if err := Parse(f, args, "test", "none", out); err != nil {
			t.Fatal(err)
		}
		if *n != 2 || f.NArg() != 2 || f.Arg(0) != "sample dir" {
			t.Fatal(f.Args())
		}
	}
	f := flag.NewFlagSet("test", flag.ContinueOnError)
	out := &Output{Writer: io.Discard}
	if err := Parse(f, []string{"--", "--help"}, "test", "none", out); err != nil || f.Arg(0) != "--help" {
		t.Fatalf("%v %v", err, f.Args())
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

type shortWriter struct{}

func (shortWriter) Write(b []byte) (int, error) { return len(b) / 2, nil }
func TestOutputShortWriteAndWriteNew(t *testing.T) {
	out := &Output{Writer: shortWriter{}}
	out.Println("hello")
	if !errors.Is(out.Err, io.ErrShortWrite) {
		t.Fatal(out.Err)
	}
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
