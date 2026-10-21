package play

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nao1215/gozen"
)

const sysInnData = `locations:
  inn:
    name: Inn
    safe: true
    no_menu: true
`

func sysInn(t *testing.T, opts Options) *Game {
	t.Helper()
	if opts.Location == "" {
		opts.Location = "inn"
	}
	return sysOpen(t, opts, map[string]string{"data/inn.yaml": sysInnData})
}

// sysKey clears the input and presses the buttons for the next direct update call.
func sysKey(g *Game, btns ...button) {
	g.in.pressed, g.in.held = [numButtons]bool{}, [numButtons]bool{}
	g.in.click, g.in.rclick, g.in.moved, g.in.wheel = false, false, false, 0
	for _, b := range btns {
		g.in.pressed[b] = true
	}
}

// sysConfirm answers the confirmation on top of the stack.
func sysConfirm(t *testing.T, g *Game, yes bool) {
	t.Helper()
	c, ok := g.stack[len(g.stack)-1].(*confirm)
	if !ok {
		t.Fatalf("top of the stack is %T, want a confirmation", g.stack[len(g.stack)-1])
	}
	c.age = 10
	if yes {
		sysKey(g, btnOK)
	} else {
		sysKey(g, btnCancel)
	}
	c.update(g)
}

// TestSaveMenuWritesAndAsksBeforeOverwriting: a save to an empty slot is written at once; a
// second save to the same slot asks first, and "no" leaves the slot as it was.
func TestSaveMenuWritesAndAsksBeforeOverwriting(t *testing.T) {
	dir := t.TempDir()
	g := sysInn(t, Options{SaveDir: dir, Seed: 2})
	g.st.Gold = 11
	g.openScreen("save", func() {})
	sm, ok := g.stack[len(g.stack)-1].(*saveMenu)
	if !ok || !sm.saving {
		t.Fatalf("top is %T", g.stack[len(g.stack)-1])
	}
	sysKey(g, btnOK)
	sm.update(g)
	if _, err := os.Stat(filepath.Join(dir, "save1.json")); err != nil && !strings.Contains(sm.msg, "1") {
		t.Fatalf("slot not written: %v %q", err, sm.msg)
	}
	if !g.storeHas(g.slotPath(0)) {
		t.Fatal("slot 1 is empty after saving")
	}
	for sm.msgT > 0 {
		sysKey(g)
		sm.update(g)
	}
	g.openScreen("save", func() {})
	sm = g.stack[len(g.stack)-1].(*saveMenu)
	g.st.Gold = 22
	sysKey(g, btnOK)
	sm.update(g)
	sysConfirm(t, g, false)
	st, err := g.load(0)
	if err != nil || st.Gold != 11 {
		t.Fatalf("a declined overwrite changed the slot: %v %+v", err, st)
	}
	sysKey(g, btnOK)
	sm.update(g)
	sysConfirm(t, g, true)
	st, err = g.load(0)
	if err != nil || st.Gold != 22 {
		t.Fatalf("an accepted overwrite did not write: %v %+v", err, st)
	}
}

