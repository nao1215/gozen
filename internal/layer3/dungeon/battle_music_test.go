package dungeon_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/testgame"
)

// Music for ordinary fights per floor band (the place's battle_music and the looks' battle_music)
const bandMusic = `
music:
  songs:
    fast: {file: a.ogg}
    west: {file: a.ogg}
    neo:  {file: a.ogg}
    big:  {file: a.ogg}
troops:
  guard: {members: [mote], music: big}
locations:
  deep:
    name: Deep
    battle_music: fast
    dungeon:
      map: "#S#"
      tiles: {S: {start: true}}
      looks:
        - {when: "var.floor >= 11", battle_music: neo}
        - {when: "var.floor >= 6", battle_music: west}
        - {when: "var.floor == 99"}
`

func TestBattleMusicByFloor(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/band.yaml": bandMusic, "assets/a.ogg": "ogg"})
	st := testgame.State(db, 1)
	st.Location = "deep"
	for floor, want := range map[float64]string{1: "fast", 3: "fast", 6: "west", 7: "west", 10: "west", 11: "neo", 12: "neo", 99: "neo"} {
		st.Vars["floor"] = floor
		if got := dungeon.BattleMusic(db, st, db.Troops["one"]).Song; got != want {
			t.Errorf("B%v: %q, want %q", floor, got, want)
		}
		// the enemy group's music (the guardian) comes first
		if got := dungeon.BattleMusic(db, st, db.Troops["guard"]).Song; got != "big" {
			t.Errorf("B%v guard: %q, want big", floor, got)
		}
	}
	// Outside a dungeon, game.yaml's battle.music (none here)
	st.Location = "camp"
	if got := dungeon.BattleMusic(db, st, db.Troops["one"]).Song; got != "" {
		t.Errorf("camp: %q, want the game's default (none)", got)
	}
}

func TestBattleMusicMustBeASong(t *testing.T) {
	for name, src := range map[string]string{
		"location": "locations: {m: {name: M, battle_music: nope, dungeon: {map: \"#S#\", tiles: {S: {start: true}}}}}",
		"look":     "locations: {m: {name: M, dungeon: {map: \"#S#\", tiles: {S: {start: true}}, looks: [{when: \"true\", battle_music: nope}]}}}",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := testgame.TryLoad(map[string]string{"data/z.yaml": src})
			if err == nil || !strings.Contains(err.Error(), "battle_music") || !strings.Contains(err.Error(), `unknown song "nope"`) {
				t.Fatalf("want an unknown song at battle_music, got %v", err)
			}
		})
	}
}
