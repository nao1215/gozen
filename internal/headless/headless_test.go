package headless_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/headless"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestMoveRevealsDungeonEntrance records the entrance of the destination in the exploration record, the same as the screen.
func TestMoveRevealsDungeonEntrance(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/entry.yaml": `
locations:
  passage:
    name: 通路
    dungeon:
      map: |
        #####
        #S.T#
        #####
      tiles:
        S: {start: true, facing: east}
        T: {name: 奥}
`})
	st := testgame.State(db, 1)
	d := headless.New(db, st)
	if err := d.Run(context.Background(), []data.Command{{Move: "passage"}}); err != nil {
		t.Fatal(err)
	}
	if len(st.Mapped["passage"]) != 15 || st.Mapped["passage"][6] != 1 || st.Vars["explored"] != float64(33) {
		t.Fatalf("到着した入口が踏破記録にない 地図%v 探索率%v", st.Mapped, st.Vars["explored"])
	}
	if err := d.Walk('T'); err != nil {
		t.Fatal(err)
	}
	if st.Vars["explored"] != float64(100) {
		t.Fatalf("入口から奥まで歩いても踏破にならない %v", st.Vars["explored"])
	}
}

// AutoBattle also runs the commands of in-battle events (gozen sim and curve use the same path).
func TestAutoBattleRunsTroopEvents(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	b := battle.New(db, st, "boss") // set flag.saw_two on turn 2
	headless.AutoBattle(db, st, b, nil, 3, nil)
	if b.Turn < 2 {
		t.Fatalf("2 ターン目まで続かなかった（turn %d, %v）", b.Turn, b.Result)
	}
	if !st.Flag("saw_two") {
		t.Error("戦闘中イベントの set が実行されていない")
	}
}

// battle.after_win works in headless too, for both event battles (ending with end) and Fights.
func TestAfterWin(t *testing.T) {
	db := testgame.Load(t, map[string]string{
		"game.yaml": testgame.Game + "  after_win: aw\n",
		"data/z.yaml": `
events:
  aw:
    do: [{set: {var.aw: "var.aw + 1"}}]
  fight:
    do:
      - battle: one
      - end: true
`,
	})
	st := testgame.State(db, 1)
	for _, a := range st.Members() {
		a.Level = 30
		a.Recover()
	}
	d := headless.New(db, st)
	if err := d.Run(context.Background(), db.Events["fight"].Do); err != nil {
		t.Fatal(err)
	}
	if d.Wins != 1 || st.Vars["aw"] != 1.0 {
		t.Fatalf("イベントの戦闘の後: wins %d, var.aw %v", d.Wins, st.Vars["aw"])
	}
	st.Location = "field"
	if err := d.Fights(1); err != nil {
		t.Fatal(err)
	}
	if d.Wins != 2 || st.Vars["aw"] != 2.0 {
		t.Fatalf("Fights の後: wins %d, var.aw %v", d.Wins, st.Vars["aw"])
	}
}

// The portrait over a provisional background is recorded as the same display as the screen, for both direct runs and exhaustive path search.
func TestStandInKeepsFaces(t *testing.T) {
	db := testgame.Load(t, map[string]string{
		"game.yaml":                     testgame.Game + "\nstand_ins: cg/stand_in.txt\n",
		"assets/cg/stand_in.txt":        "# 仮背景\ncg/placeholder.png\n",
		"assets/cg/placeholder.png":     "仮背景",
		"assets/cg/placeholder.002.png": "仮背景の差分",
		"assets/cg/finished.png":        "完成画像",
		"data/images.yaml": `
events:
  images:
    do:
      - image: cg/placeholder.png
      - image: {files: [cg/placeholder.png, cg/placeholder.*.png], page: 2}
      - image: cg/finished.png
      - image: {file: cg/finished.png, faces: true}
      - image: ""
`,
	})
	want := []headless.Step{
		{Kind: "image", File: "cg/placeholder.png", Faces: true},
		{Kind: "image", File: "cg/placeholder.002.png", Faces: true},
		{Kind: "image", File: "cg/finished.png"},
		{Kind: "image", File: "cg/finished.png", Faces: true},
		{Kind: "image"},
	}
	cmds := db.Events["images"].Do
	t.Run("直接実行", func(t *testing.T) {
		d := headless.New(db, testgame.State(db, 1))
		d.Record = true
		var observed []headless.Step
		d.OnStep = func(s headless.Step) { observed = append(observed, s) }
		if err := d.Run(context.Background(), cmds); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(d.Steps, want) || !reflect.DeepEqual(observed, want) {
			t.Fatalf("表示記録 %v、観測 %v、期待 %v", d.Steps, observed, want)
		}
	})
	t.Run("全経路探索", func(t *testing.T) {
		paths := headless.Explore(db, testgame.State(db, 1), cmds, headless.Config{})
		if len(paths) != 1 || len(paths[0].Errs) != 0 {
			t.Fatalf("探索結果が不正 %v", paths)
		}
		if !reflect.DeepEqual(paths[0].Steps, want) {
			t.Fatalf("表示記録 %v、期待 %v", paths[0].Steps, want)
		}
	})
}
