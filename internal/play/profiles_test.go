package play

import (
	"bytes"
	"fmt"
	"image"
	"slices"
	"strconv"
	"testing"
	"testing/fstest"

	lang "github.com/nao1215/gozen/internal/text"
)

// bookData is a character book of two: the keeper is open from the start, the
// stranger once flag.met is set; and an event that opens the book.
const bookData = `profiles:
  keeper:
    name: Keeper
    picture: x.png
    title: Of the door
    fields: [{label: Role, value: Keeps the door}]
    text: Has {gold} gold.
    relations: [{who: stranger, calls: Friend, text: An old friend.}, {who: you, calls: Guest}]
  stranger:
    picture: x.png
    when: flag.met
events:
  book:
    do:
      - say: "Before the book."
      - open: profiles
      - say: "After the book."
`

func bookGame(t *testing.T, opts Options, head string) *Game {
	t.Helper()
	fsys := gameFiles(t)
	fsys["data/book.yaml"] = &fstest.MapFile{Data: []byte(bookData)}
	fsys["game.yaml"] = &fstest.MapFile{Data: append([]byte("schema_version: 4\n"+head), fsys["game.yaml"].Data...)}
	opts.FS, opts.NoSound = fsys, true
	if opts.SaveDir == "" {
		opts.SaveDir = t.TempDir()
	}
	if opts.ShotDir == "" {
		opts.ShotDir = t.TempDir()
	}
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

func labels(items []listItem) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.label)
	}
	return out
}

// TestProfilesInMenus: the title and the field menu offer Characters when game.yaml
// asks for it (ui.profiles), and always under --dev; the other entries keep their place.
// The novel bar has the button only for ui.profiles.
func TestProfilesInMenus(t *testing.T) {
	for _, tt := range []struct {
		name string
		head string
		dev  bool
		want bool
	}{
		{"off by default", "", false, false},
		{"ui.profiles", "ui: {profiles: true}\n", false, true},
		{"--dev", "", true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := bookGame(t, Options{Location: "camp", Development: tt.dev}, tt.head)
			book, settings := lang.T("キャラクター"), lang.T("せってい")
			title := labels(g.titleMenu().items)
			field := labels(newMenu(g).tabs.items)
			if got := slices.Contains(title, book); got != tt.want {
				t.Errorf("title %v", title)
			}
			if got := slices.Contains(field, book); got != tt.want {
				t.Errorf("field menu %v", field)
			}
			if tt.want {
				if tt.dev {
					if title[3] != book || title[4] != lang.T("Monsters") || title[5] != settings || field[6] != book || field[7] != lang.T("Monsters") || field[8] != settings {
						t.Errorf("places: title %v, menu %v", title, field)
					}
				} else if title[3] != book || title[4] != settings || field[6] != book || field[7] != settings {
					t.Errorf("places: title %v, menu %v", title, field)
				}
			}
			buttons := g.novelButtons()
			if got := slices.ContainsFunc(buttons, func(b novelButton) bool { return b.b == btnBook }); got != (tt.head != "") {
				t.Errorf("novel bar %v", buttons)
			}
		})
	}
}

// TestProfilesScreen: open: profiles shows the book; right goes to the next character
// and wraps; the stranger is locked until flag.met, and once met stays in the record of
// unlocks; cancel goes back to the event.
func TestProfilesScreen(t *testing.T) {
	g := bookGame(t, Options{
		Location: "camp", Event: "book",
		Script: "wait 10\npress ok\nwait 5\npress right\nwait 3\npress right\nwait 3\npress cancel\nwait 20\nquit\n",
	}, "")
	var out bytes.Buffer
	g.tr = newTracer(&out)
	runToEnd(t, g)
	shown := traceLines(t, out.Bytes(), "profiles")
	var ids []string
	for _, l := range shown {
		id := fmt.Sprint(l["profile"])
		ids = append(ids, id)
	}
	if !slices.Equal(ids, []string{"keeper", "stranger", "keeper"}) {
		t.Fatalf("shown %v", shown)
	}
	if shown[0]["unlocked"] != true || shown[1]["unlocked"] != false {
		t.Fatalf("unlocked: %v", shown)
	}
	if at(g) != "After the book." {
		t.Fatalf("at %q after the book", at(g))
	}
	if g.unlocked[profileKey("stranger")] || !g.unlocked[profileKey("keeper")] {
		t.Fatalf("unlocks %v", g.unlocked)
	}
	g.st.Flags["met"] = true
	g.checkUnlocks()
	if !g.unlocked[profileKey("stranger")] {
		t.Fatal("meeting the stranger did not unlock the profile")
	}
	g.st.Flags["met"] = false
	s := newProfiles(g, nil)
	if !s.opened(g, "stranger") {
		t.Fatal("the profile closed again when the flag was cleared")
	}
}

// TestProfilesDevOpensAll: under --dev every profile is open.
func TestProfilesDevOpensAll(t *testing.T) {
	g := bookGame(t, Options{Location: "camp", Development: true}, "")
	s := newProfiles(g, nil)
	if !s.opened(g, "stranger") || g.unlocked[profileKey("stranger")] {
		t.Fatal("--dev: the stranger is not shown, or was recorded as met")
	}
}

// TestFaceIconFromFaceBoxes: a face icon is cut around the box gozen art faces wrote
// (assets/faces.json), scaled to the picture as loaded; without one, as before.
func TestFaceIconFromFaceBoxes(t *testing.T) {
	p := newPictures(fstest.MapFS{"assets/faces.json": {Data: []byte(`{"version": 1, "pictures": {
  "a/normal.png": {"box": [100, 40, 140, 80], "size": [400, 1000]},
  "b/normal.png": {"size": [400, 1000]}}}`)}})
	p.mu.Lock()
	defer p.mu.Unlock()
	r, ok := p.foundFace("a/normal.png", image.Pt(200, 500))
	if !ok || r.Min.X+r.Max.X != 2*60 || r.Min.Y+r.Max.Y != 2*30 || r.Dx() <= 20 {
		t.Fatalf("a: %v %v, want a square around (60, 30) wider than the face", r, ok)
	}
	if _, ok := p.foundFace("b/normal.png", image.Pt(400, 1000)); ok {
		t.Fatal("b: a box where none was found")
	}
	if _, ok := p.foundFace("c/normal.png", image.Pt(400, 1000)); ok {
		t.Fatal("c: a box for a picture not in the file")
	}
}

// TestProfilesF6: under --dev F6 opens the book on the field and F6 closes it; without
// --dev and ui.profiles it does nothing.
func TestProfilesF6(t *testing.T) {
	for _, dev := range []bool{true, false} {
		t.Run(strconv.FormatBool(dev), func(t *testing.T) { profilesF6(t, dev) })
	}
}

func profilesF6(t *testing.T, dev bool) {
	t.Helper()
	g := bookGame(t, Options{Location: "camp", Development: dev, Script: "wait 10\npress characters\nwait 5\nquit\n"}, "")
	runToEnd(t, g)
	_, open := g.stack[len(g.stack)-1].(*profileScreen)
	if open != dev {
		t.Fatalf("dev %v: the book is open %v after F6", dev, open)
	}
	if dev {
		g.in.pressed[btnBook] = true
		g.stack[len(g.stack)-1].update(g)
		if _, still := g.stack[len(g.stack)-1].(*profileScreen); still {
			t.Fatal("F6 did not close the book")
		}
	}
}
