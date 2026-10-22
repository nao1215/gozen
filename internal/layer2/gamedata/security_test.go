package gamedata

import (
	"strconv"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/expr"
)

func TestLiteralAssignmentsQuoteHostileNames(t *testing.T) {
	for _, name := range []string{`plain`, `a'b`, `a"b`, `a\b`, "a\nb", "a\x00b", `x" || true || "y`} {
		var set Assignments
		if err := yaml.Unmarshal([]byte("location: "+strconv.Quote(name)), &set); err != nil {
			t.Fatalf("unmarshal %q: %v", name, err)
		}
		got, err := set["location"].Eval(expr.Map{})
		if err != nil || got != name {
			t.Fatalf("location %q: got %q, %v", name, got, err)
		}
	}
}

func TestCampaignLoseQuotesPlayer(t *testing.T) {
	player := `team'"\foe`
	db := &DB{Factions: map[string]*Faction{player: {Capital: "home"}}}
	db.Game.Campaign = &Campaign{Player: player}
	e := CampaignLose(db)
	v, err := e.Eval(expr.Map{"region": expr.Map{"home": expr.Map{"owner": player}}})
	if err != nil || v != false {
		t.Fatalf("owner %q: %v, %v", player, v, err)
	}
}

func TestEditDistanceBoundsLongNames(t *testing.T) {
	if got := editDistance(strings.Repeat("x", 1_000_000), "say"); got != 3 {
		t.Fatalf("distance = %d", got)
	}
}
