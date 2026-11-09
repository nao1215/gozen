package play

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"maps"
	"strings"

	"github.com/nao1215/gozen/internal/base/expr"
	"github.com/nao1215/gozen/internal/model/rpg"
)

// Carried values (game.yaml carry) outlive a play: the carry command keeps the
// named variables and flags, and the next new game starts with them. They are
// written beside the saves as carry.json and are never part of a save.

type carryRecord struct {
	GameID string          `json:"game_id"`
	Vars   map[string]any  `json:"vars"`
	Flags  map[string]bool `json:"flags"`
}

func (g *Game) carryPath() string { return "carry.json" }

// carried matches a state name (var.x or flag.x) against game.yaml carry.
func (g *Game) carried(name string) bool {
	for _, pattern := range g.db.Game.Carry {
		if prefix, ok := strings.CutSuffix(pattern, "*"); ok {
			if strings.HasPrefix(name, prefix) {
				return true
			}
		} else if pattern == name {
			return true
		}
	}
	return false
}

// loadCarry reads the kept values once; a missing or foreign file keeps none.
func (g *Game) loadCarry() {
	if g.carry != nil || len(g.db.Game.Carry) == 0 {
		return
	}
	g.emit("carry_read", nil)
	g.carry = &carryRecord{Vars: map[string]any{}, Flags: map[string]bool{}}
	b, err := g.storeRead(g.carryPath())
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Printf("carry: %v", err)
		}
		return
	}
	var r carryRecord
	if err := json.Unmarshal(b, &r); err != nil || r.GameID != g.db.Game.ID {
		log.Printf("carry: ignoring %s (%v)", g.carryPath(), err)
		return
	}
	g.carry = &r
	g.filterCarry()
}

// keepCarry records the carried values of the state now (the carry command).
func (g *Game) keepCarry(st *rpg.State) {
	g.loadCarry()
	g.filterCarry()
	if g.carry == nil || st == nil {
		return
	}
	for k, v := range st.Vars {
		if g.carried("var." + k) {
			g.carry.Vars[k] = v
		}
	}
	for k, v := range st.Flags {
		if g.carried("flag." + k) {
			g.carry.Flags[k] = v
		}
	}
	g.emit("carry", traceLine{"vars": len(g.carry.Vars), "flags": len(g.carry.Flags)})
	if g.opts.Script != "" || g.opts.Replay != "" || g.replaying || g.testRun() && !g.storeGiven {
		return // scripted plays keep them in memory for the next new game only
	}
	b, err := json.MarshalIndent(carryRecord{GameID: g.db.Game.ID, Vars: g.carry.Vars, Flags: g.carry.Flags}, "", " ")
	if err == nil {
		err = g.store.Write(g.carryPath(), b)
	}
	if err != nil {
		log.Printf("carry: %v", err)
	}
}

// applyCarry gives a new game the kept values, over the definitions' initial ones.
func (g *Game) applyCarry(st *rpg.State) {
	g.loadCarry()
	if g.carry == nil {
		return
	}
	g.filterCarry()
	maps.Copy(st.Vars, g.carry.Vars)
	maps.Copy(st.Flags, g.carry.Flags)
}

// filterCarry applies the current allowlist even after a live definition reload.
func (g *Game) filterCarry() {
	if g.carry == nil {
		return
	}
	for k, v := range g.carry.Vars {
		name := "var." + k
		d, declared := g.db.Game.State.Names[name]
		if !g.carried(name) || !expr.IsFiniteScalar(v) || declared && expr.TypeOfValue(v) != d.ValueType() {
			delete(g.carry.Vars, k)
		}
	}
	for k := range g.carry.Flags {
		if !g.carried("flag." + k) {
			delete(g.carry.Flags, k)
		}
	}
}
