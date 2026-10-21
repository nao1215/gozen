package play

import (
	"bytes"
	"fmt"
	"slices"
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
			if tt.want && (title[3] != book || title[4] != settings || field[6] != book || field[7] != settings) {
				t.Errorf("places: title %v, menu %v", title, field)
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
