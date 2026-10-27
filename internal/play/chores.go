package play

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/nao1215/gozen/internal/layer3/dungeon"
	lang "github.com/nao1215/gozen/internal/text"
)

func (g *Game) idleChores() {
	if g.st == nil || g.replaying || g.busy() || g.fadeT > 0 || len(g.stack) != 1 {
		return
	}
	switch g.stack[0].(type) {
	case *field, *dungeonScene, *stageScene, *atlasScene, *streamLobby:
	default:
		return
	}
	g.autoSave()
	g.offerGear()
}

func (g *Game) ManualSP() bool { return g.cfg.ManualSP }

const autoSlot = saveSlots

func (g *Game) autoPath() string { return "auto.json" }

func (g *Game) autoSave() {
	if g.testRun() && !g.storeGiven {
		return
	}
	key := g.st.Location
	if f, ok := g.st.Vars[dungeon.VarFloor].(float64); ok && dungeon.Of(g.db, g.st) != nil {
		key += fmt.Sprintf("/%d", int(f))
	}
	if key == g.autoKey {
		return
	}
	if err := g.writeSave(g.autoPath()); err != nil {
		log.Printf("autosave: %v", err)
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
