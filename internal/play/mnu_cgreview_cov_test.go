package play

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	lang "github.com/nao1215/gozen/internal/text"
)

const mnuReviewYAML = `
cg_review:
  tags: [{id: ok, label: OK, ok: true}, {id: ng, label: NG}, {id: ugly, label: Ugly}]
  groups: [{label: Extras, files: ["cg/c.*"]}]
  extra: ["x.png", "cg/c.png"]
`

// mnuReviewGame is the gallery game with the picture check configured and a log in a temporary directory.
func mnuReviewGame(t *testing.T, all bool) (*Game, string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "review", "log.jsonl")
	src := filepath.Join(dir, "sources.json")
	if err := os.WriteFile(src, []byte(`{"cg/a.png": "src/a", "cg/b.001.png": {"src": "src/b"}, "cg/d.png": "stand_in:foo", "cg/missing.png": "src/m", "cg/e.png": 5}`), 0o600); err != nil {
		t.Fatal(err)
	}
	fsys := mnuGalleryFS(t)
	fsys["game.yaml"].Data = append(fsys["game.yaml"].Data, []byte(mnuReviewYAML)...)
	fsys["assets/cg/d.png"] = &fstest.MapFile{Data: fsys["assets/x.png"].Data}
	fsys["assets/cg/e.png"] = &fstest.MapFile{Data: fsys["assets/x.png"].Data}
	g, err := New(Options{FS: fsys, NoSound: true, SaveDir: t.TempDir(), Location: "camp", Seed: 1, GalleryAll: all, ReviewLog: log, ReviewSources: src})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	return g, log
}

func mnuReadLog(t *testing.T, path string) []reviewLine {
	t.Helper()
	f, err := os.Open(path) //nolint:gosec // a test file in a temporary directory
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck // read only
	var out []reviewLine
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var l reviewLine
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			t.Fatal(err)
		}
		out = append(out, l)
	}
	return out
}

func mnuDigit(g *Game, n int) {
	g.in.digit = n
	mnuTop(g).update(g)
	g.in.digit = 0
}

