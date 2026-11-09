package play

import (
	"bytes"
	"fmt"
	"image"
	"strings"
	"testing"
	"testing/fstest"
)

const mnuBookHead = `actors:
  muse: {name: Muse, class: ranger, faces: chars/muse}
profiles:
  keeper:
    picture: x.png
    name: Keeper
    title: Gold held {gold}
    fields: [{label: Role, value: Walks ahead}, {label: Habit, value: "A very long habit that does not fit on one line of the panel and runs on and on until the line breaks twice or more"}]
    text: Knows the trail.
    relations: [{who: guide, calls: "Friend {gold}", text: Trusted.}, {who: muse, calls: Muse}, {who: you, calls: ""}, {who: keeper, calls: Keeper}]
  guide:
    picture: x.png
    when: flag.guide_met
  muse:
    faces: [normal, glad]
    when: "gold >= 200"
  foe:
    enemy: mote
    when: flag.foe
`

// mnuBookFS has a book of four named profiles and twelve fillers (the list scrolls).
func mnuBookFS(t *testing.T) fstest.MapFS {
	t.Helper()
	var b strings.Builder
	b.WriteString(mnuBookHead)
	for i := range 12 {
		fmt.Fprintf(&b, "  filler%02d: {picture: x.png, name: Filler %d}\n", i, i)
	}
	fsys := gameFiles(t)
	fsys["data/book.yaml"] = &fstest.MapFile{Data: []byte(b.String())}
	for _, f := range []string{"normal", "glad"} {
		fsys["assets/chars/muse/"+f+".png"] = &fstest.MapFile{Data: fsys["assets/x.png"].Data}
	}
	fsys["game.yaml"] = &fstest.MapFile{Data: append([]byte("schema_version: 4\nui: {profiles: true}\n"), fsys["game.yaml"].Data...)}
	return fsys
}

func mnuBookGame(t *testing.T, opts Options) *Game {
	t.Helper()
	opts.FS, opts.NoSound = mnuBookFS(t), true
	if opts.SaveDir == "" {
		opts.SaveDir = t.TempDir()
	}
	if opts.Location == "" {
		opts.Location = "camp"
	}
	opts.Seed = 1
	g, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	return g
}

func TestProfilesNavigation(t *testing.T) {
	g := mnuBookGame(t, Options{})
	var out bytes.Buffer
	g.tr = newTracer(&out)
	done := 0
	s := newProfiles(g, func() { done++ })
	g.push(s)
	if len(s.ids) != 16 || s.current() != "keeper" {
		t.Fatalf("ids %v", s.ids)
	}
	mnuDrawAll(g)
	// up from the first wraps to the last, and the list scrolls to show it
	mnuPress(g, btnUp)
	if s.sel != 15 || s.top != 15-s.rows(g)+1 {
		t.Fatalf("sel %d top %d rows %d", s.sel, s.top, s.rows(g))
	}
	mnuDrawAll(g)
	mnuPress(g, btnRight) // wraps to the first
	if s.sel != 0 || s.top != 0 {
		t.Fatalf("sel %d top %d", s.sel, s.top)
	}
	mnuPress(g, btnLeft)
	if s.sel != 15 {
		t.Fatalf("sel %d", s.sel)
	}
	mnuPress(g, btnDown)
	mnuPress(g, btnDown)
	mnuPress(g, btnLog) // L of a pad: back
	if s.sel != 0 {
		t.Fatalf("sel %d", s.sel)
	}
	mnuPress(g, btnAuto) // R of a pad: on
	if s.sel != 1 {
		t.Fatalf("sel %d", s.sel)
	}
	mnuPress(g, btnAuto, btnLeft) // A on a keyboard is left too: it goes back, not on
	if s.sel != 0 {
		t.Fatalf("auto with left: sel %d", s.sel)
	}
	// the arrows of the band and the rows of the list answer to the mouse
	_, next := s.arrows(g)
	g.in.click, g.in.mouse = true, image.Pt(next.Min.X+4, next.Min.Y+4)
	mnuTop(g).update(g)
	g.in.click = false
	if s.sel != 1 {
		t.Fatalf("click on the next arrow: sel %d", s.sel)
	}
	prev, _ := s.arrows(g)
	g.in.click, g.in.mouse = true, image.Pt(prev.Min.X+4, prev.Min.Y+4)
	mnuTop(g).update(g)
	g.in.click = false
	if s.sel != 0 {
		t.Fatalf("click on the previous arrow: sel %d", s.sel)
	}
	r := s.rowRect(g, 3)
	g.in.click, g.in.mouse = true, image.Pt(r.Min.X+5, r.Min.Y+5)
	mnuTop(g).update(g)
	g.in.click = false
	if s.sel != 3 {
		t.Fatalf("click on a row: sel %d", s.sel)
	}
	// a swipe over the picture to the left is the next character, to the right the previous
	s.press, s.last, s.pressed = image.Pt(500, 300), image.Pt(300, 310), true
	mnuPress(g)
	if s.sel != 4 {
		t.Fatalf("swipe left: sel %d", s.sel)
	}
	s.press, s.last, s.pressed = image.Pt(300, 300), image.Pt(500, 300), true
	mnuPress(g)
	if s.sel != 3 {
		t.Fatalf("swipe right: sel %d", s.sel)
	}
	// a short swipe, or one that is mostly vertical, does nothing
	s.press, s.last, s.pressed = image.Pt(300, 300), image.Pt(380, 300), true
	mnuPress(g)
	s.press, s.last, s.pressed = image.Pt(300, 100), image.Pt(450, 600), true
	mnuPress(g)
	if s.sel != 3 {
		t.Fatalf("a swipe that should not count moved to %d", s.sel)
	}
	// a press that begins over the picture starts a swipe
	g.in.click, g.in.mouse = true, image.Pt(400, 300)
	mnuTop(g).update(g)
	g.in.click = false
	if !s.pressed || s.press != image.Pt(400, 300) {
		t.Fatalf("press %v %v", s.pressed, s.press)
	}
	mnuPress(g, btnCancel)
	if done != 1 || len(g.stack) != 1 {
		t.Fatalf("done %d stack %d", done, len(g.stack))
	}
	if !strings.Contains(out.String(), `"profile":"foe"`) {
		t.Fatalf("trace %s", out.String())
	}
}

