package event_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestTheLastMemberNeverLeaves: a party is never empty, because a state with an empty party cannot be loaded again.
func TestTheLastMemberNeverLeaves(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	st.Join("guide")
	var errs []error
	r := event.New(db, st, &host{})
	r.Log = func(err error) { errs = append(errs, err) }
	r.Start([]data.Command{{Leave: "guide"}, {Leave: "scout"}, {Leave: "stranger"}})
	for i := 0; r.Running() && i < 10; i++ {
		r.Next()
	}
	if len(st.Party) != 1 || st.Party[0] != "scout" {
		t.Errorf("party %v, want the scout to stay", st.Party)
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "last member") {
		t.Errorf("errors %v", errs)
	}
	b, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rpg.Parse(db, b, nil); err != nil {
		t.Errorf("the state the commands left cannot be loaded: %v", err)
	}
	if !st.Leave("stranger") || st.Leave("scout") {
		t.Error("Leave: someone who is not in the party changes nothing and says true; the last member says false")
	}
}
