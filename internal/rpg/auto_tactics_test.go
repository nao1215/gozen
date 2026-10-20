package rpg_test

import (
	"bytes"
	"math/rand/v2"
	"reflect"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

func autoTacticsDB(t *testing.T) *data.DB {
	t.Helper()
	db := testgame.Load(t, map[string]string{
		"game.yaml":         testgame.AutoTacticsGame,
		"data/tactics.yaml": testgame.Tactics, "data/auto.yaml": testgame.AutoTacticsData,
	})
	db.Classes["ranger"].AI = data.AIRef{Name: "capability"}
	db.Classes["ranger"].Tree = []data.TreeNode{{ID: "care", Skill: "group_heal", Max: 1}}
	return db
}

func TestAutoTacticsFollowCapabilities(t *testing.T) {
	db := autoTacticsDB(t)
	st := testgame.State(db, 15)
	a := st.Actors["scout"]
	check := func(want ...string) {
		t.Helper()
		source := rand.NewPCG(15, 16)
		st.Rand = rand.New(source) //nolint:gosec // Deterministic gameplay test, not a security token.
		rngBefore, err := source.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		before := saveBytes(t, st)
		var actions []string
		for _, row := range a.AutoTactics() {
			actions = append(actions, row.Action)
			if _, errs := db.CompileTactic(row, a.Skills()); len(errs) != 0 {
				t.Fatalf("表示行を現在の技で使えない %v", errs)
			}
		}
		if !reflect.DeepEqual(actions, want) {
			t.Fatalf("表示行 %v、必要 %v", actions, want)
		}
		if got := a.CompileTactics(); len(got) != 7 {
			t.Fatalf("評価順を保つ実行用の行が欠けた %v", got)
		}
		rngAfter, err := source.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, saveBytes(t, st)) || !bytes.Equal(rngBefore, rngAfter) {
			t.Fatal("生成・プレビューが状態・SP・品・乱数を消費した")
		}
	}
	check("potion", "fire", "guard", "attack")
	a.GainExp(a.ExpFor(3) - a.Exp)
	check("heal", "potion", "fire", "guard", "attack")
	a.Learnt = append(a.Learnt, "big")
	check("big", "heal", "potion", "fire", "guard", "attack")
	a.Learnt = nil
	a.Equip["weapon"] = "blade"
	check("group_heal", "heal", "potion", "fire", "guard", "attack")
	delete(a.Equip, "weapon")
	check("heal", "potion", "fire", "guard", "attack")
	if !a.Raise("care") {
		t.Fatal("ツリーの習得に失敗")
	}
	check("group_heal", "heal", "potion", "fire", "guard", "attack")
	a.ResetTree()
	st.Items["potion"] = 0
	check("heal", "potion", "fire", "guard", "attack")
	st.Join("guide")
	if rows := st.Actors["guide"].AutoTactics(); len(rows) != 4 {
		t.Fatalf("加入時の習得へ追従しない %v", rows)
	}
	loaded, err := rpg.Parse(db, saveBytes(t, st), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.AutoTactics(), loaded.Actors["scout"].AutoTactics()) || len(loaded.Actors["scout"].Tactics.Rows) != 0 {
		t.Fatal("ロード後の再生成または保存形が不正")
	}
	// Changing the returned rows or thresholds must not leak into the definition or the next preview.
	db.Game.Battle.AutoTactics["capability"][0].Condition = "low_hp"
	db.Game.Battle.AutoTactics["capability"][0].Percent = new(40)
	a.Learnt = []string{"big"}
	rows := a.AutoTactics()
	*rows[0].Percent = 99
	rows[0].Action = "attack"
	if got := a.AutoTactics(); *got[0].Percent != 40 || got[0].Action != "big" {
		t.Fatal("プレビューが定義を共有した")
	}
}

func TestManualTacticsSurviveUpdates(t *testing.T) {
	db := autoTacticsDB(t)
	st := testgame.State(db, 16)
	st.Join("guide")
	a := st.Actors["scout"]
	plan := manualTactics(
		data.TacticRow{Enabled: true, Condition: "target_hp", Percent: new(37), Target: "ally", Action: "group_heal"},
		data.TacticRow{Condition: "low_mp", Percent: new(0), Target: "self", Action: "guard"},
		data.TacticRow{Enabled: true, Condition: "any", Target: "enemy", Action: "fire"})
	if err := a.SetTactics(plan); err != nil {
		t.Fatal(err)
	}
	for _, update := range []func(){
		func() { a.GainExp(a.ExpFor(3) - a.Exp) },
		func() { a.Learnt = []string{"big"} },
		func() { a.Equip["weapon"] = "blade" },
		func() { delete(a.Equip, "weapon") },
		func() { a.Raise("care") },
		func() { a.ResetTree() },
		func() { st.Leave("guide"); st.Join("guide") },
	} {
		update()
		a.AutoTactics()
		compiled := a.CompileTactics()
		if len(compiled) != 3 || !reflect.DeepEqual(a.Tactics, &plan) {
			t.Fatal("更新で手動の行・順序・閾値・OFF が変わった")
		}
	}
	loaded, err := rpg.Parse(db, saveBytes(t, st), nil)
	if err != nil || !reflect.DeepEqual(loaded.Actors["scout"].Tactics, &plan) {
		t.Fatalf("ロードで手動の行が変わった %v", err)
	}
	if err := a.SetTactics(rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsAuto}); err != nil {
		t.Fatal(err)
	}
	if len(a.CompileTactics()) != 7 || len(a.Tactics.Rows) != 0 {
		t.Fatal("明示的におまかせへ戻せない")
	}
}

func TestAutoTacticsDefinitionPrecedence(t *testing.T) {
	db := autoTacticsDB(t)
	a := testgame.State(db, 1).Actors["scout"]
	if len(a.AutoTactics()) == 0 {
		t.Fatal("職業の設定を使わない")
	}
	db.Actors["scout"].AI = data.AIRef{Name: "original_guard"}
	if a.AutoTactics() != nil || a.CompileTactics() != nil {
		t.Fatal("仲間自身の未対応 AI を職業の作戦で上書きした")
	}
	db.Game.Battle.AutoTactics["original_guard"] = []data.TacticRow{{Enabled: true, Condition: "any", Target: "self", Action: "guard"}}
	if got := a.AutoTactics(); len(got) != 1 || got[0].Action != "guard" {
		t.Fatal("仲間自身の作戦を優先しない")
	}
	db.Game.Battle.AutoTactics["original_guard"] = nil
	if got := a.CompileTactics(); got == nil || len(got) != 0 {
		t.Fatal("明示的な空のおまかせを既存 AI へ戻した")
	}
	db.AI["original_guard"] = nil
	if got := a.CompileTactics(); got == nil || len(got) != 0 {
		t.Fatal("比較元の AI が空だと仲間自身の作戦を無視する")
	}
}
