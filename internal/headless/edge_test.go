package headless_test

import (
	"context"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/headless"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

// A choice whose options are all hidden by their when conditions must not crash the driver.
func TestChoiceWithNoVisibleOption(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/c.yaml": `
events:
  empty:
    do:
      - choice:
          - {label: A, when: "false", do: [{gold: 100}]}
          - {label: B, when: "false", do: [{gold: 200}]}
      - gold: 1
`})
	st := testgame.State(db, 1)
	d := headless.New(db, st)
	before := st.Gold
	done := make(chan error, 1)
	go func() { done <- d.Run(context.Background(), db.Events["empty"].Do) }()
	if err := <-done; err != nil {
		t.Logf("run: %v", err)
	}
	if st.Gold != before+1 {
		t.Errorf("gold %d, want %d (no option taken, the event goes on)", st.Gold, before+1)
	}
}

func TestDriverMenuAndRouting(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	d := headless.New(db, st)
	if err := d.Start(); err != nil {
		t.Fatal(err)
	}
	if !d.Has("Field") || d.Has("Nowhere") {
		t.Fatal("the camp menu has an exit to the field and nothing else of that name")
	}
	err := d.Do("Nowhere")
	if err == nil || !strings.Contains(err.Error(), `at camp there is no "Nowhere"`) || !strings.Contains(err.Error(), "Field") {
		t.Fatalf("a missing entry says where it looked and what there is: %v", err)
	}
	if err := d.GoTo("field"); err != nil || st.Location != "field" {
		t.Fatalf("goto field: %v at %s", err, st.Location)
	}
	if err := d.GoTo("field"); err != nil {
		t.Fatalf("already there: %v", err)
	}
	// the vault opens only with the key, which a mote drops
	if err := d.GoTo("vault"); err == nil || !strings.Contains(err.Error(), "no way from field to vault") {
		t.Fatalf("a closed way: %v", err)
	}
	if err := d.Fights(3); err != nil {
		t.Fatal(err)
	}
	if st.Items["key"] == 0 {
		t.Fatal("a mote always drops the key in the fixture")
	}
	if err := d.GoTo("vault"); err != nil || st.Location != "vault" {
		t.Fatalf("goto vault with the key: %v at %s", err, st.Location)
	}
	if err := d.Fights(1); err == nil || !strings.Contains(err.Error(), "no battles at vault") {
		t.Fatalf("fights where there are none: %v", err)
	}
	if d.Battles != 3 || d.Wins != 3 || d.Losses != 0 {
		t.Errorf("battles %d wins %d losses %d", d.Battles, d.Wins, d.Losses)
	}
}

func TestDriverEquipBestAndShop(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/shop.yaml": `
shops:
  smith: {goods: [blade, hat, potion]}
`})
	st := testgame.State(db, 1)
	d := headless.New(db, st)
	st.Gold = 1000
	d.Shop("smith", map[string]int{"potion": 5})
	a := st.Members()[0]
	if a.Equip["weapon"] != "blade" {
		t.Errorf("the scout buys and wears the blade, wears %q", a.Equip["weapon"])
	}
	if st.ItemCount("potion", false) != 5 {
		t.Errorf("potions %d, want 5", st.ItemCount("potion", false))
	}
	if st.Gold >= 1000 {
		t.Error("buying cost nothing")
	}
	st.AddItem("stick", 1)
	d.EquipBest()
	if a.Equip["weapon"] != "blade" {
		t.Errorf("EquipBest keeps the better blade, wears %q", a.Equip["weapon"])
	}
	a.Equip["weapon"] = ""
	st.AddItem("blade", 1)
	d.EquipBest()
	if a.Equip["weapon"] != "blade" {
		t.Errorf("EquipBest wears the best weapon in the bag, wears %q", a.Equip["weapon"])
	}
}

func TestDriverRecordsEverything(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/r.yaml": `
events:
  tour:
    do:
      - say: {who: scout, text: Hello}
      - notice: A notice
      - choice: [{label: One, do: [{gold: 1}]}, {label: Two, do: []}]
      - music: {song: theme}
      - image: {file: x.png}
      - battle: one
      - shop: smith
      - move: field
music:
  songs: {theme: {file: x.png}}
shops:
  smith: {goods: [potion]}
`})
	st := testgame.State(db, 1)
	d := headless.New(db, st)
	d.Record = true
	d.Answers = []int{1}
	var seen []string
	d.OnStep = func(s headless.Step) { seen = append(seen, s.Kind) }
	var shops []string
	d.OnShop = func(_ *rpg.State, id string) { shops = append(shops, id) }
	if err := d.Run(context.Background(), db.Events["tour"].Do); err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, s := range d.Steps {
		kinds = append(kinds, s.Kind)
	}
	want := "say,notice,choice,music,image,battle,move,say"
	if got := strings.Join(kinds, ","); got != want {
		t.Errorf("steps %s, want %s", got, want)
	}
	if strings.Join(seen, ",") != want {
		t.Errorf("OnStep saw %v", seen)
	}
	if len(shops) != 1 || shops[0] != "smith" {
		t.Errorf("OnShop saw %v", shops)
	}
	if d.Steps[2].Text != "Two" || st.Gold != 107 {
		t.Errorf("the second option was answered: %+v gold %d", d.Steps[2], st.Gold)
	}
	if !strings.Contains(strings.Join(d.Transcript, "\n"), "Scout「Hello」") {
		t.Errorf("transcript %q", d.Transcript)
	}
}

func TestDriverResultAndChooseHooks(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	d := headless.New(db, st)
	d.Choose = func(_ *rpg.State, q event.Choice) int { return len(q.Options) + 5 } // out of range clamps
	d.Result = func(*rpg.State, string) (int, bool) { return event.BattleLost, true }
	cmds := []data.Command{{Battle: "one", OnLose: []data.Command{{Heal: "all"}}}}
	if err := d.Run(context.Background(), cmds); err != nil {
		t.Fatal(err)
	}
	if d.Battles != 0 {
		t.Error("a scripted result must not fight")
	}
	if st.Members()[0].HP < 1 {
		t.Error("a battle that may be lost leaves the party alive")
	}
}
