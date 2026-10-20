package gamedata_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

func TestDeclaredStateNames(t *testing.T) {
	for _, tc := range []struct{ name, command, want string }{
		{"宣言名", `set: {var.a: "var.b + 1", var.text: "str(var.a)", var.enabled: "chance(0.5)"}`, ""},
		{"定数添字", `set: {var.a: "var['b']"}`, ""},
		{"未知参照", `if: "var.typo > 0"`, "typo"},
		{"未知代入先", `set: {var.typo: 1}`, "未宣言の代入先"},
		{"名前の空", `set: {var.: 1}`, "未宣言"},
		{"入れ子の代入先", `set: {var.a.x: 1}`, "未宣言"},
		{"数へ文字列", `set: {var.a: "'abc'"}`, "型 number"},
		{"数へ真偽", `set: {var.a: true}`, "型 number"},
		{"真偽へ数", `set: {flag.ready: 1}`, "型 bool"},
		{"文字列へ数", `set: {var.text: 1}`, "型 string"},
		{"条件の片枝", `set: {var.a: "flag.ready ? 1 : 'x'"}`, "型 number"},
		{"文字列の減算", `set: {var.a: "var.text - 1"}`, "型 number"},
		{"nil", `set: {var.a: nil}`, "型 number"},
		{"既存の型", `set: {var.text: "actor.scout.name", var.a: "party.min_hp_rate"}`, ""},
		{"動的な添字", `if: "var[var.text] > 0"`, "直接指定"},
		{"真偽のフィールド", `if: "var.enabled.typo"`, "has no fields"},
		{"討伐の生成名", `set: {var.a: "var.kill_mote + var.last_kill_boss + var.known_mote_fire"}`, ""},
		{"存在しない敵", `if: "var.kill_slmie"`, "kill_slmie"},
		{"存在しない属性", `if: "var.known_mote_fier"`, "known_mote_fier"},
		{"存在しない提案", `if: "var.offer_page1"`, "offer_page1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := testgame.TryLoad(map[string]string{"game.yaml": testgame.DeclaredGame, "data/contract.yaml": "events:\n  contract:\n    do:\n      - " + tc.command + "\n"})
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "data/contract.yaml:") || !strings.Contains(err.Error(), "events.contract.do[0]") {
				t.Fatalf("診断が違う %v", err)
			}
		})
	}
	t.Run("初期値と宣言の誤り", func(t *testing.T) {
		for _, declaration := range []string{
			"var.a: {type: number, initial: true}", "var.a: {type: string, initial: 1}",
			"flag.ready: {type: number, initial: 1}", "var.a: {type: unknown, initial: 1}",
			"var.a: {type: number}", "var.a: {type: number, initial: null}",
			"var.kill_mote: {type: number, initial: 1}", "var.a.x: {type: number, initial: 1}",
		} {
			_, err := testgame.TryLoad(map[string]string{"game.yaml": testgame.Game + "\nstate:\n  names:\n    " + declaration + "\n"})
			if err == nil || !strings.Contains(err.Error(), "game.yaml:") || !strings.Contains(err.Error(), "state.names") {
				t.Fatalf("宣言 %s の診断 %v", declaration, err)
			}
		}
	})
	t.Run("命令由来の生成名", func(t *testing.T) {
		db := testgame.Load(t, map[string]string{"game.yaml": testgame.DeclaredGame, "data/contract.yaml": `
events:
  contract:
    do:
      - if: true
        then:
          - sp_suggest: offer
          - stone_suggest: stones
          - stone_choose: pick
          - sp_decline: refusal
          - set: {var.text: "var.offer_page1 + var.offer_no + var.stones_page2 + var.pick_who + var.refusal_no"}
          - set: {var.a: "var.offer + var.stones_sets + var.pick_wearers"}
`})
		registry := db.StateRegistry()
		for _, name := range []string{"var.offer_page0", "var.offer_page3", "var.offer_typo", "var.pick_page1"} {
			if _, ok := registry[name]; ok {
				t.Errorf("存在しない生成名 %s を許した", name)
			}
		}
	})
	t.Run("回想と店", func(t *testing.T) {
		for _, extra := range []string{
			`gallery: {scenes: [{title: 回想, event: sub, flag: typo}]}`,
			`gallery: {scenes: [{title: 回想, event: sub, flag: ready, set: {var.text: 1}}]}`,
		} {
			_, err := testgame.TryLoad(map[string]string{"game.yaml": testgame.DeclaredGame + "\n" + extra})
			if err == nil || !strings.Contains(err.Error(), "gallery.scenes[0]") {
				t.Fatalf("回想の診断 %v", err)
			}
		}
		for _, name := range []string{"var.typo", "var.text"} {
			_, err := testgame.TryLoad(map[string]string{"game.yaml": testgame.DeclaredGame, "data/shop.yaml": fmt.Sprintf("shops: {test: {spent: %s}}", name)})
			if err == nil || !strings.Contains(err.Error(), "shops.test.spent") {
				t.Fatalf("店の診断 %v", err)
			}
		}
	})
	t.Run("厳密設定の型", func(t *testing.T) {
		for _, raw := range []string{"null", "1", "'true'"} {
			_, err := testgame.TryLoad(map[string]string{"game.yaml": testgame.Game + "\nstate: {strict: " + raw + "}"})
			if err == nil || !strings.Contains(err.Error(), "state.strict") {
				t.Fatalf("厳密設定 %s の診断 %v", raw, err)
			}
		}
	})
	t.Run("生成名の衝突", func(t *testing.T) {
		_, err := testgame.TryLoad(map[string]string{"game.yaml": testgame.DeclaredGame, "data/conflict.yaml": `events: {conflict: {do: [{sp_suggest: offer}, {sp_suggest: offer_page1}]}}`})
		if err == nil || !strings.Contains(err.Error(), "衝突") {
			t.Fatalf("生成名の衝突の診断 %v", err)
		}
	})

	t.Run("旧データ", func(t *testing.T) {
		testgame.Load(t, map[string]string{"data/contract.yaml": `events: {contract: {do: [{set: {var.free: "'x'", flag.free: 2}}, {set: {var.free: 1}}]}}`})
	})
}

func TestSetAssignmentContract(t *testing.T) {
	db := testgame.Load(t, map[string]string{"game.yaml": testgame.DeclaredGame + `
gallery:
  scenes:
    - title: 回想
      event: sub
      flag: ready
      set: {var.b: "var.a", var.a: 4}
`, "data/contract.yaml": `
events:
  contract:
    do:
      - if: true
        then:
          - choice:
              - label: 選択
                do:
                  - set: {var.b: "var['a']", var.a: "var.a + 1"}
                  - set: {var.a: "var.a + 1"}
`})
	warnings := fmt.Sprint(db.SetWarnings())
	for _, want := range []string{"data/contract.yaml:", "events.contract.do[0].then[0].choice[0].do[0].set.var.b", "game.yaml:", "gallery.scenes[0].set.var.b", "辞書順"} {
		if !strings.Contains(warnings, want) {
			t.Errorf("警告に %s がない %s", want, warnings)
		}
	}
	if len(db.SetWarnings()) != 2 {
		t.Fatalf("自己参照を警告した %s", warnings)
	}
	for range 5 {
		if fmt.Sprint(db.SetWarnings()) != warnings {
			t.Fatal("警告の順が変わった")
		}
	}
}
