package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func puppetDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "rig")
	if err := os.CopyFS(dir, os.DirFS("../../e2e/atago/fixtures/puppet")); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestArtPuppetDescribesWhereTheOverlaysLie(t *testing.T) {
	dir := puppetDir(t)
	out, stderr := invoke(t, 0, "puppet", filepath.Join(dir, "puppet.yaml"))
	if stderr != "" {
		t.Fatalf("stderr: %q", stderr)
	}
	for _, want := range []string{"base base.png 64x96\n", "face default: the base\n", "  eyes closed: at [", "  mouth 1: the face's own\n", "  mouth 2: at [26, 32] size 12x6\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "packed")); err == nil {
		t.Error("describing wrote files")
	}
}

func TestArtPuppetOutWritesSmallPicturesThatLoadAgain(t *testing.T) {
	dir := puppetDir(t)
	packed := filepath.Join(t.TempDir(), "packed")
	out, _ := invoke(t, 0, "puppet", filepath.Join(dir, "puppet.yaml"), "--out", packed)
	if !strings.Contains(out, "wrote "+filepath.ToSlash(filepath.Join(packed, "puppet.yaml"))) {
		t.Fatalf("stdout: %q", out)
	}
	yaml := string(read(t, filepath.Join(packed, "puppet.yaml")))
	if !strings.Contains(yaml, "image: eyes-default.png") || !strings.Contains(yaml, "at:") {
		t.Errorf("the packed spec names the cut pictures and where they go:\n%s", yaml)
	}
	for _, name := range []string{"base.png", "eyes-default.png", "mouth-default-2.png"} {
		if info, err := os.Stat(filepath.Join(packed, name)); err != nil || info.Size() == 0 {
			t.Errorf("%s: %v", name, err)
		}
	}
	// What was packed describes itself as the original did.
	orig, _ := invoke(t, 0, "puppet", filepath.Join(dir, "puppet.yaml"))
	again, _ := invoke(t, 0, "puppet", filepath.Join(packed, "puppet.yaml"))
	if !strings.Contains(again, "  mouth 2: at [26, 32] size 12x6\n") || strings.Count(again, "\n") != strings.Count(orig, "\n") {
		t.Errorf("packed:\n%s\noriginal:\n%s", again, orig)
	}
	// An output is never overwritten.
	invoke(t, 1, "puppet", filepath.Join(dir, "puppet.yaml"), "--out", packed)
}

// A face's name is copied into the names of the packed pictures: one with ../
// is refused when the puppet is read, and nothing is written outside --out.
func TestPuppetOutStaysInItsDirectory(t *testing.T) {
	dir := puppetDir(t)
	write(t, filepath.Join(dir, "evil.yaml"), "base: base.png\nface: \"x/../../../escaped\"\nfaces:\n  \"x/../../../escaped\":\n    eyes: {closed: closed.png}\n")
	root := t.TempDir()
	packed := filepath.Join(root, "a", "b", "packed")
	_, stderr := invoke(t, 1, "puppet", filepath.Join(dir, "evil.yaml"), "--out", packed)
	if !strings.Contains(stderr, "faces: name") {
		t.Errorf("stderr: %q", stderr)
	}
	for _, p := range []string{filepath.Join(root, "a", "escaped.png"), filepath.Join(root, "a", "eyes-x"), packed} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("gozen puppet --out wrote %s", p)
		}
	}
	if runtime.GOOS == "windows" {
		return // the file below cannot be made there
	}
	// A base picture whose name another system refuses is not packed either.
	write(t, filepath.Join(dir, "a*b.png"), string(read(t, filepath.Join(dir, "base.png"))))
	write(t, filepath.Join(dir, "star.yaml"), "base: \"a*b.png\"\n")
	_, stderr = invoke(t, 1, "puppet", filepath.Join(dir, "star.yaml"), "--out", packed)
	if !strings.Contains(stderr, "a*b.png") || !strings.Contains(stderr, "portable") {
		t.Errorf("stderr: %q", stderr)
	}
	if _, err := os.Stat(packed); err == nil {
		t.Error("a refused puppet left its output directory")
	}
}

func TestArtPuppetRejectsBadInput(t *testing.T) {
	dir := puppetDir(t)
	invoke(t, 2, "art", "puppet")
	invoke(t, 2, "puppet", filepath.Join(dir, "puppet.yaml"), filepath.Join(dir, "puppet.yaml"))
	invoke(t, 1, "puppet", filepath.Join(dir, "absent.yaml"))
	write(t, filepath.Join(dir, "broken.yaml"), "base: [")
	invoke(t, 1, "puppet", filepath.Join(dir, "broken.yaml"))
	// A picture the spec names but the folder lacks.
	if err := os.Remove(filepath.Join(dir, "mouth.png")); err != nil {
		t.Fatal(err)
	}
	_, stderr := invoke(t, 1, "puppet", filepath.Join(dir, "puppet.yaml"))
	if !strings.Contains(stderr, "mouth.png") {
		t.Errorf("stderr: %q", stderr)
	}
	// A bare file name is read from the current directory.
	t.Chdir(dir)
	write(t, filepath.Join(dir, "broken.yaml"), "base: [")
	invoke(t, 1, "puppet", "broken.yaml")
}
