package sound

import (
	"math"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/testgame"
)

// TestVoiceClip: a voice is decoded from the game's files once and kept; its length is
// known without playing it; a file that is not there or not a sound is an error.
func TestVoiceClip(t *testing.T) {
	t.Cleanup(func() {
		mu.Lock()
		closeVoices()
		mu.Unlock()
	})
	fsys := fstest.MapFS{"assets/v/hi.wav": {Data: testgame.Wav(0.5)}, "assets/v/bad.wav": {Data: []byte("not a sound")}}
	pcm, err := VoiceClip(fsys, "assets/v/hi.wav")
	if err != nil {
		t.Fatal(err)
	}
	if s := ClipSeconds(pcm); math.Abs(s-0.5) > 0.01 {
		t.Fatalf("%v seconds", s)
	}
	delete(fsys, "assets/v/hi.wav")
	if again, err := VoiceClip(fsys, "assets/v/hi.wav"); err != nil || len(again) != len(pcm) {
		t.Fatalf("not kept: %v", err)
	}
	for _, f := range []string{"assets/v/none.wav", "assets/v/bad.wav"} {
		if _, err := VoiceClip(fsys, f); err == nil {
			t.Errorf("%s: no error", f)
		}
	}
	PlayVoice(pcm, 1) // without an audio context nothing plays, and nothing breaks
	if VoicePlaying() {
		t.Fatal("a voice plays without an audio context")
	}
	StopVoice()
}
