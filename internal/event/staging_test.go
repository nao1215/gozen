package event_test

import (
	"testing"

	"github.com/nao1215/gozen/internal/event"
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
