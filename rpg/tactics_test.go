package rpg_test

import (
	"bytes"
	"encoding/json"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
	"github.com/nao1215/gozen/rpg"
)

func manualTactics(rows ...data.TacticRow) rpg.ActorTactics {
	return rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsManual, Rows: rows}
}

func TestActorTacticsRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name string
		plan *rpg.ActorTactics
	}{
		{"未編集", nil},
		{"手動の空", new(manualTactics([]data.TacticRow{}...))},
		{"手動のnil", new(manualTactics())},
		{"全行OFF", new(manualTactics(data.TacticRow{Condition: "any", Target: "self", Action: "guard"}))},
		{"順序と閾値０と無効ID", new(manualTactics(
			data.TacticRow{Enabled: true, Condition: "low_mp", Percent: new(0), Target: "self", Action: "guard"},
			data.TacticRow{Enabled: true, Condition: "gone", Target: "enemy", Action: "fire"},
			data.TacticRow{Enabled: true, Condition: "any", Target: "enemy", Action: "attack"},
		))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := testgame.Load(t, map[string]string{"data/tactics.yaml": testgame.Tactics})
			st := testgame.State(db, 11)
			st.Join("guide")
			a := st.Actors["scout"]
			if tc.plan != nil {
				if err := a.SetTactics(*tc.plan); err != nil {
					t.Fatal(err)
				}
			}
			if err := st.Actors["guide"].SetTactics(manualTactics(data.TacticRow{Enabled: true, Condition: "any", Target: "enemy", Action: "fire"})); err != nil {
				t.Fatal(err)
			}
			st.Leave("guide")
			b := saveBytes(t, st)
			got, err := rpg.Parse(db, b, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got.LoadReport.Changed() || !bytes.Equal(b, saveBytes(t, got)) {
				t.Fatal("保存再開で確定済みの作戦が変わった")
			}
			if !reflect.DeepEqual(a.Tactics, got.Actors["scout"].Tactics) || !got.Join("guide") || !reflect.DeepEqual(st.Actors["guide"].Tactics, got.Actors["guide"].Tactics) {
				t.Fatal("複数人または離脱再加入で作戦を失った")
			}
			if tc.plan == nil && (a.Tactics.Mode != rpg.TacticsAuto || a.CompileTactics() != nil) {
				t.Fatal("未編集はおまかせでなければならない")
			}
			if tc.plan != nil && a.CompileTactics() == nil {
				t.Fatal("手動の空とおまかせを区別していない")
			}
		})
	}
}

func TestTacticsIsolation(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/tactics.yaml": testgame.Tactics})
	st := testgame.State(db, 1)
	st.Join("guide")
	a := st.Actors["scout"]
	draft := manualTactics(data.TacticRow{Enabled: true, Condition: "low_mp", Percent: new(30), Target: "self", Action: "guard"})
	before := saveBytes(t, st)
	if a.TacticsDraft().Mode != rpg.TacticsAuto || !bytes.Equal(before, saveBytes(t, st)) {
		t.Fatal("下書きの作成が保存状態を変更した")
	}
	if err := a.SetTactics(draft); err != nil {
		t.Fatal(err)
	}
	committed := saveBytes(t, st)
	*draft.Rows[0].Percent = 90
	draft.Rows[0].Action = "attack"
	edit := a.TacticsDraft()
	*edit.Rows[0].Percent = 0
	edit.Rows[0].Enabled = false
	if !bytes.Equal(committed, saveBytes(t, st)) {
		t.Fatal("未確定の下書きが保存へ漏れた")
	}
	loaded, err := rpg.Parse(db, committed, nil)
	if err != nil {
		t.Fatal(err)
	}
	*loaded.Actors["scout"].Tactics.Rows[0].Percent = 70
	loaded.Actors["scout"].Tactics.Rows[0].Action = "fire"
	if *a.Tactics.Rows[0].Percent != 30 || a.Tactics.Rows[0].Action != "guard" || st.Actors["guide"].Tactics.Mode != rpg.TacticsAuto || testgame.State(db, 1).Actors["scout"].Tactics.Mode != rpg.TacticsAuto {
		t.Fatal("別の仲間または別セーブへ波及した")
	}
	bad := a.TacticsDraft()
	bad.Version++
	if err := a.SetTactics(bad); err == nil || !bytes.Equal(committed, saveBytes(t, st)) {
		t.Fatal("不正な確定で前の作戦を変えた")
	}
	a.Level = 10
	a.Learnt = []string{"heal"}
	a.Equip["weapon"] = "blade"
	a.SpendSP()
	if !reflect.DeepEqual(a.Tactics, loadedPlan(t, db, committed)) {
		t.Fatal("習得・成長・装備変更で手動を上書きした")
	}
}

