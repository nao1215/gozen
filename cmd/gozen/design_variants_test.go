package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// edit replaces text in a file of the game once and fails if it is not there.
func edit(t *testing.T, path, old, repl string) {
	t.Helper()
	s := string(read(t, path))
	if !strings.Contains(s, old) {
		t.Fatalf("%s lacks %q", path, old)
	}
	write(t, path, strings.Replace(s, old, repl, 1))
}

func designReportOf(t *testing.T, want int, args ...string) (report, stderr string) {
	t.Helper()
	out := filepath.Join(t.TempDir(), "run")
	stdout, stderr := invoke(t, want, append([]string{"sim", "design", "-o", out}, args...)...)
	if want == 0 && strings.TrimSpace(stdout) != filepath.Join(out, "report.md") {
		t.Fatalf("stdout %q", stdout)
	}
	return string(read(t, filepath.Join(out, "report.md"))), stderr
}

// A guardian nobody can beat: nobody reaches the end, the targets are marked, and the table
// says how the fights went.
func TestDesignReportsPersonasThatDoNotReachTheEnd(t *testing.T) {
	dir := designGame(t)
	edit(t, filepath.Join(dir, "data", "world.yaml"), "stats: {mhp: 400, mmp: 0, atk: 30,", "stats: {mhp: 90000, mmp: 0, atk: 400,")
	report, _ := designReportOf(t, 0, dir)
	for _, want := range []string{
		"did not reach",
		"- Steady: did not reach the end (down to B3F, 8 days) ×",
		"- Eager vs Steady: one of them did not reach the end",
		"| B3F |",
		"Did not reach the end (",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report lacks %q:\n%s", want, report)
		}
	}
}

func TestDesignMarksTargetsThatAreMissed(t *testing.T) {
	dir := designGame(t)
	// A tolerance of a hair: two personas that clear in different times are off target.
	edit(t, filepath.Join(dir, "designeval.yaml"), "within: 0.5", "within: 0.0001")
	report, _ := designReportOf(t, 0, dir)
	if !strings.Contains(report, "Eager vs Steady to the end") || !strings.Contains(report, "×") {
		t.Errorf("report:\n%s", report)
	}
	if !strings.Contains(report, "- Steady: reached the end") {
		t.Errorf("must_clear ○ line missing:\n%s", report)
	}
}

func TestDesignShowsHoursAndWhoGetsTheReward(t *testing.T) {
	dir := designGame(t)
	cfg := filepath.Join(dir, "designeval.yaml")
	edit(t, cfg, "battle: 5", "battle: 500")
	edit(t, cfg, "who: Anyone}", "who: Anyone}\n  - {kind: friend, label: Friend, events: \"^(gift)$\", names: {gift: The giver}, flow: gift}")
	edit(t, cfg, "release: gift", "release: friend")
	report, _ := designReportOf(t, 0, dir, "--only", "steady")
	if !regexp.MustCompile(`\d+h\d\dm`).MatchString(report) {
		t.Errorf("a run of more than an hour is shown in hours:\n%s", report)
	}
	if !strings.Contains(report, "reward targets: The giver ") || !strings.Contains(report, "reward routes: gift ") {
		t.Errorf("who and by which route the release came:\n%s", report)
	}
}

// A command that fails while a persona plays is not a failure of the tool: the report keeps
// the first of them and the exit status says the run found problems.
func TestDesignKeepsTheReportWhenThePlayFindsProblems(t *testing.T) {
	dir := designGame(t)
	edit(t, filepath.Join(dir, "data", "world.yaml"), "      - {id: gift, label: Open gift,", "      - {id: broken, label: Break it, do: [{set: {var.x: \"floor + 1\"}}]}\n      - {id: gift, label: Open gift,")
	edit(t, filepath.Join(dir, "designeval.yaml"), `rotate: ["@yes", "@no"]}`, `rotate: ["@yes", "@no"]}`+"\n      - {at: camp, do: \"@broken\"}")
	out := filepath.Join(t.TempDir(), "run")
	stdout, stderr := invoke(t, 1, "sim", "design", dir, "-o", out)
	if !strings.Contains(stderr, "reported simulation errors; see report.md") {
		t.Errorf("stderr: %q", stderr)
	}
	if strings.TrimSpace(stdout) != filepath.Join(out, "report.md") {
		t.Errorf("the report is still announced: %q", stdout)
	}
	report := string(read(t, filepath.Join(out, "report.md")))
	if !strings.Contains(report, "Could not play (") || !strings.Contains(report, "a function") {
		t.Errorf("report:\n%s", report)
	}
	var j struct {
		Personas []struct {
			Problems int `json:"problems"`
		}
	}
	if err := json.Unmarshal(read(t, filepath.Join(out, "summary.json")), &j); err != nil || j.Personas[0].Problems == 0 {
		t.Errorf("summary: %v %+v", err, j)
	}
}

