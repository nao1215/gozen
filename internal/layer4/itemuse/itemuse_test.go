package itemuse_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/layer4/itemuse"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestUseHealsTheTargetAndSpendsTheItem(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	st.Join("guide")
	use := itemuse.For(db, st)
	scout, guide := st.Actors["scout"], st.Actors["guide"]
	scout.HP, guide.HP = 1, 1
	before := st.Items["potion"]

	lines, err := use("potion", "guide")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) == 0 {
		t.Error("using an item reports what happened")
	}
	if guide.HP != 31 || scout.HP != 1 {
		t.Errorf("the named target is healed by 30, the user is not: guide %d scout %d", guide.HP, scout.HP)
	}
	if st.Items["potion"] != before-1 {
		t.Errorf("potions %d, want %d", st.Items["potion"], before-1)
	}
	// no target means the first member
	if _, err := use("potion", ""); err != nil {
		t.Fatal(err)
	}
	if scout.HP != 31 {
		t.Errorf("the first member is healed by default, got %d", scout.HP)
	}
}

func TestUseRefusals(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	use := itemuse.For(db, st)
	if _, err := use("potion", "ghost"); err == nil || !strings.Contains(err.Error(), "ghost is not in the party") {
		t.Errorf("a target outside the party: %v", err)
	}
	if st.Items["potion"] != 2 {
		t.Error("a refused use spent the item")
	}
	if _, err := use("key", ""); err == nil {
		t.Error("a key cannot be used")
	}
	if _, err := use("nothing", ""); err == nil {
		t.Error("an unknown item cannot be used")
	}
	st.Party = nil
	if _, err := use("potion", ""); err == nil || !strings.Contains(err.Error(), "no party member") {
		t.Errorf("no one to use it: %v", err)
	}
}