func TestProfilesFacesOnlyWhenUnlocked(t *testing.T) {
	g := mnuBookGame(t, Options{})
	var out bytes.Buffer
	g.tr = newTracer(&out)
	s := newProfiles(g, nil)
	g.push(s)
	mnuPress(g, btnDown)
	mnuPress(g, btnDown) // later: locked at 100 gold
	if s.current() != "muse" || s.opened(g, "muse") {
		t.Fatalf("current %q", s.current())
	}
	mnuPress(g, btnOK)
	if s.face != 0 {
		t.Fatal("a locked character switched faces")
	}
	mnuDrawAll(g)
	g.st.Gold = 200
	if !s.opened(g, "muse") {
		t.Fatal("the condition holds but the profile is closed")
	}
	mnuPress(g, btnOK)
	if s.face != 1 || s.faceName(g) != "glad" {
		t.Fatalf("face %d %q", s.face, s.faceName(g))
	}
	mnuDrawAll(g)
	mnuPress(g, btnOK)
	if s.face != 0 {
		t.Fatalf("faces do not wrap: %d", s.face)
	}
	// the buttons under the picture pick a face; moving on resets to the first face
	buttons := s.faceButtons(g, []string{"normal", "glad"})
	if len(buttons) != 2 || s.faceButtons(g, []string{"normal"}) != nil {
		t.Fatalf("buttons %v", buttons)
	}
	g.in.click, g.in.mouse = true, image.Pt(buttons[1].Min.X+3, buttons[1].Min.Y+3)
	mnuTop(g).update(g)
	g.in.click = false
	if s.face != 1 {
		t.Fatalf("click on a face: %d", s.face)
	}
	mnuPress(g, btnDown)
	if s.face != 0 {
		t.Fatalf("a new character kept the face: %d", s.face)
	}
	if !strings.Contains(out.String(), `"face":"glad"`) || !strings.Contains(out.String(), `"unlocked":false`) {
		t.Fatalf("trace %s", out.String())
	}
	// the enemy and actor profiles find their pictures
	if got := g.profilePicture("foe", ""); got != g.db.Enemies["mote"].Image.File {
		t.Fatalf("enemy picture %q", got)
	}
	if got := g.profilePicture("keeper", "calm"); got != "x.png" {
		t.Fatalf("picture %q", got)
	}
	if got := g.profilePicture("muse", "glad"); got != "chars/muse/glad.png" {
		t.Fatalf("actor face %q", got)
	}
	if got := g.profilePicture("muse", ""); got != "" {
		t.Fatalf("no face, no picture: %q", got)
	}
}

