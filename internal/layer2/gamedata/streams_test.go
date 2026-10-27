package gamedata_test

import (
	"fmt"
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
		warned = warned || strings.Contains(w.Error(), "less than 10")
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
		{"an unknown host of a duo", edit("data/streams.yaml", "host: ann, with: bo", "host: cy, with: bo"), `unknown channel "cy"`},
		{"a null channel", edit("data/streams.yaml", "channels:\n", "channels:\n  - null\n"), "a channel needs an id"},
		{"an unknown face", edit("streams/bo/1.yaml", "face: smile", "face: grin"), `has no face "grin"`},
		{"one option", edit("streams/bo/1.yaml", ", {c: Peas, r: [Peas it is.]}", ""), "want 2 to 4 options, not 1"},
		{"an answer out of range", edit("streams/bo/1.yaml", "[sprout, 1]", "[sprout, 2]"), "option 2 is not one of the 2 options"},
		{"a speaker who is no host", edit("streams/bo/1.yaml", "say: Hello", "ann: Hello"), `"ann" is not a host`},
		{"an unknown field", edit("data/streams.yaml", "{days} days", "{weeks} days"), "unknown {weeks}"},
		{"an episode outside streams/", edit("data/streams.yaml", "episodes: [streams/bo/1.yaml]", "episodes: [shows/bo/1.yaml]"), "under streams/"},
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
		{"a logo missing", edit("data/streams.yaml", "tips:", "logo: gone.png\ntips:"), "no file assets/gone.png"},
		{"a slot of no minutes", edit("data/streams.yaml", "tips:", "slot: {minutes: -5}\ntips:"), "slot.minutes"},
		{"a negative rest", edit("data/streams.yaml", "tips:", "slot: {minutes: 60, rest: -1}\ntips:"), "slot.rest"},
		{"a slot over a day", edit("data/streams.yaml", "tips:", "slot: {minutes: 1500}\ntips:"), "at most 1440 minutes"},
		{"a family that is no bool", edit("streams/bo/1.yaml", "title: Garden tour", "title: Garden tour\nfamily: maybe"), "Family of type bool"},
		{"an unlock below 0", edit("data/streams.yaml", "host: ann, with: bo,", "host: ann, with: bo, unlock: -1,"), "duos[0].unlock"},
		{"a guest with an episode", edit("data/streams.yaml", "  - id: bo\n", "  - id: bo\n    guest: true\n"), "a guest has no episodes"},
		{"a bridge with a field", edit("data/streams.yaml", "first: [", "bridges: [\"Back, {from}.\"]\n    first: ["), "unknown {from}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := testgame.TryLoad(tc.files)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

// TestStreamsVoices: a line's voice is named by the SHA-1 of the line as written, and the
// check warns of a missing voices folder, of what in it is not a channel's folder and of a
// file that is the voice of no line of its channel, but not of the voices of its lines.
func TestStreamsVoices(t *testing.T) {
	if got := data.VoiceKey("Hello, everyone."); got != "6f2529aa52538b73" {
		t.Fatalf("voice key %q", got)
	}
	warnings := func(files map[string]string) string {
		var b strings.Builder
		for _, w := range testgame.Load(t, files).SceneWarnings() {
			b.WriteString(w.Error() + "\n")
		}
		return b.String()
	}
	g := streamGame()
	g["data/streams.yaml"] = "voices: voice\n" + g["data/streams.yaml"]
	if w := warnings(g); !strings.Contains(w, "GZN0119") || !strings.Contains(w, "no folder assets/voice") {
		t.Errorf("a missing voices folder is not warned of:\n%s", w)
	}
	// an Ogg page's header, 27 bytes and a segment table of one segment, then the packet
	ogg := func(packet string) string { return "OggS" + strings.Repeat("\x00", 22) + "\x01\x1e" + packet }
	g["assets/voice/ann/6f2529aa52538b73.ogg"] = ogg("\x01vorbis") // Hello, everyone.
	g["assets/voice/ann/70624a7e70244116.ogg"] = ogg("OpusHead")   // Welcome, {from}.
	g["assets/voice/bo/6f2529aa52538b73.ogg"] = "RIFF....WAVE"     // in the duo too
	g["assets/voice/ann/0000000000000000.ogg"] = "x"               // no line
	g["assets/voice/cy/6f2529aa52538b73.ogg"] = "x"                // no channel
	w := warnings(g)
	for _, want := range []string{"assets/voice/ann/0000000000000000.ogg is the voice of no line of ann", "assets/voice/cy is not the folder of a channel"} {
		if !strings.Contains(w, want) {
			t.Errorf("no warning %q:\n%s", want, w)
		}
	}
	if strings.Count(w, "GZN0119") != 2 {
		t.Errorf("the voices of lines are warned of:\n%s", w)
	}
	// gozen reads Ogg Vorbis: gozen check warns of a voice of a line in Opus or WAV, not of
	// a Vorbis one; loading the game reads no voice
	if strings.Contains(w, "GZN0120") {
		t.Errorf("loading the game reads the voices:\n%s", w)
	}
	w = fmt.Sprint(testgame.Load(t, g).VoiceWarnings())
	for _, want := range []string{"assets/voice/ann/70624a7e70244116.ogg is Ogg Opus", "assets/voice/bo/6f2529aa52538b73.ogg is not an Ogg file"} {
		if !strings.Contains(w, want) {
			t.Errorf("no warning %q:\n%s", want, w)
		}
	}
	if strings.Count(w, "GZN0120") != 2 {
		t.Errorf("not 2 voices warned of as unreadable:\n%s", w)
	}
	if s := testgame.Load(t, g).Streams; s.Voice("ann", "Hello, everyone.") != "assets/voice/ann/6f2529aa52538b73.ogg" || s.Voice("ann", "") != "" {
		t.Errorf("voice file %q", s.Voice("ann", "Hello, everyone."))
	}
}

// TestStreamsGuest: a guest is a channel with no stream of its own that streams in a duo
// with its host only: it loads with no episodes and no room, and the check refuses a guest
// with episodes or a duo a guest hosts.
func TestStreamsGuest(t *testing.T) {
	guest := func(duo string) map[string]string {
		g := streamGame()
		g["data/streams.yaml"] = strings.NewReplacer("    room: x.png\n    frame: {cx: 100, top: 0, bottom: 200}\n    episodes: [streams/bo/1.yaml]\n", "    guest: true\n    frame: {cx: 100, top: 0, bottom: 200}\n", "host: ann, with: bo", duo).Replace(g["data/streams.yaml"])
		return g
	}
	db, err := testgame.TryLoad(guest("host: ann, with: bo"))
	if err != nil {
		t.Fatal(err)
	}
	if bo := db.Streams.Channel("bo"); bo == nil || !bo.Guest || len(bo.Shows) != 0 {
		t.Fatalf("the guest %+v", bo)
	}
	if _, err := testgame.TryLoad(guest("host: bo, with: ann")); err == nil || !strings.Contains(err.Error(), "a guest hosts no duo") {
		t.Fatalf("a duo hosted by a guest: %v", err)
	}
}
