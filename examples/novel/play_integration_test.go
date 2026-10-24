//go:build !js

package main

import (
	"context"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"unicode"

	gozen "github.com/nao1215/gozen/internal/session"
)

// endings are the pictures the night ends on, by the option of the last choice.
var endings = map[string]string{"brenna": "cg/toast.jpg", "wren": "cg/wine.jpg", "hedda": "cg/table.jpg"}

func load(t *testing.T, extra ...fs.FS) *gozen.Session {
	t.Helper()
	fsys := os.DirFS("game")
	g, err := gozen.Load(fsys, ".", extra...)
	if err != nil {
		t.Fatal(err)
	}
	s, err := gozen.New(g, gozen.Config{Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func query(t *testing.T, s *gozen.Session, src string) string {
	t.Helper()
	var v string
	if err := s.Query(src, &v); err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	return v
}

// TestEveryWayEnds reads every way through the story: each ends the game, on the
// picture of the last choice, and the first toast is remembered as a flag.
func TestEveryWayEnds(t *testing.T) {
	s := load(t)
	paths, err := gozen.Explore(context.Background(), s, "- call: opening\n", gozen.ExploreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, p := range paths {
		if len(p.Errs) > 0 {
			t.Fatalf("answers %v: %v", p.Answers, p.Errs)
		}
		var cleared bool
		if err := p.End.Query("cleared", &cleared); err != nil || !cleared {
			t.Fatalf("answers %v: the story does not end the game", p.Answers)
		}
		night := query(t, p.End, "chose.night")
		picture := ""
		stands := 0
		for _, e := range p.Effects {
			switch e.Kind {
			case "image":
				if e.Name != "" {
					picture = e.Name
				}
			case "stand":
				stands++
			}
		}
		if picture != endings[night] {
			t.Errorf("answers %v: the night with %q ends on %q, want %q", p.Answers, night, picture, endings[night])
		}
		if stands == 0 {
			t.Errorf("answers %v: no one stands on the stage", p.Answers)
		}
		toast := query(t, p.End, "chose.toast")
		var thanked bool
		if err := p.End.Query("flag.thanked_hedda", &thanked); err != nil {
			t.Fatal(err)
		}
		if thanked != (toast == "hedda") {
			t.Errorf("answers %v: toast %q but flag.thanked_hedda %v", p.Answers, toast, thanked)
		}
		says := 0
		for _, r := range p.Requests {
			if r.Kind == "say" {
				says++
			}
		}
		// About 5 to 8 minutes of reading.
		if says < 70 || says > 160 {
			t.Errorf("answers %v: %d lines", p.Answers, says)
		}
		seen[night] = true
	}
	if len(seen) != len(endings) {
		t.Fatalf("the ways reach the nights %v, want all of %v", seen, endings)
	}
}

// TestJapanese reads the story with game.yaml's language set to Japanese, as a player
// who chose it in the settings: every line, speaker and option is in Japanese.
func TestJapanese(t *testing.T) {
	fsys := os.DirFS("game")
	b, err := fs.ReadFile(fsys, "game.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ja := strings.Replace(string(b), "\nlanguage: en\n", "\nlanguage: ja\n", 1)
	if ja == string(b) {
		t.Fatal("game.yaml has no language: en")
	}
	s := load(t, fstest.MapFS{"game.yaml": {Data: []byte(ja)}})
	paths, err := gozen.Explore(context.Background(), s, "- call: opening\n", gozen.ExploreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	japanese := func(text string) bool {
		for _, r := range text {
			if unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Han) {
				return true
			}
		}
		return false
	}
	for _, p := range paths {
		for _, r := range p.Requests {
			texts := append([]string{r.Text, r.Who}, r.Options...)
			for _, text := range texts {
				if text != "" && !japanese(text) {
					t.Errorf("answers %v: a %s not in Japanese: %q", p.Answers, r.Kind, text)
				}
			}
		}
	}
}

// TestCharacterBook: the book (data/profiles.yaml) has Hedda from the start and Brenna
// and Wren once the toast has been poured, on every way through the story (ADR 0031).
func TestCharacterBook(t *testing.T) {
	s := load(t)
	var open bool
	for id, want := range map[string]bool{"hedda": true, "brenna": false, "wren": false} {
		if err := s.Query("profile."+id+".unlocked", &open); err != nil || open != want {
			t.Fatalf("%s at the start: unlocked %v (%v), want %v", id, open, err, want)
		}
	}
	paths, err := gozen.Explore(context.Background(), s, "- call: opening\n", gozen.ExploreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		for _, id := range []string{"brenna", "wren", "hedda"} {
			if err := p.End.Query("profile."+id+".unlocked", &open); err != nil || !open {
				t.Fatalf("answers %v: %s is not in the book at the end (%v)", p.Answers, id, err)
			}
		}
	}
}
