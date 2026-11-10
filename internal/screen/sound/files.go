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
func (session *Session) LoadEffects(fsys fs.FS) int {
	n, err := session.loadEffects(fsys, SfxDir, false)
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
func (session *Session) Load(fsys fs.FS, dir string) (int, error) {
	return session.loadEffects(fsys, dir, true)
}

func (session *Session) loadEffects(fsys fs.FS, dir string, lengthsOnly bool) (int, error) {
	if !fs.ValidPath(dir) {
		return 0, fmt.Errorf("sfx directory %q: %w", dir, fs.ErrInvalid)
	}

	session.mu.Lock()
	restoreBuiltins := false
	for name := range session.effectDurations {
		if _, ok := session.effects[name]; ok {
			delete(session.effects, name)
			restoreBuiltins = restoreBuiltins || builtinEffectSeconds[name] != 0
		}
	}
	clear(session.effectDurations)
	session.mu.Unlock()
	if restoreBuiltins && !lengthsOnly {
		session.makeEffects(false)
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
		b, err := readSound(fsys, path.Join(dir, e.Name()))
		if err != nil {
			log.Printf("sfx %s: %v", e.Name(), err)
			continue
		}
		var pcm []byte
		var seconds float64
		if lengthsOnly {
			var stream effectStream
			stream, err = openSound(b, ext, maxClipBytes)
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
		session.mu.Lock()
		name := strings.TrimSuffix(e.Name(), path.Ext(e.Name()))
		session.effectDurations[name] = seconds
		if !lengthsOnly {
			session.effects[name] = pcm
		}
		session.mu.Unlock()
		n++
	}
	return n, nil
}

type effectStream interface {
	io.Reader
	Length() int64
}

// maxSoundFile is the most encoded bytes read from one sound file (music, effect
// or voice), the same as a picture's limit.
const maxSoundFile = 64 << 20

// maxClipSeconds is the longest effect or voice the player decodes. A clip is
// kept whole in memory as 32-bit stereo (about 20 MiB a minute, and sixteen
// voices are cached); longer sound belongs to the music.
const (
	maxClipSeconds = 60
	// maxClipBytes is that length as the decoders give it: 16-bit stereo.
	maxClipBytes = maxClipSeconds * SampleRate * 4
)

// readSound reads one sound file of the game. A file past maxSoundFile is
// refused from its reported size, before a byte is read, and the read itself
// stops at the limit when the size is not reported.
func readSound(fsys fs.FS, name string) ([]byte, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	tooLarge := fmt.Errorf("%s: %w", name, errSoundFileTooLarge)
	var b []byte
	if info, statErr := f.Stat(); statErr == nil && info.Size() > maxSoundFile {
		err = tooLarge
	} else {
		b, err = io.ReadAll(io.LimitReader(f, maxSoundFile+1))
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil && len(b) > maxSoundFile {
		err = tooLarge
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}

var errSoundFileTooLarge = fmt.Errorf("sound file exceeds %d encoded bytes", maxSoundFile)

// openSound opens a sound file (ext is ".wav", anything else is Ogg Vorbis) as
// 16-bit stereo at SampleRate and refuses one that claims more than maxBytes of
// it, before anything is decoded. The claim is a field of the file, so a
// negative (wrapped) one is refused too.
func openSound(b []byte, ext string, maxBytes int64) (r effectStream, err error) {
	defer decoderFailure(&err)
	if ext == ".wav" {
		r, err = wav.DecodeWithSampleRate(SampleRate, bytes.NewReader(b))
	} else {
		r, err = vorbis.DecodeWithSampleRate(SampleRate, bytes.NewReader(b))
	}
	if err != nil {
		return nil, err
	}
	if n := r.Length(); n < 0 || n > maxBytes {
		return nil, fmt.Errorf("sound claims %d seconds, over the limit of %d", n/(4*SampleRate), maxBytes/(4*SampleRate))
	}
	return guarded{r}, nil
}

// guarded turns a panic of a decoder into an error of the read. The decoders
// index by numbers of the file (a last Ogg page that claims fewer frames than
// its packets hold makes the Vorbis reader slice below zero), and a broken
// sound file must not end the game.
type guarded struct{ effectStream }

func (g guarded) Read(p []byte) (n int, err error) {
	defer decoderFailure(&err)
	return g.effectStream.Read(p)
}

func decoderFailure(err *error) {
	if r := recover(); r != nil {
		*err = fmt.Errorf("sound decoder failed: %v", r)
	}
}

// DecodeEffect decodes an effect or a voice (ext is ".wav" or ".ogg") into
// 32-bit float stereo at SampleRate. A clip longer than maxClipSeconds is an
// error, whether its header says so or its data runs past what the header says.
func DecodeEffect(b []byte, ext string) ([]byte, error) {
	r, err := openSound(b, ext, maxClipBytes)
	if err != nil {
		return nil, err
	}
	return readClip(r, maxClipBytes)
}

// readClip converts an opened clip, reading at most limit bytes of it: the
// length a file claims is not the length of its data.
func readClip(r io.Reader, limit int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("sound is longer than the limit of %d seconds", limit/(4*SampleRate))
	}
	out := make([]byte, len(raw)/2*4)
	for i := range len(raw) / 2 {
		//nolint:gosec // PCM reinterprets a signed two-complement 16-bit sample.
		v := float32(int16(binary.LittleEndian.Uint16(raw[i*2:]))) / 32768
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(v))
	}
	return out, nil
}
