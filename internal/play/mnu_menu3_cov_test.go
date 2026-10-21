package play

import (
	"testing"
	"testing/fstest"

	lang "github.com/nao1215/gozen/internal/text"
)

const mnuEventItems = `
items:
  lamp: {name: Lamp, kind: key, event: light, command: true, event_when: "flag.dark", consume: false}
  charm: {name: Charm, kind: key, event: bless, event_when: "flag.dark"}
  spare: {name: Spare charm, kind: key, event: bless}
events:
  light: {do: [{set: {flag.lit: true}}]}
  bless: {do: [{set: {flag.blessed: true}}]}
`

func mnuEventGame(t *testing.T) *Game {
	t.Helper()
	fsys := gameFiles(t)
	fsys["data/items2.yaml"] = &fstest.MapFile{Data: []byte(mnuEventItems)}
	g, err := New(Options{FS: fsys, NoSound: true, SaveDir: t.TempDir(), Location: "camp", Seed: 3})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	g.st.Join("guide")
	return g
}

// An item with a command shows at the top of the menu only while its condition holds, and
// running it closes the menu and starts its event.
func TestMenuCommandItem(t *testing.T) {
	g := mnuEventGame(t)
	g.st.AddItem("lamp", 1)
	m := newMenu(g)
	if len(m.cmds) != 0 {
		t.Fatalf("a command item shown while its condition fails: %v", m.cmds)
	}
	g.st.Flags["dark"] = true
	g.push(newMenu(g))
	m = mnuTop(g).(*menu)
	if len(m.cmds) != 1 || m.tabs.items[0].label != "Lamp" {
		t.Fatalf("commands %v tabs %v", m.cmds, labels(m.tabs.items))
	}
	mnuDrawAll(g)
	mnuPress(g, btnOK)
	if len(g.stack) != 1 {
		t.Fatalf("the menu stayed open: %d scenes", len(g.stack))
	}
	if !g.events.Running() {
		t.Fatal("the lamp's event did not start")
	}
	for range 5 {
		_ = g.Update()
	}
	if !g.st.Flags["lit"] {
		t.Fatalf("event did not run: flags %v", g.st.Flags)
	}
	if g.st.Items["lamp"] != 1 {
		t.Fatal("a consume:false item was used up")
	}
	// the condition turned false while the menu was open: the entry buzzes and nothing starts
	g.push(newMenu(g))
	m = mnuTop(g).(*menu)
	g.st.Flags["dark"] = false
	mnuPress(g, btnOK)
	if len(g.stack) != 2 || g.st.Items["lamp"] != 1 {
		t.Fatalf("stack %d lamp %d", len(g.stack), g.st.Items["lamp"])
	}
}

func TestMenuEventItemsInTheItemList(t *testing.T) {
	g := mnuEventGame(t)
	g.st.AddItem("charm", 1)
	g.st.AddItem("spare", 1)
	g.push(newMenu(g))
	m := mnuTop(g).(*menu)
	mnuTab(t, g, lang.T("どうぐ"))
	rows := map[string]listItem{}
	for i, id := range m.ids {
		rows[id] = m.items.items[i]
	}
	if !rows["charm"].disabled || rows["spare"].disabled {
		t.Fatalf("usable flags: charm disabled %v, spare disabled %v", rows["charm"].disabled, rows["spare"].disabled)
	}
	mnuDrawAll(g)
	// the Charm's condition fails: a message, the item stays
	m.items.sel = indexOf(m.ids, "charm")
	m.items.items[m.items.sel].disabled = false // the list greys it, force it through to reach the condition check
	mnuPress(g, btnOK)
	if m.msgT == 0 || m.msgs[0] != lang.T("いまは つかえない。") || g.st.Items["charm"] != 1 {
		t.Fatalf("msgs %v charm %d", m.msgs, g.st.Items["charm"])
	}
	m.msgT = 0
	// the spare has no condition and is consumed; the menu closes and the event runs
	m.items.sel = indexOf(m.ids, "spare")
	mnuPress(g, btnOK)
	if g.st.Items["spare"] != 0 || len(g.stack) != 1 {
		t.Fatalf("spare %d stack %d", g.st.Items["spare"], len(g.stack))
	}
	for range 5 {
		_ = g.Update()
	}
	if !g.st.Flags["blessed"] {
		t.Fatal("the item event did not run")
	}
}

