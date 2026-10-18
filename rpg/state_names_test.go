package rpg_test

import (
	"bytes"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
	"github.com/nao1215/gozen/rpg"
)

func TestDeclaredStateNames(t *testing.T) {
	db := testgame.Load(t, map[string]string{"game.yaml": testgame.DeclaredGame})
	st := testgame.State(db, 3)
	if !st.Flags["ready"] || st.Vars["a"] != 2.0 || st.Vars["text"] != "初期値" || st.Vars["enabled"] != false {
		t.Fatalf("初期値 %v %v", st.Flags, st.Vars)
	}
	if _, ok := st.Vars["dive_seed"]; ok {
		t.Fatal("未設定であるべき生成名を初期化した")
	}
	for _, pair := range []struct {
		path  string
		value any
	}{
		{"var.a", 0.0}, {"var.b", 11}, {"var.text", ""}, {"var.enabled", true}, {"flag.ready", false}, {"var.kill_mote", 2.0},
	} {
		if err := st.SetPath(pair.path, pair.value); err != nil {
			t.Fatal(err)
		}
	}
	if st.Vars["enabled"] != true || st.Vars["b"] != 11.0 {
		t.Fatal("真偽と数の型を保持していない")
	}
	for _, pair := range []struct {
		path  string
		value any
	}{
		{"var.a", true}, {"var.a", "2"}, {"var.a", nil}, {"var.enabled", 1.0}, {"flag.ready", 1}, {"var.typo", 1}, {"var.kill_typo", 1},
	} {
		before, err := st.Save()
		if err != nil {
			t.Fatal(err)
		}
		if err := st.SetPath(pair.path, pair.value); err == nil {
			t.Fatalf("不正な代入 %s=%v を許した", pair.path, pair.value)
		}
		after, err := st.Save()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Fatal("拒否した代入で状態が変わった")
		}
	}
	for _, source := range []string{"var.typo", "flag.typo"} {
		if _, err := testgame.Expr(source).Eval(st.Env()); err == nil {
			t.Fatalf("未宣言の参照 %s を許した", source)
		}
	}
	saved, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := rpg.Parse(db, saved, nil)
	if err != nil {
		t.Fatal(err)
	}
	again, err := loaded.Save()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved, again) {
		t.Fatal("保存と再開で値が変わった")
	}
	// 後から追加された宣言の初期値だけを補い、既存のゼロ・空文字・false を上書きしない。
	delete(st.Vars, "b")
	saved, err = st.Save()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err = rpg.Parse(db, saved, nil)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Vars["b"] != 7.0 || loaded.Vars["a"] != 0.0 || loaded.Vars["text"] != "" || loaded.Flags["ready"] {
		t.Fatalf("再開時の初期値 %v %v", loaded.Flags, loaded.Vars)
	}
	for _, invalid := range []string{"型", "未知"} {
		st = testgame.State(db, 3)
		if invalid == "型" {
			st.Vars["a"] = "abc"
		} else {
			st.Vars["typo"] = 1.0
		}
		saved, err = st.Save()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := rpg.Parse(db, saved, nil); err == nil {
			t.Fatalf("%s の不正セーブを許した", invalid)
		}
	}
}

func TestDeclaredStateNamesLegacy(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	if err := st.SetPath("var.any", true); err != nil {
		t.Fatal(err)
	}
	if st.Vars["any"] != 1.0 {
		t.Fatal("旧版の真偽から数への変換が変わった")
	}
	if err := st.SetPath("flag.any", 2); err != nil || !st.Flag("any") {
		t.Fatalf("旧版のフラグ %v", err)
	}
	for _, source := range []string{"var.unknown == 0", "flag.unknown == false"} {
		got, err := testgame.Expr(source).Eval(st.Env())
		if err != nil || got != true {
			t.Fatalf("旧版の未設定参照 %s %v %v", source, got, err)
		}
	}
}
