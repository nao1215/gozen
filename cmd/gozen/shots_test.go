package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type shotRun struct {
	dir, bin, list, out string
}

func newShotRun(t *testing.T, list string) shotRun {
	t.Helper()
	t.Setenv("GOZEN_SHOT_HELPER", "ok")
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	r := shotRun{dir: fixture(t), bin: bin, list: filepath.Join(t.TempDir(), "shots.yaml"), out: filepath.Join(t.TempDir(), "capture")}
	write(t, r.list, list)
	return r
}

func (r shotRun) run(t *testing.T, want int, extra ...string) (string, string) {
	t.Helper()
	args := append([]string{"play", "shots", r.dir, r.list, "--bin", r.bin, "--raw", "--out", r.out}, extra...)
	return invoke(t, want, args...)
}

type shotRow struct {
	N        int
	ID       string
	Status   string
	Reason   string
	PNG      string
	JPEG     string
	Size     string
	Line     string
	Pictures []string
}

func (r shotRun) rows(t *testing.T) []shotRow {
	t.Helper()
	var rows []shotRow
	if err := json.Unmarshal(read(t, filepath.Join(r.out, "shots.json")), &rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestShotsPassTheListedStateToTheGame(t *testing.T) {
	r := newShotRun(t, `
seed: 5
shots:
  - id: scene_shot
    scene: Opening
    picture: mote.png
    line: 3
    answers: [1, 0]
    wait: 7
    script: "key z"
  - id: event_shot
    event: finish
    seed: 9
  - id: field
    start: grotto
    party: [scout]
    level: 4
    floor: 2
    flags: [met, seen]
    battle: lesson
    calm: true
`)
	log := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv("GOZEN_SHOT_ARGS", log)
	t.Setenv("GOZEN_SHOT_TRACE", `{"t":"from_memo_ready","why":"line"}`+"\n"+`{"t":"view","pictures":["assets/mote.png","assets/mote.png"]}`+"\n"+`{"t":"say","text":"The last line."}`)
	out, _ := r.run(t, 0)
	rows := r.rows(t)
	if len(rows) != 3 {
		t.Fatalf("%+v", rows)
	}
	for i, row := range rows {
		if row.N != i+1 || row.Status != "ok" || row.Size != "8x8" || row.PNG == "" || row.JPEG == "" {
			t.Errorf("row %d: %+v", i, row)
		}
		if row.Line != "The last line." || len(row.Pictures) != 1 || row.Pictures[0] != "mote.png" {
			t.Errorf("row %d: the trace gives the last line and each picture once: %+v", i, row)
		}
		read(t, filepath.Join(r.out, row.PNG))
		read(t, filepath.Join(r.out, row.JPEG))
	}
	if !strings.Contains(out, "3 shot, 0 awaiting approval, 0 failed") {
		t.Errorf("stdout: %q", out)
	}
	runs := strings.Split(strings.TrimSpace(string(read(t, log))), "\n")
	if len(runs) != 3 {
		t.Fatalf("one game run per shot: %q", runs)
	}
	for _, want := range []string{"--scene Opening", "--picture mote.png", "--line 3", "--answers 1,0", "--seed 5", "--window 1", "--mute", "--dev"} {
		if !strings.Contains(runs[0], want) {
			t.Errorf("scene shot lacks %q: %s", want, runs[0])
		}
	}
	if !strings.Contains(runs[1], "--event finish") || !strings.Contains(runs[1], "--seed 9") || strings.Contains(runs[1], "--scene") || strings.Contains(runs[1], "--line") {
		t.Errorf("event shot: %s", runs[1])
	}
	for _, want := range []string{"--start grotto", "--party scout", "--level 4", "--floor 2", "--flags met,seen", "--battle lesson", "--calm"} {
		if !strings.Contains(runs[2], want) {
			t.Errorf("field shot lacks %q: %s", want, runs[2])
		}
	}
	html := string(read(t, filepath.Join(r.out, "index.html")))
	for _, want := range []string{"An unchecked build (--raw)", "Opening", "mote.png", "event finish", "scene_shot", "grotto B2"} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html lacks %q", want)
		}
	}
}