func indexOf(ids []string, id string) int {
	for i, s := range ids {
		if s == id {
			return i
		}
	}
	return 0
}

func TestMenuEquipAndBest(t *testing.T) {
	g := mnuEventGame(t)
	g.st.AddItem("blade", 1)
	g.push(newMenu(g))
	m := mnuTop(g).(*menu)
	scout := g.st.Members()[0]
	mnuTab(t, g, lang.T("そうび"))
	if !m.picker || m.mode != "equip" {
		t.Fatalf("picker %v mode %q", m.picker, m.mode)
	}
	mnuPress(g, btnOK)
	if m.picker || m.slots == nil {
		t.Fatal("equip screen did not open")
	}
	mnuDrawAll(g)
	// the weapon slot: open the candidates, wear the blade
	mnuPress(g, btnOK)
	if m.cands == nil || m.cands.items[0].label != lang.T("（はずす）") {
		t.Fatalf("candidates %+v", m.cands)
	}
	var bladeAt = -1
	for i, it := range m.cands.items {
		if it.id == "blade" {
			bladeAt = i
		}
	}
	if bladeAt < 0 {
		t.Fatalf("blade not offered: %+v", m.cands.items)
	}
	// the stick that is worn is selected at the start
	if m.cands.items[m.cands.sel].id != "" && m.cands.items[m.cands.sel].id != scout.Equip["weapon"] {
		t.Fatalf("selected %q", m.cands.items[m.cands.sel].id)
	}
	m.cands.sel = bladeAt
	mnuDrawAll(g)
	mnuPress(g, btnOK)
	if scout.Equip["weapon"] != "blade" || g.st.Items["stick"] != 1 || g.st.Items["blade"] != 0 {
		t.Fatalf("equip %v bag %v", scout.Equip, g.st.Items)
	}
	if m.cands != nil {
		t.Fatal("the candidate list stayed open after wearing")
	}
	// take it off again
	mnuPress(g, btnOK)
	m.cands.sel = 0
	mnuPress(g, btnOK)
	if scout.Equip["weapon"] != "" || g.st.Items["blade"] != 1 {
		t.Fatalf("remove: equip %v bag %v", scout.Equip, g.st.Items)
	}
	// tab to the next member while choosing
	mnuPress(g, btnOK)
	mnuPress(g, btnNext)
	if m.member != 1 || m.cands == nil {
		t.Fatalf("member %d", m.member)
	}
	mnuShiftTab(g)
	if m.member != 0 {
		t.Fatalf("member %d", m.member)
	}
	mnuPress(g, btnCancel)
	if m.cands != nil {
		t.Fatal("cancel kept the candidates")
	}
	// left / right change member while on the slots
	mnuPress(g, btnRight)
	mnuPress(g, btnLeft)
	mnuPress(g, btnLeft)
	if m.member != 1 {
		t.Fatalf("member %d", m.member)
	}
	mnuPress(g, btnNext)
	mnuShiftTab(g)
	// the best-of-all entry
	m.slots.sel = len(g.db.Game.Slots)
	mnuPress(g, btnOK)
	if m.msgT == 0 {
		t.Fatal("no message after wearing the best")
	}
	for range 80 {
		mnuPress(g)
	}
	if g.st.Members()[0].Equip["weapon"] != "blade" {
		t.Fatalf("best weapon not worn: %v", g.st.Members()[0].Equip)
	}
	mnuDrawAll(g)
	// again: nothing better
	mnuPress(g, btnOK)
	if len(m.msgs) != 1 || m.msgs[0] != lang.T("いまの そうびが いちばん 強い。") {
		t.Fatalf("msgs %v", m.msgs)
	}
	for range 80 {
		mnuPress(g)
	}
	mnuPress(g, btnCancel)
	if !m.picker {
		t.Fatal("cancel from the slots did not return to the picker")
	}
}

