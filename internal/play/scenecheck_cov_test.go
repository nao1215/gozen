package play

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The recollection check page of the gallery (a debug mode: --review with the whole gallery
// open, game.yaml scene_review): scenes are listed under their group, a verdict is a tag chosen
// from the game's tags and appended to scene-review.jsonl, the last one for a scene wins, and
// "later" takes the verdict back.

const wldReviewYAML = `gallery:
  scenes:
    - {title: Meeting, event: ev_auto, flag: met_a, group: Act 1, picture: pics/bg.png}
    - {title: Parting, event: ev_box, flag: met_b, group: Act 1}
    - {title: Dawn, event: ev_long, flag: met_c}
scene_review:
  title: Scene check
  groups: [Act 2, Act 1]
  tags:
    - {id: ok, label: Fine, ok: true}
    - {id: bad, label: Needs work}
`

func wldReviewGame(t *testing.T, logDir string) (*Game, *galleryScreen) {
	t.Helper()
	opts := Options{
		FS:       wldWorldFiles(t, wldReviewYAML, map[string]string{"data/novel_cov.yaml": wldNovelData}),
		Location: "camp", GalleryAll: true, Review: true, ReviewLog: filepath.Join(logDir, "review.jsonl"),
	}
	g := wldOpenWith(t, opts)
	wldFrames(t, g, 5)
	s := newGallery(g)
	g.push(s)
	wldFrames(t, g, 2)
	return g, s
}

func wldReviewLines(t *testing.T, dir string) []sceneReviewLine {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, DefaultSceneReviewLog)) //nolint:gosec // The test's own temporary file.
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	}()
	var out []sceneReviewLine
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var l sceneReviewLine
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			t.Fatal(err)
		}
		out = append(out, l)
	}
	return out
}

func TestWldSceneCheckListsScenesByGroupAndRecordsVerdicts(t *testing.T) {
	dir := t.TempDir()
	g, s := wldReviewGame(t, dir)
	if got := s.tabIDs(g); len(got) != 3 || got[2] != galleryCheckTab {
		t.Fatalf("tabs %v: the check page should be the third", got)
	}
	if labels := s.tabLabels(g); labels[2] != "Scene check" {
		t.Fatalf("tab labels %v", labels)
	}
	wldPlay(t, g, "press right\nwait 6\npress right\nwait 6\n", 100)
	if s.tab != galleryCheckTab || s.check == nil {
		t.Fatalf("tab %d", s.tab)
	}
	// "Act 1" has two scenes and comes before the group without a name; the group Act 2 has none
	var labels []string
	for _, it := range s.menu.items {
		labels = append(labels, it.label)
	}
	want := []string{"■ Act 1", "　Meeting", "　Parting", "■ Other", "　Dawn"}
	if strings.Join(labels, "|") != strings.Join(want, "|") {
		t.Fatalf("rows %q, want %q", labels, want)
	}
	if !s.menu.items[0].disabled || !s.menu.items[3].disabled {
		t.Fatal("a group heading can be chosen")
	}
	if s.menu.sel != 1 {
		t.Fatalf("the cursor is on row %d, want the first scene", s.menu.sel)
	}
	// F10 opens the tags (and a last "later"); the second tag is NG
	wldPlay(t, g, "press tag\nwait 6\npress down\nwait 6\npress ok\nwait 6\n", 100)
	lines := wldReviewLines(t, dir)
	if len(lines) != 1 || lines[0].Event != "ev_auto" || lines[0].Title != "Meeting" || lines[0].Tag != "bad" || lines[0].Label != "Needs work" || lines[0].OK || lines[0].Group != "Act 1" {
		t.Fatalf("log %+v", lines)
	}
	if note := s.menu.items[1].note; note != "Needs work" {
		t.Fatalf("the row shows %q", note)
	}
	// the last verdict of a scene wins; the first tag is OK
	wldPlay(t, g, "press tag\nwait 6\npress ok\nwait 6\n", 100)
	lines = wldReviewLines(t, dir)
	if len(lines) != 2 || lines[1].Tag != "ok" || !lines[1].OK {
		t.Fatalf("log %+v", lines)
	}
	if note := s.menu.items[1].note; note != "Fine" {
		t.Fatalf("the row shows %q after the second verdict", note)
	}
	// Tab shows only what is still to check: the scene with a verdict goes
	wldPlay(t, g, "press tab\nwait 6\n", 100)
	if !s.check.onlyTodo {
		t.Fatal("Tab did not switch to the unchecked")
	}
	for _, it := range s.menu.items {
		if it.label == "　Meeting" {
			t.Fatalf("a checked scene is listed among the unchecked: %v", labels)
		}
	}
}

