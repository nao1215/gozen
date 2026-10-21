package play

import (
	"testing"

	"github.com/nao1215/gozen/internal/layer1/memo"
)

func sysOpenMemoBox(t *testing.T) *Game {
	t.Helper()
	g := sysOpen4(t, Options{Seed: 1, Location: "hall", MemoDir: t.TempDir()}, map[string]string{"data/hall.yaml": sysCorridor, "game.yaml+": sysMemoYAML})
	g.openMemo()
	g.memo.age = 10
	return g
}

// TestMemoBoxDrawsEveryStateOfTheInput: the box is drawn while choosing the kind, while typing, with
// text in composition, with long text, and while the note is typed in another window.
func TestMemoBoxDrawsEveryStateOfTheInput(t *testing.T) {
	g := sysOpenMemoBox(t)
	sysSafeDraw(g)
	g.memo.typed = true
	g.memo.text = "plain"
	sysSafeDraw(g)
	g.memo.comp = "かな"
	sysSafeDraw(g)
	g.memo.text = ""
	for range 80 {
		g.memo.text += "long note "
	}
	g.memo.comp = ""
	sysSafeDraw(g)
	g.memo.ext = make(chan extInput, 1)
	sysSafeDraw(g)
	if g.memo == nil {
		t.Fatal("the box vanished while drawing")
	}
}

// TestMemoTypedInAnotherWindow: a note typed in another window is saved when it returns, dropped
// when it is cancelled, and waited for while it has not come back.
func TestMemoTypedInAnotherWindow(t *testing.T) {
	g := sysOpenMemoBox(t)
	g.memo.typed = true
	g.memo.ext = make(chan extInput, 1)
	g.updateMemo()
	if g.memo == nil || g.memo.ext == nil {
		t.Fatal("the box closed before the other window answered")
	}
	g.memo.ext <- extInput{text: "from outside", ok: true}
	g.updateMemo()
	if g.memo != nil {
		t.Fatal("the box stayed open after the note came back")
	}
	files := sysMemoFiles(t, g.opts.MemoDir)
	if len(files) != 1 {
		t.Fatalf("notes %v", files)
	}
	m, _, err := memo.Read(files[0])
	if err != nil || m.Text != "from outside" {
		t.Fatalf("note %+v (%v)", m, err)
	}

	g.openMemo()
	g.memo.age = 10
	g.memo.typed = true
	g.memo.ext = make(chan extInput, 1)
	g.memo.ext <- extInput{ok: false}
	g.updateMemo()
	if g.memo != nil || len(sysMemoFiles(t, g.opts.MemoDir)) != 1 {
		t.Fatal("a cancelled note was kept")
	}
}

// TestMemoKindChoiceWaitsFourTicks: the first ticks of the box ignore input so the key that opened
// it does not choose a kind.
func TestMemoKindChoiceWaitsFourTicks(t *testing.T) {
	g := sysOpen4(t, Options{Seed: 1, Location: "hall", MemoDir: t.TempDir()}, map[string]string{"data/hall.yaml": sysCorridor})
	g.openMemo()
	g.in.script = nil
	sysKey(g, btnOK)
	g.updateMemo()
	if g.memo.typed {
		t.Fatal("a kind was chosen on the first tick")
	}
	g.memo.age = 10
	sysKey(g, btnOK)
	g.updateMemo()
	if !g.memo.typed {
		t.Fatal("ok did not choose the kind")
	}
	g.closeMemo()
	if g.memo != nil {
		t.Fatal("closeMemo left the box open")
	}
}

// TestTypeMemoGoesToTheBoxThatAsksForIt: text typed by a script goes to the memo box once a kind is
// chosen, and nowhere before.
func TestTypeMemoGoesToTheBoxThatAsksForIt(t *testing.T) {
	g := sysOpenMemoBox(t)
	g.typeMemo("early")
	if g.memo.text != "" {
		t.Fatal("text typed before a kind was chosen")
	}
	g.memo.typed = true
	g.typeMemo("late")
	if g.memo.text != "late" {
		t.Fatalf("text %q", g.memo.text)
	}
	g.closeMemo()
	g.typeMemo("nobody listens")
}