func TestShotsReportFailuresPerShot(t *testing.T) {
	list := "shots:\n  - {id: reach, event: finish}\n  - {id: field, start: camp}\n"
	r := newShotRun(t, list)
	// A scene or event shot that never reaches its line is a failure, not a picture.
	t.Setenv("GOZEN_SHOT_TRACE", `{"t":"from_memo_ready","why":"no such event"}`)
	out, _ := r.run(t, 1)
	rows := r.rows(t)
	if rows[0].Status != "failed" || !strings.Contains(rows[0].Reason, "did not reach the line (no such event") || rows[0].PNG != "" {
		t.Errorf("%+v", rows[0])
	}
	if rows[1].Status != "ok" {
		t.Errorf("a start shot needs no line: %+v", rows[1])
	}
	if !strings.Contains(out, "1 shot, 0 awaiting approval, 1 failed") {
		t.Errorf("stdout: %q", out)
	}
	if html := string(read(t, filepath.Join(r.out, "index.html"))); !strings.Contains(html, "class=f") {
		t.Errorf("a failure is marked in the page")
	}

	// A game that dies, or one that leaves no picture.
	r = newShotRun(t, list)
	t.Setenv("GOZEN_SHOT_EXIT", "1")
	r.run(t, 1)
	for _, row := range r.rows(t) {
		if row.Status != "failed" || !strings.Contains(row.Reason, "game capture failed") {
			t.Errorf("%+v", row)
		}
	}
	t.Setenv("GOZEN_SHOT_EXIT", "")
	r = newShotRun(t, list)
	t.Setenv("GOZEN_SHOT_NOPNG", "1")
	t.Setenv("GOZEN_SHOT_TRACE", `{"t":"from_memo_ready","why":"line"}`)
	r.run(t, 1)
	for _, row := range r.rows(t) {
		if row.Status != "failed" || !strings.Contains(row.Reason, "could not capture the screen") {
			t.Errorf("%+v", row)
		}
	}
	// A broken trace is a failure of that shot.
	t.Setenv("GOZEN_SHOT_NOPNG", "")
	r = newShotRun(t, list)
	t.Setenv("GOZEN_SHOT_TRACE", "{")
	r.run(t, 1)
	if rows := r.rows(t); rows[1].Status != "failed" {
		t.Errorf("%+v", rows)
	}
}

func TestShotsOnlyTakesTheNamedOnes(t *testing.T) {
	r := newShotRun(t, "shots:\n  - {id: a, start: camp}\n  - {id: b, start: camp}\n  - {id: c, start: camp}\n")
	log := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv("GOZEN_SHOT_ARGS", log)
	out, _ := r.run(t, 0, "--only", "a, c")
	rows := r.rows(t)
	if rows[0].Status != "ok" || rows[1].Status != "skipped" || rows[2].Status != "ok" {
		t.Fatalf("%+v", rows)
	}
	if strings.Count(string(read(t, log)), "\n") != 2 {
		t.Error("the shot that is not named must not start the game")
	}
	if strings.Contains(out, " b ") {
		t.Errorf("a skipped shot is not listed: %q", out)
	}
	invoke(t, 2, "play", "shots", r.dir, r.list, "--bin", r.bin, "--raw", "--out", filepath.Join(t.TempDir(), "x"), "--only", "zzz")
}

func TestShotsListIsValidated(t *testing.T) {
	for _, tc := range []struct{ name, list, msg string }{
		{"empty", "shots: []\n", "empty list"},
		{"unknown key", "shots: [{id: a, start: camp, surprise: 1}]\n", "surprise"},
		{"no id", "shots: [{start: camp}]\n", "the id is missing or repeated"},
		{"bad id", "shots: [{id: ../a, start: camp}]\n", "the id is missing or repeated"},
		{"repeated id", "shots: [{id: a, start: camp}, {id: a, start: camp}]\n", "the id is missing or repeated"},
		{"negative wait", "shots: [{id: a, start: camp, wait: -1}]\n", "negative shot count"},
		{"negative line", "shots: [{id: a, start: camp, line: -1}]\n", "negative shot count"},
		{"negative answer", "shots: [{id: a, start: camp, answers: [-1]}]\n", "negative answer"},
		{"picture path", "shots: [{id: a, start: camp, picture: ../x.png}]\n", "invalid picture path"},
		{"scene and event", "shots: [{id: a, scene: s, event: e}]\n", "only one of scene and event"},
		{"nothing to start", "shots: [{id: a}]\n", "give scene, event or start"},
		{"watch prefix", "watch: ['../x']\nshots: [{id: a, start: camp}]\n", "invalid watch prefix"},
		{"not yaml", "shots: [", "shots.yaml"},
	} {
		r := newShotRun(t, tc.list)
		_, stderr := r.run(t, 1)
		if !strings.Contains(stderr, tc.msg) {
			t.Errorf("%s: %q", tc.name, stderr)
		}
		if _, err := os.Lstat(r.out); err == nil {
			t.Errorf("%s: a rejected list created the output", tc.name)
		}
	}
}

