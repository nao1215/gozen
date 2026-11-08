package sound

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
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

// LoadEffects replaces the game effects and durations for startup and live reload.
// Removed overrides revert to built-in sounds; unreadable files are logged and skipped.
func LoadEffects(fsys fs.FS) int {
	n, err := loadEffects(fsys, SfxDir, false)
	if err != nil {
		log.Printf("sfx: %v", err)
	}
	return n
}

// Load installs effect durations for the player's muted startup and live reload,
// returning the number of readable effects in dir (a relative FS directory).
// It creates no players or PCM buffers. Missing directories and a nil FS mean
// no custom effects. Invalid directories return an error; unreadable individual
// effects are logged and skipped, as in LoadEffects. Each call replaces custom
// durations; Close clears them. Call only between sessions or on the game thread.
func Load(fsys fs.FS, dir string) (int, error) { return loadEffects(fsys, dir, true) }

func loadEffects(fsys fs.FS, dir string, lengthsOnly bool) (int, error) {
	if !fs.ValidPath(dir) {
		return 0, fmt.Errorf("sfx directory %q: %w", dir, fs.ErrInvalid)
	}

	mu.Lock()
	restoreBuiltins := false
	for name := range effectDurations {
		if _, ok := effects[name]; ok {
			delete(effects, name)
			restoreBuiltins = restoreBuiltins || builtinEffectSeconds[name] != 0
		}
	}
	clear(effectDurations)
	mu.Unlock()
	if restoreBuiltins && !lengthsOnly {
		makeEffects(false)
	}

	if fsys == nil {
		return 0, nil
	}
	ents, err := fs.ReadDir(fsys, dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	n := 0
	for _, e := range ents {
		ext := strings.ToLower(path.Ext(e.Name()))
		if e.IsDir() || (ext != ".ogg" && ext != ".wav") {
			continue
		}
		b, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
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
	return n, nil
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
