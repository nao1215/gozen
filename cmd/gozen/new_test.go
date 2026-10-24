package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

func TestNewPlayableTemplates(t *testing.T) {
	for _, template := range []string{"rpg", "novel", "campaign"} {
		t.Run(template, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "my game")
			title := "My \"story\": a new journey\nChapter one"
			invoke(t, 0, "new", dir, "--template", template, "--id", "my-game", "--title", title)
			db, err := data.Load(os.DirFS(dir))
			if err != nil {
				t.Fatal(err)
			}
			if db.Game.ID != "my-game" || db.Game.Title != title {
				t.Fatalf("metadata: %+v", db.Game)
			}
			invoke(t, 0, "check", dir)
			for _, name := range []string{"go.mod", "main.go", "Makefile"} {
				if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
					t.Errorf("unexpected programming file %s: %v", name, err)
				}
			}
			original := string(read(t, filepath.Join(dir, "game.yaml")))
			invoke(t, 1, "new", dir, "--template", template)
			if string(read(t, filepath.Join(dir, "game.yaml"))) != original {
				t.Fatal("existing game changed")
			}
		})
	}
}

func TestNewArgumentsAndSaveIdentity(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"a", "b"},
		{"unused", "--template", "unknown"},
		{"unused", "--id", "../bad"},
		{"unused", "--id", ""},
		{"unused", "--id", "-bad"},
		{"unused", "--title", " "},
	} {
		invoke(t, 2, append([]string{"new"}, args...)...)
	}
	var previous string
	for range 2 {
		dir := filepath.Join(t.TempDir(), "same name")
		invoke(t, 0, "new", dir)
		db, err := data.Load(os.DirFS(dir))
		if err != nil {
			t.Fatal(err)
		}
		if db.Game.ID == previous || !strings.HasPrefix(db.Game.ID, "game-") {
			t.Fatalf("save identity %q", db.Game.ID)
		}
		previous = db.Game.ID
	}
}