func TestShotsArgumentsAndBinary(t *testing.T) {
	r := newShotRun(t, "shots: [{id: a, start: camp}]\n")
	base := []string{"play", "shots", r.dir, r.list}
	for _, tc := range []struct {
		args []string
		want int
		msg  string
	}{
		{[]string{"--raw"}, 2, "requires --bin"},
		{[]string{"--bin", r.bin}, 2, "exactly one of --overlay DIR or --raw"},
		{[]string{"--bin", r.bin, "--raw", "--overlay", t.TempDir()}, 2, "exactly one of"},
		{[]string{"--bin", r.bin, "--raw", "--timeout", "0s"}, 2, "--timeout must be positive"},
		{[]string{"--bin", filepath.Join(t.TempDir(), "absent"), "--raw"}, 1, "game binary unavailable"},
	} {
		_, stderr := invoke(t, tc.want, append(append([]string{}, base...), tc.args...)...)
		if !strings.Contains(stderr, tc.msg) {
			t.Errorf("%v: %q", tc.args, stderr)
		}
	}
	invoke(t, 2, "play", "shots", r.dir, "--bin", r.bin, "--raw")
	// The output never overwrites and never goes inside the game.
	invoke(t, 1, "play", "shots", r.dir, r.list, "--bin", r.bin, "--raw", "--out", filepath.Join(r.dir, "out"))
	if _, stderr := invoke(t, 1, "play", "shots", r.dir, filepath.Join(t.TempDir(), "absent.yaml"), "--bin", r.bin, "--raw", "--out", r.out); stderr == "" {
		t.Error("an absent list must be reported")
	}
}

func TestShotsPageWarnsOfRepeatedPhasesAndPoses(t *testing.T) {
	r := newShotRun(t, `
title: Gallery <b>
shots:
  - {id: a, title: First, start: camp, who: Mira, phase: p1, pose: stand, note: "keep <this>"}
  - {id: b, title: Second, start: grotto, floor: 3, battle: lesson, phase: p1, pose: stand}
  - {id: c, title: Third, event: finish, who: Rin, phase: p2, pose: sit}
`)
	r.run(t, 1)
	html := string(read(t, filepath.Join(r.out, "index.html")))
	for _, want := range []string{
		"<title>Gallery &lt;b&gt;</title>",
		"Overlaps in order: a and b share the phase p1",
		"a and b share the pose stand",
		"keep &lt;this&gt;",
		"grotto B3",
		"lesson",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html lacks %q", want)
		}
	}
	if strings.Count(html, "<img") != 2 {
		t.Errorf("a picture per shot that was taken: %s", html)
	}
	if !strings.Contains(html, "<h2>") {
		t.Error("shots are grouped by status")
	}
}

func TestShotsTimeoutKillsAGameThatDoesNotFinish(t *testing.T) {
	r := newShotRun(t, "shots: [{id: a, start: camp}]\n")
	t.Setenv("GOZEN_SHOT_HELPER", "wait")
	began := time.Now()
	r.run(t, 1, "--timeout", "80ms")
	if time.Since(began) > 4*time.Second {
		t.Fatal("the capture outlived its timeout")
	}
	if rows := r.rows(t); rows[0].Status != "failed" || !strings.Contains(rows[0].Reason, "game capture failed") {
		t.Errorf("%+v", rows)
	}
}

func TestShotsOverlayModeChecksApprovalOfThePicturesShown(t *testing.T) {
	r := newShotRun(t, "shots: [{id: a, start: camp, picture: mote.png}]\n")
	top, _ := approvals(t, r.dir, true)
	t.Setenv("GOZEN_SHOT_TRACE", `{"t":"view","pictures":["assets/mote.png"]}`)
	out, _ := invoke(t, 0, "play", "shots", r.dir, r.list, "--bin", r.bin, "--overlay", top, "--out", r.out)
	if !strings.Contains(out, "1 shot, 0 awaiting approval, 0 failed") {
		t.Errorf("stdout: %q", out)
	}
	if html := string(read(t, filepath.Join(r.out, "index.html"))); strings.Contains(html, "An unchecked build") {
		t.Error("an approved build is not marked unchecked")
	}
	// An unapproved picture holds the shot back before the game is even started.
	top, _ = approvals(t, r.dir, false)
	r.out = filepath.Join(t.TempDir(), "pending")
	log := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv("GOZEN_SHOT_ARGS", log)
	_, stderr := invoke(t, 1, "play", "shots", r.dir, r.list, "--bin", r.bin, "--overlay", top, "--out", r.out)
	if !strings.Contains(stderr, "1 awaiting approval") {
		t.Errorf("stderr: %q", stderr)
	}
	if _, err := os.Stat(log); err == nil {
		t.Error("the game was started for a picture that is not approved")
	}
	// A picture the trace shows but the list did not name is checked too, and its screenshot is removed.
	r.list = filepath.Join(t.TempDir(), "other.yaml")
	write(t, r.list, "shots: [{id: a, start: camp}]\n")
	r.out = filepath.Join(t.TempDir(), "late")
	invoke(t, 1, "play", "shots", r.dir, r.list, "--bin", r.bin, "--overlay", top, "--out", r.out)
	rows := r.rows(t)
	if rows[0].Status != "approval_wait" || rows[0].PNG != "" {
		t.Errorf("%+v", rows[0])
	}
	if _, err := os.Stat(filepath.Join(r.out, "01_a.png")); err == nil {
		t.Error("the screenshot of an unapproved picture was kept")
	}
}
