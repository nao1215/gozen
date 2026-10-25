package rpg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"slices"
	"sync"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	lang "github.com/nao1215/gozen/internal/text"
)

// checkedSave is a save whose JSON passed the checks of ParseWithConfig for one game: the
// identity, the versions, the required fields, unique keys and the shape of every field.
// Those depend only on the bytes, the data and the Config, so the same save read again
// (the trials of the F7 measurement and of gozen sim, which start each battle from one
// save) only decodes the state and binds it to the data.
type checkedSave struct {
	db        *data.DB
	opt       Config
	b         []byte // a copy of the save
	payload   []byte // the state's JSON
	report    LoadReport
	resume    *Resume
	savedRand []byte
}

// checkedSaves are the last saves checked, the newest first.
var (
	checkedMu    sync.Mutex
	checkedSaves []*checkedSave
)

const maxCheckedSaves = 4

func checkedSaveOf(db *data.DB, b []byte, opt Config) *checkedSave {
	checkedMu.Lock()
	defer checkedMu.Unlock()
	for _, c := range checkedSaves {
		// the data's identity too: a reload may change the game in place
		if c.db == db && c.opt == opt && c.report.GameID == db.Game.ID && c.report.DataVersion == db.Game.DataVersion && bytes.Equal(c.b, b) {
			return c
		}
	}
	return nil
}

func remember(c *checkedSave) {
	checkedMu.Lock()
	defer checkedMu.Unlock()
	checkedSaves = slices.Insert(checkedSaves, 0, c)
	if len(checkedSaves) > maxCheckedSaves {
		checkedSaves = checkedSaves[:maxCheckedSaves]
	}
}

// state decodes the checked save again into a new state, as ParseWithConfig does.
func (c *checkedSave) state(rng *rand.Rand) (*State, error) {
	s := &State{DB: c.db, Rand: rng, LoadReport: c.report, Resume: c.resume.clone(), SavedRand: slices.Clone(c.savedRand)}
	dec := json.NewDecoder(bytes.NewReader(c.payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(s); err != nil {
		return nil, fmt.Errorf(lang.T("セーブの構造が不正。%w"), err)
	}
	return s.bind()
}

// clone is a copy of the resume that shares nothing with it (nil stays nil).
func (r *Resume) clone() *Resume {
	if r == nil {
		return nil
	}
	c := *r
	c.Cursor.Answers = slices.Clone(r.Cursor.Answers)
	c.Rand = slices.Clone(r.Rand)
	return &c
}
