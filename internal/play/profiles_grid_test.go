package play

import (
	"image"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/event"
)

// gridBookFS is a book at schema version 10 laid out as a grid, with a muse whose glad
// face opens on a flag of its own, a voice without pictures, and two groups.
func gridBookFS(t *testing.T) fstest.MapFS {
	t.Helper()
	var b strings.Builder
	b.WriteString(`actors:
  muse: {name: Muse, class: ranger, faces: chars/muse}
  voice: {name: Voice, class: ranger}
profiles:
  muse:
    affiliation: Team
    faces: [{face: normal, label: As seen}, {face: glad, label: As it is, when: flag.real}]
    text: Sings.
  voice:
    affiliation: Team
    when: flag.heard
  keeper:
    affiliation: Others
    picture: x.png
    name: Keeper
`)
	for i := range 30 {
		b.WriteString("  filler" + itoa(i) + ": {affiliation: Others, picture: x.png, name: Filler}\n")
	}
	fsys := gameFiles(t)
	fsys["data/book.yaml"] = &fstest.MapFile{Data: []byte(b.String())}
	for _, f := range []string{"normal", "glad"} {
		fsys["assets/chars/muse/"+f+".png"] = &fstest.MapFile{Data: fsys["assets/x.png"].Data}
	}
	head := "schema_version: 10\nui: {profiles: true, profile_book: {title: The book, layout: grid}, material: {kind: glass, wallpaper: ['#b5cbf4', '#49c9ca', '#dc97bc']}}\n"
	fsys["game.yaml"] = &fstest.MapFile{Data: append([]byte(head), fsys["game.yaml"].Data...)}
	return fsys
}

