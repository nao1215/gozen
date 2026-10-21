package designeval

import (
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/testgame"
)

// Any text given as designeval.yaml is read or refused, and one that is accepted can be
// played: it never panics, whatever the personas, rules and policies say.
func FuzzDesignConfig(f *testing.F) {
	for _, seed := range []string{
		"personas: [{id: a}]\n",
		"time: {chars_per_sec: 12, images: [{match: x, sec: 1}]}\ngame: {days: 2, town: [camp], never: [x], prefer: [y], stairs: '>'}\nrewards: [{kind: k, events: sub}]\nexpects: [{kind: e, value: gold}]\nrelease: k\npersonas: [{id: a, days: 1, explore: all, grind: 0.5, policies: [{id: p, phase: step, try: [{explore: events}]}]}]\n",
		"personas: [{id: a, town: [{at: camp, do: '@x', words: ['@y']}]}]\n",
		"duels: {fighters: [scout], first: [0, 1], list: [{troop: one, open: 'true'}]}\npersonas: [{id: a}]\n",
		"personas: [{id: a, policies: [{id: p, phase: night, try: [{item: potion}]}]}]\n",
		"", "[", "personas: 3\n", "personas: [{id: ''}]\n", "game: {days: -1}\npersonas: [{id: a}]\n",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		cfg, err := Load(fstest.MapFS{"designeval.yaml": {Data: []byte(text)}}, "designeval.yaml")
		if err != nil {
			return
		}
		db, err := testgame.TryLoad(nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := cfg.Validate(db); err != nil {
			return
		}
		cfg.Game.Days = min(max(cfg.Game.Days, 1), 2)
		for i := range cfg.Personas {
			cfg.Personas[i].Days = min(cfg.Personas[i].Days, 2)
		}
		run := Play(db, cfg, cfg.Personas[0])
		if run == nil || run.Days < 0 {
			t.Fatalf("a run that was played: %+v", run)
		}
		if s := Summarize(run, cfg.Release); s.ID != cfg.Personas[0].ID {
			t.Fatalf("the summary is of the persona played: %+v", s)
		}
	})
}
