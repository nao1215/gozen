package sound

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io/fs"
	"math"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/testgame"
)

func TestBuiltInEffectsAreAllMadeAndBounded(t *testing.T) {
	made := synthEffects()
	for _, name := range Effects {
		b, ok := made[name]
		if !ok {
			t.Errorf("effect %q is listed but not made", name)
			continue
		}
		if len(b) == 0 || len(b)%8 != 0 {
			t.Errorf("effect %q has %d bytes, want whole stereo float32 frames", name, len(b))
			continue
		}
		for i := 0; i+4 <= len(b); i += 4 {
			v := math.Float32frombits(binary.LittleEndian.Uint32(b[i:]))
			if v < -1 || v > 1 || v != v {
				t.Errorf("effect %q sample %d out of range: %v", name, i/4, v)
				break
			}
		}
	}
	if len(made) != len(Effects) {
		t.Errorf("made %d effects, listed %d", len(made), len(Effects))
	}
}

func TestBuiltInEffectsAreDeterministic(t *testing.T) {
	a, b := synthEffects(), synthEffects()
	for _, name := range Effects {
		if !bytes.Equal(a[name], b[name]) {
			t.Errorf("effect %q differs between runs", name)
		}
	}
}

func TestEffectEnvelopes(t *testing.T) {
	if env(0, 0.1, 1) != 0 || env(0.05, 0.1, 1) != 0.5 || env(0.1, 0.1, 1) != 1 {
		t.Fatal("attack ramp")
	}
	if got := env(1.1, 0.1, 1); math.Abs(got-math.Exp(-1)) > 1e-12 {
		t.Fatalf("decay %v", got)
	}
	if square(0.1, 1) != 1 || square(0.6, 1) != -1 {
		t.Fatal("square")
	}
	if step(0) != 1 || step(-1e-9) != 0 {
		t.Fatal("step")
	}
	if math.Abs(tone(0.25, 1)-1) > 1e-12 {
		t.Fatal("tone")
	}
	// Notes start one gap apart, so before the second note only the first sounds.
	f := arpeggio([]float64{440, 660}, 0.1, 0.2, 1)
	if f(-0.01) != 0 {
		t.Fatal("arpeggio sounds before it starts")
	}
}

func TestSynthClampsToUnitRange(t *testing.T) {
	b := synth(0.002, func(float64) float64 { return 5 })
	if len(b) != 88*8 {
		t.Fatalf("length %d", len(b))
	}
	if v := math.Float32frombits(binary.LittleEndian.Uint32(b)); v != 1 {
		t.Fatalf("clamped to %v", v)
	}
	if l, r := binary.LittleEndian.Uint32(b), binary.LittleEndian.Uint32(b[4:]); l != r {
		t.Fatal("effects are dual mono")
	}
}

func TestMakeEffectsGameFilesWin(t *testing.T) {
	freshSession(t)
	custom := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	mu.Lock()
	effects["cursor"] = custom
	mu.Unlock()
	makeEffects(false)
	mu.Lock()
	got, other := effects["cursor"], effects["decide"]
	mu.Unlock()
	if !bytes.Equal(got, custom) {
		t.Fatal("a game's own effect was replaced by the built-in one")
	}
	if len(other) == 0 {
		t.Fatal("built-in effects missing")
	}
}

func TestMakeEffectsInBackground(t *testing.T) {
	freshSession(t)
	makeEffects(true)
	waitFor(t, "background effects", func() bool { return EffectSeconds("fanfare") > 1 })
}

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

func stereo(vals ...float32) []byte {
	b := make([]byte, len(vals)*8)
	for i, v := range vals {
		binary.LittleEndian.PutUint32(b[i*8:], math.Float32bits(v))
		binary.LittleEndian.PutUint32(b[i*8+4:], math.Float32bits(-v))
	}
	return b
}

func TestResample(t *testing.T) {
	src := stereo(0, 1, 2, 3)
	if got := resample(src, 1); &got[0] != &src[0] {
		t.Fatal("rate 1 must return the input")
	}
	if got := resample(src, 1+1e-6); &got[0] != &src[0] {
		t.Fatal("an inaudible change must return the input")
	}
	if got := resample(src, 0); &got[0] != &src[0] {
		t.Fatal("non-positive rate must return the input")
	}
	one := stereo(1)
	if got := resample(one, 2); &got[0] != &one[0] {
		t.Fatal("a single frame cannot be resampled")
	}
	// Twice as fast: every other frame, half as long.
	fast := resample(src, 2)
	if len(fast)/8 != 2 {
		t.Fatalf("fast frames %d", len(fast)/8)
	}
	if l := math.Float32frombits(binary.LittleEndian.Uint32(fast[8:])); l != 2 {
		t.Fatalf("second frame left %v", l)
	}
	// Half speed: new frames between the old ones are interpolated; the channels stay mirrored.
	slow := resample(src, 0.5)
	if len(slow)/8 != 8 {
		t.Fatalf("slow frames %d", len(slow)/8)
	}
	l := math.Float32frombits(binary.LittleEndian.Uint32(slow[8:]))
	r := math.Float32frombits(binary.LittleEndian.Uint32(slow[12:]))
	if l != 0.5 || r != -0.5 {
		t.Fatalf("interpolated frame %v %v", l, r)
	}
}

func TestVaryOnlyChangesQuickRepeats(t *testing.T) {
	freshSession(t)
	t.Cleanup(func() { SetVary(0.03, 0.10) })
	src := stereo(0, 1, 2, 3, 4, 5, 6, 7, 8, 9)
	SetVary(0.2, 0.1)
	b, k := vary("a", src)
	if k != 1 || len(b) != len(src) {
		t.Fatal("first play must be untouched")
	}
	changed := false
	for range 20 {
		b, k = vary("a", src)
		if k < 0.9-1e-9 || k > 1.1+1e-9 {
			t.Fatalf("volume factor %v out of range", k)
		}
		if len(b) != len(src) {
			changed = true
		}
	}
	if !changed {
		t.Fatal("repeated plays never varied")
	}
	if b, k := vary("other", src); k != 1 || len(b) != len(src) {
		t.Fatal("another effect's first play must be untouched")
	}
	SetVary(-1, -1)
	if b, k := vary("a", src); k != 1 || len(b) != len(src) {
		t.Fatal("variation disabled must keep the effect as is")
	}
	if varyPitch != 0 || varyVolume != 0 {
		t.Fatal("negative variation is clamped to zero")
	}
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
	if VoicePlaying() {
		t.Fatal("nothing plays after Close")
	}
}

func TestClipLoadRejectsBadInput(t *testing.T) {
	if _, err := Load(nil, "a.wav"); err == nil {
		t.Fatal("nil FS accepted")
	}
	for _, name := range []string{"../a.wav", "/a.wav", "", "a//b.wav"} {
		if _, err := Load(fstest.MapFS{}, name); err == nil {
			t.Errorf("invalid path %q accepted", name)
		}
	}
	c, err := Load(fstest.MapFS{"UP.WAV": {Data: testgame.Wav(0.01)}}, "UP.WAV")
	if err != nil || c == nil {
		t.Fatalf("upper-case extension: %v", err)
	}
}
