// Package dungeon provides an entry point that assembles the game's own exploration controls and
// map observation without a screen.
// The caller owns the DB and State and must not operate them from several goroutines at once.
// State is made from the same DB with rpg.New or rpg.Parse, and sets the target dungeon as the
// current place.
// Pass CellAt and SlideEnd a non-nil definition of that dungeon.
// For operations that return event commands, run the returned commands with event or headless.
package dungeon

import (
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	core "github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/rpg"
)

// Cell is the position, terrain and passability of the current cell. Tile is a borrowed definition
// and must not be modified.
type Cell = core.Cell

// Of returns the definition of the current floor. The returned definition is borrowed; the caller
// must not modify it.
func Of(db *data.DB, st *rpg.State) *data.Dungeon {
	return core.Of(db, st)
}

// CellAt returns the cell with conditions and state applied. Tile is borrowed.
func CellAt(d *data.Dungeon, st *rpg.State, x, y int) Cell {
	return core.CellAt(d, st, x, y)
}

// Dive prepares the given floor from the saved seed and the generator settings, and places the
// coordinates.
// st.Location must be set to loc first. The same state yields the same map.
func Dive(db *data.DB, st *rpg.State, loc string, floor int) {
	core.DiveAt(db, st, loc, floor, "", "")
}

// Place places the coordinates at the given place, mark and direction, and records the cell
// arrived at.
// The caller sets st.Location to loc before calling.
func Place(db *data.DB, st *rpg.State, loc, at, facing string) {
	core.Place(db, st, loc, at, facing)
}

// Turn rotates the facing by n steps in four directions. It advances neither the map nor events.
func Turn(st *rpg.State, n int) { core.Turn(st, n) }

// Forward steps forward and returns the event commands to run. The caller runs the returned
// commands once.
func Forward(db *data.DB, st *rpg.State) (bool, []data.Command) {
	return core.Forward(db, st)
}

// Look returns the commands produced by the action in front. The caller runs the returned commands
// once.
func Look(db *data.DB, st *rpg.State) []data.Command {
	return core.Look(db, st)
}

// Front returns the cell currently in front.
func Front(db *data.DB, st *rpg.State) Cell {
	return core.Front(db, st)
}

// Seen reports whether the cell at the given place has been visited.
func Seen(db *data.DB, st *rpg.State, loc string, x, y int) bool {
	return core.Seen(db, st, loc, x, y)
}

// Shown returns the cells displayable on the current floor as a new row-major array.
func Shown(db *data.DB, st *rpg.State) []bool {
	return core.Shown(db, st)
}

// Spent reports whether the once-only action on the given cell has been completed.
func Spent(st *rpg.State, loc string, x, y int) bool {
	return core.Spent(st, loc, x, y)
}

// Completed reports whether the current floor of the given place has been completed.
func Completed(st *rpg.State, loc string) bool {
	return core.Completed(st, loc)
}

// Walked decides reachability over the whole floor, records it in the state, and returns the
// first-time completion commands.
func Walked(db *data.DB, st *rpg.State) []data.Command {
	return core.Walked(db, st)
}

// Hunters updates the conditional roaming foes and returns the commands for their appearance.
func Hunters(db *data.DB, st *rpg.State) []data.Command {
	return core.Hunters(db, st)
}

// Trigger returns the operation method (step, bump, look) of a non-nil tile.
func Trigger(t *data.Tile, solid bool) string {
	return core.Trigger(t, solid)
}

// Slides reports whether a cell has a transfer setting.
func Slides(c Cell) bool {
	return core.Slides(c)
}

// SlideEnd computes the position, facing and route after a transfer. It does not modify the state.
func SlideEnd(d *data.Dungeon, st *rpg.State, x, y, moving int) (ex, ey, facing int, way [][2]int) {
	return core.SlideEnd(d, st, x, y, moving)
}

// BattleMusic chooses the battle music from the floor and enemy group settings.
func BattleMusic(db *data.DB, st *rpg.State, troop *data.Troop) data.MusicCue {
	return core.BattleMusic(db, st, troop)
}

// Next returns the next action toward a mark: here, left, right, look or forward. It returns false
// when there is no route.
func Next(db *data.DB, st *rpg.State, goal rune) (string, bool) {
	return core.Next(db, st, goal)
}
