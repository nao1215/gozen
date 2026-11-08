package sound

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/testgame"
)

func TestDecodeEffectWavAndOgg(t *testing.T) {
	wavPCM, err := DecodeEffect(testgame.Wav(0.1), ".wav")
	if err != nil {
		t.Fatal(err)
	}
	// 0.1 s mono at 44.1 kHz becomes stereo float32 frames.
	if frames := len(wavPCM) / 8; frames < 4300 || frames > 4500 {
		t.Fatalf("wav frames %d", frames)
	}
	oggPCM, err := DecodeEffect(readTone(t), ".ogg")
	if err != nil || len(oggPCM) == 0 {
		t.Fatalf("ogg: %v", err)
	}
	if _, err := DecodeEffect([]byte("junk"), ".wav"); err == nil {
		t.Fatal("bad wav decoded")
	}
	if _, err := DecodeEffect([]byte("junk"), ".ogg"); err == nil {
		t.Fatal("bad ogg decoded")
	}
}

func TestLoadEffectsFromGameFiles(t *testing.T) {
	freshSession(t)
	logs := captureLog(t)
	if LoadEffects(nil) != 0 || LoadEffects(fstest.MapFS{}) != 0 {
		t.Fatal("no files, no effects")
	}
	fsys := fstest.MapFS{
		"assets/sfx/ping.wav":   {Data: testgame.Wav(0.1)},
		"assets/sfx/Pong.OGG":   {Data: readTone(t)},
		"assets/sfx/bad.wav":    {Data: []byte("junk")},
		"assets/sfx/notes.txt":  {Data: []byte("not a sound")},
		"assets/sfx/sub/in.wav": {Data: testgame.Wav(0.1)},
	}
	if n := LoadEffects(fsys); n != 2 {
		t.Fatalf("loaded %d, want 2 (ping, Pong)", n)
	}
	if s := EffectSeconds("ping"); math.Abs(s-0.1) > 0.01 {
		t.Fatalf("ping lasts %v", s)
	}
	if EffectSeconds("Pong") == 0 {
		t.Fatal("upper-case extension skipped")
	}
	if EffectSeconds("bad") != 0 || EffectSeconds("notes") != 0 || EffectSeconds("in") != 0 {
		t.Fatal("broken, non-audio or nested files must not become effects")
	}
	if !strings.Contains(logs.String(), "sfx bad.wav") {
		t.Fatalf("decode failure not logged: %q", logs.String())
	}
}

func TestLoadEffectsReadFailureIsSkipped(t *testing.T) {
	freshSession(t)
	logs := captureLog(t)
	fsys := brokenReadFS{MapFS: fstest.MapFS{"assets/sfx/a.wav": {Data: testgame.Wav(0.05)}}}
	if n := LoadEffects(fsys); n != 0 {
		t.Fatalf("loaded %d from unreadable files", n)
	}
	if !strings.Contains(logs.String(), "sfx a.wav") {
		t.Fatalf("read failure not logged: %q", logs.String())
	}
}

// brokenReadFS lists files but fails to open them.
type brokenReadFS struct{ fstest.MapFS }

func (b brokenReadFS) Open(name string) (fs.File, error) {
	if name == "assets/sfx" || name == "." {
		return b.MapFS.Open(name)
	}
	return nil, fmt.Errorf("open %s: %w", name, fs.ErrPermission)
}

func (b brokenReadFS) ReadFile(name string) ([]byte, error) {
	return nil, fmt.Errorf("read %s: %w", name, fs.ErrPermission)
}

func TestEffectDurationsWithoutPlayback(t *testing.T) {
	freshSession(t)
	fsys := fstest.MapFS{
		"assets/sfx/decide.wav": {Data: testgame.Wav(0.37)},
		"assets/sfx/tone.ogg":   {Data: readTone(t)},
	}
	makeEffects(false)
	LoadEffects(fsys)
	want := map[string]float64{}
	for name, pcm := range effects {
		want[name] = ClipSeconds(pcm)
	}
	if err := Close(); err != nil {
		t.Fatal(err)
	}
	if n, err := Load(fsys, SfxDir); err != nil || n != 2 {
		t.Fatalf("loaded %d durations", n)
	}
	for name, seconds := range want {
		if got := EffectSeconds(name); got != seconds {
			t.Errorf("%s: silent %v, decoded %v", name, got, seconds)
		}
	}
	if len(effects) != 0 || len(effectPlayers) != 0 {
		t.Fatal("silent load created PCM or players")
	}
	if err := Close(); err != nil {
		t.Fatal(err)
	}
	if EffectSeconds("tone") != 0 || EffectSeconds("decide") == want["decide"] {
		t.Fatal("custom duration leaked across sessions")
	}
}

func TestLoadDurationDirectory(t *testing.T) {
	freshSession(t)
	fsys := fstest.MapFS{"clips/tone.wav": {Data: testgame.Wav(0.25)}}
	if n, err := Load(fsys, "clips"); err != nil || n != 1 {
		t.Fatalf("load clips: count=%d, error=%v", n, err)
	}
	if math.Abs(EffectSeconds("tone")-0.25) > 0.01 || len(effects) != 0 {
		t.Fatal("directory load changed duration or created PCM")
	}
	if _, err := Load(fsys, "../clips"); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("invalid directory: %v", err)
	}
	if n, err := Load(fsys, "clips/tone.wav"); err == nil || n != 0 {
		t.Fatalf("file used as directory: count=%d, error=%v", n, err)
	}
	if n, err := Load(fsys, "missing"); err != nil || n != 0 {
		t.Fatalf("optional directory: count=%d, error=%v", n, err)
	}
}

func TestReloadEffectsDropsRemovedFiles(t *testing.T) {
	for _, lengthsOnly := range []bool{true, false} {
		name := "durations"
		if !lengthsOnly {
			name = "playback"
		}
		t.Run(name, func(t *testing.T) {
			freshSession(t)
			captureLog(t)
			if !lengthsOnly {
				makeEffects(false)
			}
			builtin := EffectSeconds("decide")
			for _, replacement := range []fs.FS{nil, fstest.MapFS{}, fstest.MapFS{
				"assets/sfx/decide.wav": {Data: []byte("invalid")},
			}} {
				fsys := fstest.MapFS{
					"assets/sfx/custom.wav": {Data: testgame.Wav(0.75)},
					"assets/sfx/decide.wav": {Data: testgame.Wav(0.75)},
				}
				if n, err := loadEffects(fsys, SfxDir, lengthsOnly); err != nil || n != 2 {
					t.Fatalf("loaded %d effects", n)
				}
				if n, err := loadEffects(replacement, SfxDir, lengthsOnly); err != nil || n != 0 {
					t.Fatalf("loaded %d removed effects", n)
				}
				if EffectSeconds("custom") != 0 || EffectSeconds("decide") != builtin {
					t.Fatal("removed custom duration survived reload")
				}
				if _, ok := effects["custom"]; ok {
					t.Fatal("removed PCM survived reload")
				}
				if !lengthsOnly && ClipSeconds(effects["decide"]) != builtin {
					t.Fatal("removed override did not restore built-in PCM")
				}
				if lengthsOnly && (len(effects) != 0 || len(effectPlayers) != 0) {
					t.Fatal("duration reload created PCM or players")
				}
			}
		})
	}
}
