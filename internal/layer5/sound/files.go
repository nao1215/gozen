package sound

import (
	"bytes"
	"encoding/binary"
	"io"
	"io/fs"
	"log"
	"math"
	"path"
	"strings"

	"github.com/hajimehoshi/ebiten/v2/audio/vorbis"
	"github.com/hajimehoshi/ebiten/v2/audio/wav"
)

const SfxDir = "assets/sfx"

func LoadEffects(fsys fs.FS) int { return loadEffects(fsys, false) }

// LoadEffectDurations loads only stream lengths for play's muted sessions.
// It creates no players or PCM buffers; unreadable effects are logged and skipped
// just as in LoadEffects. Close releases the session's custom durations.
func LoadEffectDurations(fsys fs.FS) int { return loadEffects(fsys, true) }

func loadEffects(fsys fs.FS, lengthsOnly bool) int {
	if fsys == nil {
		return 0
	}
	ents, err := fs.ReadDir(fsys, SfxDir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range ents {
		ext := strings.ToLower(path.Ext(e.Name()))
		if e.IsDir() || (ext != ".ogg" && ext != ".wav") {
			continue
		}
		b, err := fs.ReadFile(fsys, SfxDir+"/"+e.Name())
		if err != nil {
			log.Printf("sfx %s: %v", e.Name(), err)
			continue
		}
		var pcm []byte
		var seconds float64
		if lengthsOnly {
			var stream effectStream
			stream, err = decodeEffectStream(b, ext)
			if err == nil {
				seconds = float64(stream.Length()/4) / SampleRate
			}
		} else {
			pcm, err = DecodeEffect(b, ext)
			seconds = ClipSeconds(pcm)
		}
		if err != nil {
			log.Printf("sfx %s: %v", e.Name(), err)
			continue
		}
		mu.Lock()
		name := strings.TrimSuffix(e.Name(), path.Ext(e.Name()))
		effectDurations[name] = seconds
		if !lengthsOnly {
			effects[name] = pcm
		}
		mu.Unlock()
		n++
	}
	return n
}

type effectStream interface {
	io.Reader
	Length() int64
}

func decodeEffectStream(b []byte, ext string) (effectStream, error) {
	if ext == ".wav" {
		return wav.DecodeWithSampleRate(SampleRate, bytes.NewReader(b))
	}
	return vorbis.DecodeWithSampleRate(SampleRate, bytes.NewReader(b))
}

func DecodeEffect(b []byte, ext string) ([]byte, error) {
	r, err := decodeEffectStream(b, ext)
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(raw)/2*4)
	for i := range len(raw) / 2 {
		//nolint:gosec // PCM reinterprets a signed two-complement 16-bit sample.
		v := float32(int16(binary.LittleEndian.Uint16(raw[i*2:]))) / 32768
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(v))
	}
	return out, nil
}
