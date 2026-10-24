package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Whatever is given as an operand or an option value, a command ends with a status and a
// message; it never panics, and it writes nothing to stdout for input it refused.
func TestCommandsRefuseOddInputWithoutPanicking(t *testing.T) {
	dir := designGame(t)
	file := filepath.Join(t.TempDir(), "plain-file")
	write(t, file, "just a file")
	empty := t.TempDir()
	weird := []string{"", " ", "-", "--", "\x00", "../..", "日本語/😀", "a\nb", strings.Repeat("x", 4096), file, empty, filepath.Join(file, "child")}
	for _, bad := range weird {
		for _, cmd := range [][]string{
			{"check", "--"},
			{"check", "--only", "structure", "--"},
			{"check", "--format", "json", "--"},
			{"book", "--"},
			{"sim", "battle", "--troop", "TROOP", "--"},
			{"sim", "dive", "--location", "cave", "--"},
			{"sim", "floors", "--location", "cave", "--"},
			{"sim", "design", "--list", "--"},
			{"trace", "--"},
			{"notes", "--"},
			{"trial", "-n", "--"},
			{"puppet", "--"},
			{"faces", "--"},
		} {
			// The operand is placed after "--" so a value starting with a dash is an operand.
			args := append(append([]string{}, cmd...), bad)
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("%q panicked: %v", args, r)
					}
				}()
				var out, errOut strings.Builder
				got := run(args, &out, &errOut)
				if got != 0 && errOut.Len() == 0 && out.Len() == 0 {
					t.Errorf("%q failed with %d and said nothing", args, got)
				}
				if got != 0 && out.Len() != 0 && cmd[0] != "check" {
					t.Errorf("%q wrote stdout (%q) and failed with %d", args, out.String(), got)
				}
			}()
		}
	}
	// Numbers that do not fit or do not parse are usage errors.
	for _, args := range [][]string{
		{"sim", "battle", dir, "--troop", "lesson", "-n", "99999999999999999999"},
		{"sim", "battle", dir, "--troop", "lesson", "-n", "1e3"},
		{"sim", "battle", dir, "--troop", "lesson", "--level", "1.5"},
		{"sim", "battle", dir, "--troop", "lesson", "--seed", "-5"},
		{"sim", "battle", dir, "--troop", "lesson", "--turns", ""},
		{"sim", "dive", dir, "--location", "cave", "--grind", "abc"},
		{"sim", "floors", dir, "--location", "cave", "--from", "--to"},
		{"trace", file, "--from", "1,5"},
		{"play", "shots", dir, file, "--bin", file, "--raw", "--timeout", "forever"},
		{"sim", "design", dir, "--seed", "-1"},
	} {
		out, stderr := invoke(t, 2, args...)
		if out != "" || stderr == "" {
			t.Errorf("%q: %q %q", args, out, stderr)
		}
	}
	// A game directory that is a file, or is empty, is unreadable (GZN1110).
	for _, d := range []string{file, empty, filepath.Join(empty, "absent")} {
		for _, args := range [][]string{{"check", d}, {"book", d}, {"sim", "battle", d, "--troop", "x"}, {"sim", "dive", d, "--location", "x"}, {"sim", "floors", d, "--location", "x"}, {"sim", "design", d}} {
			_, stderr := invoke(t, 1, args...)
			if !strings.Contains(stderr, "GZN1110") {
				t.Errorf("%q: %q", args, stderr)
			}
		}
	}
	// The files given stay as they were.
	if b, err := os.ReadFile(file); err != nil || string(b) != "just a file" { //nolint:gosec // Test-owned path under t.TempDir.
		t.Errorf("%q %v", b, err)
	}
}
