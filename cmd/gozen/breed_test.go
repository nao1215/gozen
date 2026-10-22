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
	if result.Reachable[1].ParentSexes[0] != "male" || result.Reachable[1].ParentSexes[1] != "female" || result.Reachable[1].Sex == "" {
		t.Fatalf("recipe sexes = %+v", result.Reachable[1])
	}
	invoke(t, 2, "sim", "breed", dir, "--format", "bad")
	book, _ := invoke(t, 0, "book", dir)
	if !strings.Contains(book, "Recruit: 100% after victory; class: ranger") || !strings.Contains(book, "## Monster breeding") {
		t.Fatalf("book omitted monster rules: %s", book)
	}
	jsonBook, _ := invoke(t, 0, "book", dir, "--format", "json")
	var catalog struct {
		Monsters []struct {
			ID      string `json:"id"`
			Lineage struct {
				Recipe struct {
					Kind string `json:"kind"`
				} `json:"recipe"`
				Parents []struct {
					Ref string `json:"ref"`
				} `json:"parents"`
			} `json:"lineage"`
		} `json:"monsters"`
	}
	if err := json.Unmarshal([]byte(jsonBook), &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Monsters) != 2 || catalog.Monsters[1].Lineage.Recipe.Kind != "pair" || len(catalog.Monsters[1].Lineage.Parents) != 2 || catalog.Monsters[1].Lineage.Parents[1].Ref != "seen" {
		t.Fatalf("book lineage = %+v", catalog)
	}
}

func TestGoZenBreedGraphReachesEveryOrdinarySpecies(t *testing.T) {
	out, _ := invoke(t, 0, "sim", "breed", "../../examples/go-zen/game", "--format", "json")
	var result struct {
		Reachable   []breedRow `json:"reachable"`
		Unreachable []string   `json:"unreachable"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Reachable) != 44 || len(result.Unreachable) != 3 {
		t.Fatalf("Go Zen graph: %d reached, unreachable %v", len(result.Reachable), result.Unreachable)
	}
	paths := map[string]breedRow{}
	for _, row := range result.Reachable {
		paths[row.ID] = row
	}
	if p := paths["succubus"]; p.Steps != 2 || len(p.Parents) != 2 ||
		(p.Parents[0] != "vampire" || p.Parents[1] != "whisper_imp") && (p.Parents[1] != "vampire" || p.Parents[0] != "whisper_imp") {
		t.Fatalf("succubus has no two-generation path: %+v", p)
	}
	for _, id := range []string{"vampire", "whisper_imp"} {
		if p := paths[id]; p.Steps != 1 || len(p.Parents) != 2 || paths[p.Parents[0]].Steps != 0 || paths[p.Parents[1]].Steps != 0 {
			t.Fatalf("%s is not traceable to captures: %+v", id, p)
		}
	}
	for _, id := range result.Unreachable {
		if id != "brass_warden" && id != "tollkeeper" && id != "vance" {
			t.Fatalf("ordinary species %q is unreachable", id)
		}
	}
}
