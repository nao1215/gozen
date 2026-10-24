package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlayRunArguments(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"one", "two"},
		{""},
		{"--dir", "game"},
		{"game", "--start", "camp"},
		{"game", "--dev", "--script", "missing"},
		{"--dev", "game", "--review"},
		{"--seed", "bad", "game"},
		{"game", "-seed", "2"},
		{"game", "--window", "-1"},
		{"game", "--dev", "--replay-battle", "1"},
		{"game", "--dev", "--record", "unused.mp4"},
		{"--dev", "game", "--answers", "x"},
	} {
		out, stderr := invoke(t, 2, append([]string{"play", "run"}, args...)...)
		if out != "" || !strings.Contains(stderr, `Run "gozen play run --help" for usage.`) {
			t.Fatalf("%v: %q %q", args, out, stderr)
		}
	}
}

func TestPlayRunLoadsTheOperandAndReportsIOFailures(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "game with spaces")
	write(t, filepath.Join(dir, "game.yaml"), "title: [unclosed")
	for _, args := range [][]string{
		{dir, "--mute"},
		{"--mute", "--seed=7", dir},
		{"--", dir},
	} {
		out, stderr := invoke(t, 1, append([]string{"play", "run"}, args...)...)
		if out != "" || !strings.Contains(stderr, "game.yaml") {
			t.Fatalf("%v: %q %q", args, out, stderr)
		}
	}
	invoke(t, 1, "play", "run", filepath.Join(t.TempDir(), "missing"))
	invoke(t, 1, "play", "run", dir, "--dev", "--script", filepath.Join(t.TempDir(), "absent"), "--shots", t.TempDir())
	var stderr bytes.Buffer
	if code := run([]string{"play", "run", "--help"}, &failAfter{}, &stderr); code != 1 {
		t.Fatalf("failed help write: exit %d", code)
	}
}

func TestPlayRunUnknownGameNames(t *testing.T) {
	dir := fixture(t)
	for _, args := range [][]string{
		{"--start", "absent"},
		{"--start", "camp", "--battle", "absent"},
		{"--start", "camp", "--party", "absent"},
		{"--start", "camp", "--items", "absent"},
	} {
		out, stderr := invoke(t, 2, append([]string{"play", "run", dir, "--dev", "--mute"}, args...)...)
		if out != "" || !strings.Contains(stderr, "unknown") {
			t.Fatalf("%v: %q %q", args, out, stderr)
		}
	}
}
