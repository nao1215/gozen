package play

import (
	"fmt"
	"log"
	"sort"
	"strings"

	lang "github.com/nao1215/gozen/internal/base/text"
	"github.com/nao1215/gozen/internal/model/rpg"
	"github.com/nao1215/gozen/internal/rules/dungeon"
)

// idle reports whether the game rests on a screen the autosave is written on.
func (g *Game) idle() bool { return len(g.stack) == 1 && g.rests() && !g.busy() }

// rests reports whether the game rests on a screen the autosave is written on, under at
// most screens that do not change the state: the watch view, the menu and the settings
// (keepOpen writes the time the game is open there).
func (g *Game) rests() bool {
	if g.st == nil || g.replaying || g.fadeT > 0 || len(g.stack) == 0 || g.req != nil || g.talk.active || g.events != nil && g.events.Running() {
		return false
	}
	if g.walking() {
		return false
	}
	for _, s := range g.stack[1:] {
		switch s.(type) {
		case *streamWatch, *menu, *configScreen:
		default:
			return false
		}
	}
	switch g.stack[0].(type) {
	case *field, *dungeonScene, *stageScene, *atlasScene, *streamLobby:
		return true
	}
	return false
}

func (g *Game) idleChores() {
	if !g.idle() {
		return
	}
	g.autoSave()
	g.offerGear()
}

func (g *Game) ManualSP() bool { return g.cfg.ManualSP }

func (g *Game) autoPath() string { return "auto.json" }

// Failed background saves wait five seconds of game updates between attempts.
const saveRetryTicks = 300

type saveRetry struct {
	next int
	err  string
}

func (r *saveRetry) record(tick int, operation string, err error) {
	if err == nil {
		*r = saveRetry{}
		return
	}
	if msg := err.Error(); msg != r.err {
		log.Printf("%s: %v", operation, err)
		r.err = msg
	}
	r.next = tick + saveRetryTicks
}

func (g *Game) saveNotice() string {
	if g.autoSaveRetry.err != "" || g.unlockSaveRetry.err != "" {
		return lang.T("保存できていません。再試行しています。")
	}
	return ""
}

func (g *Game) autoSave() {
	if g.tick < g.autoSaveRetry.next || g.testRun() && !g.storeGiven {
		return
	}
	key := g.st.Location
	if f, ok := g.st.Vars[dungeon.VarFloor].(float64); ok && dungeon.Of(g.db, g.st) != nil {
		key += fmt.Sprintf("/%d", int(f))
	}
	if key == g.autoKey && g.autoSaveRetry.err == "" {
		return
	}
	err := g.writeSave(g.autoPath())
	g.autoSaveRetry.record(g.tick, "autosave", err)
	if err != nil {
		return
	}
	g.autoKey = key
	g.emit("autosave", traceLine{"at": key})
}

// autosaveAt writes the autosave from an event (the autosave command): the state as
// it is now, with a cursor that starts event on loading. The event that asked runs on,
// so nothing of it is kept; a replay and the title write nothing, and a failed write is
// tried again by the next command as by the idle autosave.
func (g *Game) autosaveAt(event string) {
	if g.replaying || g.rep != nil || g.st == nil || g.testRun() && !g.storeGiven || g.tick < g.autoSaveRetry.next {
		return
	}
	err := g.writeResumeAt(g.autoPath(), event)
	g.autoSaveRetry.record(g.tick, "autosave", err)
	if err == nil {
		g.emit("autosave", traceLine{"at": g.st.Location, "event": event})
	}
}

func (g *Game) writeResumeAt(path, event string) error {
	b, err := g.st.Save()
	if err != nil {
		return err
	}
	var rnd []byte
	if g.pcg != nil {
		if rnd, err = g.pcg.MarshalBinary(); err != nil {
			return err
		}
	}
	c := rpg.Cursor{Event: event}
	if err := c.Seal(g.db); err != nil {
		return err
	}
	if b, err = rpg.WithCursor(b, c, rnd); err != nil {
		return err
	}
	return g.store.Write(path, b)
}

// suspendSave writes the autosave when the page is hidden in the middle of play (another
// app on a phone, which may close the browser's page unseen), where the save screen could
// save: the next visit continues from it as from any autosave. The stream screens keep
// their own (keepOpen); the title, a replay and a scene that refuses saving write nothing.
func (g *Game) suspendSave() {
	if g.closed || g.desk != nil || g.replaying || g.rep != nil || g.saveBlocked() != "" {
		return
	}
	if !g.idle() && !g.talk.active {
		return
	}
	for _, s := range g.stack {
		if _, ok := s.(*title); ok {
			return
		}
	}
	if g.testRun() && !g.storeGiven {
		return
	}
	// Only the save: its picture is drawn by the game's loop, which is not running here
	// (a hidden page draws no frames), and the autosave has no picture of its own.
	err := g.writeSave(g.autoPath())
	g.autoSaveRetry.record(g.tick, "autosave", err)
	if err == nil {
		g.autoKey = ""
		g.emit("autosave", traceLine{"at": g.st.Location})
	}
}

func (g *Game) offerGear() {
	if g.isAutopilot() || (g.rep != nil && g.rep.version == 1) {
		return
	}
	var keys, names []string
	for _, a := range g.st.Members() {
		up := g.st.Upgrades(a)
		for _, sl := range g.db.Game.Slots {
			if id, ok := up[sl.ID]; ok {
				keys = append(keys, a.ID+":"+sl.ID+":"+id)
				names = append(names, a.Name()+lang.T("：")+g.db.Items[id].Name)
			}
		}
	}
	sort.Strings(keys)
	sig := strings.Join(keys, ",")
	if sig == g.offered {
		return
	}
	g.offered = sig
	if sig == "" {
		return
	}
	list := strings.Join(names[:min(2, len(names))], lang.T("、"))
	if len(names) > 2 {
		list += fmt.Sprintf(lang.T(" ほか%d"), len(names)-2)
	}
	g.emit("request", traceLine{"what": "offer_gear", "items": names})
	// An offer on the field asks nothing: it passes by itself, a tap on it
	// equips, and the equipment menu keeps the same action for the keys.
	g.notices.offer(fmt.Sprintf(lang.T("いまより 強い そうびが あります（%s）。ここを おすと さいきょうそうび。"), list), func() {
		if g.st != nil {
			g.equipBestAll()
		}
	})
}

func (g *Game) equipBestAll() int {
	n := 0
	for _, a := range g.st.Members() {
		n += g.st.EquipUpgrades(a)
	}
	if n > 0 {
		g.sfx("item")
	}
	g.offered = ""
	return n
}
