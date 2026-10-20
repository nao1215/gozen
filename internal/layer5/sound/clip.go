package sound

import (
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/hajimehoshi/ebiten/v2/audio"
)

// Clip loads sound effects for its own screens (moved from the public sound package on 2026-10-19; ADR 0021).
// Clip owns decoded stereo float32 audio. Independent players may share a clip.
type Clip struct{ pcm []byte }

// Load decodes an Ogg Vorbis (.ogg) or WAV (.wav) file from fsys. The FS is
// borrowed only during this call. Read and decode failures return nil and an error.
func Load(fsys fs.FS, name string) (*Clip, error) {
	if fsys == nil || !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "audio", Path: name, Err: fs.ErrInvalid}
	}
	ext := strings.ToLower(path.Ext(name))
	if ext != ".ogg" && ext != ".wav" {
		return nil, fmt.Errorf("audio %q: unsupported extension", name)
	}
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}
	pcm, err := DecodeEffect(b, ext)
	if err != nil {
		return nil, fmt.Errorf("audio %q: %w", name, err)
	}
	return &Clip{pcm: pcm}, nil
}

// Play starts an independent player on ctx, which must use SampleRate. The host
// creates the process's single audio context and must stop the returned player with PauseAndStopReading.
// Volume, pause, and rewind use Ebitengine's Player methods.
func (c *Clip) Play(ctx *audio.Context) (*audio.Player, error) {
	if ctx == nil || ctx.SampleRate() != SampleRate {
		return nil, fmt.Errorf("audio: context must use %d Hz", SampleRate)
	}
	p := ctx.NewPlayerF32FromBytes(c.pcm)
	p.Play()
	return p, nil
}
