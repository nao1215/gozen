package gamedata_test

import (
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestFormDiagnostics(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Enemies["mote"].Images = []data.HPImage{
		{Below: 0.2, File: "missing.png"},
		{Below: 0.5, File: "x.png"},
	}
	db.Enemies["mote"].HPTriggers = []data.HPTrigger{{Below: 0.2, Transform: "unknown_trigger"}, {Below: 0.5, Transform: "boss"}}
	db.Skills["fire"].Effects[0].Damage = data.Expr{}
	db.Skills["fire"].Effects[0].Transform = "unknown_form"
	db.Skills["heal"].Effects[0].Heal = data.Expr{}
	db.Skills["heal"].Effects[0].Merge = &data.Merge{Into: "unknown_merge", From: []string{"mote"}, Count: 3, HP: "sum"}
	db.Enemies["mote"].AI.Rules[0].Use = "heal"
	var all []string
	for _, err := range db.Check() {
		all = append(all, err.Error())
	}
	text := strings.Join(all, "\n")
	for _, want := range []string{"unknown enemy \"unknown_form\" [GZN1101]", "unknown enemy \"unknown_merge\" [GZN1101]", "unknown enemy \"unknown_trigger\" [GZN1101]", "no file assets/missing.png [GZN1105]", "HP thresholds must descend strictly", "HP trigger thresholds must descend strictly", "no troop with this skill user has 3 eligible members"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in diagnostics:\n%s", want, text)
		}
	}
}
