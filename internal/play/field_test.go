package play

import (
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/world"
)

// A campaign whose place is a plain field lists its orders and actions the same way as on
// a map: grouped under their headings, the end of the turn pinned last.
func TestFieldListGroupsAndPinsLikeTheMap(t *testing.T) {
	g := wldOpenWith(t, Options{FS: clashFiles(t, map[string]string{
		"  location: war": "  location: hall\n  pin_end_turn: true\n  end_turn: Next month",
		"label: Attack,":  "label: Attack, group: Abroad,",
		"label: Storm,":   "label: Storm, group: Abroad,",
		"locations:\n":    "locations:\n  hall: {name: Hall, actions: [{id: rest, label: Rest, group: Home, do: []}]}\n",
	}), Location: "hall"})
	wldFrames(t, g, 10)
	f, ok := g.stack[0].(*field)
	if !ok {
		t.Fatalf("scene %T, want the field", g.stack[0])
	}
	var got []string
	for _, it := range f.menu.items {
		if it.heading {
			got = append(got, "#"+it.label)
		} else {
			got = append(got, it.label)
		}
	}
	l := newEntryList(g, func(*Game, world.Entry) {})
	var want []string
	for _, it := range l.menu.items {
		if it.heading {
			want = append(want, "#"+it.label)
		} else {
			want = append(want, it.label)
		}
	}
	if strings.Join(got, "|") != strings.Join(want, "|") || got[len(got)-1] != "Next month" || f.menu.pin != l.menu.pin {
		t.Fatalf("field rows %v (pin %v), the list's %v (pin %v)", got, f.menu.pin, want, l.menu.pin)
	}
	// choosing a row takes its entry, not the one at the same index of the plain entries
	f.menu.sel = len(f.menu.items) - 1
	g.in.pressed[btnOK] = true
	f.update(g)
	g.in.pressed = [numButtons]bool{}
	wldFrames(t, g, 30)
	if turn := data.Turn(g.db, g.st.Campaign()); turn != 2 {
		t.Errorf("the last row left turn %d, want 2", turn)
	}
}
