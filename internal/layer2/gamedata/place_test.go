package gamedata

import "testing"

// イベントの 場所は 近い 順に 決まり、同じ 近さで 割れれば 決めない（EventPlaces）。
func TestEventPlaces(t *testing.T) {
	call := func(n string) Command { return Command{Call: n} }
	db := &DB{
		Locations: map[string]*Location{
			"bar":   {People: []Person{{Talk: []Command{{Then: []Command{call("chat")}}}}}},
			"room":  {Actions: []Action{{Do: []Command{call("night"), call("shared")}}}},
			"hall":  {Actions: []Action{{Do: []Command{{Choice: []ChoiceOption{{Do: []Command{call("shared")}}}}}}}},
			"vault": {Dungeon: &Dungeon{}},
		},
		Events: map[string]*Event{
			"chat":    {Do: []Command{call("deep")}},
			"deep":    {},
			"night":   {Do: []Command{call("shared")}}, // shared は 直に 2 か所から 呼ばれる
			"shared":  {},
			"walk":    {Trigger: "auto", Location: "vault", Do: []Command{call("lost")}},
			"lost":    {},
			"wrap":    {Do: []Command{call("lost")}}, // どこからも 呼ばれない 包み
			"nowhere": {},
		},
	}
	got := db.EventPlaces()
	want := map[string]string{"chat": "bar", "deep": "bar", "night": "room", "walk": "vault", "lost": "vault", "wrap": "vault"}
	for ev, l := range want {
		if got[ev] != l {
			t.Errorf("%s: got %q, want %q", ev, got[ev], l)
		}
	}
	for _, ev := range []string{"shared", "nowhere"} {
		if l, ok := got[ev]; ok {
			t.Errorf("%s: got %q, want none", ev, l)
		}
	}
}
