package gozen_test

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestVoiceIsAnEffect: a line's voice reaches a session as the effect voice with its file,
// before the line, and the paths of Explore have it at the line's place (ADR 0028).
func TestVoiceIsAnEffect(t *testing.T) {
	fsys := fstest.MapFS{
		"game.yaml":                  {Data: []byte("schema_version: 4\nid: voiced\n" + testgame.Game)},
		"data/main.yaml":             {Data: []byte(testgame.Data)},
		"assets/x.png":               {Data: []byte("a stub")},
		"assets/teller/voice/hi.wav": {Data: testgame.Wav(0.1)},
		"data/voiced.yaml": {Data: []byte(`actors: {teller: {name: Teller, class: ranger, voices: teller/voice}}
events:
  talk:
    do:
      - say: "Quiet."
      - say: {who: teller, voice: hi, text: "Hi."}
`)},
	}
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	s, err := gozen.New(g, gozen.Config{
		Seed:      1,
		OnEffect:  func(e gozen.Effect) { order = append(order, e.Kind+":"+e.Name) },
		OnRequest: func(r gozen.Request) { order = append(order, r.Kind+":"+r.Text) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Do(context.Background(), "- call: talk"); err != nil {
		t.Fatal(err)
	}
	want := []string{"say:Quiet.", "voice:teller/voice/hi.wav", "say:Hi."}
	if len(order) != len(want) {
		t.Fatalf("order %q", order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order %q, want %q", order, want)
		}
	}
	paths, err := gozen.Explore(context.Background(), s, "- call: talk", gozen.ExploreOptions{})
	if err != nil || len(paths) != 1 {
		t.Fatalf("%d paths: %v", len(paths), err)
	}
	var at []int
	for _, e := range paths[0].Effects {
		if e.Kind == "voice" && e.Name == "teller/voice/hi.wav" {
			at = append(at, e.At)
		}
	}
	if len(at) != 1 || at[0] != 1 {
		t.Fatalf("voice at %v", at)
	}
}