func TestMenuStatusScreen(t *testing.T) {
	g, m := mnuMenu(t)
	mnuTab(t, g, lang.T("ステータス"))
	if m.mode != "status" || m.member != 0 {
		t.Fatalf("mode %q member %d", m.mode, m.member)
	}
	mnuDrawAll(g)
	mnuPress(g, btnRight)
	if m.member != 1 {
		t.Fatalf("member %d", m.member)
	}
	mnuPress(g, btnRight)
	if m.member != 0 {
		t.Fatalf("no wrap: %d", m.member)
	}
	mnuPress(g, btnLeft)
	mnuPress(g, btnUp)
	mnuPress(g, btnNext)
	mnuShiftTab(g)
	mnuPress(g, btnDown)
	if m.member != 1 {
		t.Fatalf("member %d", m.member)
	}
	mnuDrawAll(g)
	mnuPress(g, btnOK)
	if m.mode != "" {
		t.Fatalf("mode %q", m.mode)
	}
	// liveKeep rebuilds the menu around the same selection and member
	m.mode, m.member = "status", 1
	m.tabs.sel = 0
	n := m.liveKeep(g).(*menu)
	if n.mode != "status" || n.member != 1 || n.tabs.sel != 0 {
		t.Fatalf("liveKeep mode %q member %d sel %d", n.mode, n.member, n.tabs.sel)
	}
}

func TestMenuDirectModes(t *testing.T) {
	for _, mode := range []string{"status", "item", "skill", "equip"} {
		t.Run(mode, func(t *testing.T) { mnuDirect(t, mode) })
	}
}

func mnuDirect(t *testing.T, mode string) {
	{
		g := openGame(t, Options{Location: "camp", Seed: 3})
		g.st.Join("guide")
		done := 0
		m := newDirectMenu(g, mode, func() { done++ })
		g.push(m)
		mnuDrawAll(g)
		if m.mode != mode {
			t.Fatalf("%s: mode %q", mode, m.mode)
		}
		switch mode {
		case "status":
			mnuPress(g, btnCancel)
		case "item":
			mnuPress(g, btnCancel)
		default:
			mnuPress(g, btnCancel) // picker closes
		}
		mnuPress(g)
		if done != 1 || len(g.stack) != 1 {
			t.Fatalf("%s: done %d stack %d", mode, done, len(g.stack))
		}
	}
}

func TestMenuTitleAndConfirm(t *testing.T) {
	g, m := mnuMenu(t)
	mnuTab(t, g, lang.T("タイトルへ"))
	c, ok := mnuTop(g).(*confirm)
	if !ok {
		t.Fatalf("scene %T", mnuTop(g))
	}
	mnuDrawAll(g)
	// the confirmation ignores input for its first few ticks
	for range 3 {
		mnuPress(g, btnOK)
	}
	if mnuTop(g) != scene(c) {
		t.Fatal("answered before the guard time")
	}
	for range 5 {
		mnuPress(g)
	}
	mnuPress(g, btnRight)
	if !c.no {
		t.Fatal("right did not move to No")
	}
	mnuPress(g, btnOK) // OK on No refuses
	if mnuTop(g) != scene(m) {
		t.Fatalf("No did not return to the menu: %T", mnuTop(g))
	}
	mnuTab(t, g, lang.T("タイトルへ"))
	for range 8 {
		mnuPress(g)
	}
	mnuPress(g, btnOK)
	if g.st != nil {
		t.Fatal("the run was not dropped")
	}
	if _, ok := mnuTop(g).(*title); !ok {
		t.Fatalf("scene %T", mnuTop(g))
	}
}

