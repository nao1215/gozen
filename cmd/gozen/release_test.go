package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/diag"
)

func TestPackReleaseTakesItsOverlayFromTheGamePolicy(t *testing.T) {
	dir := fixture(t)
	top, inspect := approvals(t, dir, true)
	// The overlay folder lives in the game; the policy names it, so --overlay is not needed.
	if err := os.CopyFS(filepath.Join(dir, "ovl"), os.DirFS(top)); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "asset-policy.yaml"), "overlay: ovl\nreport: approvals.json\ninspection: {file: file, source: source, required: required, approved: approved}\nrecords: {file: file, source: source, required: required, approved: approved, output: output}\n")
	out := filepath.Join(t.TempDir(), "release")
	stdout, _ := invoke(t, 0, "pack", "release", dir, out, "--inspect", inspect)
	if !strings.Contains(stdout, out+": ") || !strings.Contains(stdout, "(1 approval records)") {
		t.Errorf("stdout: %q", stdout)
	}
	if _, err := os.Stat(filepath.Join(out, "asset-policy.yaml")); err == nil {
		t.Error("the policy file is not part of the release")
	}
	read(t, filepath.Join(out, "game.yaml"))
	// The output is never overwritten, nor placed inside the game.
	invoke(t, 1, "pack", "release", dir, out, "--inspect", inspect)
	invoke(t, 1, "pack", "release", dir, filepath.Join(dir, "out"), "--inspect", inspect)
}

func TestPackReleaseDiagnosesWhatIsMissing(t *testing.T) {
	dir := fixture(t)
	top, inspect := approvals(t, dir, true)
	release := func(want int, extra ...string) string {
		t.Helper()
		_, stderr := invoke(t, want, append([]string{"pack", "release", dir, filepath.Join(t.TempDir(), "r")}, extra...)...)
		return stderr
	}
	if stderr := release(2); !strings.Contains(stderr, "usage: gozen pack release --inspect FILE") || !strings.Contains(stderr, diag.OptionRequired.String()) {
		t.Errorf("no --inspect: %q", stderr)
	}
	if stderr := release(2, "--inspect", inspect, "--overlay", top, "--prefix", "../assets"); !strings.Contains(stderr, "invalid --prefix") || !strings.Contains(stderr, diag.BadPath.String()) {
		t.Errorf("a prefix outside the game: %q", stderr)
	}
	// With neither --overlay nor a policy there is no replacement directory.
	if stderr := release(1, "--inspect", inspect); !strings.Contains(stderr, "give the replacement directory with --overlay or in the game's asset policy") || !strings.Contains(stderr, diag.ApprovalRecords.String()) {
		t.Errorf("no overlay: %q", stderr)
	}
	if stderr := release(1, "--inspect", filepath.Join(t.TempDir(), "absent.jsonl"), "--overlay", top); !strings.Contains(stderr, diag.ApprovalRecords.String()) {
		t.Errorf("absent inspections: %q", stderr)
	}
	write(t, filepath.Join(dir, "asset-policy.yaml"), "nonsense: true\n")
	if stderr := release(1, "--inspect", inspect, "--overlay", top); !strings.Contains(stderr, diag.ApprovalRecords.String()) {
		t.Errorf("a policy that does not parse: %q", stderr)
	}
	if err := os.Remove(filepath.Join(dir, "asset-policy.yaml")); err != nil {
		t.Fatal(err)
	}
	// A picture the data names but the folder lacks.
	if err := os.Remove(filepath.Join(dir, "assets", "mote.png")); err != nil {
		t.Fatal(err)
	}
	if stderr := release(1, "--inspect", inspect, "--overlay", top); !strings.Contains(stderr, "referenced assets are missing: [") || !strings.Contains(stderr, "mote.png") || !strings.Contains(stderr, diag.ReferencedAssetMissing.String()) {
		t.Errorf("a missing asset: %q", stderr)
	}
}

func TestPackReleaseRefusesWhatIsNotApproved(t *testing.T) {
	dir := fixture(t)
	top, inspect := approvals(t, dir, false)
	out := filepath.Join(t.TempDir(), "r")
	_, stderr := invoke(t, 1, "pack", "release", dir, out, "--inspect", inspect, "--overlay", top)
	if !strings.Contains(stderr, diag.Unapproved.String()) || !strings.Contains(stderr, "mote.png") {
		t.Errorf("stderr: %q", stderr)
	}
	if _, err := os.Lstat(out); err == nil {
		t.Error("a release with an unapproved picture produced output")
	}
}
