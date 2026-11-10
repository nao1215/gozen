package sound

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
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
	if testSession.LoadEffects(nil) != 0 || testSession.LoadEffects(fstest.MapFS{}) != 0 {
		t.Fatal("no files, no effects")
	}
	fsys := fstest.MapFS{
		"assets/sfx/ping.wav":   {Data: testgame.Wav(0.1)},
		"assets/sfx/Pong.OGG":   {Data: readTone(t)},
		"assets/sfx/bad.wav":    {Data: []byte("junk")},
		"assets/sfx/notes.txt":  {Data: []byte("not a sound")},
		"assets/sfx/sub/in.wav": {Data: testgame.Wav(0.1)},
	}
	if n := testSession.LoadEffects(fsys); n != 2 {
		t.Fatalf("loaded %d, want 2 (ping, Pong)", n)
	}
	if s := testSession.EffectSeconds("ping"); math.Abs(s-0.1) > 0.01 {
		t.Fatalf("ping lasts %v", s)
	}
	if testSession.EffectSeconds("Pong") == 0 {
		t.Fatal("upper-case extension skipped")
	}
	if testSession.EffectSeconds("bad") != 0 || testSession.EffectSeconds("notes") != 0 || testSession.EffectSeconds("in") != 0 {
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
	if n := testSession.LoadEffects(fsys); n != 0 {
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
	testSession.makeEffects(false)
	testSession.LoadEffects(fsys)
	want := map[string]float64{}
	for name, pcm := range testSession.effects {
		want[name] = ClipSeconds(pcm)
	}
	if err := testSession.Close(); err != nil {
		t.Fatal(err)
	}
	if n, err := testSession.Load(fsys, SfxDir); err != nil || n != 2 {
		t.Fatalf("loaded %d durations", n)
	}
	for name, seconds := range want {
		if got := testSession.EffectSeconds(name); got != seconds {
			t.Errorf("%s: silent %v, decoded %v", name, got, seconds)
		}
	}
	if len(testSession.effects) != 0 || len(testSession.effectPlayers) != 0 {
		t.Fatal("silent load created PCM or players")
	}
	if err := testSession.Close(); err != nil {
		t.Fatal(err)
	}
	if testSession.EffectSeconds("tone") != 0 || testSession.EffectSeconds("decide") == want["decide"] {
		t.Fatal("custom duration leaked across sessions")
	}
}

func TestLoadDurationDirectory(t *testing.T) {
	freshSession(t)
	fsys := fstest.MapFS{"clips/tone.wav": {Data: testgame.Wav(0.25)}}
	if n, err := testSession.Load(fsys, "clips"); err != nil || n != 1 {
		t.Fatalf("load clips: count=%d, error=%v", n, err)
	}
	if math.Abs(testSession.EffectSeconds("tone")-0.25) > 0.01 || len(testSession.effects) != 0 {
		t.Fatal("directory load changed duration or created PCM")
	}
	if _, err := testSession.Load(fsys, "../clips"); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("invalid directory: %v", err)
	}
	if n, err := testSession.Load(fsys, "clips/tone.wav"); err == nil || n != 0 {
		t.Fatalf("file used as directory: count=%d, error=%v", n, err)
	}
	if n, err := testSession.Load(fsys, "missing"); err != nil || n != 0 {
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
				testSession.makeEffects(false)
			}
			builtin := testSession.EffectSeconds("decide")
			for _, replacement := range []fs.FS{nil, fstest.MapFS{}, fstest.MapFS{
				"assets/sfx/decide.wav": {Data: []byte("invalid")},
			}} {
				fsys := fstest.MapFS{
					"assets/sfx/custom.wav": {Data: testgame.Wav(0.75)},
					"assets/sfx/decide.wav": {Data: testgame.Wav(0.75)},
				}
				if n, err := testSession.loadEffects(fsys, SfxDir, lengthsOnly); err != nil || n != 2 {
					t.Fatalf("loaded %d effects", n)
				}
				if n, err := testSession.loadEffects(replacement, SfxDir, lengthsOnly); err != nil || n != 0 {
					t.Fatalf("loaded %d removed effects", n)
				}
				if testSession.EffectSeconds("custom") != 0 || testSession.EffectSeconds("decide") != builtin {
					t.Fatal("removed custom duration survived reload")
				}
				if _, ok := testSession.effects["custom"]; ok {
					t.Fatal("removed PCM survived reload")
				}
				if !lengthsOnly && ClipSeconds(testSession.effects["decide"]) != builtin {
					t.Fatal("removed override did not restore built-in PCM")
				}
				if lengthsOnly && (len(testSession.effects) != 0 || len(testSession.effectPlayers) != 0) {
					t.Fatal("duration reload created PCM or players")
				}
			}
		})
	}
}

