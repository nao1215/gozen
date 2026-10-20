package headless

import (
	"bytes"
	"testing"

	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestSaveEnvelopeCompatibility checks that branch exploration without rendering carries over the main state under the same save contract.
func TestSaveEnvelopeCompatibility(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.ID = "neutral"
	db.Game.DataVersion = 4
	st := testgame.State(db, 1)
	st.Gold = 321
	st.Flags["met"] = true
	st.Items = map[string]int{}
	st.Actors["scout"].HP = 17
	before, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	paths := Explore(db, st, db.Events["sub"].Do, Config{NoArrive: true})
	if len(paths) != 1 || len(paths[0].Errs) > 0 {
		t.Fatalf("保存からの分岐探索 %v", paths)
	}
	got := paths[0].St
	if got.Gold != 326 || !got.Flags["met"] || len(got.Items) != 0 || got.Actors["scout"].HP != 17 {
		t.Fatal("分岐探索の写しが状態を失った")
	}
	b, err := st.Save()
	if err != nil || !bytes.Equal(b, before) {
		t.Fatal("分岐探索で元の状態を変えた")
	}
	b, err = got.Save()
	if err != nil {
		t.Fatal(err)
	}
	again, err := rpg.Parse(db, b, nil)
	if err != nil || again.Gold != 326 {
		t.Fatalf("描画なしの保存を再開できない %v", err)
	}
	db.Game.ID = "another"
	if _, err := rpg.Parse(db, b, nil); err == nil {
		t.Fatal("別作品の保存を読み込んだ")
	}
}
