package session_test

import (
	"context"
	"encoding/base64"
	"testing"
	"testing/fstest"

	gozen "github.com/nao1215/gozen/internal/session"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestProfileQuery: a session reads the character book with profile.<id>: unlocked
// follows the profile's when on the state now, and name is the name shown (ADR 0031).
func TestProfileQuery(t *testing.T) {
	fsys := fstest.MapFS{
		"game.yaml":      {Data: []byte("schema_version: 4\nid: book\n" + testgame.Game)},
		"data/main.yaml": {Data: []byte(testgame.Data)},
		"assets/x.png":   {Data: []byte("a stub")},
		"data/book.yaml": {Data: []byte(`profiles:
  keeper: {name: The keeper, picture: x.png}
  stranger: {picture: x.png, when: "flag.met"}
`)},
	}
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	s, err := gozen.New(g, gozen.Config{Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	ask := func(q string) any {
		t.Helper()
		var v any
		if err := s.Query(q, &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	if ask("profile.keeper.unlocked") != true || ask("profile.keeper.name") != "The keeper" {
		t.Errorf("keeper: %v %v", ask("profile.keeper.unlocked"), ask("profile.keeper.name"))
	}
	if ask("profile.stranger.unlocked") != false || ask("profile.stranger.name") != "stranger" {
		t.Errorf("stranger before: %v %v", ask("profile.stranger.unlocked"), ask("profile.stranger.name"))
	}
	if err := s.Do(context.Background(), "- set: {flag.met: true}"); err != nil {
		t.Fatal(err)
	}
	if ask("profile.stranger.unlocked") != true {
		t.Error("stranger is not unlocked after flag.met")
	}
}

func TestBestiaryProgressQueryAndSave(t *testing.T) {
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/z6sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	fsys := fstest.MapFS{
		"game.yaml":      {Data: []byte("schema_version: 4\nid: bestiary\n" + testgame.Game)},
		"data/main.yaml": {Data: []byte(testgame.Data)},
		"assets/x.png":   {Data: png},
	}
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	s := mustNew(t, g, gozen.Config{Seed: 1})
	if err := s.Start("- battle: one"); err != nil {
		t.Fatal(err)
	}
	if q := s.Next(); q.Kind != "battle" {
		t.Fatalf("request = %+v", q)
	}
	var seen bool
	if err := s.Query("bestiary.mote.seen", &seen); err != nil || !seen {
		t.Fatalf("seen = %v, %v", seen, err)
	}
	s.Answer(gozen.Won)
	var defeated float64
	if err := s.Query("bestiary.mote.defeated", &defeated); err != nil || defeated != 1 {
		t.Fatalf("defeated = %v, %v", defeated, err)
	}
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	r, err := gozen.Restore(g, b, gozen.Config{Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Query("bestiary.mote.defeated", &defeated); err != nil || defeated != 1 {
		t.Fatalf("restored defeated = %v, %v", defeated, err)
	}
}
