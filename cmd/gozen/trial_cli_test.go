package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/diag"
)

func TestPackTrialOverlayReplacesApprovedAssets(t *testing.T) {
	dir := fixture(t)
	top, inspect := approvals(t, dir, true)
	out := filepath.Join(t.TempDir(), "trial")
	stdout, _ := invoke(t, 0, "trial", dir, "--out", out, "--overlay", top, "--inspect", inspect)
	if !strings.Contains(stdout, "trial: ") || !strings.Contains(stdout, "assets replaced (0 required for the trial)") {
		t.Fatalf("stdout: %q", stdout)
	}
	if _, err := os.Stat(filepath.Join(out, "assets", "mote.png")); err != nil {
		t.Fatalf("the approved asset is missing from the copy: %v", err)
	}
	// An unapproved one is a required asset the overlay does not supply: nothing is written.
	top, inspect = approvals(t, dir, false)
	bad := filepath.Join(t.TempDir(), "rejected")
	invoke(t, 1, "trial", dir, "--out", bad, "--overlay", top, "--inspect", inspect)
	if _, err := os.Lstat(bad); err == nil {
		t.Error("a rejected trial produced output")
	}
	// The pair of options go together, and --dry-run needs no destination.
	_, stderr := invoke(t, 2, "trial", dir, "--out", filepath.Join(t.TempDir(), "x"), "--overlay", top)
	if !strings.Contains(stderr, "--overlay and --inspect must be supplied together") {
		t.Errorf("stderr: %q", stderr)
	}
	stdout, _ = invoke(t, 0, "trial", dir, "-n")
	if !strings.HasPrefix(stdout, "trial: ") {
		t.Errorf("dry run: %q", stdout)
	}
}

func TestPackTrialListAndConfigDiagnostics(t *testing.T) {
	dir := fixture(t)
	list := filepath.Join(t.TempDir(), "deep", "list.txt")
	out := filepath.Join(t.TempDir(), "trial")
	invoke(t, 0, "trial", dir, "--out", out, "--list", list)
	text := string(read(t, list))
	for _, want := range []string{"# events: kept ", "# why kept (kept <- referenced by)", "# ranges of variables and flags", "# pictures and sounds "} {
		if !strings.Contains(text, want) {
			t.Errorf("list lacks %q:\n%s", want, text)
		}
	}
	// A list never overwrites, and never lands inside the game.
	invoke(t, 1, "trial", dir, "--out", filepath.Join(t.TempDir(), "again"), "--list", list)
	invoke(t, 1, "trial", dir, "--out", filepath.Join(t.TempDir(), "again"), "--list", filepath.Join(dir, "list.txt"))

	// Explicit configuration paths are used as given; the game is a separate input.
	write(t, filepath.Join(dir, "other.yaml"), "cut: [finish]\nblocked: [{say: Stop.}]\n")
	cut := filepath.Join(t.TempDir(), "cut")
	invoke(t, 0, "trial", dir, "--out", cut, "--config", filepath.Join(dir, "other.yaml"))
	if strings.Contains(string(read(t, filepath.Join(cut, "data", "world.yaml"))), "The markers are all in place.") {
		t.Error("a cut event is still in the copy")
	}
	for _, tc := range []struct {
		name, config string
		code         diag.Code
	}{
		{"unknown key", "surprise: 1\n", diag.TrialConfig},
		{"not yaml", "assume: [\n", diag.TrialConfig},
		{"dive_max without blocked", "dive_max: {grotto: 2}\n", diag.TrialConfig},
	} {
		write(t, filepath.Join(dir, "bad.yaml"), tc.config)
		_, stderr := invoke(t, 1, "trial", dir, "--out", filepath.Join(t.TempDir(), "never"), "--config", filepath.Join(dir, "bad.yaml"))
		if !strings.Contains(stderr, tc.code.String()) {
			t.Errorf("%s: want %s in %q", tc.name, tc.code, stderr)
		}
	}
	_, stderr := invoke(t, 1, "trial", dir, "--out", filepath.Join(t.TempDir(), "never"), "--config", filepath.Join(dir, "absent.yaml"))
	if !strings.Contains(stderr, "absent.yaml") {
		t.Errorf("a missing configuration: %q", stderr)
	}
	// A file the data names but the game lacks is reported by the dry run and stops the copy.
	write(t, filepath.Join(dir, "keepmissing.yaml"), "keep: [lost/file.png]\n")
	stdout, _ := invoke(t, 0, "trial", dir, "-n", "--config", filepath.Join(dir, "keepmissing.yaml"))
	if !strings.Contains(stdout, "1 files named in the data are missing: lost/file.png") {
		t.Errorf("dry run: %q", stdout)
	}
	_, stderr = invoke(t, 1, "trial", dir, "--out", filepath.Join(t.TempDir(), "never"), "--config", filepath.Join(dir, "keepmissing.yaml"))
	if !strings.Contains(stderr, "lost/file.png") {
		t.Errorf("stderr: %q", stderr)
	}
}
