package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/diag"
	data "github.com/nao1215/gozen/schema"
)

// bookGames are the games of this repository that together use most of the schema:
// maps, painted stages, dungeons with roamers, music rules, galleries and class trees.
var bookGames = []string{
	"../../examples/go-zen/game",
	"../../examples/novel/game",
	"../../examples/minimal",
	"../../internal/dev/docs/testdata/docgame",
	"testdata/design",
}

func TestBookListsEveryDefinitionOfTheGame(t *testing.T) {
	for _, dir := range bookGames {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			game := dir
			if dir == "testdata/design" {
				game = designGame(t)
			}
			out, stderr := invoke(t, 0, "book", game)
			if stderr != "" {
				t.Fatalf("stderr: %q", stderr)
			}
			if again, _ := invoke(t, 0, "book", game); again != out {
				t.Fatal("the book is not deterministic")
			}
			db, err := data.Load(os.DirFS(game), ".")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(out, "# "+db.Game.Title+"\n") {
				t.Errorf("the book starts with the title %q: %q", db.Game.Title, out[:min(80, len(out))])
			}
			if !strings.HasSuffix(out, "\n") || strings.HasSuffix(out, "\n\n") || strings.Contains(out, "\n\n\n") {
				t.Error("blocks are separated by one blank line and the book ends with one newline")
			}
			for _, id := range db.Ordered("locations") {
				if !strings.Contains(out, "("+id+")") {
					t.Errorf("location %s is not in the book", id)
				}
			}
			for _, id := range db.Ordered("events") {
				if !strings.Contains(out, "### "+id+"\n") {
					t.Errorf("event %s is not in the book", id)
				}
			}
			for _, id := range db.Ordered("enemies") {
				if !strings.Contains(out, "| "+id+" | ") {
					t.Errorf("enemy %s is not in the book", id)
				}
			}
			for _, id := range db.Ordered("skills") {
				if !strings.Contains(out, "| "+id+" | ") {
					t.Errorf("skill %s is not in the book", id)
				}
			}
			// A section holds an entry or is left out: no heading is followed by another heading or the end.
			lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
			for i, l := range lines {
				if !strings.HasPrefix(l, "## ") {
					continue
				}
				next := ""
				for _, m := range lines[i+1:] {
					if strings.TrimSpace(m) != "" {
						next = m
						break
					}
				}
				if next == "" || strings.HasPrefix(next, "## ") {
					t.Errorf("empty section %q", l)
				}
			}
			if len(db.Events) > 0 {
				if !strings.Contains(out, "## ") || !regexp.MustCompile("(?m)^- [a-z_0-9]+: ").MatchString(out) {
					t.Error("the flow lists every event")
				}
			}
		})
	}
}

func TestBookShowsWhatTheDesignSampleDefines(t *testing.T) {
	out, _ := invoke(t, 0, "book", designGame(t))
	for _, want := range []string{
		"# Design sample",
		"start: camp, party scout, aide, gold 300, items shard ×1",
		"### Camp (camp)",
		"- Person: Courier",
		"- Actions: Descend",
		"- Exit: Inn → inn",
		"- Dungeon: 11x11, 2 rooms",
		"  - Tile > Stairs",
		"| warden | Warden | 400 | 30 | 2 |",
		"- warden: [warden]",
		"| shard | Shard | key | 5 |",
		"### gift",
		`  - choice "Take the offer"`,
		"  - → gift",
		"- home: sets location; moves camp",
		"graph TD",
		"  home -->|move| at_camp",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the book lacks %q", want)
		}
	}
}

// A dive names its floor apart from the place: "dive cave 1", not "dive cave1".
func TestBookSeparatesTheFloorOfADiveFromThePlace(t *testing.T) {
	out, _ := invoke(t, 0, "book", designGame(t))
	for _, want := range []string{"(dive cave 1)", "(dive cave var.floor + 1)", "(move camp)"} {
		if !strings.Contains(out, want) {
			t.Errorf("the book lacks %q", want)
		}
	}
	if strings.Contains(out, "cave1") || strings.Contains(out, "cavevar") {
		t.Error("the floor is glued to the place name")
	}
}

func TestBookFailures(t *testing.T) {
	_, stderr := invoke(t, 1, "book", filepath.Join(t.TempDir(), "absent"))
	if !strings.Contains(stderr, diag.GameUnreadable.String()) {
		t.Errorf("stderr: %q", stderr)
	}
	dir := fixture(t)
	write(t, filepath.Join(dir, "data", "world.yaml"), "locations: [")
	if out, stderr := invoke(t, 1, "book", dir); out != "" || stderr == "" {
		t.Errorf("a game that does not load writes no book: %q %q", out, stderr)
	}
	invoke(t, 2, "book")
	invoke(t, 2, "book", dir, dir)
}
