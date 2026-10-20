package schema_test

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/testgame"
	data "github.com/nao1215/gozen/schema"
)

func TestLoadSubdirectoryAndFailureContract(t *testing.T) {
	files := fstest.MapFS{
		"bundle/game.yaml":      {Data: []byte(testgame.Game)},
		"bundle/data/game.yaml": {Data: []byte(testgame.Data)},
		"bundle/assets/x.png":   {Data: []byte("test sentinel")},
	}
	db, err := data.Load(files, "bundle")
	if err != nil {
		t.Fatal(err)
	}
	if db.Game.Start.Location != "camp" {
		t.Fatal(db.Game.Start)
	}
	files["bundle/data/bad.yaml"] = &fstest.MapFile{Data: []byte("events: {bad: {do: [{move: absent}]}}")}
	if db, err := data.Load(files, "bundle"); err == nil || db != nil {
		t.Fatal("semantic failure returned a usable DB")
	}
	if _, err := data.Load(files, "../bundle"); !errors.Is(err, fs.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := data.Load(files, "missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
}
