package play

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
	"log"
	"slices"

	"github.com/nao1215/gozen/internal/event"
	lang "github.com/nao1215/gozen/internal/text"
)

// readFormatVersion is the version of read.json's envelope (ADR 0026).
const readFormatVersion = 1

// readRecord is read.json: the keys of the lines read and the options taken, for one game.
type readRecord struct {
	FormatVersion int      `json:"format_version"`
	GameID        string   `json:"game_id"`
	Keys          []uint64 `json:"keys"`
}

// legacyKey is the key of a line as shown (its name and text after {expressions}), the key
// of read.json before ADR 0026. It is still looked up for one version, and it is the key of
// a line the runner did not make (a choice's prompt shown alone).
func legacyKey(s event.Say) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s.Name))
	h.Write([]byte{0})
	h.Write([]byte(s.Text))
	return h.Sum64()
}

// lineKey is the key a line is remembered by: its source's, or the legacy key without one.
func lineKey(s event.Say) uint64 {
	if s.Key != 0 {
		return s.Key
	}
	return legacyKey(s)
}

// readKeys reads read.json. A bare list of keys is the record before the envelope (legacy).
func readKeys(b []byte, gameID string) (keys []uint64, legacy bool, err error) {
	if t := bytes.TrimSpace(b); len(t) > 0 && t[0] == '[' {
		if err := json.Unmarshal(b, &keys); err != nil {
			return nil, true, err // no half-read list beside an error
		}
		return keys, true, nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, false, err
	}
	if raw == nil {
		return nil, false, errors.New(lang.T("既読の記録が辞書でない"))
	}
	var r readRecord
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&r); err != nil {
		return nil, false, err
	}
	if r.FormatVersion != readFormatVersion {
		return nil, false, fmt.Errorf(lang.T("既読の保存形式版 %d には対応していない"), r.FormatVersion)
	}
	if raw["game_id"] == nil || r.GameID != gameID {
		return nil, false, fmt.Errorf(lang.T("既読の作品 ID が一致しない（%q / %q）"), r.GameID, gameID)
	}
	if r.Keys == nil {
		return nil, false, errors.New(lang.T("既読の記録がない"))
	}
	return r.Keys, false, nil
}

func (g *Game) readPath() string { return "read.json" }

func (g *Game) keepsRead() bool { return g.storeGiven || !g.testRun() }

func (g *Game) loadRead() {
	g.read = map[uint64]bool{}
	g.readErr, g.readNeedsBackup = nil, false
	if !g.keepsRead() {
		return
	}
	b, err := g.storeRead(g.readPath())
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			g.readErr = err
			log.Printf("read lines: %v", err)
		}
		return
	}
	keys, legacy, err := readKeys(b, g.db.Game.ID)
	if err != nil {
		// Not overwritten: another game's record or a newer engine's stays as it is.
		g.readErr = err
		log.Printf("read lines: %v", err)
		return
	}
	for _, k := range keys {
		g.read[k] = true
	}
	g.readNeedsBackup = legacy
}

// isRead reports whether the line was read, by its source key or by its legacy key.
func (g *Game) isRead(s event.Say) bool {
	return s.Key != 0 && g.read[s.Key] || g.read[legacyKey(s)]
}

func (g *Game) markRead(s event.Say) { g.markKey(lineKey(s)) }

// markKey remembers a read line or a taken option.
func (g *Game) markKey(k uint64) {
	if g.read == nil {
		g.read = map[uint64]bool{}
	}
	if !g.read[k] {
		g.read[k] = true
		g.readDirty = true
	}
}

func (g *Game) flushRead() {
	if !g.readDirty || !g.keepsRead() || g.readErr != nil {
		return
	}
	keys := make([]uint64, 0, len(g.read))
	for k := range g.read {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	b, err := json.Marshal(readRecord{FormatVersion: readFormatVersion, GameID: g.db.Game.ID, Keys: keys})
	if err == nil {
		if g.readNeedsBackup {
			_, err = g.writePreserving(g.readPath(), b)
		} else {
			err = g.store.Write(g.readPath(), b)
		}
	}
	if err != nil {
		g.failure = errors.Join(g.failure, fmt.Errorf("read progress: %w", err))
		return
	}
	g.readDirty, g.readNeedsBackup = false, false
}

func (g *Game) testRun() bool {
	return g.auto != nil || g.in.script != nil || g.rep != nil || g.opts.Location != "" || len(g.opts.Party) > 0
}