func TestReviewPagesAndSources(t *testing.T) {
	g, _ := mnuReviewGame(t, true)
	items := reviewPages(g)
	var pages []string
	for _, it := range items {
		pages = append(pages, it.page)
	}
	// gallery pictures first, then sources in name order, then the extra patterns;
	// stand-ins and files that do not exist are left out
	want := []string{"cg/a.png", "cg/b.001.png", "cg/b.002.png", "cg/c.png", "x.png"}
	if !slices.Equal(pages, want) {
		t.Fatalf("pages %v, want %v", pages, want)
	}
	if items[0].src != "src/a" || items[1].src != "src/b" || items[0].title != "Alpha" || items[0].slot != "a" {
		t.Fatalf("items %+v", items[:2])
	}
	if items[1].slot != "b" || items[2].slot != "b" {
		t.Fatalf("slots %q %q: later pages share the picture's slot", items[1].slot, items[2].slot)
	}
	if items[3].group != 0 || items[0].group != -1 {
		t.Fatalf("groups %d %d", items[3].group, items[0].group)
	}
	if items[0].key() != "src/a" || items[2].key() != "cg/b.002.png" {
		t.Fatalf("keys %q %q", items[0].key(), items[2].key())
	}
	srcs := loadReviewSources(g.opts.ReviewSources)
	if len(srcs) != 4 || srcs["cg/b.001.png"] != "src/b" || srcs["cg/e.png"] != "" {
		t.Fatalf("sources %v (a value that is neither a string nor an object is ignored)", srcs)
	}
	if got := loadReviewSources(""); len(got) != 0 {
		t.Fatalf("sources %v", got)
	}
	if got := loadReviewSources(filepath.Join(t.TempDir(), "none.json")); len(got) != 0 {
		t.Fatalf("missing file: %v", got)
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte("[1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := loadReviewSources(bad); len(got) != 0 {
		t.Fatalf("broken file: %v", got)
	}
}

func TestReviewEnabledRules(t *testing.T) {
	g, _ := mnuReviewGame(t, true)
	if !g.cgReviewEnabled() {
		t.Fatal("a configured check with a log is off")
	}
	g.opts.ReviewLog = ""
	if g.cgReviewEnabled() {
		t.Fatal("the check is on without a log")
	}
	if g.reviewLog() != DefaultReviewLog {
		t.Fatalf("default log %q", g.reviewLog())
	}
	g.opts.ReviewLog = "x.jsonl"
	g.db.Game.CGReview = data.CGReviewCfg{}
	if g.cgReviewEnabled() {
		t.Fatal("an unconfigured check is on")
	}
	g.opts.Review = true
	if !g.cgReviewEnabled() {
		t.Fatal("--review does not turn the check on")
	}
}

func TestReviewMenuOptions(t *testing.T) {
	g, _ := mnuReviewGame(t, true)
	r := newCGReview(g)
	g.push(r)
	if len(r.all) != 5 || len(r.filter(g)) != 4 {
		t.Fatalf("all %d filtered %d (the Extras group is left out)", len(r.all), len(r.filter(g)))
	}
	mnuDrawAll(g)
	if r.menu.items[0].right != "4 "+strings.TrimPrefix(lang.T("%d 枚"), "%d ") {
		t.Fatalf("start row %q", r.menu.items[0].right)
	}
	// flip "by name" with the arrow keys, then the two filters, then the group
	mnuPress(g, btnDown)
	mnuPress(g, btnRight)
	if !r.byName || r.menu.items[1].right != lang.T("名前の順") {
		t.Fatalf("byName %v %q", r.byName, r.menu.items[1].right)
	}
	mnuPress(g, btnDown)
	mnuPress(g, btnLeft)
	if r.skipOK {
		t.Fatal("skip-OK stayed on")
	}
	mnuPress(g, btnDown)
	mnuPress(g, btnOK)
	if !r.skipAny {
		t.Fatal("skip-any stayed off")
	}
	mnuPress(g, btnDown)
	mnuPress(g, btnOK) // the Extras group
	if !r.groups[0] || len(r.filter(g)) != 5 {
		t.Fatalf("groups %v items %d", r.groups, len(r.filter(g)))
	}
	if r.menu.items[len(r.menu.items)-2].right != lang.T("する") {
		t.Fatalf("group row %+v", r.menu.items)
	}
	mnuDrawAll(g)
	// back out: the last row, or cancel
	mnuPress(g, btnDown)
	mnuPress(g, btnOK)
	if mnuTop(g) == scene(r) {
		t.Fatal("the last row did not leave")
	}
	g.push(r)
	mnuPress(g, btnCancel)
	if mnuTop(g) == scene(r) {
		t.Fatal("cancel did not leave")
	}
}

func TestReviewSkimTagsAndLog(t *testing.T) {
	g, logPath := mnuReviewGame(t, true)
	r := newCGReview(g)
	g.push(r)
	mnuPress(g, btnOK) // start
	if !r.viewing || r.at != 0 || r.items[0].page != "cg/a.png" || !r.started {
		t.Fatalf("viewing %v at %d", r.viewing, r.at)
	}
	mnuDrawAll(g)
	mnuDigit(g, 1) // OK: logged under the source, and on to the next picture
	if r.at != 1 || !r.marks["src/a"]["ok"] {
		t.Fatalf("at %d marks %v", r.at, r.marks)
	}
	lines := mnuReadLog(t, logPath)
	if len(lines) != 1 || lines[0].Page != "cg/a.png" || lines[0].Src != "src/a" || lines[0].Tag != "ok" || !lines[0].OK || lines[0].Off || lines[0].Slot != "a" {
		t.Fatalf("log %+v", lines)
	}
	mnuDigit(g, 2) // NG on b.001: stays
	if r.at != 1 || !r.marks["src/b"]["ng"] {
		t.Fatalf("at %d marks %v", r.at, r.marks)
	}
	mnuDigit(g, 3)
	mnuDigit(g, 9) // no ninth tag: nothing
	mnuDigit(g, 3) // Ugly off again
	if r.marks["src/b"]["ugly"] {
		t.Fatalf("toggling twice left the tag: %v", r.marks["src/b"])
	}
	mnuDrawAll(g)
	mnuDigit(g, 1) // OK replaces NG; the NG is logged off
	if r.marks["src/b"]["ng"] || !r.marks["src/b"]["ok"] || r.at != 2 {
		t.Fatalf("at %d marks %v", r.at, r.marks["src/b"])
	}
	lines = mnuReadLog(t, logPath)
	off := 0
	for _, l := range lines {
		if l.Off && l.Tag == "ng" {
			off++
		}
	}
	if off != 1 {
		t.Fatalf("ng off written %d times: %+v", off, lines)
	}
	// back and forth, hide the text
	mnuPress(g, btnLeft)
	if r.at != 1 {
		t.Fatalf("at %d", r.at)
	}
	mnuPress(g, btnUp)
	mnuPress(g, btnUp) // already first... at 0 stays
	if r.at != 0 {
		t.Fatalf("at %d", r.at)
	}
	mnuPress(g, btnHide)
	if !r.hide {
		t.Fatal("V did not hide the text")
	}
	mnuDrawAll(g)
	mnuPress(g, btnHide)
	// go to the end
	for i := 0; i < 10 && r.viewing; i++ {
		mnuPress(g, btnRight)
	}
	if r.viewing || r.at != len(r.items) || r.toast != lang.T("最後まで見た") {
		t.Fatalf("viewing %v at %d toast %q", r.viewing, r.at, r.toast)
	}
	mnuDrawAll(g)
	// "resume" now stays out of the menu (at is past the end)
	for _, row := range r.rows() {
		if row == reviewRowResume {
			t.Fatal("resume offered after the end")
		}
	}
	// a second skim with OK pictures left out: a, b.001 are OK
	r2 := newCGReview(g)
	if r2.marks["src/a"]["ok"] != true || r2.marks["src/b"]["ok"] != true || r2.marks["src/b"]["ng"] {
		t.Fatalf("log not replayed: %v", r2.marks)
	}
	var left []string
	for _, it := range r2.filter(g) {
		left = append(left, it.page)
	}
	if !slices.Equal(left, []string{"cg/b.002.png", "x.png"}) {
		t.Fatalf("pictures left after the OK ones are skipped: %v", left)
	}
	r2.skipAny = true
	r2.skipOK = false
	if got := len(r2.filter(g)); got != 2 {
		t.Fatalf("skip-any leaves %d", got)
	}
}

func TestReviewResumeAndBox(t *testing.T) {
	g, logPath := mnuReviewGame(t, true)
	r := newCGReview(g)
	g.push(r)
	mnuPress(g, btnOK)
	mnuPress(g, btnRight)
	mnuPress(g, btnRight)
	mnuPress(g, btnCancel) // out of the skim, back to the menu
	if r.viewing || r.at != 2 {
		t.Fatalf("viewing %v at %d", r.viewing, r.at)
	}
	if !slices.Contains(r.rows(), reviewRowResume) {
		t.Fatal("no resume row")
	}
	if r.menu.items[1].right != "3 / 4" {
		t.Fatalf("resume row %q", r.menu.items[1].right)
	}
	mnuDrawAll(g)
	mnuPress(g, btnDown)
	mnuPress(g, btnOK)
	if !r.viewing || r.at != 2 {
		t.Fatalf("resume: viewing %v at %d", r.viewing, r.at)
	}
	// the tag box
	mnuPress(g, btnMemo)
	if r.box == nil || len(r.box.list.items) != 4 || r.box.list.items[3].id != reviewNoteTag {
		t.Fatalf("box %+v", r.box)
	}
	mnuDrawAll(g)
	mnuPress(g, btnDown)
	mnuPress(g, btnOK) // NG
	if !r.marks["cg/b.002.png"]["ng"] || r.box == nil || r.box.list.items[1].note != "✓" {
		t.Fatalf("marks %v", r.marks)
	}
	mnuDigit(g, 3)
	if !r.marks["cg/b.002.png"]["ugly"] {
		t.Fatal("digit in the box did not tag")
	}
	// a note: the box takes text from an outside entry
	r.box.list.sel = 3
	r.box.text = "  needs a look  "
	mnuPress(g, btnOK)
	if r.notes["cg/b.002.png"] != "needs a look" || r.toast != lang.T("一言を残した") {
		t.Fatalf("notes %v toast %q", r.notes, r.toast)
	}
	r.box.ext = make(chan extInput, 1)
	r.box.ext <- extInput{text: "from outside", ok: true}
	mnuPress(g, btnOK)
	if r.notes["cg/b.002.png"] != "from outside" || r.box.ext != nil {
		t.Fatalf("notes %v", r.notes)
	}
	r.box.ext = make(chan extInput, 1)
	mnuPress(g, btnOK) // still waiting: nothing happens
	r.box.ext <- extInput{text: "cancelled", ok: false}
	mnuPress(g, btnOK)
	if r.notes["cg/b.002.png"] != "from outside" {
		t.Fatalf("a cancelled entry was kept: %v", r.notes)
	}
	mnuDrawAll(g)
	r.box.ext = make(chan extInput, 1)
	mnuDrawAll(g)
	r.box.ext = nil
	// OK through the box moves on (not in play)
	r.box.list.sel = 0
	at := r.at
	mnuPress(g, btnOK)
	if r.box != nil || r.at != at+1 {
		t.Fatalf("box %v at %d", r.box, r.at)
	}
	mnuPress(g, btnNext)
	mnuPress(g, btnNext) // close
	if r.box != nil {
		t.Fatal("tab did not close the box")
	}
	mnuPress(g, btnMemo)
	mnuPress(g, btnCancel)
	if r.box != nil {
		t.Fatal("cancel did not close the box")
	}
	lines := mnuReadLog(t, logPath)
	notes := 0
	for _, l := range lines {
		if l.Tag == reviewNoteTag {
			notes++
		}
	}
	if notes != 2 {
		t.Fatalf("notes in the log: %d of %d lines", notes, len(lines))
	}
	// the notes come back from the log
	r2 := newCGReview(g)
	if r2.notes["cg/b.002.png"] != "from outside" {
		t.Fatalf("notes %v", r2.notes)
	}
	// both the skim and the box take the memo key
	g.push(r2)
	if !g.memoKeyTaken() || g.reviewOpen() != r2 {
		t.Fatal("the skim does not own the memo key")
	}
}

func TestReviewLogFailureIsReportedNotFatal(t *testing.T) {
	g, logPath := mnuReviewGame(t, true)
	// the log's directory is a file: writing fails and the player is told
	if err := os.WriteFile(filepath.Dir(logPath), []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := newCGReview(g)
	g.push(r)
	mnuPress(g, btnOK)
	mnuDigit(g, 2)
	if g.failure == nil {
		t.Fatal("a log that cannot be created was not reported")
	}
	g.failure = nil
	// a log that cannot be opened for appending says so on screen
	dir := t.TempDir()
	g.opts.ReviewLog = filepath.Join(dir, "sub")
	if err := os.Mkdir(g.opts.ReviewLog, 0o700); err != nil {
		t.Fatal(err)
	}
	r.write(g, r.items[0], r.tagOf(g, "ng"), false, "")
	if r.toast != lang.T("記録を書けなかった") {
		t.Fatalf("toast %q", r.toast)
	}
	if errors.Is(g.failure, os.ErrNotExist) {
		t.Fatal("unexpected failure")
	}
}

func TestReviewFromGalleryAndInPlayTag(t *testing.T) {
	g, logPath := mnuReviewGame(t, true)
	s := newGallery(g)
	g.push(s)
	mnuPress(g, btnNext) // Tab on the picture column opens the skim
	if _, ok := mnuTop(g).(*cgReview); !ok {
		t.Fatalf("scene %T", mnuTop(g))
	}
	g.stack = g.stack[:1]

	// tagging in play with F10: needs a picture on screen
	g.in.pressed[btnTag] = true
	if g.updateTag() {
		t.Fatal("the tag box opened with no picture")
	}
	if g.tag == nil || g.tag.toast != lang.T("一枚絵が出ていない") {
		t.Fatalf("tag %+v", g.tag)
	}
	g.tag.toastT = 1
	g.in.pressed[btnTag] = false
	g.updateTag()
	if g.tag != nil {
		t.Fatal("the toast stayed")
	}
	g.Image(data.CG{File: "cg/a.png"})
	g.in.pressed[btnTag] = true
	if !g.updateTag() || g.tag == nil || g.tag.box == nil {
		t.Fatalf("tag %+v", g.tag)
	}
	g.in.pressed[btnTag] = false
	mnuDrawAll(g)
	g.in.digit = 2
	g.updateTag()
	g.in.digit = 0
	lines := mnuReadLog(t, logPath)
	if len(lines) != 1 || lines[0].Page != "cg/a.png" || lines[0].Tag != "ng" {
		t.Fatalf("log %+v", lines)
	}
	// OK in play does not move on (there is no next picture) and closes the box
	g.in.digit = 1
	g.updateTag()
	g.in.digit = 0
	if g.tag.box != nil {
		t.Fatal("OK left the box open")
	}
	g.in.pressed[btnTag] = true
	g.rep = &replay{}
	if g.updateTag() {
		t.Fatal("tagging during a replay")
	}
	g.rep = nil
	g.in.pressed[btnTag] = false
}