func TestMenuSaveAndLoadSlots(t *testing.T) {
	g := openGame(t, Options{Location: "camp", Seed: 3})
	g.db.Locations["camp"].Safe = true
	g.push(newMenu(g))
	m := mnuTop(g).(*menu)
	if m.tabs.items[4].disabled {
		t.Fatal("saving is off at a safe place")
	}
	mnuTab(t, g, lang.T("セーブ"))
	sm, ok := mnuTop(g).(*saveMenu)
	if !ok {
		t.Fatalf("scene %T", mnuTop(g))
	}
	mnuDrawAll(g)
	mnuPress(g, btnDown)
	mnuPress(g, btnOK)
	if !g.storeHas(g.slotPath(1)) || sm.msgT == 0 {
		t.Fatalf("slot 2 was not written (msgT %d)", sm.msgT)
	}
	mnuDrawAll(g)
	mnuPress(g, btnOK) // skip the message: the screen closes and the menu's Load is now enabled
	if mnuTop(g) != scene(m) || m.tabs.items[5].disabled {
		t.Fatalf("scene %T, load disabled %v", mnuTop(g), m.tabs.items[5].disabled)
	}
	// saving over a used slot asks first
	mnuTab(t, g, lang.T("セーブ"))
	sm = mnuTop(g).(*saveMenu)
	sm.slots.sel = 1
	mnuPress(g, btnOK)
	c, ok := mnuTop(g).(*confirm)
	if !ok {
		t.Fatalf("no overwrite confirmation: %T", mnuTop(g))
	}
	for range 8 {
		mnuPress(g)
	}
	mnuPress(g, btnCancel) // keep it
	if mnuTop(g) != scene(sm) || c.age == 0 {
		t.Fatalf("scene %T", mnuTop(g))
	}
	mnuPress(g, btnCancel)
	if mnuTop(g) != scene(m) {
		t.Fatalf("scene %T", mnuTop(g))
	}

	// load: a slot that is empty is greyed out; the used one asks first (in play)
	g.st.Gold = 777
	mnuTab(t, g, lang.T("ロード"))
	lm := mnuTop(g).(*saveMenu)
	if !lm.inPlay || lm.slots.sel != 1 {
		t.Fatalf("inPlay %v, newest slot %d", lm.inPlay, lm.slots.sel)
	}
	if !lm.slots.items[0].disabled || lm.slots.items[1].disabled {
		t.Fatalf("slots %+v", lm.slots.items)
	}
	mnuDrawAll(g)
	lm.slots.sel = 0
	mnuPress(g, btnOK) // an empty slot cannot be chosen
	if mnuTop(g) != scene(lm) {
		t.Fatalf("scene %T", mnuTop(g))
	}
	lm.slots.sel = 1
	mnuPress(g, btnOK)
	if _, ok := mnuTop(g).(*confirm); !ok {
		t.Fatalf("no load confirmation: %T", mnuTop(g))
	}
	for range 8 {
		mnuPress(g)
	}
	mnuPress(g, btnOK)
	if g.st.Gold != 100 {
		t.Fatalf("gold %d: the saved state was not restored", g.st.Gold)
	}
}

func TestSaveMenuReportsBrokenSlots(t *testing.T) {
	g := openGame(t, Options{Location: "camp", Seed: 3})
	if err := g.store.Write(g.slotPath(0), []byte("not json")); err != nil {
		t.Fatal(err)
	}
	sm := newSaveMenu(g, false, func() {})
	g.push(sm)
	if !sm.slots.items[0].disabled {
		t.Fatal("a broken save is offered")
	}
	sm.slots.items[0].disabled = false
	sm.slots.sel = 0
	mnuPress(g, btnOK)
	if sm.msgT == 0 || len(sm.msg) == 0 {
		t.Fatal("no message about the broken save")
	}
	mnuDrawAll(g)
	done := 0
	sm.done = func() { done++ }
	for range 100 {
		mnuPress(g)
	}
	if done != 1 {
		t.Fatalf("done called %d times", done)
	}
}
