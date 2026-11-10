package sound

import (
	"io/fs"
	"path"
	"slices"
	"strings"
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

// VoiceClip is the decoded voice at a path under the game's files (from the cache when
// it was read before).
func (session *Session) VoiceClip(fsys fs.FS, file string) ([]byte, error) {
	session.mu.Lock()
	for i, c := range session.voiceCache {
		if c.path == file {
			session.voiceCache = append(slices.Delete(session.voiceCache, i, i+1), c)
			session.mu.Unlock()
			return c.pcm, nil
		}
	}
	session.mu.Unlock()
	b, err := readSound(fsys, file)
	if err != nil {
		return nil, err
	}
	pcm, err := DecodeEffect(b, strings.ToLower(path.Ext(file)))
	if err != nil {
		return nil, err
	}
	session.mu.Lock()
	session.voiceCache = append(session.voiceCache, voiceClip{file, pcm})
	if len(session.voiceCache) > voiceCacheSize {
		session.voiceCache = slices.Delete(session.voiceCache, 0, len(session.voiceCache)-voiceCacheSize)
	}
	session.mu.Unlock()
	return pcm, nil
}

// ClipSeconds is how long a decoded clip lasts.
func ClipSeconds(pcm []byte) float64 { return float64(len(pcm)/8) / SampleRate }

// PlayVoice stops the voice playing and plays this one at gain (the speaker's volume; the
// voice volume and mute apply on top).
func (session *Session) PlayVoice(pcm []byte, gain float64) {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.stopVoiceLocked()
	if session.rec != nil && pcm != nil {
		session.rec.voices = append(session.rec.voices, voice{b: pcm})
		return
	}
	if session.ctx == nil || session.muted || pcm == nil {
		return
	}
	p := session.ctx.NewPlayerF32FromBytes(pcm)
	p.SetVolume(min(1, session.voiceScale*gain))
	p.Play()
	session.voicePlayer = p
}

// StopVoice stops the voice playing.
func (session *Session) StopVoice() {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.stopVoiceLocked()
}

func (session *Session) stopVoiceLocked() {
	if session.voicePlayer != nil {
		session.voicePlayer.PauseAndStopReading()
		session.voicePlayer = nil
	}
}

// SetVoiceVolume sets the volume of voices (0 to 1).
func (session *Session) SetVoiceVolume(v float64) {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.voiceScale = v
	if session.voicePlayer != nil && v == 0 {
		session.voicePlayer.SetVolume(0)
	}
}

// closeVoices drops the voice and the cache of a session that ends.
func (session *Session) closeVoices() {
	session.stopVoiceLocked()
	session.voiceCache = nil
	session.voiceScale = 1
}