func gridBookGame(t *testing.T, dir string) *Game {
	t.Helper()
	g, err := New(Options{FS: gridBookFS(t), NoSound: true, SaveDir: dir, Location: "camp", Seed: 1})
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

// TestProfileGridNavigation: the grid groups the cards by affiliation, the arrows move
// along a row and between rows, a click picks a card, and the grid scrolls to show it.
func TestProfileGridNavigation(t *testing.T) {
	g := gridBookGame(t, t.TempDir())
	if g.profileTitle() != "The book" || !g.profileGrid() {
		t.Fatalf("title %q grid %v", g.profileTitle(), g.profileGrid())
	}
	s := newProfiles(g, nil)
	g.push(s)
	cells, height := s.gridCells(g)
	headings := 0
	for _, c := range cells {
		if c.index < 0 {
			headings++
		}
	}
	if headings != 2 || cells[0].heading != "Team" {
		t.Fatalf("headings %d first %+v", headings, cells[0])
	}
	mnuDrawAll(g)
	mnuPress(g, btnRight)
	if s.current() != "voice" {
		t.Fatalf("right: %q", s.current())
	}
	mnuPress(g, btnDown) // the next row is the other group's, in the same column
	if s.current() != "filler0" {
		t.Fatalf("down: %q", s.current())
	}
	mnuPress(g, btnUp)
	if s.current() != "voice" {
		t.Fatalf("up: %q", s.current())
	}
	mnuPress(g, btnLeft)
	mnuPress(g, btnLeft) // wraps to the last, far down: the grid scrolls
	if s.sel != len(s.ids)-1 {
		t.Fatalf("left: %d", s.sel)
	}
	_, _, _, h := s.gridArea(g)
	if height <= h || s.scroll <= 0 {
		t.Fatalf("height %v view %v scroll %v", height, h, s.scroll)
	}
	mnuDrawAll(g)
	r, ok := s.gridCard(g, cells, s.sel-1)
	if !ok {
		t.Fatal("no card")
	}
	g.in.click, g.in.mouse = true, image.Pt(r.Min.X+5, r.Min.Y+5)
	mnuTop(g).update(g)
	g.in.click = false
	if s.sel != len(s.ids)-2 {
		t.Fatalf("click: %d", s.sel)
	}
	back := s.gridBack()
	g.in.click, g.in.mouse = true, image.Pt(back.Min.X+5, back.Min.Y+5)
	mnuTop(g).update(g)
	g.in.click = false
	if _, open := mnuTop(g).(*profileScreen); open {
		t.Fatal("the button back left the book open")
	}
}

// TestProfileGridFaces: a face with a when of its own is not offered until it holds,
// then stays offered in the next run; a voice without pictures draws an outline.
func TestProfileGridFaces(t *testing.T) {
	dir := t.TempDir()
	g := gridBookGame(t, dir)
	s := newProfiles(g, nil)
	g.push(s)
	if got := s.faceLabels(g, "muse"); len(got) != 1 || got[0] != "As seen" {
		t.Fatalf("labels before: %v", got)
	}
	if s.gridFaceButtons(g, "muse") != nil {
		t.Fatal("buttons for one face")
	}
	mnuPress(g, btnOK)
	if s.face != 0 {
		t.Fatal("switched to a locked face")
	}
	mnuDrawAll(g)
	g.st.Flags["real"] = true
	s = newProfiles(g, nil)
	g.pop()
	g.push(s)
	if !g.unlocked[profileFaceKey("muse", "glad")] {
		t.Fatalf("face not recorded: %v", g.unlocked)
	}
	buttons := s.gridFaceButtons(g, "muse")
	if len(buttons) != 2 {
		t.Fatalf("buttons %v", buttons)
	}
	g.in.click, g.in.mouse = true, image.Pt(buttons[1].Min.X+3, buttons[1].Min.Y+3)
	mnuTop(g).update(g)
	g.in.click = false
	if s.faceName(g) != "glad" {
		t.Fatalf("face %q", s.faceName(g))
	}
	mnuDrawAll(g)
	mnuPress(g, btnRight) // the voice: locked, then heard
	if s.opened(g, "voice") || g.profilePicture("voice", s.faceName(g)) != "" {
		t.Fatal("voice")
	}
	mnuDrawAll(g)
	g.st.Flags["heard"] = true
	mnuDrawAll(g)
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	g2 := gridBookGame(t, dir)
	s2 := newProfiles(g2, nil)
	if !s2.faceOpen(g2, "muse", 1) || s2.faceOpen(g2, "nobody", 0) == false {
		t.Fatal("the face closed again in the next run")
	}
}

// TestSysMenuOffersTheBook: the menu over a planning screen offers the book after
// loading, and the items after it keep their screens.
func TestSysMenuOffersTheBook(t *testing.T) {
	g := gridBookGame(t, t.TempDir())
	m := newSysMenu(g)
	if m.l.items[sysProfiles].label != "The book" || m.l.items[sysProfiles].id != "profiles" || len(m.l.items) != 6 {
		t.Fatalf("items %+v", m.l.items)
	}
	g.push(m)
	m.l.sel = sysProfiles
	mnuPress(g, btnOK)
	if _, open := mnuTop(g).(*profileScreen); !open {
		t.Fatalf("top %T", mnuTop(g))
	}
	g.pop()
	m = newSysMenu(g)
	g.push(m)
	m.l.sel = sysProfiles + 1 // the settings
	mnuPress(g, btnOK)
	if _, open := mnuTop(g).(*configScreen); !open {
		t.Fatalf("top %T", mnuTop(g))
	}
}

// TestProfileMetByALine: a line of the actor opens a met profile, and a line with a
// seen face opens that face; both stay open in the next run.
func TestProfileMetByALine(t *testing.T) {
	dir := t.TempDir()
	fsys := gridBookFS(t)
	book := strings.Replace(string(fsys["data/book.yaml"].Data), "    faces: [{face: normal, label: As seen}, {face: glad, label: As it is, when: flag.real}]\n", "    met: true\n    faces: [{face: normal, label: As seen}, {face: glad, label: As it is, seen: true}]\n", 1)
	fsys["data/book.yaml"] = &fstest.MapFile{Data: []byte(book)}
	g, err := New(Options{FS: fsys, NoSound: true, SaveDir: dir, Location: "camp", Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	s := newProfiles(g, nil)
	if s.opened(g, "muse") || s.faceOpen(g, "muse", 1) {
		t.Fatal("open before meeting")
	}
	g.meetLine(event.Say{Actor: "muse", Face: "chars/muse/normal.png"})
	if !s.opened(g, "muse") || s.faceOpen(g, "muse", 1) {
		t.Fatal("meeting did not open the page, or opened the unseen face")
	}
	g.meetLine(event.Say{Actor: "muse", Face: "chars/muse/glad.png"})
	if !s.faceOpen(g, "muse", 1) {
		t.Fatal("the seen face is closed")
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	g2, err := New(Options{FS: fsys, NoSound: true, SaveDir: dir, Location: "camp", Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g2.Close(); err != nil {
			t.Error(err)
		}
	})
	s2 := newProfiles(g2, nil)
	if !s2.opened(g2, "muse") || !s2.faceOpen(g2, "muse", 1) {
		t.Fatal("the next run forgot the meeting")
	}
}