func loadedPlan(t *testing.T, db *data.DB, b []byte) *rpg.ActorTactics {
	t.Helper()
	st, err := rpg.Parse(db, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	return st.Actors["scout"].Tactics
}

func TestTacticsMigration(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/tactics.yaml": testgame.Tactics})
	st := testgame.State(db, 1)
	st.Join("guide")
	b := saveBytes(t, st)
	old := rewriteSave(t, b, func(m map[string]any) {
		for _, v := range m["state"].(map[string]any)["actors"].(map[string]any) { //nolint:errcheck // A malformed fixture assertion fails this test; optional counters use zero.
			delete(v.(map[string]any), "tactics") //nolint:errcheck // A malformed fixture assertion fails this test; optional counters use zero.
		}
	})
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "包みあり旧状態", true: "版なし旧状態"}[legacy], func(t *testing.T) {
			input := old
			if legacy {
				input = mustJSON(t, readSaveObject(t, old)["state"])
			}
			got, err := rpg.ParseWithConfig(db, input, nil, rpg.Config{AllowLegacy: legacy})
			if err != nil {
				t.Fatal(err)
			}
			if !got.LoadReport.Changed() || len(got.LoadReport.Changes) != 2 || !bytes.Equal(b, saveBytes(t, got)) {
				t.Fatalf("旧作戦の移行または通知が不正 %+v", got.LoadReport)
			}
			for _, c := range got.LoadReport.Changes {
				if !strings.HasSuffix(c.Path, "/tactics") || c.Reason == "" {
					t.Fatal("移行箇所と理由がない")
				}
			}
			path := filepath.Join(t.TempDir(), "save.json")
			if err := os.WriteFile(path, input, 0o600); err != nil {
				t.Fatal(err)
			}
			backup, err := got.SaveFile(path)
			if err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(backup) //nolint:gosec // Paths are isolated test fixtures or declared schema artifact paths.
			if err != nil || !bytes.Equal(original, input) {
				t.Fatal("移行前の原本を保持していない")
			}
			again, err := rpg.Parse(db, saveBytes(t, got), nil)
			if err != nil || again.LoadReport.Changed() {
				t.Fatalf("移行を繰り返した %v", err)
			}
		})
	}
	for _, tc := range []struct {
		name string
		plan any
	}{
		{"未来版", map[string]any{"version": 2, "rows": "未来の構造"}},
		{"負の版", map[string]any{"version": -1}},
		{"明示０", map[string]any{"version": 0}},
		{"版欠損", map[string]any{"mode": "manual"}},
		{"版null", map[string]any{"version": nil}},
		{"版文字列", map[string]any{"version": "1"}},
		{"行列欠損", map[string]any{"version": 1, "mode": "manual"}},
		{"モード欠損", map[string]any{"version": 1, "rows": []any{}}},
		{"全体null", nil},
		{"モードnull", map[string]any{"version": 1, "mode": nil, "rows": []any{}}},
		{"未知モード", map[string]any{"version": 1, "mode": "future", "rows": []any{}}},
		{"自動に手動行", map[string]any{"version": 1, "mode": "auto", "rows": []any{map[string]any{}}}},
		{"行null", map[string]any{"version": 1, "mode": "manual", "rows": []any{nil}}},
		{"行列の型違い", map[string]any{"version": 1, "mode": "manual", "rows": "行ではない"}},
		{"有効状態null", map[string]any{"version": 1, "mode": "manual", "rows": []any{map[string]any{"enabled": nil}}}},
		{"閾値null", map[string]any{"version": 1, "mode": "manual", "rows": []any{map[string]any{"percent": nil}}}},
		{"未知の行項目", map[string]any{"version": 1, "mode": "manual", "rows": []any{map[string]any{"future": true}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := rewriteSave(t, b, func(m map[string]any) {
				m["state"].(map[string]any)["actors"].(map[string]any)["scout"].(map[string]any)["tactics"] = tc.plan //nolint:errcheck // A malformed fixture assertion fails this test; optional counters use zero.
			})
			rng := rand.New(rand.NewPCG(1, 2))    //nolint:gosec // Deterministic gameplay randomness, not security-sensitive data.
			oracle := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // Deterministic gameplay randomness, not security-sensitive data.
			got, err := rpg.Parse(db, bad, rng)
			if err == nil || got != nil || rng.Uint64() != oracle.Uint64() || !bytes.Equal(b, saveBytes(t, st)) {
				t.Fatal("不正形式の拒否で状態または乱数が変わった")
			}
			if tc.name == "未来版" && !strings.Contains(err.Error(), "作戦形式版 2") {
				t.Fatalf("未来版の本文を先に解釈した %v", err)
			}
		})
	}
	t.Run("未知形式の保存を拒否", func(t *testing.T) {
		st.Actors["scout"].Tactics.Version++
		if _, err := st.Save(); err == nil {
			t.Fatal("未知形式を保存してしまった")
		}
	})
}

func readSaveObject(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal(b, &obj); err != nil {
		t.Fatal(err)
	}
	return obj
}