func TestDesignComparesWithAPreviousRunThatLacksAPersona(t *testing.T) {
	dir := designGame(t)
	root := filepath.Join(t.TempDir(), "runs")
	invoke(t, 0, "sim", "design", dir, "--only", "steady", "-o", filepath.Join(root, "20200101-000000"))
	// A directory without a summary, and one with a summary of another format, are skipped.
	if err := os.MkdirAll(filepath.Join(root, "20200101-000001"), 0o750); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "20200101-000002", "summary.json"), `{"format_version": 2, "personas": []}`)
	write(t, filepath.Join(root, "20200101-000003", "summary.json"), `not json`)
	out, _ := invoke(t, 0, "sim", "design", dir, "--root", root, "-o", filepath.Join(root, "20200102-000000"))
	report := string(read(t, strings.TrimSpace(out)))
	if !strings.Contains(report, "Previous run: 20200101-000000") {
		t.Errorf("the newest usable run before this one is the one compared:\n%s", report[strings.Index(report, "## Change"):])
	}
	if !strings.Contains(report, "| Eager | (not in the previous run) | | | |") {
		t.Errorf("a persona the previous run lacks is said so:\n%s", report[strings.Index(report, "## Change"):])
	}
	if !strings.Contains(report, "| Steady | To the end (min) |") {
		t.Errorf("rows of the persona present in both:\n%s", report[strings.Index(report, "## Change"):])
	}
	// A run is only compared with runs made before it.
	later, _ := invoke(t, 0, "sim", "design", dir, "--root", root, "-o", filepath.Join(root, "20190101-000000"))
	if r := string(read(t, strings.TrimSpace(later))); !strings.Contains(r, "No previous run") {
		t.Errorf("an older name has nothing before it:\n%s", r[strings.Index(r, "## Change"):])
	}
}

func TestDesignOutputPlacementAndVerboseLog(t *testing.T) {
	dir := designGame(t)
	// The report may not go inside the game, nor over an existing directory.
	invoke(t, 1, "sim", "design", dir, "-o", filepath.Join(dir, "report"))
	taken := t.TempDir()
	invoke(t, 1, "sim", "design", dir, "-o", taken)
	// A failed run leaves no half-made directory behind.
	bad := filepath.Join(t.TempDir(), "bad")
	cfg := filepath.Join(t.TempDir(), "cfg.yaml")
	write(t, cfg, "personas: [{id: a}]\nmemos: {dry: absent}\n")
	invoke(t, 1, "sim", "design", dir, "--config", cfg, "-o", bad)
	if _, err := os.Lstat(bad); err == nil {
		t.Error("a configuration that does not validate created the output")
	}
	// -v writes the persona's choices to debug.log; without it there is none.
	out := filepath.Join(t.TempDir(), "v")
	invoke(t, 0, "sim", "design", dir, "-v", "--only", "eager", "-o", out)
	log := string(read(t, filepath.Join(out, "debug.log")))
	if !strings.Contains(log, "eager d1 ") || !strings.Contains(log, "dive (deepest 0") {
		t.Errorf("debug.log:\n%s", log[:min(500, len(log))])
	}
	quiet := filepath.Join(t.TempDir(), "q")
	invoke(t, 0, "sim", "design", dir, "--only", "eager", "-o", quiet)
	if _, err := os.Stat(filepath.Join(quiet, "debug.log")); err == nil {
		t.Error("debug.log without -v")
	}
}
