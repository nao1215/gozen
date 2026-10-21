package gamedata_test

import (
	"testing"
	"time"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

// FuzzLoadScenes mutates the definition of a town of maps and painted scenes: loading never panics, and a definition that loads
// can be checked again, has its warnings listed and its places entered.
func FuzzLoadScenes(f *testing.F) {
	f.Add(testgame.ScenesData)
	f.Add("")
	f.Add("locations: {world: {name: W, map: {picture: maps/world.png, points: {a: {at: [1, 1]}}}}}")
	f.Add("locations: {plaza: {name: P, background: bg/plaza.png, stage: {walk: [[[0, 0], [9, 0], [9, 9]]]}}}")
	f.Fuzz(func(t *testing.T, town string) {
		if len(town) > 32*1024 {
			return
		}
		type loaded struct {
			db  *data.DB
			err error
		}
		done := make(chan loaded, 1)
		go func() {
			db, err := data.Load(testgame.ScenesFS(map[string]string{"data/town.yaml": town}))
			done <- loaded{db, err}
		}()
		var res loaded
		select {
		case res = <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("loading took more than 5 seconds (%d bytes)", len(town))
		}
		db, err := res.db, res.err
		if err != nil {
			return
		}
		if problems := db.Check(); len(problems) != 0 {
			t.Fatalf("a definition that loaded fails its own check: %v", problems)
		}
		_ = db.SceneWarnings()
		for id, l := range db.Locations {
			if l == nil {
				t.Fatalf("location %q is nil", id)
			}
			_ = l.Spotted("")
			if l.Map != nil {
				_ = l.Map.StartPoint()
				_ = l.Map.PointIDs()
			}
			if l.Stage != nil {
				_ = l.Stage.StartSpot()
				_ = l.Stage.SpotIDs()
			}
		}
	})
}
