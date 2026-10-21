package event_test

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/testgame"
)

const moreEvents = `
classes:
  adept:
    name: Adept
    stats: {mhp: "30", mmp: "10", atk: "5", def: "5", mat: "5", mdf: "5", agi: "5", luk: "5"}
    exp_for: "level * 10"
    sp: 2
    start_sp: 4
    tree:
      - {id: spark, skill: fire, max: 2}
      - {id: toughness, stats: {def: 3}, max: 3}
actors:
  monk: {name: Monk, class: adept, tree: {toughness: 2}}
locations:
  deep:
    name: Deep
    dungeon:
      generate: {width: 11, height: 11, rooms: 2, start: S, place: [{char: ">", far: true}]}
      tiles:
        S: {name: Up}
        ">": {name: Down, solid: "true", do: [{dive: deep, floor: "var.floor + 1"}]}
events:
  looping: {do: [{if: "true", then: [{call: looping}]}]}
  learn_it: {do: [{learn: {actor: scout, skill: heal}}, {learn: {actor: scout, skill: heal}}]}
  forget_it: {do: [{reset_tree: all}]}
  forget_one: {do: [{reset_tree: monk}]}
  acting: {do: [{act: {actor: scout, skill: guard}}]}
  hurting: {do: [{hurt: {who: scout, amount: "9999"}}, {affect: {who: scout, add: poison}}, {affect: {who: guide, add: sleep}}]}
  asking: {do: [{save: true}, {open: skills}, {say: after}]}
  descend: {do: [{move: deep}, {dive: deep}]}
`

func moreFiles(extra map[string]string) map[string]string {
	files := map[string]string{"game.yaml": "schema_version: 4\nid: m\n" + testgame.Game, "data/m.yaml": moreEvents}
	maps.Copy(files, extra)
	return files
}

func TestCallLoopIsCutOff(t *testing.T) {
	db := testgame.Load(t, moreFiles(nil))
	st := testgame.State(db, 1)
	var errs []error
	r := event.New(db, st, &host{})
	r.Log = func(err error) { errs = append(errs, err) }
	r.Start(db.Events["looping"].Do)
	for i := 0; r.Running() && i < 1000; i++ {
		r.Next()
	}
	if r.Running() {
		t.Fatal("a call loop must stop")
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "nested too deep") {
		t.Errorf("errors %v", errs)
	}
}

func TestLearnIsOnceAndResetGivesBackPoints(t *testing.T) {
	db := testgame.Load(t, moreFiles(nil))
	st := testgame.State(db, 1)
	r := event.New(db, st, &host{})
	r.Log = func(err error) { t.Error(err) }
	run := func(name string) {
		r.Start(db.Events[name].Do)
		for i := 0; r.Running() && i < 100; i++ {
			r.Next()
		}
	}
	run("learn_it")
	skills := st.Actors["scout"].Skills()
	if n := strings.Count(strings.Join(skills, ","), "heal"); n != 1 {
		t.Errorf("a skill is learned once, skills %v", skills)
	}
	st.Join("monk")
	monk := st.Actors["monk"]
	if monk.Tree["toughness"] != 2 {
		t.Fatalf("the monk starts with 2 levels of toughness: %v", monk.Tree)
	}
	sp := monk.SP
	run("forget_one")
	if len(monk.Tree) != 0 || monk.SP != sp+2 {
		t.Errorf("tree %v sp %d, want it empty and %d", monk.Tree, monk.SP, sp+2)
	}
	monk.Tree["toughness"] = 1
	run("forget_it")
	if len(monk.Tree) != 0 {
		t.Errorf("reset_tree all: %v", monk.Tree)
	}
}

func TestActWithoutASessionIsReported(t *testing.T) {
	db := testgame.Load(t, moreFiles(nil))
	st := testgame.State(db, 1)
	var errs []error
	r := event.New(db, st, &host{})
	r.Log = func(err error) { errs = append(errs, err) }
	r.Start(db.Events["acting"].Do)
	for i := 0; r.Running() && i < 10; i++ {
		r.Next()
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "Act is nil") {
		t.Errorf("%v", errs)
	}
	var got []string
	r2 := event.New(db, st, &host{})
	r2.Act = func(actor, skill, _, _ string) { got = append(got, actor+"/"+skill) }
	r2.Start(db.Events["acting"].Do)
	for i := 0; r2.Running() && i < 10; i++ {
		r2.Next()
	}
	if !slices.Equal(got, []string{"scout/guard"}) {
		t.Errorf("act handed over %v", got)
	}
}

