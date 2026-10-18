package data_test

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/data"
	"github.com/nao1215/gozen/internal/testgame"
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
	if err := data.Validate(db); err != nil {
		t.Fatal(err)
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
	if err := data.Validate(nil); err == nil {
		t.Fatal("nil DB accepted")
	}
}

func TestValidateRejectsMalformedDefinitionMaps(t *testing.T) {
	for _, mutate := range []func(*data.DB){
		func(db *data.DB) { db.Actors["scout"] = nil },
		func(db *data.DB) { db.Order["actors"] = append(db.Order["actors"], "missing") },
		func(db *data.DB) { db.Actors["new"] = &data.Actor{} },
		func(db *data.DB) { db.FS = nil },
	} {
		db := testgame.Load(t, nil)
		mutate(db)
		if err := data.Validate(db); err == nil {
			t.Fatal("malformed definition map accepted")
		}
	}
}