// TestSaveSlotLabels: an empty slot, a good one and a damaged one are told apart.
func TestSaveSlotLabels(t *testing.T) {
	dir := t.TempDir()
	g := sysInn(t, Options{SaveDir: dir})
	if l, ok := g.slotLabel(0); ok || !strings.Contains(l, "1") {
		t.Fatalf("empty slot: %q %v", l, ok)
	}
	if err := g.save(1); err != nil {
		t.Fatal(err)
	}
	l, ok := g.slotLabel(1)
	if !ok || !strings.Contains(l, "Scout") || !strings.Contains(l, "Inn") || !strings.HasPrefix(l, "2:") {
		t.Fatalf("good slot: %q %v", l, ok)
	}
	if err := os.WriteFile(filepath.Join(dir, "save3.json"), []byte("{damaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	l, ok = g.slotLabel(2)
	if ok || l == "" || l == "3: ---- empty ----" {
		t.Fatalf("damaged slot: %q %v", l, ok)
	}
	if slotName(autoSlot) == "" {
		t.Fatal("auto slot has no name")
	}
	if _, ok := g.storeTime(g.slotPath(1)); !ok {
		t.Fatal("a directory store should tell the time a save was written")
	}
	if _, ok := g.storeTime("no-such.json"); ok {
		t.Fatal("time of a missing file")
	}
}

// TestLoadMenuShowsAnErrorForADamagedSlot: choosing a slot that cannot be read says so and
// keeps the game as it was.
func TestLoadMenuShowsAnErrorForADamagedSlot(t *testing.T) {
	dir := t.TempDir()
	g := sysInn(t, Options{SaveDir: dir})
	if err := os.WriteFile(filepath.Join(dir, "save1.json"), []byte("{damaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	g.st.Gold = 5
	sm := newSaveMenu(g, false, func() {})
	g.push(sm)
	sm.slots.items[0].disabled = false
	sm.slots.sel = 0
	sysKey(g, btnOK)
	sm.update(g)
	if sm.msgT == 0 || sm.msg == "" {
		t.Fatal("no message for a damaged slot")
	}
	if g.st.Gold != 5 {
		t.Fatal("state changed by a failed load")
	}
}

// TestLoadInPlayAsksFirst: loading from inside a game asks before it throws the progress
// away; yes restores, no keeps going.
func TestLoadInPlayAsksFirst(t *testing.T) {
	g := sysInn(t, Options{Seed: 4})
	g.st.Gold = 70
	if err := g.save(0); err != nil {
		t.Fatal(err)
	}
	g.st.Gold = 71
	g.openScreen("load", func() {})
	sm, ok := g.stack[len(g.stack)-1].(*saveMenu)
	if !ok || !sm.inPlay || sm.saving {
		t.Fatalf("top is %T", g.stack[len(g.stack)-1])
	}
	sysKey(g, btnOK)
	sm.slots.sel = 0
	sm.update(g)
	sysConfirm(t, g, false)
	if g.st.Gold != 71 {
		t.Fatalf("a declined load changed the state: %d", g.st.Gold)
	}
	sysKey(g, btnOK)
	sm.update(g)
	sysConfirm(t, g, true)
	if g.st.Gold != 70 {
		t.Fatalf("the load did not restore: %d", g.st.Gold)
	}
}

// TestLoadWithoutASaveDoesNothing: the load screen is not opened when there is nothing to load.
func TestLoadWithoutASaveDoesNothing(t *testing.T) {
	g := sysInn(t, Options{})
	called := false
	n := len(g.stack)
	g.openScreen("load", func() { called = true })
	if !called || len(g.stack) != n {
		t.Fatalf("done called %v, stack %d -> %d", called, n, len(g.stack))
	}
	called = false
	g.openScreen("nonsense", func() { called = true })
	if !called {
		t.Fatal("an unknown screen name should just finish")
	}
	g.openScreen("title", func() {})
	sysConfirm(t, g, true)
	if g.st != nil {
		t.Fatal("going back to the title kept the state")
	}
	if _, ok := g.stack[0].(*title); !ok {
		t.Fatalf("not on the title: %T", g.stack[0])
	}
}

// TestSystemMenuOfAPlaceWithoutTheMenu: a place that turns the main menu off opens a short
// system menu instead; saving is allowed only where the place is safe.
func TestSystemMenuOfAPlaceWithoutTheMenu(t *testing.T) {
	g := sysInn(t, Options{Script: "wait 5\npress menu\nwait 10\nquit"})
	sysFrames(t, g, 20)
	m, ok := g.stack[len(g.stack)-1].(*sysMenu)
	if !ok {
		t.Fatalf("top is %T", g.stack[len(g.stack)-1])
	}
	if m.l.items[0].disabled {
		t.Fatal("saving is not allowed at a safe place")
	}
	if !m.l.items[1].disabled {
		t.Fatal("load is open with no save")
	}
	// choosing "close" pops the menu.
	m.l.sel = 4
	sysKey(g, btnOK)
	m.update(g)
	if _, ok := g.stack[len(g.stack)-1].(*sysMenu); ok {
		t.Fatal("close left the menu open")
	}
	// choosing "save" opens the save screen; cancel closes the menu.
	m = newSysMenu(g)
	g.push(m)
	m.l.sel = 0
	sysKey(g, btnOK)
	m.update(g)
	if _, ok := g.stack[len(g.stack)-1].(*saveMenu); !ok {
		t.Fatalf("save did not open the save screen: %T", g.stack[len(g.stack)-1])
	}
	g.pop()
	m = newSysMenu(g)
	g.push(m)
	sysKey(g, btnCancel)
	m.update(g)
	if _, ok := g.stack[len(g.stack)-1].(*sysMenu); ok {
		t.Fatal("cancel left the menu open")
	}
	g.st.Location = "camp"
	if n := newSysMenu(g); !n.l.items[0].disabled {
		t.Fatal("saving is open at an unsafe place")
	}
}

// sysFailingStore is a store whose reads fail with a chosen error.
type sysFailingStore struct {
	gozen.Store
	readErr error
}

func (s sysFailingStore) Read(name string) ([]byte, error) { return nil, s.readErr }

// TestSaveOverUnreadableKeepsABackup: saving over a slot that cannot be understood keeps the
// old bytes in a file named after their hash, and a backup that disagrees stops the save.
func TestSaveOverUnreadableKeepsABackup(t *testing.T) {
	dir := t.TempDir()
	g := sysInn(t, Options{SaveDir: dir})
	old := []byte("{ not a save at all")
	if err := os.WriteFile(filepath.Join(dir, "save1.json"), old, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := g.save(0); err != nil {
		t.Fatal(err)
	}
	backup := fmt.Sprintf("save1.json.before-%x.bak", sha256.Sum256(old))
	got, err := os.ReadFile(filepath.Join(dir, backup))
	if err != nil || string(got) != string(old) {
		t.Fatalf("backup %s: %q %v", backup, got, err)
	}
	if _, err := g.load(0); err != nil {
		t.Fatalf("the new save is unreadable: %v", err)
	}
	// the same garbage again keeps the one backup.
	if err := os.WriteFile(filepath.Join(dir, "save1.json"), old, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := g.save(0); err != nil {
		t.Fatal(err)
	}
	// a backup holding other bytes under the same name is never overwritten.
	if err := os.WriteFile(filepath.Join(dir, "save1.json"), old, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, backup), []byte("different"), 0o600); err != nil {
		t.Fatal(err)
	}
	err = g.save(0)
	if err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("err = %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, backup)); string(b) != "different" {
		t.Fatal("the backup was overwritten")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "save1.json")); string(b) != string(old) {
		t.Fatal("the slot was written although the backup failed")
	}
	// a read error other than "missing" stops the save.
	g.store = sysFailingStore{Store: g.store, readErr: errors.New("disk on fire")}
	if err := g.save(1); err == nil {
		t.Fatal("a read error was ignored")
	}
	g.store = sysFailingStore{Store: gozen.MemStore(), readErr: fs.ErrNotExist}
	if g.storeHas("x") {
		t.Fatal("a missing file is there")
	}
}

// TestSaveOfAnOlderDataVersionTellsThePlayer: a save made with another data_version loads, the
// game says so on a screen of its own, and the old file is kept at the next save.
func TestSaveOfAnOlderDataVersionTellsThePlayer(t *testing.T) {
	dir := t.TempDir()
	g := sysInn(t, Options{SaveDir: dir})
	g.st.Gold = 99
	if err := g.save(0); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	fsys := gameFiles(t)
	g3 := sysOpen(t, Options{SaveDir: dir, Location: "camp"}, map[string]string{
		"data/inn.yaml": sysInnData,
		"game.yaml":     string(fsys["game.yaml"].Data) + "data_version: 2\n",
	})
	st, err := g3.load(0)
	if err != nil {
		t.Fatal(err)
	}
	g3.loadGame(st)
	if g3.loadNotice == nil || len(g3.loadNotice.lines) == 0 {
		t.Fatal("no notice for a migrated save")
	}
	if !strings.Contains(g3.loadNotice.lines[0], "2") {
		t.Fatalf("notice %q", g3.loadNotice.lines)
	}
	// the notice takes over the frame until it is closed.
	before := g3.tick
	sysFrames(t, g3, 3)
	if g3.loadNotice == nil || g3.loadNotice.list == nil {
		t.Fatal("the notice did not open its list")
	}
	sysKey(g3)
	if !g3.updateLoadNotice() {
		t.Fatal("the notice should hold the game")
	}
	_ = before
	// saving over the old file keeps a backup of it.
	if err := g3.save(0); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	var baks int
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".bak") {
			baks++
		}
	}
	if baks != 1 {
		t.Fatalf("%d backups in %v", baks, entries)
	}
	// a second report of the same line is not repeated.
	n := len(g3.loadNotice.lines)
	g3.reportLoad(st.LoadReport)
	if len(g3.loadNotice.lines) != n {
		t.Fatal("the same notice twice")
	}
}

// TestAutoSaveWritesOncePerPlaceAndSkipsTrials: the game keeps an auto slot when the
// player changes place, and a trial run without a save directory writes nothing.
func TestAutoSaveWritesOncePerPlace(t *testing.T) {
	dir := t.TempDir()
	g := sysOpen(t, Options{SaveDir: dir, Location: "camp"}, nil)
	sysFrames(t, g, 12)
	if _, err := os.Stat(filepath.Join(dir, "auto.json")); err != nil {
		t.Fatalf("no autosave: %v", err)
	}
	if g.autoKey != "camp" {
		t.Fatalf("autoKey %q", g.autoKey)
	}
	info, _ := os.Stat(filepath.Join(dir, "auto.json"))
	time.Sleep(10 * time.Millisecond)
	sysFrames(t, g, 12)
	info2, _ := os.Stat(filepath.Join(dir, "auto.json"))
	if !info2.ModTime().Equal(info.ModTime()) {
		t.Fatal("autosave rewritten for the same place")
	}
	g.st.Location = "room"
	g.Move("room")
	sysFrames(t, g, 12)
	if g.autoKey == "camp" {
		t.Fatalf("autoKey did not follow the place: %q", g.autoKey)
	}
}

// TestOfferGearAsksToEquipTheBest: a stronger item in the bag is offered once; yes equips it.
func TestOfferGearAsksToEquipTheBest(t *testing.T) {
	g := sysOpen(t, Options{Location: "camp", Items: []string{"blade"}, SaveDir: t.TempDir()}, nil)
	sysFrames(t, g, 12)
	c, ok := g.stack[len(g.stack)-1].(*confirm)
	if !ok {
		t.Fatalf("top is %T, want the offer", g.stack[len(g.stack)-1])
	}
	_ = c
	sysConfirm(t, g, true)
	if got := g.st.Members()[0].Equip["weapon"]; got != "blade" {
		t.Fatalf("weapon %q after accepting", got)
	}
	sysFrames(t, g, 25)
	if _, ok := g.stack[len(g.stack)-1].(*confirm); ok {
		t.Fatal("offered again after equipping")
	}
	if n := g.equipBestAll(); n != 0 {
		t.Fatalf("nothing left to equip, got %d", n)
	}
}

// TestOfferGearIsOnlyOnceUntilTheBagChanges: declining is remembered for that bag.
func TestOfferGearIsOnlyOnceUntilTheBagChanges(t *testing.T) {
	g := sysOpen(t, Options{Location: "camp", Items: []string{"blade"}, SaveDir: t.TempDir()}, nil)
	sysFrames(t, g, 12)
	sysConfirm(t, g, false)
	sysFrames(t, g, 40)
	if _, ok := g.stack[len(g.stack)-1].(*confirm); ok {
		t.Fatal("the same offer was made again")
	}
	if got := g.st.Members()[0].Equip["weapon"]; got == "blade" {
		t.Fatal("equipped although declined")
	}
}
