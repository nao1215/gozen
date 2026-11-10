package play

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"testing/fstest"

	lang "github.com/nao1215/gozen/internal/base/text"
	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/screen/gfx"
)

// mnuTop is the scene on top of the stack.
func mnuTop(g *Game) scene { return g.stack[len(g.stack)-1] }

// mnuAs is the scene on top of the stack as type T; the test stops when it is another one.
func mnuAs[T scene](t *testing.T, g *Game) T {
	t.Helper()
	v, ok := mnuTop(g).(T)
	if !ok {
		t.Fatalf("top scene is %T", mnuTop(g))
	}
	return v
}

// mnuKeep is the scene a live reload keeps, as type T.
func mnuKeep[T scene](t *testing.T, s scene) T {
	t.Helper()
	v, ok := s.(T)
	if !ok {
		t.Fatalf("kept scene is %T", s)
	}
	return v
}

// mnuPress feeds one tick of input (the given buttons pressed) to the top scene.
func mnuPress(g *Game, btns ...button) {
	g.in.pressed = [numButtons]bool{}
	for _, b := range btns {
		g.in.pressed[b] = true
	}
	mnuTop(g).update(g)
	g.in.pressed = [numButtons]bool{}
}

// mnuShiftTab is a Shift+Tab tick.
func mnuShiftTab(g *Game) {
	g.in.shift = true
	mnuPress(g, btnNext)
	g.in.shift = false
}

// mnuDrawAll draws the whole game once (only queues GPU commands, no pixel reads).
func mnuDrawAll(g *Game) { g.Draw(covScreen()) }

// mnuMenu opens the field menu of a new camp game with the guide in the party.
func mnuMenu(t *testing.T) (*Game, *menu) {
	t.Helper()
	g := openGame(t, Options{Location: "camp", Seed: 3})
	g.st.Join("guide")
	g.push(newMenu(g))
	return g, mnuAs[*menu](t, g)
}

// mnuTab picks the tab with the label and presses OK.
func mnuTab(t *testing.T, g *Game, label string) {
	t.Helper()
	m := mnuAs[*menu](t, g)
	for i, it := range m.tabs.items {
		if it.label == label {
			m.tabs.sel = i
			mnuPress(g, btnOK)
			return
		}
	}
	t.Fatalf("no tab %q in %v", label, labels(m.tabs.items))
}

func TestMenuTabsAndSaveRules(t *testing.T) {
	g, m := mnuMenu(t)
	// camp is not marked safe in the test game: saving is disabled (shown with a cross).
	var save, load *listItem
	for i := range m.tabs.items {
		switch m.tabs.items[i].label {
		case lang.T("セーブ"):
			save = &m.tabs.items[i]
		case lang.T("ロード"):
			load = &m.tabs.items[i]
		}
	}
	if save == nil || load == nil {
		t.Fatalf("tabs %v", labels(m.tabs.items))
	}
	if !save.disabled || save.right != "×" {
		t.Fatalf("save on an unsafe place is allowed: %+v", save)
	}
	if !load.disabled {
		t.Fatal("load offered with no save")
	}
	// choosing the disabled save says why
	mnuTab(t, g, lang.T("セーブ"))
	if m.msgT == 0 || len(m.msgs) != 1 || !strings.Contains(m.msgs[0], lang.T("ここでは セーブ できない。安全な 場所で セーブ しよう。")) {
		t.Fatalf("no reason for the disabled save: %v", m.msgs)
	}
	m.msgT = 0
	// an empty bag says so in the item list
	for id := range g.st.Items {
		delete(g.st.Items, id)
	}
	m.openItems(g)
	if len(m.items.items) != 0 || m.items.empty == "" {
		t.Fatalf("empty bag: %d rows, empty text %q", len(m.items.items), m.items.empty)
	}
	m.mode = "item"
	mnuDrawAll(g)
	m.mode = ""
	mnuPress(g, btnCancel)
	if len(g.stack) != 1 {
		t.Fatalf("cancel left the menu open: %d scenes", len(g.stack))
	}
}

