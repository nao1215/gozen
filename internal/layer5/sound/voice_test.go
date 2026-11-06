package sound

import (
	"fmt"
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
	if voicePlayer != nil {
		t.Fatal("a voice plays without an audio context")
	}
	StopVoice()
}

func TestVoiceCacheKeepsTheMostRecentlyUsed(t *testing.T) {
	freshSession(t)
	fsys := fstest.MapFS{}
	for i := range voiceCacheSize + 1 {
		fsys[fmt.Sprintf("v/%02d.wav", i)] = &fstest.MapFile{Data: testgame.Wav(0.01)}
	}
	for i := range voiceCacheSize {
		if _, err := VoiceClip(fsys, fmt.Sprintf("v/%02d.wav", i)); err != nil {
			t.Fatal(err)
		}
	}
	// Touch the oldest, then overflow: the second oldest must go, not the touched one.
	if _, err := VoiceClip(fsys, "v/00.wav"); err != nil {
		t.Fatal(err)
	}
	if _, err := VoiceClip(fsys, fmt.Sprintf("v/%02d.wav", voiceCacheSize)); err != nil {
		t.Fatal(err)
	}
	for name := range fsys {
		delete(fsys, name)
	}
	if _, err := VoiceClip(fsys, "v/00.wav"); err != nil {
		t.Fatalf("recently used voice dropped: %v", err)
	}
	if _, err := VoiceClip(fsys, "v/01.wav"); err == nil {
		t.Fatal("least recently used voice was kept")
	}
	if len(voiceCache) > voiceCacheSize {
		t.Fatalf("cache holds %d voices", len(voiceCache))
	}
}

func TestVoiceVolumeAndClose(t *testing.T) {
	freshSession(t)
	SetVoiceVolume(0.4)
	if voiceScale != 0.4 {
		t.Fatal("SetVoiceVolume")
	}
	if _, err := VoiceClip(fstest.MapFS{"a.wav": {Data: testgame.Wav(0.01)}}, "a.wav"); err != nil {
		t.Fatal(err)
	}
	if err := Close(); err != nil {
		t.Fatal(err)
	}
	if voiceScale != 1 || len(voiceCache) != 0 {
		t.Fatal("Close keeps the voice settings of the ended session")
	}
	if voicePlayer != nil {
		t.Fatal("nothing plays after Close")
	}
}
