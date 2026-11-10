package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/cmd/internal/distribute"
	"github.com/nao1215/gozen/internal/base/diag"
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

func TestPackTrialListsTheRangesOfVariables(t *testing.T) {
	dir := designGame(t)
	write(t, filepath.Join(dir, "trial.yaml"), "{}\n")
	list := filepath.Join(t.TempDir(), "list.txt")
	invoke(t, 0, "trial", dir, "--out", filepath.Join(t.TempDir(), "trial"), "--list", list)
	text := string(read(t, list))
	if !strings.Contains(text, "# ranges of variables and flags\n") || !strings.Contains(text, "var.offer = ") || !strings.Contains(text, "<- ") {
		t.Errorf("the ranges of the variables are listed with what sets them:\n%s", text)
	}
}

// A trial made without --overlay is as unapproved as its game: it carries the
// asset policy, so the export gate that stops the game stops the trial too. One
// made with --overlay and --inspect is approved and carries none.
func TestPackTrialCarriesTheAssetPolicyUnlessApproved(t *testing.T) {
	dir := fixture(t)
	const policy = "report: approvals.json\nextensions: [.png]\ninspection: {file: file, source: source, required: required, approved: approved}\nrecords: {file: file, source: source, required: required, approved: approved, output: output}\n"
	write(t, filepath.Join(dir, "asset-policy.yaml"), policy)
	if _, _, err := distribute.CopyGame(t.Context(), dir, t.TempDir()); err == nil {
		t.Fatal("the fixture with a policy passes the export gate")
	}
	plain := filepath.Join(t.TempDir(), "trial")
	invoke(t, 0, "trial", dir, "--out", plain)
	if got := string(read(t, filepath.Join(plain, "asset-policy.yaml"))); got != policy {
		t.Errorf("the policy in the trial: %q", got)
	}
	if _, _, err := distribute.CopyGame(t.Context(), plain, t.TempDir()); err == nil || !strings.Contains(err.Error(), "game has an asset policy") {
		t.Errorf("an unapproved trial passes the export gate: %v", err)
	}
	top, inspect := approvals(t, dir, true)
	approved := filepath.Join(t.TempDir(), "trial")
	invoke(t, 0, "trial", dir, "--out", approved, "--overlay", top, "--inspect", inspect)
	if _, _, err := distribute.CopyGame(t.Context(), approved, t.TempDir()); err != nil {
		t.Errorf("an approved trial does not pass the export gate: %v", err)
	}
}

// A trial that fails leaves nothing behind: no list to make the next run fail
// with "output already exists", and no staging directory.
func TestPackTrialFailureLeavesNothingBehind(t *testing.T) {
	dir := fixture(t)
	work, temp := t.TempDir(), t.TempDir()
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, temp)
	}
	list, out := filepath.Join(work, "list.txt"), filepath.Join(work, "trial")
	missing := filepath.Join(work, "missing.yaml")
	write(t, missing, "keep: [lost/file.png]\n")
	top, inspect := approvals(t, dir, true)
	for _, extra := range [][]string{nil, {"--overlay", top, "--inspect", inspect}} {
		args := append([]string{"trial", dir, "--out", out, "--list", list, "--config", missing}, extra...)
		if _, stderr := invoke(t, 1, args...); !strings.Contains(stderr, "lost/file.png") {
			t.Fatalf("%v: stderr %q", extra, stderr)
		}
		for _, p := range []string{list, out} {
			if _, err := os.Lstat(p); err == nil {
				t.Errorf("%v: a failed trial left %s", extra, p)
			}
		}
		if left, err := os.ReadDir(temp); err != nil || len(left) != 0 {
			t.Errorf("%v: a failed trial left staging files: %v, %v", extra, left, err)
		}
	}
	// The same destinations serve the run that succeeds.
	invoke(t, 0, "trial", dir, "--out", out, "--list", list)
	if !strings.Contains(string(read(t, list)), "# events: kept ") {
		t.Error("the list of the trial that succeeded is not written")
	}
}
