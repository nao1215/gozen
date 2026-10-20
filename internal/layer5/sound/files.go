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

func LoadEffects(fsys fs.FS) int {
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
		pcm, err := DecodeEffect(b, ext)
		if err != nil {
			log.Printf("sfx %s: %v", e.Name(), err)
			continue
		}
		effects[strings.TrimSuffix(e.Name(), path.Ext(e.Name()))] = pcm
		n++
	}
	return n
}

func DecodeEffect(b []byte, ext string) ([]byte, error) {
	var r io.Reader
	if ext == ".wav" {
		s, err := wav.DecodeWithSampleRate(SampleRate, bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		r = s
	} else {
		s, err := vorbis.DecodeWithSampleRate(SampleRate, bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		r = s
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
