package headless_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/headless"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestHeadlessDecisionTrace(t *testing.T) {
	db := testgame.Load(t, nil)
	plain := headless.New(db, testgame.State(db, 8))
	want := plain.Fight("boss")
	wantSave, err := plain.St.Save()
	if err != nil {
		t.Fatal(err)
	}
	traced := headless.New(db, testgame.State(db, 8))
	var records []battle.Decision
	traced.OnDecision = func(d battle.Decision) { records = append(records, d) }
	got := traced.Fight("boss")
	gotSave, err := traced.St.Save()
	if err != nil {
		t.Fatal(err)
	}
	if got != want || !bytes.Equal(wantSave, gotSave) || !reflect.DeepEqual(plain.Transcript, traced.Transcript) || plain.St.Rand.Uint64() != traced.St.Rand.Uint64() {
		t.Fatal("headless の記録で結果・文・保存状態・次の乱数が変わった")
	}
	if len(records) == 0 {
		t.Fatal("headless の判断理由が記録されない")
	}
	for _, d := range records {
		if d.Troop != "boss" || d.Turn < 1 || d.Timing == battle.DecisionDirect {
			t.Fatalf("実評価の情報が足りない %v", d)
		}
	}
}
