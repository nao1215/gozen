package gamedata

import (
	"bufio"
	"bytes"
	"io/fs"
	"path"
	"strings"
)

// loadStandIns reads the list of stand_ins in game.yaml (empty if absent or unwritten).
// Blank lines and lines starting with # are skipped.
func (db *DB) loadStandIns() error {
	db.standIns = nil
	if db.Game.StandIns == "" {
		return nil
	}
	b, err := fs.ReadFile(db.FS, path.Join("assets", db.Game.StandIns))
	if err != nil {
		return err
	}
	db.standIns = map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		db.standIns[PageBase(line)] = true
	}
	return sc.Err()
}

// StandIn reports whether file (a name relative to assets/) is a provisional single picture (a stage x.002.jpg is treated the same as x.jpg).
func (db *DB) StandIn(file string) bool {
	return file != "" && db.standIns[PageBase(file)]
}
