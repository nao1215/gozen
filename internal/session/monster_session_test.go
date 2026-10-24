package session_test

import (
	"context"
	"testing"
	"testing/fstest"

	gozen "github.com/nao1215/gozen/internal/session"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestMonsterCaptureFlowsThroughSession(t *testing.T) {
	fsys := fstest.MapFS{"assets/x.png": {Data: []byte("fixture")}}
	for path, body := range testgame.MonsterFiles() {
		fsys[path] = &fstest.MapFile{Data: []byte(body)}
	}
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	s, err := gozen.New(g, gozen.Config{Seed: 5, OnRequest: func(q gozen.Request) { kinds = append(kinds, q.Kind) }})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Do(context.Background(), "- battle: one\n"); err != nil {
		t.Fatal(err)
	}
	var caught float64
	if err := s.Query("bestiary.mote.caught", &caught); err != nil || caught != 1 {
		t.Fatalf("caught %v: %v", caught, err)
	}
	var family string
	if err := s.Query("monster.monster_1.family", &family); err != nil || family != "slime" {
		t.Fatalf("family %q: %v", family, err)
	}
	if len(kinds) < 3 || kinds[0] != "battle" {
		t.Fatalf("requests = %v", kinds)
	}
	foundChoice := false
	for _, kind := range kinds {
		if kind == "choice" {
			foundChoice = true
		}
	}
	if !foundChoice {
		t.Fatalf("capture did not ask through a request: %v", kinds)
	}
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	got, err := gozen.Restore(g, b, gozen.Config{Seed: 5})
	if err != nil {
		t.Fatal(err)
	}
	if err := got.Query("bestiary.mote.caught", &caught); err != nil || caught != 1 {
		t.Fatalf("restored caught %v: %v", caught, err)
	}
}