// wavClaiming is an 8-bit 8 kHz mono WAV whose header claims n bytes of data
// (n/8000 seconds) and that holds only the bytes given.
func wavClaiming(n uint32, data []byte) []byte {
	const rate = 8000
	le := binary.LittleEndian
	b := []byte("RIFF")
	b = le.AppendUint32(b, 36+n)
	b = append(b, "WAVEfmt "...)
	b = le.AppendUint32(b, 16)
	b = le.AppendUint16(b, 1) // PCM
	b = le.AppendUint16(b, 1) // mono
	b = le.AppendUint32(b, rate)
	b = le.AppendUint32(b, rate)
	b = le.AppendUint16(b, 1)
	b = le.AppendUint16(b, 8)
	b = append(b, "data"...)
	b = le.AppendUint32(b, n)
	return append(b, data...)
}

// An effect or a voice is decoded whole into memory, so its length is capped:
// by the header before anything is decoded (a few bytes may claim hours), and by
// the bytes read when the header understates the data.
func TestDecodeEffectCapsTheClipLength(t *testing.T) {
	second := bytes.Repeat([]byte{128}, 8000)
	if pcm, err := DecodeEffect(wavClaiming(8000, second), ".wav"); err != nil || math.Abs(ClipSeconds(pcm)-1) > 0.01 {
		t.Fatalf("a one-second clip: %.2f s, %v", ClipSeconds(pcm), err)
	}
	for name, clip := range map[string][]byte{
		"wav of 61 s in 44 bytes":  wavClaiming(8000*(maxClipSeconds+1), nil),
		"wav of 131 s in 44 bytes": wavClaiming(1<<20, nil),
		"ogg of 61 s":              toneClaiming(t, SampleRate*(maxClipSeconds+1)),
		"ogg past any length":      toneClaiming(t, 1<<47),
	} {
		ext := "." + name[:3]
		if pcm, err := DecodeEffect(clip, ext); err == nil || !strings.Contains(err.Error(), "claims") {
			t.Errorf("%s decoded to %.0f s: %v", name, ClipSeconds(pcm), err)
		}
		freshSession(t)
		captureLog(t)
		if n, err := testSession.Load(fstest.MapFS{"clips/long" + ext: {Data: clip}}, "clips"); err != nil || n != 0 {
			t.Errorf("%s gave a muted player %d durations: %v", name, n, err)
		}
	}
	// The claim is not the length of the data: the bytes read are counted too.
	data := bytes.Repeat([]byte{0, 64}, 8)
	if pcm, err := readClip(bytes.NewReader(data), 16); err != nil || len(pcm) != 32 {
		t.Fatalf("a clip at the limit: %d bytes, %v", len(pcm), err)
	}
	if _, err := readClip(bytes.NewReader(data), 15); err == nil {
		t.Fatal("a clip whose data runs past the limit was decoded whole")
	}
}

// The Vorbis reader slices below zero when the last page claims fewer frames
// than its packets hold. That panic of a broken file is an error of the decode,
// for an effect and for the music.
func TestDecoderPanicIsAnError(t *testing.T) {
	short := toneClaiming(t, 1)
	if _, err := DecodeEffect(short, ".ogg"); err == nil || !strings.Contains(err.Error(), "decoder failed") {
		t.Fatalf("effect: %v", err)
	}
	if _, err := decode(short); err == nil || !strings.Contains(err.Error(), "decoder failed") {
		t.Fatalf("music: %v", err)
	}
}

// A sound file past the encoded limit is refused from its size, unread.
func TestReadSoundRefusesLargeFiles(t *testing.T) {
	f := &largeSound{}
	if _, err := readSound(f, "voice/long.ogg"); !errors.Is(err, errSoundFileTooLarge) || !strings.Contains(err.Error(), "voice/long.ogg") {
		t.Fatalf("oversized sound file: %v", err)
	}
	if f.read || !f.closed {
		t.Fatalf("read=%v closed=%v", f.read, f.closed)
	}
	if b, err := readSound(fstest.MapFS{"a.wav": {Data: []byte("riff")}}, "a.wav"); err != nil || string(b) != "riff" {
		t.Fatalf("%q %v", b, err)
	}
	if _, err := readSound(fstest.MapFS{}, "a.wav"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing file: %v", err)
	}
}

// largeSound is a file system of one file that reports a size past the limit.
type largeSound struct {
	fs.FileInfo
	read, closed bool
}

func (f *largeSound) Open(string) (fs.File, error) { return f, nil }
func (f *largeSound) Stat() (fs.FileInfo, error)   { return f, nil }
func (f *largeSound) Size() int64                  { return maxSoundFile + 1 }
func (f *largeSound) Read([]byte) (int, error)     { f.read = true; return 0, io.EOF }
func (f *largeSound) Close() error                 { f.closed = true; return nil }
