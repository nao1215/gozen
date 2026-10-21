package dungeon_test

import (
	"image/color"
	"testing"

	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/testgame"
)

const looksGame = `
music:
  songs: {night: {file: x.png}, day: {file: x.png}, fight: {file: x.png}}
locations:
  hall:
    name: Hall
    encounters: {rate: "0.2", gauge: 1, troops: [{troop: one}]}
    dungeon:
      darkness: 0.3
      floor: "#101010"
      pictures: {a.png: b.png}
      map: |
        #########
        #S.....>#
        #########
      tiles:
        S: {start: true, facing: east}
        ">": {name: Stairs, solid: "true"}
      looks:
        - when: "flag.night"
          wall: {file: x.png}
          door: {file: x.png}
          floor: "#202020"
          ceiling: "#303030"
          floor_texture: {file: x.png}
          ceiling_texture: {file: x.png}
          darkness: 0
          no_map: true
          pictures: {c.png: d.png}
          battle_background: {file: x.png}
          music: {song: night}
          battle_music: {song: fight}
        - when: "flag.day"
          music: {song: day}
`

func TestStyledOverlaysTheFirstHoldingLook(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/hall.yaml": looksGame, "assets/a.png": "x", "assets/b.png": "x", "assets/d.png": "x", "assets/c.png": "x"})
	st := testgame.State(db, 1)
	st.Location = "hall"
	base := dungeon.Styled(db, st)
	if base != dungeon.Of(db, st) {
		t.Error("with no look holding the dungeon itself is returned")
	}
	if got := dungeon.Music(db, st); got.Song != "" {
		t.Errorf("no look holds: no music, got %q", got.Song)
	}
	if got := dungeon.BattleBG(db, st); got.File != "" {
		t.Errorf("no look holds: no battle background, got %q", got.File)
	}
	st.Flags["night"] = true
	styled := dungeon.Styled(db, st)
	orig := dungeon.Of(db, st)
	if styled == orig {
		t.Fatal("a holding look is laid over a copy")
	}
	want := color.NRGBA{0x20, 0x20, 0x20, 255}
	if styled.Wall.File != "x.png" || styled.Door.File != "x.png" || styled.Floor.NRGBA != want || styled.Ceiling.NRGBA != (color.NRGBA{0x30, 0x30, 0x30, 255}) {
		t.Errorf("look: wall %q door %q floor %+v ceiling %+v", styled.Wall.File, styled.Door.File, styled.Floor, styled.Ceiling)
	}
	if styled.FloorTex.File != "x.png" || styled.CeilingTex.File != "x.png" || styled.Dark != 0 || !styled.NoMap {
		t.Errorf("textures %q %q dark %v nomap %v", styled.FloorTex.File, styled.CeilingTex.File, styled.Dark, styled.NoMap)
	}
	if styled.Pictures["a.png"] != "b.png" || styled.Pictures["c.png"] != "d.png" {
		t.Errorf("a look's pictures are layered over the dungeon's: %v", styled.Pictures)
	}
	if orig.Pictures["c.png"] != "" || orig.Dark != 0.3 || orig.NoMap {
		t.Error("styling changed the dungeon itself")
	}
	if got := dungeon.Music(db, st); got.Song != "night" {
		t.Errorf("music %q", got.Song)
	}
	if got := dungeon.BattleBG(db, st); got.File != "x.png" {
		t.Errorf("battle background %q", got.File)
	}
	// the first look that holds wins; a look that leaves things empty leaves them as they were
	st.Flags["night"] = false
	st.Flags["day"] = true
	day := dungeon.Styled(db, st)
	if day.Wall.File != "" || day.Dark != 0.3 || day.Floor.NRGBA != (color.NRGBA{0x10, 0x10, 0x10, 255}) {
		t.Errorf("an empty look keeps the dungeon: wall %q dark %v floor %+v", day.Wall.File, day.Dark, day.Floor)
	}
	if got := dungeon.Music(db, st); got.Song != "day" {
		t.Errorf("music %q", got.Song)
	}
	if got := dungeon.BattleMusic(db, st, nil); got.Song != "" && got.Song == "fight" {
		t.Errorf("the day look has no battle music of its own, got %q", got.Song)
	}
	st.Flags["day"] = false
	st.Flags["night"] = true
	if got := dungeon.BattleMusic(db, st, nil); got.Song != "fight" {
		t.Errorf("the night look's battle music: %q", got.Song)
	}
	// away from a dungeon there is nothing to style
	st.Location = "camp"
	if dungeon.Styled(db, st) != nil || dungeon.Music(db, st).Song != "" || dungeon.BattleBG(db, st).File != "" {
		t.Error("a place that is not a dungeon has no look")
	}
}

// TestEncounterGauge: with a gauge, every step fills it by the rate; the chance is the fill times the gauge, and a battle empties it.
func TestEncounterGauge(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/hall.yaml": looksGame, "assets/a.png": "x", "assets/b.png": "x", "assets/d.png": "x", "assets/c.png": "x"})
	st := testgame.State(db, 5)
	st.Location = "hall"
	dungeon.Place(db, st, "hall", "", "")
	if dungeon.Gauge(st) != 0 {
		t.Error("the gauge starts empty")
	}
	fought := 0
	last := 0.0
	for range 40 {
		moved, cmds := dungeon.Forward(db, st)
		if !moved {
			dungeon.Turn(st, 2)
			continue
		}
		g := dungeon.Gauge(st)
		if len(cmds) > 0 && cmds[0].Battle != "" {
			fought++
			if g != 0 {
				t.Errorf("a battle empties the gauge, it holds %v", g)
			}
		} else if g < last {
			t.Errorf("without a battle the gauge only fills: %v after %v", g, last)
		}
		last = g
	}
	if fought == 0 {
		t.Error("a gauge that fills to its full chance meets a battle within forty steps")
	}
	st.Calm = true
	st.Vars["gauge"] = 0.0
	for range 10 {
		if _, cmds := dungeon.Forward(db, st); len(cmds) > 0 && cmds[0].Battle != "" {
			t.Fatal("a calm party meets no battles")
		}
		dungeon.Turn(st, 2)
	}
	if dungeon.Gauge(st) != 0 {
		t.Error("a calm party does not fill the gauge")
	}
}
