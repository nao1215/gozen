package world

import (
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/layer3/field"
	"github.com/nao1215/gozen/internal/rpg"
	data "github.com/nao1215/gozen/schema"
)

// Kind identifies a field menu operation for game hosts.
type Kind = field.Kind

// Entry describes an available action; Commands returns its event commands.
type Entry = field.Entry

// Field menu operations.
const (
	Talk   = field.Talk
	Do     = field.Do
	Search = field.Search
	Exit   = field.Exit
	Menu   = field.Menu
)

// Entries returns currently available actions in definition order.
func Entries(db *data.DB, st *rpg.State) []Entry { return field.Entries(db, st) }

// Arrive applies once-only arrival markers and returns commands to execute.
// It includes dungeon completion without process-wide registration.
func Arrive(db *data.DB, st *rpg.State, battles bool) []data.Command {
	cmds := field.Arrive(db, st, battles)
	if db.Locations[st.Location].Dungeon != nil {
		cmds = append(cmds, dungeon.Walked(db, st)...)
	}
	return cmds
}

// Auto marks and returns the first eligible automatic event.
func Auto(db *data.DB, st *rpg.State) []data.Command { return field.Auto(db, st) }

// PickTroop draws a weighted encounter using the state's random source.
func PickTroop(db *data.DB, st *rpg.State) string { return field.PickTroop(db, st) }
