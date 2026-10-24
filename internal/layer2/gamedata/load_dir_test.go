package gamedata_test

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestLoadSubdirectoryAndFailureContract(t *testing.T) {
	files := fstest.MapFS{
		"bundle/game.yaml":      {Data: []byte(testgame.Game)},
		"bundle/data/game.yaml": {Data: []byte(testgame.Data)},
		"bundle/assets/x.png":   {Data: []byte("test sentinel")},
	}
	db, err := data.LoadDir(files, "bundle")
	if err != nil {
		t.Fatal(err)
	}
	if db.Game.Start.Location != "camp" {
		t.Fatal(db.Game.Start)
	}
	files["bundle/data/bad.yaml"] = &fstest.MapFile{Data: []byte("events: {bad: {do: [{move: absent}]}}")}
	if db, err := data.LoadDir(files, "bundle"); err == nil || db != nil {
		t.Fatal("semantic failure returned a usable DB")
	}
	if _, err := data.LoadDir(files, "../bundle"); !errors.Is(err, fs.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := data.LoadDir(files, "missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
}
