package play

import (
	"testing"

	lang "github.com/nao1215/gozen/internal/text"
)

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
