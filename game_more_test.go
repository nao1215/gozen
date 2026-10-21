package gozen_test

import (
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen"
)

func TestLoadAndValidateRefuseABadWorld(t *testing.T) {
	if ds := gozen.Validate(nil, "."); len(ds) != 1 || ds[0].Severity != "error" || ds[0].Message == "" {
		t.Errorf("a nil filesystem is one error: %v", ds)
	}
	for _, dir := range []string{"../x", "/abs", "a/../b"} {
		if _, err := gozen.Load(trail, dir); err == nil {
			t.Errorf("Load(%q) must fail", dir)
		}
		if ds := gozen.Validate(trail, dir); len(ds) == 0 {
			t.Errorf("Validate(%q) must report", dir)
		}
	}
	if _, err := gozen.Load(trail, "missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a missing directory is a file system error: %v", err)
	}
	if _, err := gozen.Load(trail, ".", fstest.MapFS{}); err != nil {
		t.Errorf("an empty extra file system changes nothing: %v", err)
	}
	if _, err := gozen.Load(trail, "missing", fstest.MapFS{}); err == nil {
		t.Error("a missing directory with an extra layer")
	}
}

// TestYAMLReadsAcrossFilesAndFolders: a section split over several files (and folders under data/) is read as one;
// .yml files count too.
func TestYAMLReadsAcrossFilesAndFolders(t *testing.T) {
	fsys := fstest.MapFS{
		"game.yaml":                 {Data: []byte("schema_version: 2\nid: split\nstart: {location: camp, party: [walker]}\n")},
		"data/a.yaml":               {Data: []byte("classes:\n  walker:\n    stats: {mhp: 10, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}\n    exp_for: level\nactors: {walker: {name: Walker, class: walker}}\nskills: {attack: {}, guard: {}}\nlocations: {camp: {name: Camp}}\n")},
		"data/places/b.yml":         {Data: []byte("locations: {hill: {name: Hill, exits: [{to: camp}]}}\n")},
		"data/places/deep/c.yaml":   {Data: []byte("locations: {cave: {name: Cave, exits: [{to: hill}]}}\n")},
		"data/places/deep/note.txt": {Data: []byte("not yaml")},
	}
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	if got := g.IDs("locations"); !slices.Equal(got, []string{"camp", "cave", "hill"}) && !slices.Equal(got, []string{"camp", "hill", "cave"}) {
		t.Errorf("locations %v", got)
	}
	for _, p := range []string{"locations", "locations.camp", "locations.hill.name", "locations.cave.exits[0].to"} {
		if b, err := g.YAML(p); err != nil || len(b) == 0 {
			t.Errorf("%q: %q %v", p, b, err)
		}
	}
	whole, err := g.YAML("locations")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"camp:", "hill:", "cave:"} {
		if !strings.Contains(string(whole), id) {
			t.Errorf("the merged section lacks %s:\n%s", id, whole)
		}
	}
}

// TestExtraLayersReplaceAndAdd: a layer's files replace the game's by name and add new ones; its folders merge.
func TestExtraLayersReplaceAndAdd(t *testing.T) {
	over := fstest.MapFS{
		"data/zz.yaml":    {Data: []byte("locations: {attic: {name: Attic, exits: [{to: camp}]}}\n")},
		"data/world.yaml": {Data: trail["data/world.yaml"].Data},
	}
	g, err := gozen.Load(trail, ".", over)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(g.IDs("locations"), "attic") {
		t.Errorf("the extra layer adds a place: %v", g.IDs("locations"))
	}
	entries, err := fs.ReadDir(g.FS(), "data")
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	if !slices.Equal(names, []string{"world.yaml", "zz.yaml"}) {
		t.Errorf("the folder lists both layers once, sorted: %v", names)
	}
	if _, err := fs.ReadDir(g.FS(), "nowhere"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a folder in no layer: %v", err)
	}
	if b, err := fs.ReadFile(g.FS(), "assets/mote.png"); err != nil || string(b) != "sentinel" {
		t.Errorf("a file only the lower layer has: %q %v", b, err)
	}
}
