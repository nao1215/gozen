package gamedata

import (
	"bufio"
	"bytes"
	"io/fs"
	"path"
	"strings"
)

// loadStandIns は game.yaml の stand_ins の 一覧を 読む（無い・書いていない ときは 空）。
// 空行と # で 始まる 行は 飛ばす。
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

// StandIn は file（assets/ からの 名前）が 仮の 一枚絵か（段階 x.002.jpg は x.jpg と 同じに 見る）。
func (db *DB) StandIn(file string) bool {
	return file != "" && db.standIns[PageBase(file)]
}
