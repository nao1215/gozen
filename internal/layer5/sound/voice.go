package sound

import (
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/hajimehoshi/ebiten/v2/audio"
)

// Voices (ADR 0028): the voice of a line plays alone, the one before stopped. A voice is
// read from the game's files when it is first played and kept in a small cache (a browser
// holds the whole game in memory, so the voices are not loaded ahead).

// voiceCacheSize is how many decoded voices are kept.
const voiceCacheSize = 16

type voiceClip struct {
	path string
	pcm  []byte
}

var (
	voicePlayer *audio.Player
	voiceCache  []voiceClip // the most recently used last
	voiceScale  = 1.0
)

// VoiceClip is the decoded voice at a path under the game's files (from the cache when
// it was read before).
func VoiceClip(fsys fs.FS, file string) ([]byte, error) {
	mu.Lock()
	for i, c := range voiceCache {
		if c.path == file {
			voiceCache = append(slices.Delete(voiceCache, i, i+1), c)
			mu.Unlock()
			return c.pcm, nil
		}
	}
	mu.Unlock()
	b, err := fs.ReadFile(fsys, file)
	if err != nil {
		return nil, err
	}
	pcm, err := DecodeEffect(b, strings.ToLower(path.Ext(file)))
	if err != nil {
		return nil, err
	}
	mu.Lock()
	voiceCache = append(voiceCache, voiceClip{file, pcm})
	if len(voiceCache) > voiceCacheSize {
		voiceCache = slices.Delete(voiceCache, 0, len(voiceCache)-voiceCacheSize)
	}
	mu.Unlock()
	return pcm, nil
}

// ClipSeconds is how long a decoded clip lasts.
func ClipSeconds(pcm []byte) float64 { return float64(len(pcm)/8) / SampleRate }

// PlayVoice stops the voice playing and plays this one at gain (the speaker's volume; the
// voice volume and mute apply on top).
func PlayVoice(pcm []byte, gain float64) {
	mu.Lock()
	defer mu.Unlock()
	stopVoiceLocked()
	if rec != nil && pcm != nil {
		rec.voices = append(rec.voices, voice{b: pcm})
		return
	}
	if ctx == nil || muted || pcm == nil {
		return
	}
	p := ctx.NewPlayerF32FromBytes(pcm)
	p.SetVolume(min(1, voiceScale*gain))
	p.Play()
	voicePlayer = p
}

// StopVoice stops the voice playing.
func StopVoice() {
	mu.Lock()
	defer mu.Unlock()
	stopVoiceLocked()
}

func stopVoiceLocked() {
	if voicePlayer != nil {
		voicePlayer.PauseAndStopReading()
		voicePlayer = nil
	}
}

// VoicePlaying reports whether a voice is heard now.
func VoicePlaying() bool {
	mu.Lock()
	defer mu.Unlock()
	return voicePlayer != nil && voicePlayer.IsPlaying()
}

// SetVoiceVolume sets the volume of voices (0 to 1).
func SetVoiceVolume(v float64) {
	mu.Lock()
	defer mu.Unlock()
	voiceScale = v
	if voicePlayer != nil && v == 0 {
		voicePlayer.SetVolume(0)
	}
}

// closeVoices drops the voice and the cache of a session that ends.
func closeVoices() {
	stopVoiceLocked()
	voiceCache = nil
	voiceScale = 1
}
