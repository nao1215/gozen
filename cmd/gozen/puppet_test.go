package main

import (
	"os"
	"path/filepath"
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
	out, stderr := invoke(t, 0, "art", "puppet", filepath.Join(dir, "puppet.yaml"))
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
	out, _ := invoke(t, 0, "art", "puppet", filepath.Join(dir, "puppet.yaml"), "--out", packed)
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
	orig, _ := invoke(t, 0, "art", "puppet", filepath.Join(dir, "puppet.yaml"))
	again, _ := invoke(t, 0, "art", "puppet", filepath.Join(packed, "puppet.yaml"))
	if !strings.Contains(again, "  mouth 2: at [26, 32] size 12x6\n") || strings.Count(again, "\n") != strings.Count(orig, "\n") {
		t.Errorf("packed:\n%s\noriginal:\n%s", again, orig)
	}
	// An output is never overwritten.
	invoke(t, 1, "art", "puppet", filepath.Join(dir, "puppet.yaml"), "--out", packed)
}

func TestArtPuppetRejectsBadInput(t *testing.T) {
	dir := puppetDir(t)
	invoke(t, 2, "art", "puppet")
	invoke(t, 2, "art", "puppet", filepath.Join(dir, "puppet.yaml"), filepath.Join(dir, "puppet.yaml"))
	invoke(t, 1, "art", "puppet", filepath.Join(dir, "absent.yaml"))
	write(t, filepath.Join(dir, "broken.yaml"), "base: [")
	invoke(t, 1, "art", "puppet", filepath.Join(dir, "broken.yaml"))
	// A picture the spec names but the folder lacks.
	if err := os.Remove(filepath.Join(dir, "mouth.png")); err != nil {
		t.Fatal(err)
	}
	_, stderr := invoke(t, 1, "art", "puppet", filepath.Join(dir, "puppet.yaml"))
	if !strings.Contains(stderr, "mouth.png") {
		t.Errorf("stderr: %q", stderr)
	}
	// A bare file name is read from the current directory.
	t.Chdir(dir)
	write(t, filepath.Join(dir, "broken.yaml"), "base: [")
	invoke(t, 1, "art", "puppet", "broken.yaml")
}
