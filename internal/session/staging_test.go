package session_test

import (
	"context"
	"slices"
	"testing"
	"testing/fstest"

	gozen "github.com/nao1215/gozen/internal/session"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestStandIsAnEffect: who stands on the stage is seen by a session's OnEffect and by the
// paths of Explore, with its place among the requests (ADR 0028).
func TestStandIsAnEffect(t *testing.T) {
	fsys := fstest.MapFS{
		"game.yaml":      {Data: []byte("schema_version: 4\nid: staged\n" + testgame.Game)},
		"data/main.yaml": {Data: []byte(testgame.Data)},
		"assets/x.png":   {Data: []byte("a stub")},
		"data/scene.yaml": {Data: []byte(`events:
  scene:
    do:
      - stand: {who: Courier, face: x.png, at: left}
      - say: {who: Courier, face: x.png, text: "Hello."}
      - choice: [{label: Stay}, {label: Go, do: [{stand: {who: Courier, leave: fade}}]}]
      - stand: none
`)},
	}
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	s, err := gozen.New(g, gozen.Config{Seed: 1, OnEffect: func(e gozen.Effect) {
		if e.Kind == "stand" {
			seen = append(seen, e.Name)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Do(context.Background(), "- call: scene"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(seen, []string{"Courier", ""}) {
		t.Fatalf("stand effects %q", seen)
	}
	paths, err := gozen.Explore(context.Background(), s, "- call: scene", gozen.ExploreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("%d paths", len(paths))
	}
	var stands [][]int
	for _, p := range paths {
		var at []int
		for _, e := range p.Effects {
			if e.Kind == "stand" {
				at = append(at, e.At)
			}
		}
		stands = append(stands, at)
	}
	if !slices.Equal(stands[0], []int{0, 2}) || !slices.Equal(stands[1], []int{0, 2, 2}) {
		t.Fatalf("stands at %v", stands)
	}
}
