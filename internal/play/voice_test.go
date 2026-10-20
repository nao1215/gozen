package play

import (
	"bytes"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/testgame"
)

const voicedData = `actors: {teller: {name: Teller, class: ranger, voices: teller/voice}}
events:
  talk:
    do:
      - say: {who: teller, voice: hi, text: "Hi."}
      - say: {who: teller, voice: hi, text: "Again."}
      - say: "Done."
`

func voicedGame(t *testing.T, opts Options) *Game {
	t.Helper()
	fsys := gameFiles(t)
	fsys["game.yaml"] = &fstest.MapFile{Data: append([]byte("schema_version: 4\n"), fsys["game.yaml"].Data...)}
	fsys["data/voiced.yaml"] = &fstest.MapFile{Data: []byte(voicedData)}
	fsys["assets/teller/voice/hi.wav"] = &fstest.MapFile{Data: testgame.Wav(0.5)}
	opts.FS, opts.NoSound, opts.SaveDir, opts.ShotDir = fsys, true, t.TempDir(), t.TempDir()
	g, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	return g
}

// TestLineVoice: a line's voice plays when it opens (the trace has it with its length), auto
// waits for it, and the settings show the voice rows.
func TestLineVoice(t *testing.T) {
	g := voicedGame(t, Options{Location: "camp", Event: "talk", Script: "wait 10\nquit\n"})
	var out bytes.Buffer
	g.tr = newTracer(&out)
	runToEnd(t, g)
	v := traceLines(t, out.Bytes(), "voice")
	if len(v) != 1 || v[0]["file"] != "teller/voice/hi.wav" || v[0]["who"] != "teller" || v[0]["ticks"] != 30.0 {
		t.Fatalf("voice lines %v", v)
	}
	g.autoRead = true
	g.talk.full, g.talk.age = 10000, 10
	if g.autoAdvance() {
		t.Fatal("auto did not wait for the voice")
	}
	g.talk.age = g.talk.voiceT + 20
	if !g.autoAdvance() {
		t.Fatal("auto waited after the voice")
	}
	s := newConfigScreen(g)
	if s.voiceRow < 0 || len(s.voiced) != 1 || s.voiced[0] != "teller" {
		t.Fatalf("voice rows at %d for %v", s.voiceRow, s.voiced)
	}
	s.list.sel = s.voiceRow + 1
	s.change(g, -1, false)
	if g.cfg.VoiceBy["teller"] != 9 || g.voiceGain("teller") != 0.9 {
		t.Fatalf("the teller's volume %v", g.cfg.VoiceBy)
	}
}

// TestSkipPassesVoices: skip mode passes the voices of the lines it passes.
func TestSkipPassesVoices(t *testing.T) {
	g := voicedGame(t, Options{Location: "camp", Event: "talk", Script: "wait 10\npress skip\nwait 60\nquit\n"})
	g.read = map[uint64]bool{event.SourceKey("teller", "Hi."): true, event.SourceKey("teller", "Again."): true}
	var out bytes.Buffer
	g.tr = newTracer(&out)
	runToEnd(t, g)
	if v := traceLines(t, out.Bytes(), "voice"); len(v) != 1 {
		t.Fatalf("voice lines %v", v)
	}
	if at(g) != "Done." {
		t.Fatalf("at %q", at(g))
	}
}