func TestMenuUseItemOnAMember(t *testing.T) {
	g, m := mnuMenu(t)
	scout, guide := g.st.Members()[0], g.st.Members()[1]
	scout.HP, guide.HP = 5, 5
	mnuTab(t, g, lang.T("どうぐ"))
	if m.mode != "item" || len(m.ids) == 0 {
		t.Fatalf("mode %q ids %v", m.mode, m.ids)
	}
	for i, id := range m.ids {
		if id == "potion" {
			m.items.sel = i
		}
	}
	mnuPress(g, btnOK)
	if !m.targetOn {
		t.Fatal("a one-ally item did not ask for a target")
	}
	mnuDrawAll(g)
	mnuPress(g, btnDown)
	if m.tsel != 1 {
		t.Fatalf("target %d", m.tsel)
	}
	mnuPress(g, btnUp)
	mnuPress(g, btnDown)
	mnuPress(g, btnOK)
	if guide.HP != 35 || scout.HP != 5 {
		t.Fatalf("heal landed on the wrong member: scout %d guide %d", scout.HP, guide.HP)
	}
	if g.st.Items["potion"] != 1 || len(m.msgs) == 0 || m.msgT == 0 {
		t.Fatalf("potions %d msgs %v", g.st.Items["potion"], m.msgs)
	}
	mnuDrawAll(g)
	// the message is soft: it counts down by itself and the list is usable again
	for range 120 {
		mnuPress(g)
	}
	if m.msgT != 0 {
		t.Fatalf("message stuck at %d", m.msgT)
	}
	// use the last potion; with the bag empty the target picker closes
	scout.HP = 5
	m.targetOn = false
	mnuPress(g, btnOK)
	if !m.targetOn {
		t.Fatal("no target picker for the last potion")
	}
	mnuPress(g, btnOK)
	if g.st.Items["potion"] != 0 || m.targetOn {
		t.Fatalf("last potion: %d target %v", g.st.Items["potion"], m.targetOn)
	}
	mnuDrawAll(g)
	// cancel leaves the item list
	for range 120 {
		mnuPress(g)
	}
	mnuPress(g, btnCancel)
	if m.mode != "" {
		t.Fatalf("mode %q", m.mode)
	}
}

func TestMenuItemWithFullHealthIsRefused(t *testing.T) {
	g, m := mnuMenu(t)
	mnuTab(t, g, lang.T("どうぐ"))
	for i, id := range m.ids {
		if id == "potion" {
			m.items.sel = i
		}
	}
	mnuPress(g, btnOK)
	mnuPress(g, btnOK) // everyone is at full health: a heal changes nothing
	if g.st.Items["potion"] != 2 {
		t.Fatalf("a wasted potion was consumed: %d left", g.st.Items["potion"])
	}
	if m.msgT == 0 || len(m.msgs) != 1 || m.msgs[0] != lang.T("つかっても こうかが ない") {
		t.Fatalf("msgs %v", m.msgs)
	}
	// a hard message blocks input until it is clicked away or runs out
	m.msgT = 50
	mnuPress(g, btnOK)
	if m.msgT != 0 {
		t.Fatalf("OK did not dismiss the message: %d", m.msgT)
	}
	mnuPress(g, btnCancel)
	if m.targetOn {
		t.Fatal("cancel did not close the target picker")
	}
}

