package gamedata_test

import (
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

// streamGame is testgame's stream game.
func streamGame() map[string]string {
	g := testgame.StreamFiles()
	g["game.yaml"] = testgame.Game + "id: streams-test\n" + testgame.StreamsGame
	return g
}

func TestStreamsLoad(t *testing.T) {
	db := testgame.Load(t, streamGame())
	s := db.Streams
	if s == nil || len(s.Channels) != 2 || len(s.Duos) != 1 {
		t.Fatalf("streams %+v", s)
	}
	beats := s.Channels[0].Shows[0].Beats()
	if len(beats) != 4 || beats[1].Line.Face != "smile" || beats[2].Chat[1] != "Beans!" || beats[3].Ask.NPC[0] != (data.NPCAnswer{Name: "sprout", Option: 1}) {
		t.Fatalf("beats %+v", beats)
	}
	if !s.Bounces("Beans!") || s.Bounces("Beans.") || s.MaxStep() != 2 || len(s.Crowd.Lines.At(7)) != 2 || len(s.Crowd.Lines.At(13)) != 1 {
		t.Errorf("bounce, steps or crowd lines: %+v", s)
	}
	if b := s.Duos[0].Shows[0].Beats()[4]; b.Speaker != "bo" || b.Line.Text != "My turn." {
		t.Fatalf("duo line %+v", b)
	}
	warned := false
	for _, w := range db.SceneWarnings() {
		warned = warned || strings.Contains(w.Error(), "less than 20")
	}
	if !warned {
		t.Error("a short episode is not warned of")
	}
	texts := map[string]bool{}
	for _, st := range db.SourceTexts() {
		texts[st.Text] = true
	}
	for _, want := range []string{"Garden tour", "Planting day", "Today we plant beans.", "Beans!", "Which seeds?", "Peas", "{from} picked beans.", "Gardening", "Seeds every week", "Hi!", "Good morning", "For seeds", "{from}, you came back."} {
		if !texts[want] {
			t.Errorf("%q is not a text to translate", want)
		}
	}
}

func TestStreamsCheck(t *testing.T) {
	edit := func(file, from, to string) map[string]string {
		g := streamGame()
		if !strings.Contains(g[file], from) {
			t.Fatalf("%s has no %q", file, from)
		}
		g[file] = strings.Replace(g[file], from, to, 1)
		return g
	}
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"an old schema", edit("game.yaml", "schema_version: 9", "schema_version: 8"), "streams needs schema_version: 9"},
		{"an unknown actor", edit("data/streams.yaml", "actor: bo", "actor: nobody"), `unknown actor "nobody"`},
		{"an unknown channel of a duo", edit("data/streams.yaml", "with: bo", "with: cy"), `unknown channel "cy"`},
		{"an unknown face", edit("streams/bo/1.yaml", "face: smile", "face: grin"), `has no face "grin"`},
		{"one option", edit("streams/bo/1.yaml", ", {c: Peas, r: [Peas it is.]}", ""), "want 2 to 4 options, not 1"},
		{"an answer out of range", edit("streams/bo/1.yaml", "[sprout, 1]", "[sprout, 2]"), "option 2 is not one of the 2 options"},
		{"a speaker who is no host", edit("streams/bo/1.yaml", "say: Hello", "ann: Hello"), `"ann" is not a host`},
		{"an unknown field", edit("data/streams.yaml", "{days} days", "{weeks} days"), "unknown {weeks}"},
		{"a missing episode", edit("data/streams.yaml", "streams/bo/1.yaml", "streams/bo/2.yaml"), "streams/bo/2.yaml"},
		{"a beat of two keys", edit("streams/bo/1.yaml", "- chat: [sprout, Beans!]", "- {chat: [a, b], say: c}"), "a beat is one of"},
		{"an unknown key", edit("data/streams.yaml", "tips:", "tipz: [1]\ntips:"), "tipz"},
		{"tips out of order", edit("data/streams.yaml", "tips: [10, 50]", "tips: [50, 10]"), "smallest first"},
		{"a live share above 1", edit("data/streams.yaml", "share: 0.8", "share: 1.5"), "from 0 to 1"},
		{"a lobby without streams", map[string]string{"game.yaml": testgame.Game + "schema_version: 9\nid: t\n", "data/z.yaml": "locations: {booth: {stream: lobby}}"}, "stream: lobby needs streams"},
		{"a stream that is no lobby", edit("data/cast.yaml", "stream: lobby", "stream: hall"), "want lobby"},
		{"open streams without streams", map[string]string{"game.yaml": testgame.Game + "schema_version: 9\nid: t\n", "data/z.yaml": "events: {e: {do: [{open: streams}]}}"}, "open: streams needs streams"},
		{"steps not from 0", edit("data/streams.yaml", "steps: [0, 10, 20]", "steps: [5, 10, 20]"), "steps start at 0"},
		{"an answer chance missing", edit("data/streams.yaml", "answer_chance: [0.5, 0.8, 1]", "answer_chance: [0.5]"), "for each of the 3 affection steps"},
		{"a broken bounce", edit("data/streams.yaml", `on: "!$"`, `on: "("`), "bounce.on"},
		{"too many greetings", edit("data/streams.yaml", `greet: [[], ["{from}, you came back."]]`, `greet: [[], [], [], []]`), "affection step 0 to 2"},
		{"a room missing", edit("data/streams.yaml", "room: x.png", "room: none.png"), "no file assets/none.png"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := testgame.TryLoad(tc.files)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}
