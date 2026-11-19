package event_test

import (
	"encoding/json"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/model/rpg"
	"github.com/nao1215/gozen/internal/rules/event"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestStagingEffects: stand is an effect with the actor's face picture resolved, an effect
// map carries its cue, a wait keeps whether skipping passes it, and an sfx that waits is a
// wait for that sound (ADR 0028).
func TestStagingEffects(t *testing.T) {
	db := testgame.Load(t, map[string]string{
		"game.yaml": "schema_version: 4\nid: b\n" + testgame.Game,
		"data/z.yaml": `actors: {teller: {name: Teller, class: ranger, faces: teller}}
events: {staged: {do: [
{stand: {who: teller, face: smile, at: right, enter: slide}},
{stand: {who: teller, at: left}},
{effect: {name: shake, who: teller, seconds: 0.5}},
{wait: {seconds: 0.25, skip: false}},
{sfx: {name: door, wait: true}},
{stand: none}
]}}`,
		"assets/teller/smile.png":  "a stub",
		"assets/teller/normal.png": "a stub",
	})
	var got []event.Effect
	r := event.New(db, testgame.State(db, 1), event.HostFunc(func(e event.Effect) { got = append(got, e) }))
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["staged"].Do)
	w, ok := r.Next().(event.Wait)
	if !ok || w.Seconds != 0.25 || !w.NoSkip {
		t.Fatalf("wait %+v", w)
	}
	s, ok := r.Next().(event.Wait)
	if !ok || s.Sound != "door" {
		t.Fatalf("sound wait %+v", s)
	}
	if r.Next() != nil {
		t.Fatal("more requests")
	}
	if len(got) != 5 {
		t.Fatalf("effects %+v", got)
	}
	in := testgame.As[event.StandEffect](t, got[0])
	if in.Who != "teller" || in.Face != "teller/smile.png" || in.At != 0.75 || in.Enter != "slide" || in.Default {
		t.Errorf("stand in %+v", in)
	}
	if moved := testgame.As[event.StandEffect](t, got[1]); moved.At != 0.25 || !moved.Default {
		t.Errorf("stand moved %+v", moved)
	}
	if v := testgame.As[event.VisualEffect](t, got[2]); v.Name != "shake" || v.Cue.Who != "teller" || v.Cue.Seconds != 0.5 {
		t.Errorf("effect %+v", v)
	}
	if snd := testgame.As[event.SoundEffect](t, got[3]); snd.Name != "door" {
		t.Errorf("sound %+v", snd)
	}
	if none := testgame.As[event.StandEffect](t, got[4]); !none.None {
		t.Errorf("stand none %+v", none)
	}
}

// TestWeatherAndToneLook: weather and tone change the state's look, which a save keeps,
// and tell the screen how long the change takes; none takes each away, and the look is
// gone when neither is left (ADR 0066).
func TestWeatherAndToneLook(t *testing.T) {
	db := testgame.Load(t, map[string]string{
		"game.yaml": "schema_version: 12\nid: b\n" + testgame.Game,
		"data/z.yaml": `events:
  storm: {do: [{weather: {kind: rain, power: 0.8, wind: 0.5, color: "#a0b0c0", seconds: 2}}, {tone: night}]}
  quiet: {do: [{weather: {kind: none, seconds: 0}}]}
  clear: {do: [{tone: none}]}`,
	})
	st := testgame.State(db, 1)
	var got []event.Effect
	run := func(id string) {
		t.Helper()
		got = nil
		r := event.New(db, st, event.HostFunc(func(e event.Effect) { got = append(got, e) }))
		r.Log = func(err error) { t.Error(err) }
		r.Start(db.Events[id].Do)
		for r.Next() != nil {
		}
	}
	run("storm")
	if st.Look == nil || st.Look.Weather == nil || *st.Look.Weather != (data.Weather{Kind: "rain", Power: 0.8, Wind: 0.5, Color: "#a0b0c0"}) {
		t.Fatalf("weather %+v", st.Look)
	}
	if st.Look.Tone == nil || !tonePreset(t, *st.Look.Tone, "night") {
		t.Fatalf("tone %+v", st.Look.Tone)
	}
	if len(got) != 2 {
		t.Fatalf("effects %+v", got)
	}
	if v := testgame.As[event.VisualEffect](t, got[0]); v.Name != "weather" || v.Cue.Seconds != 2 {
		t.Errorf("weather effect %+v", v)
	}
	if v := testgame.As[event.VisualEffect](t, got[1]); v.Name != "tone" || v.Cue.Seconds != 1 {
		t.Errorf("tone effect (a second when left out) %+v", v)
	}

	// A save keeps the look, and loading it gives the same look.
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	var back rpg.State
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Look == nil || *back.Look.Weather != *st.Look.Weather || *back.Look.Tone != *st.Look.Tone {
		t.Fatalf("saved look %s", b)
	}

	run("quiet")
	if st.Look == nil || st.Look.Weather != nil || st.Look.Tone == nil {
		t.Fatalf("weather none keeps the tone: %+v", st.Look)
	}
	if v := testgame.As[event.VisualEffect](t, got[0]); v.Cue.Seconds != 0 {
		t.Errorf("seconds: 0 is at once: %+v", v)
	}
	run("clear")
	if st.Look != nil {
		t.Fatalf("nothing left, the look stays: %+v", st.Look)
	}
	if b, err := json.Marshal(st); err != nil || strings.Contains(string(b), `"look"`) {
		t.Errorf("a save without a look writes one: %s", b)
	}
}

func tonePreset(t *testing.T, got data.Tone, word string) bool {
	t.Helper()
	want, ok := data.TonePreset(word)
	return ok && got == want
}