// A profile that opened stays open, also in the next run.
func TestProfilesUnlockPersistsAcrossRuns(t *testing.T) {
	dir := t.TempDir()
	g := mnuBookGame(t, Options{SaveDir: dir})
	s := newProfiles(g, nil)
	if s.opened(g, "guide") || g.unlocked[profileKey("guide")] {
		t.Fatal("open before its condition")
	}
	g.st.Flags["guide_met"] = true
	s = newProfiles(g, nil) // opening the book checks the conditions
	if !g.unlocked[profileKey("guide")] || !s.opened(g, "guide") {
		t.Fatalf("not recorded: %v", g.unlocked)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	g2 := mnuBookGame(t, Options{SaveDir: dir})
	g2.st.Flags["guide_met"] = false
	if !newProfiles(g2, nil).opened(g2, "guide") {
		t.Fatal("the profile closed again in the next run")
	}
	// at the title (no state) the book reads a new game's state and still shows what was opened
	g2.st = nil
	s2 := newProfiles(g2, nil)
	if !s2.opened(g2, "guide") || s2.opened(g2, "foe") {
		t.Fatal("title book")
	}
	if s2.env == nil {
		t.Fatal("no environment at the title")
	}
}

func TestProfilesBookKeyRules(t *testing.T) {
	g := mnuBookGame(t, Options{})
	press := func() {
		g.in.pressed[btnBook] = true
		g.bookKey()
	}
	press()
	if _, ok := mnuTop(g).(*profileScreen); !ok {
		t.Fatalf("F6 did not open the book: %T", mnuTop(g))
	}
	if g.in.pressed[btnBook] {
		t.Fatal("the key press was not consumed")
	}
	press() // already open: the book closes itself, bookKey leaves it alone
	if n := len(g.stack); n != 2 {
		t.Fatalf("stack %d", n)
	}
	g.in.pressed[btnBook] = false
	g.stack = g.stack[:1]
	g.replaying = true
	press()
	g.replaying = false
	if len(g.stack) != 1 {
		t.Fatal("the book opened inside a recollection")
	}
	g.in.pressed[btnBook] = false
	g.opts.Development = false
	g.db.Game.UI.Profiles = false
	press()
	if len(g.stack) != 1 {
		t.Fatal("the book opened though the game does not offer it")
	}
	g.in.pressed[btnBook] = false
	g.stack = nil
	g.db.Game.UI.Profiles = true
	press()
	if len(g.stack) != 0 {
		t.Fatal("opened with an empty stack")
	}
}

func TestProfilesLiveKeepAndEmptyBook(t *testing.T) {
	g := mnuBookGame(t, Options{})
	s := newProfiles(g, nil)
	g.push(s)
	s.sel, s.top = 14, 9
	n := mnuKeep[*profileScreen](t, s.liveKeep(g))
	if n.sel != 14 || n.top != 9 {
		t.Fatalf("sel %d top %d", n.sel, n.top)
	}
	// the book shrank on a reload: the selection is kept inside
	g.db.Order["profiles"] = g.db.Order["profiles"][:3]
	n = mnuKeep[*profileScreen](t, s.liveKeep(g))
	if n.sel != 2 || n.top > n.sel {
		t.Fatalf("sel %d top %d", n.sel, n.top)
	}
	// an empty book shows nothing and only closes
	g.db.Order["profiles"] = nil
	e := newProfiles(g, nil)
	g.push(e)
	if e.current() != "" || e.puppet(g) != nil {
		t.Fatal("an empty book has a current character")
	}
	mnuDrawAll(g)
	mnuPress(g, btnDown)
	mnuPress(g, btnOK)
	mnuPress(g, btnCancel)
	if mnuTop(g) == scene(e) {
		t.Fatal("cancel did not close the empty book")
	}
	if e.faceName(g) != "" {
		t.Fatal("face of nobody")
	}
}

func TestProfilesDrawAllKinds(t *testing.T) {
	g := mnuBookGame(t, Options{Development: true})
	g.st.Gold = 300
	s := newProfiles(g, nil)
	g.push(s)
	// every profile in turn: actor, locked-by-flag but open under --dev, enemy, picture
	for i := range s.ids {
		s.sel, s.face = i, 0
		mnuDrawAll(g)
		mnuPress(g)
	}
	if s.sel != len(s.ids)-1 {
		t.Fatalf("sel %d", s.sel)
	}
	// the scout's relation to a locked profile hides the name; unlock changes the line
	g.opts.Development = false
	s.sel = 0
	mnuDrawAll(g)
	if absInt(-3) != 3 || absInt(4) != 4 {
		t.Fatal("absInt")
	}
}
