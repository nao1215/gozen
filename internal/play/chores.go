package play

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/nao1215/gozen/internal/layer3/dungeon"
	lang "github.com/nao1215/gozen/internal/text"
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

const autoSlot = saveSlots

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
	g.push(newConfirm(fmt.Sprintf(lang.T("いまより 強い そうびが あります（%s）。さいきょうそうびに しますか？"), list), func() {
		g.equipBestAll()
	}))
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
