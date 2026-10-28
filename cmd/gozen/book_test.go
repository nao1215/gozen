package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/diag"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
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
			db, err := data.LoadDir(os.DirFS(game), ".")
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
			if len(db.Enemies) > 0 && (!strings.Contains(out, "## Monster book pages") || !strings.Contains(out, "- Picture: `")) {
				t.Error("the enabled monster book has no pages in the book output")
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

// The optional parts of a game each have a line in the book when the game uses them.
func TestBookShowsDescriptionsRoamersLooksMusicAndBranches(t *testing.T) {
	dir := designGame(t)
	world := filepath.Join(dir, "data", "world.yaml")
	edit(t, world, "    name: Camp\n", "    name: Camp\n    desc: A quiet clearing.\n")
	edit(t, world, "      tiles:\n        S: {start: true}", `      roamers:
        - name: Ghoul
          picture: mote.png
          move: chase
          appear: "flag.met"
          every: "3"
          troops: [{troop: lesson}]
          meet: [{say: Boo}]
        - name: Brute
          picture: mote.png
          move: wander
          count: "1"
          troops: [{troop: pair}]
      looks:
        - {when: "var.floor >= 2", wall: mote.png, battle_background: mote.png, area: Deep}
      tiles:
        S: {start: true}`)
	edit(t, world, "events:\n", `events:
  duel:
    do:
      - battle: rival
        fighters: [scout]
        on_win: [{say: Won}]
        on_lose: [{say: Lost}]
  watch: {trigger: enter, location: camp, when: "gold > 0", do: [{say: Welcome back.}]}
`)
	write(t, filepath.Join(dir, "assets", "calm.ogg"), "sentinel")
	write(t, filepath.Join(dir, "data", "music.yaml"), `music:
  songs:
    theme: {file: calm.ogg, name: Theme}
    wind: {dir: wind, bpm: 90, moods: {calm: calm, tense: tense}}
  rules:
    field:
      - {when: "gold > 100", play: theme, now: true}
      - {when: "gold <= 100", mood: calm}
`)
	write(t, filepath.Join(dir, "assets", "wind", "calm.ogg"), "sentinel")
	write(t, filepath.Join(dir, "assets", "wind", "tense.ogg"), "sentinel")
	out, _ := invoke(t, 0, "book", dir)
	for _, want := range []string{
		"A quiet clearing.",
		"  - Chaser Ghoul (chase, appears flag.met, 1 cell per 3 steps)",
		"    - Boo",
		"  - Strong foe Brute (wander, count 1): [pair]",
		"  - Look var.floor >= 2: wall mote.png, battle background mote.png",
		"- battle rival (only [scout])",
		"- on a win\n  - Won",
		"- on a loss\n  - Lost",
		"### watch\n\n- Welcome back.",
		"at_camp -->|enter gold > 0| watch",
		"## Music",
		"- theme (Theme): ",
		"- wind (): calm tense",
		"- Rule field: gold > 100 → theme (now)",
		"- Rule field: gold <= 100 → calm",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the book lacks %q", want)
		}
	}
}

func TestBookFormatsShareACompleteDetachedReport(t *testing.T) {
	dir := designGame(t)
	db, err := data.LoadDir(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	model := buildBook(db)
	before := renderBook(model)
	encoded, err := json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	db.Game.Title = "changed after projection"
	for _, enemy := range db.Enemies {
		enemy.Name = "changed enemy"
	}
	for _, actor := range db.Actors {
		actor.Name = "changed actor"
	}
	for family := range db.MonsterFamilies {
		db.MonsterFamilies[family] = "changed family"
	}
	after, err := json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != string(after) || before != renderBook(model) {
		t.Fatal("book retained mutable source definitions")
	}
	out, _ := invoke(t, 0, "book", dir, "--format", "json")
	raw := resultData(t, out, "book")
	var sections map[string]json.RawMessage
	if err := json.Unmarshal(raw, &sections); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"game", "locations", "monsters", "troops", "skills", "items", "classes", "events", "music", "flow", "lineage"} {
		if len(sections[name]) == 0 || string(sections[name]) == "null" {
			t.Fatalf("missing %s: %s", name, raw)
		}
	}
	lines, _ := invoke(t, 0, "book", dir, "--format", "jsonl")
	dec := json.NewDecoder(strings.NewReader(lines))
	items := 0
	for {
		var row struct {
			Type, ID, Status string
			Data             json.RawMessage
		}
		err := dec.Decode(&row)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if row.Type == "summary" {
			var counts struct{ Sections int }
			if json.Unmarshal(row.Data, &counts) != nil || counts.Sections != items {
				t.Fatalf("%s", row.Data)
			}
			continue
		}
		items++
		want := sections[row.ID]
		if row.ID == "breeding" && len(want) == 0 && string(row.Data) == "null" {
			continue
		}
		if string(want) != string(row.Data) {
			t.Fatalf("section %s changed between encodings", row.ID)
		}
	}
	if items != len(model.sections()) {
		t.Fatalf("%d sections", items)
	}
	lineage, _ := invoke(t, 0, "book", dir, "--section", "lineage")
	if strings.Contains(lineage, "## Locations") || strings.Contains(lineage, "## Items") {
		t.Fatal("lineage selection included other sections")
	}
	invoke(t, 2, "book", "missing", "--section", "unknown", "--format", "json")
}

// User-authored pipes and line breaks must stay inside their table cell; nested
// dialog must remain under its choice rather than turning into a new paragraph.
func TestBookMarkdownCellsAndNestedDialogue(t *testing.T) {
	b, err := loadBook(designGame(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range b.Enemies {
		e.Name = "Enemy|name\ncontinued"
	}
	for _, s := range b.Skills {
		s.Desc = "Skill|detail\ncontinued"
	}
	for _, i := range b.Items {
		i.Desc = "Item|detail\ncontinued"
	}
	b.Events["formatting"] = &bookEvent{Do: []bookCommand{{Kind: "choice", Choice: []bookChoice{{Label: "Pick", Do: []bookCommand{{Kind: "say", Say: &bookSay{Text: "Line one\nLine two"}}}}}}}}
	b.order["events"] = append(b.order["events"], "formatting")
	out := renderBook(b)
	for _, want := range []string{`Enemy\|name<br>continued`, `Skill\|detail<br>continued`, `Item\|detail<br>continued`, "\n  - Line one\n    Line two"} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q", want)
		}
	}
}

func TestBookStreamVoices(t *testing.T) {
	dir := fixture(t)
	write(t, filepath.Join(dir, "assets", "x.png"), string(read(t, filepath.Join(dir, "assets", "mote.png"))))
	for name, body := range testgame.StreamFiles() {
		write(t, filepath.Join(dir, name), body)
	}
	game := strings.Replace(string(read(t, filepath.Join(dir, "game.yaml"))), "schema_version: 1\n", "", 1)
	write(t, filepath.Join(dir, "game.yaml"), game+testgame.StreamsGame)
	out, _ := invoke(t, 0, "book", dir, "--section", "voices", "--format", "json")
	var got struct {
		Version int                    `json:"version"`
		Lines   []data.StreamVoiceLine `json:"lines"`
	}
	if err := json.Unmarshal(resultData(t, out, "book"), &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || len(got.Lines) == 0 {
		t.Fatalf("inventory: %s", out)
	}
	again, _ := invoke(t, 0, "book", dir, "--section", "voices", "--format", "json")
	if out != again {
		t.Fatal("unstable inventory")
	}
	text, _ := invoke(t, 0, "book", dir, "--section", "voices")
	if !strings.Contains(text, "runtime fields: from") {
		t.Fatal(text)
	}
}