func TestMenuSkillsAndMembers(t *testing.T) {
	g, m := mnuMenu(t)
	guide := g.st.Members()[1]
	guide.HP = 1
	mnuTab(t, g, lang.T("スキル"))
	if !m.picker || m.mode != "skill" {
		t.Fatalf("picker %v mode %q", m.picker, m.mode)
	}
	mnuDrawAll(g)
	mnuPress(g, btnDown)
	mnuPress(g, btnDown) // wraps to the first
	mnuPress(g, btnRight)
	mnuPress(g, btnLeft)
	mnuPress(g, btnUp) // up from the first wraps to the guide
	if m.member != 1 {
		t.Fatalf("member %d", m.member)
	}
	mnuPress(g, btnOK)
	if m.picker || m.items == nil {
		t.Fatal("picking a member did not open the skills")
	}
	mnuDrawAll(g)
	mnuShiftTab(g)
	if m.member != 0 {
		t.Fatalf("shift-tab member %d", m.member)
	}
	mnuPress(g, btnNext)
	if m.member != 1 {
		t.Fatalf("tab member %d", m.member)
	}
	guide.GainExp(1_000_000)
	guide.MP = 10
	m.openSkills(g)
	healAt := -1
	for i, id := range m.ids {
		if id == "heal" {
			healAt = i
		}
	}
	if healAt < 0 {
		t.Fatalf("the guide does not know Heal: %v", m.ids)
	}
	m.items.sel = healAt
	mnuPress(g, btnOK)
	if !m.targetOn {
		t.Fatal("Heal did not ask for a target")
	}
	for m.tsel != 1 {
		mnuPress(g, btnDown)
	}
	before := guide.MP
	mnuPress(g, btnOK)
	if guide.HP <= 1 || guide.MP != before-3 {
		t.Fatalf("hp %d mp %d (was %d)", guide.HP, guide.MP, before)
	}
	// run out of MP: the target picker closes by itself
	for range 120 {
		mnuPress(g)
	}
	guide.MP = 3
	guide.HP = 1
	mnuPress(g, btnOK)
	mnuDrawAll(g)
	for m.tsel != 1 {
		mnuPress(g, btnDown)
	}
	mnuPress(g, btnOK)
	if guide.MP != 0 || m.targetOn {
		t.Fatalf("mp %d target %v", guide.MP, m.targetOn)
	}
	for range 120 {
		mnuPress(g)
	}
	mnuPress(g, btnCancel) // back to the picker
	if !m.picker {
		t.Fatal("cancel from the skill list did not return to the member picker")
	}
	mnuPress(g, btnCancel)
	if m.picker || m.mode != "" {
		t.Fatalf("picker %v mode %q", m.picker, m.mode)
	}
}

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
	m = mnuAs[*menu](t, g)
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
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
	}
	if !g.st.Flags["lit"] {
		t.Fatalf("event did not run: flags %v", g.st.Flags)
	}
	if g.st.Items["lamp"] != 1 {
		t.Fatal("a consume:false item was used up")
	}
	// the condition turned false while the menu was open: the entry buzzes and nothing starts
	g.push(newMenu(g))
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
	m := mnuAs[*menu](t, g)
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
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
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
	m := mnuAs[*menu](t, g)
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
	bladeAt := -1
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
	n := mnuKeep[*menu](t, m.liveKeep(g))
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
	m := mnuAs[*menu](t, g)
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
	sm = mnuAs[*saveMenu](t, g)
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
	lm := mnuAs[*saveMenu](t, g)
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
	if sm.msgT == 0 || sm.msg == "" {
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

func TestConfirmFitsPortraitAndLandscape(t *testing.T) {
	for _, dims := range []image.Point{image.Pt(960, 540), image.Pt(360, 540), image.Pt(640, 360)} {
		t.Run(fmt.Sprintf("%dx%d", dims.X, dims.Y), func(t *testing.T) {
			g, _ := mnuMenu(t)
			g.view.w, g.view.h = dims.X, dims.Y
			const warning = "Loading this saved game will replace your current progress. Any progress since your last save will be lost. Do you want to continue?"
			accepted, refused := 0, 0
			c := newConfirm(warning, func() { accepted++ })
			c.onNo = func() { refused++ }
			lines, size, y, h, yes, no := c.layout(g)
			if y < 0 || y+h > float64(dims.Y) {
				t.Fatalf("dialog outside screen: y=%v h=%v", y, h)
			}
			for _, line := range lines {
				if gfx.TextWidth(line, size) > float64(dims.X-40) {
					t.Fatalf("warning outside screen: %q", line)
				}
			}
			if dims.X < 720 && strings.Contains(strings.Join(lines, ""), "…") {
				t.Fatal("compact warning was truncated instead of wrapped")
			}
			bounds := image.Rect(0, 0, dims.X, dims.Y)
			if !yes.In(bounds) || !no.In(bounds) || yes.Overlaps(no) {
				t.Fatalf("buttons outside screen: %v %v", yes, no)
			}
			for _, button := range []image.Rectangle{no, yes} {
				c.age = 6
				g.push(c)
				g.in = input{click: true, mouse: button.Min.Add(image.Pt(button.Dx()/2, button.Dy()/2))}
				c.update(g)
				if mnuTop(g) == scene(c) {
					t.Fatal("pointer did not close confirmation")
				}
			}
			if accepted != 1 || refused != 1 {
				t.Fatalf("wrong callbacks yes=%d no=%d", accepted, refused)
			}
		})
	}
}

// The field menu offers the character book when the game asks for it, and the entries
// after it keep working (Settings, Title, Close are numbered past the extra row).
func TestMenuCharactersEntryKeepsOtherEntries(t *testing.T) {
	g := mnuBookGame(t, Options{})
	g.push(newMenu(g))
	m := mnuAs[*menu](t, g)
	mnuTab(t, g, lang.T("キャラクター"))
	if _, ok := mnuTop(g).(*profileScreen); !ok {
		t.Fatalf("scene %T", mnuTop(g))
	}
	mnuPress(g, btnCancel)
	if mnuTop(g) != scene(m) {
		t.Fatalf("scene %T", mnuTop(g))
	}
	mnuTab(t, g, lang.T("せってい"))
	if _, ok := mnuTop(g).(*configScreen); !ok {
		t.Fatalf("scene %T", mnuTop(g))
	}
	g.stack = g.stack[:2]
	mnuTab(t, g, lang.T("とじる"))
	if len(g.stack) != 1 {
		t.Fatalf("Close left %d scenes", len(g.stack))
	}
}

// The status page lists the states a member has that describe themselves, and the stats.
func TestMenuStatusShowsStates(t *testing.T) {
	g, m := mnuMenu(t)
	g.db.States["poison"].Desc = "Loses a little each turn."
	g.st.Members()[0].States["poison"] = 3
	mnuTab(t, g, lang.T("ステータス"))
	mnuDrawAll(g)
	if m.mode != "status" {
		t.Fatalf("mode %q", m.mode)
	}
	// a click leaves the status page
	g.in.click, g.in.mouse = true, image.Pt(5, 5)
	mnuTop(g).update(g)
	g.in.click = false
	if m.mode != "" {
		t.Fatalf("mode %q", m.mode)
	}
}

// With the mouse the item list follows the pointer and a click chooses.
func TestMenuMouseChoosesItemAndMember(t *testing.T) {
	g, m := mnuMenu(t)
	g.st.Members()[1].HP = 3
	mnuTab(t, g, lang.T("どうぐ"))
	row := m.items.rowRect(0)
	g.in.click, g.in.mouse = true, image.Pt(row.Min.X+5, row.Min.Y+5)
	mnuTop(g).update(g)
	g.in.click = false
	if !m.targetOn {
		t.Fatal("a click on the item did not ask for a target")
	}
	card := m.cardRect(g, 1)
	g.in.moved, g.in.mouse = true, image.Pt(card.Min.X+5, card.Min.Y+5)
	mnuTop(g).update(g)
	g.in.moved = false
	if m.tsel != 1 {
		t.Fatalf("pointer over the second card: tsel %d", m.tsel)
	}
	g.in.click = true
	mnuTop(g).update(g)
	g.in.click = false
	if g.st.Members()[1].HP <= 3 {
		t.Fatalf("hp %d: the click did not heal the member under it", g.st.Members()[1].HP)
	}
}

// An event item used in the item screen an event opened (open: item) lets that event go on.
func TestMenuDirectItemEventItemReleasesTheOpener(t *testing.T) {
	g := mnuEventGame(t)
	g.st.AddItem("spare", 1)
	g.events.Start([]data.Command{{Open: data.ScreenCall{Screen: "item"}}})
	var m *menu
	for range 30 {
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
		if top, ok := mnuTop(g).(*menu); ok {
			m = top
			break
		}
	}
	if m == nil {
		t.Fatalf("open: item did not open the item screen (top %T)", mnuTop(g))
	}
	at := indexOf(m.ids, "spare")
	if at < 0 {
		t.Fatalf("no spare charm in %v", m.ids)
	}
	m.items.sel = at
	mnuPress(g, btnOK)
	if g.st.Items["spare"] != 0 || len(g.stack) != 1 {
		t.Fatalf("the item was not used: spare %d stack %d", g.st.Items["spare"], len(g.stack))
	}
	for range 60 {
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
	}
	if !g.st.Flags["blessed"] || g.req != nil || g.events.Running() {
		t.Fatalf("the play is stuck after the item screen closed: item event ran %v, request %T still waited for, running %v", g.st.Flags["blessed"], g.req, g.events.Running())
	}
}
