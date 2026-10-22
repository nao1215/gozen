package main

import (
	"encoding/json"
	"testing"
)

func TestSimBattleFormsAreDeterministic(t *testing.T) {
	for _, tc := range []struct {
		troop string
		args  []string
		wins  int
	}{
		{"drips", []string{"--level", "5"}, 28},
		{"tollkeeper", []string{"--level", "9", "--party", "maren,ox,tilly,kes"}, 30},
	} {
		t.Run(tc.troop, func(t *testing.T) {
			args := append([]string{"sim", "battle", "--battles", "30", "--seed", "42", "--format", "json"}, tc.args...)
			args = append(args, "../../examples/go-zen/game", tc.troop)
			out, _ := invoke(t, 0, args...)
			var rows []struct{ Won, Lost, Draw int }
			if err := json.Unmarshal([]byte(out), &rows); err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || rows[0].Won != tc.wins || rows[0].Won+rows[0].Lost+rows[0].Draw != 30 {
				t.Fatalf("simulation: %+v", rows)
			}
		})
	}
}