// "Later" (the last entry of the tags) decides nothing: no line is written and an earlier verdict stays.
func TestWldSceneCheckLaterWritesNothing(t *testing.T) {
	dir := t.TempDir()
	g, s := wldReviewGame(t, dir)
	wldPlay(t, g, "press right\nwait 6\npress right\nwait 6\n", 100)
	wldPlay(t, g, "press tag\nwait 6\npress ok\nwait 6\n", 100)
	wldPlay(t, g, "press tag\nwait 6\npress down\nwait 4\npress down\nwait 4\npress ok\nwait 6\n", 100)
	if v := s.check.verdict[sceneKey(g.db.Game.Gallery.Scenes[0])]; v.Tag != "ok" {
		t.Fatalf("later changed the verdict: %+v", v)
	}
	if n := len(wldReviewLines(t, dir)); n != 1 {
		t.Fatalf("%d lines written, want 1", n)
	}
	if s.check.box != nil {
		t.Fatal("the question stayed open")
	}
}

func TestWldSceneCheckReadsTheLogOfEarlierRuns(t *testing.T) {
	dir := t.TempDir()
	log := `{"time":"2026-10-20T10:00:00Z","event":"ev_box","title":"Parting","group":"Act 1","tag":"ok","label":"Fine","ok":true}
{"time":"2026-10-20T10:01:00Z","event":"ev_long","title":"Dawn","tag":"bad","label":"Needs work"}
not json
{"time":"2026-10-20T10:02:00Z","event":"ev_long","title":"Dawn","tag":"","label":""}
`
	if err := os.WriteFile(filepath.Join(dir, DefaultSceneReviewLog), []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}
	g, s := wldReviewGame(t, dir)
	wldPlay(t, g, "press right\nwait 6\npress right\nwait 6\n", 100)
	if len(s.check.verdict) != 1 {
		t.Fatalf("verdicts %v: the line that takes a verdict back and the broken line should leave one", s.check.verdict)
	}
	if v := s.check.verdict["ev_box|Parting"]; v.Tag != "ok" || !v.OK {
		t.Fatalf("verdict %+v", v)
	}
	// the heading counts: one fine, none bad, one not checked
	if !strings.Contains(s.menu.items[0].right, "1") {
		t.Fatalf("heading %+v", s.menu.items[0])
	}
}

func TestWldSceneCheckPlaysTheSceneThenAsksTheVerdict(t *testing.T) {
	dir := t.TempDir()
	g, s := wldReviewGame(t, dir)
	wldPlay(t, g, "press right\nwait 6\npress right\nwait 6\n", 100)
	wldPlay(t, g, "press ok\nwait 30\n", 100)
	if _, ok := g.stack[len(g.stack)-1].(*sceneReplay); !ok {
		t.Fatalf("OK on a scene opened %T", g.stack[len(g.stack)-1])
	}
	// read it to the end
	for i := 0; i < 60; i++ {
		if _, ok := g.stack[len(g.stack)-1].(*sceneReplay); !ok {
			break
		}
		wldPlay(t, g, "wait 40\npress ok\nwait 10\n", 200)
	}
	wldFrames(t, g, 20)
	if s.check.box == nil {
		t.Fatalf("no verdict asked after the scene (top %T)", g.stack[len(g.stack)-1])
	}
	wldPlay(t, g, "press ok\nwait 6\n", 100)
	lines := wldReviewLines(t, dir)
	if len(lines) != 1 || lines[0].Event != "ev_auto" || lines[0].Tag != "ok" {
		t.Fatalf("log %+v", lines)
	}
	// cancel closes the question without a verdict
	wldPlay(t, g, "press tag\nwait 6\npress cancel\nwait 6\n", 100)
	if s.check.box != nil || len(wldReviewLines(t, dir)) != 1 {
		t.Fatal("cancel recorded a verdict or left the question open")
	}
}
