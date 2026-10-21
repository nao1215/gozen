package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

func TestSimBreedJSON(t *testing.T) {
	dir := t.TempDir()
	for path, body := range testgame.MonsterFiles() {
		write(t, filepath.Join(dir, path), body)
	}
	write(t, filepath.Join(dir, "assets", "x.png"), "fixture")
	out, _ := invoke(t, 0, "sim", "breed", dir, "--format", "json")
	var result struct {
		Reachable   []breedRow `json:"reachable"`
		Unreachable []string   `json:"unreachable"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Reachable) != 2 || result.Reachable[0].ID != "mote" || result.Reachable[1].ID != "boss" || result.Reachable[1].Steps != 1 || len(result.Unreachable) != 0 {
		t.Fatalf("graph = %+v", result)
	}
	invoke(t, 2, "sim", "breed", dir, "--format", "bad")
	book, _ := invoke(t, 0, "book", dir)
	if !strings.Contains(book, "Recruit: 100% after victory; class: ranger") || !strings.Contains(book, "## Monster breeding") {
		t.Fatalf("book omitted monster rules: %s", book)
	}
}