func TestHurtNeverKillsAndAffectNamesWho(t *testing.T) {
	db := testgame.Load(t, moreFiles(nil))
	st := testgame.State(db, 1)
	st.Join("guide")
	r := event.New(db, st, &host{})
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["hurting"].Do)
	for i := 0; r.Running() && i < 10; i++ {
		r.Next()
	}
	if st.Actors["scout"].HP != 1 {
		t.Errorf("hurt leaves 1 HP, got %d", st.Actors["scout"].HP)
	}
	if st.Actors["scout"].States["poison"] == 0 || st.Actors["guide"].States["poison"] != 0 {
		t.Errorf("only the named member is poisoned: %v %v", st.Actors["scout"].States, st.Actors["guide"].States)
	}
	if st.Actors["guide"].States["sleep"] == 0 || st.Actors["scout"].States["sleep"] != 0 {
		t.Errorf("only the named member sleeps: %v %v", st.Actors["scout"].States, st.Actors["guide"].States)
	}
}

func TestSaveAndOpenAreRequests(t *testing.T) {
	db := testgame.Load(t, moreFiles(nil))
	st := testgame.State(db, 1)
	r := event.New(db, st, &host{})
	r.Start(db.Events["asking"].Do)
	var kinds []string
	for i := 0; r.Running() && i < 10; i++ {
		switch q := r.Next().(type) {
		case event.Save:
			kinds = append(kinds, "save")
		case event.Open:
			kinds = append(kinds, "open:"+q.Screen)
		case event.Say:
			kinds = append(kinds, "say:"+q.Text)
		}
	}
	if !slices.Equal(kinds, []string{"save", "open:skills", "say:after"}) {
		t.Errorf("%v", kinds)
	}
}

func TestFramesShowWhereTheRunnerIs(t *testing.T) {
	db := testgame.Load(t, moreFiles(map[string]string{"data/n.yaml": "events: {outer: {do: [{say: one}, {call: inner}, {say: three}]}, inner: {do: [{say: two}]}}\n"}))
	st := testgame.State(db, 1)
	r := event.New(db, st, &host{})
	r.Start(db.Events["outer"].Do)
	if cmds, pc := r.Frames(); len(cmds) != 1 || len(pc) != 1 {
		t.Errorf("before it runs: %v %v", cmds, pc)
	}
	r.Next() // one
	r.Next() // two, inside inner
	cmds, pc := r.Frames()
	if len(cmds) != 2 || len(pc) != 2 || len(cmds[0]) != 3 || len(cmds[1]) != 1 {
		t.Errorf("two frames, the outer of three commands and the inner of one: %d frames, pcs %v", len(cmds), pc)
	}
	r.Next() // three
	if cmds, _ := r.Frames(); len(cmds) != 1 {
		t.Errorf("back in the outer frame: %d", len(cmds))
	}
	r.Stop()
	if r.Running() {
		t.Error("Stop ends it")
	}
	if cmds, pc := r.Frames(); len(cmds) != 0 || len(pc) != 0 {
		t.Errorf("a stopped runner has no frames: %v %v", cmds, pc)
	}
}

func TestDescendingTheDungeonWithDive(t *testing.T) {
	db := testgame.Load(t, moreFiles(nil))
	st := testgame.State(db, 1)
	h := &host{}
	r := event.New(db, st, h)
	r.Log = func(err error) { t.Error(err) }
	st.Vars["floor"] = 1.0
	st.Location = "deep"
	r.Start(db.Events["descend"].Do)
	for i := 0; r.Running() && i < 50; i++ {
		r.Next()
	}
	if st.Location != "deep" || h.moved != "deep" {
		t.Errorf("location %q moved %q", st.Location, h.moved)
	}
	if got := st.Vars["floor"]; got == nil {
		t.Error("the floor number is kept in var.floor")
	}
}
