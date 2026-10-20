package gamedata_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

// voiced is a game with an actor whose voices are in a folder, and a voice by path.
func voiced(head, do string) map[string]string {
	return map[string]string{
		"game.yaml":                  head + testgame.Game,
		"assets/teller/voice/hi.wav": string(testgame.Wav(0.1)),
		"assets/voice/stranger.ogg":  "a stub",
		"data/z.yaml":                "actors: {teller: {name: Teller, class: ranger, voices: teller/voice}}\nevents: {e: {do: " + do + "}}",
	}
}

// TestVoiceWords: say.voice is found in the actor's voices (the extension may be left out)
// or as a path; a missing file, another kind of sound and the words in version 3 are errors;
// the lines of a voiced actor without a voice are counted in a warning (ADR 0028).
func TestVoiceWords(t *testing.T) {
	v4 := "schema_version: 4\nid: b\n"
	for _, c := range []struct{ head, do, want string }{
		{v4, `[{say: {who: teller, voice: hi, text: Hi.}}, {say: {who: teller, voice: hi.wav, text: Hi.}}, {say: {who: Stranger, voice: voice/stranger.ogg, text: Yes.}}]`, ""},
		{v4, `[{say: {who: teller, voice: gone, text: Hi.}}]`, "no voice assets/teller/voice/gone"},
		{v4, `[{say: {who: teller, voice: hi.mp3, text: Hi.}}]`, "a voice is .ogg or .wav"},
		{"schema_version: 3\nid: b\n", `[{say: {who: Stranger, voice: voice/stranger.ogg, text: Yes.}}]`, "voice needs schema_version: 4"},
	} {
		_, err := testgame.TryLoad(voiced(c.head, c.do))
		if c.want == "" && err != nil || c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%s: %v", c.do, err)
		}
	}
	files := voiced(v4, `[{say: {who: teller, voice: hi, text: Hi.}}, {say: {who: teller, text: Bye.}}, {if: "true", then: [{say: {who: teller, text: Again.}}]}]`)
	db := testgame.Load(t, files)
	w := errors.Join(db.SceneWarnings()...)
	if w == nil || !strings.Contains(w.Error(), "2 of 3 lines of teller have no voice: events.e.do[1].say, events.e.do[2].then[0].say") {
		t.Errorf("warning %v", w)
	}
	if p, ok := db.VoicePath("teller", "hi"); !ok || p != "teller/voice/hi.wav" {
		t.Errorf("voice path %q %v", p, ok)
	}
	files["data/z.yaml"] = strings.Replace(files["data/z.yaml"], "voices: teller/voice", "voices: teller/none", 1)
	if _, err := testgame.TryLoad(files); err == nil || !strings.Contains(err.Error(), "no folder assets/teller/none") {
		t.Errorf("a missing folder: %v", err)
	}
}
