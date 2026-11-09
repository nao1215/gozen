package sound

import (
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2/audio"

	"github.com/nao1215/gozen/internal/base/pace"
)

// Close releases this session; the process-wide audio device remains reusable.
func (session *Session) Close() error {
	session.mu.Lock()
	defer session.mu.Unlock()
	var err error
	if session.bgmPlay != nil {
		session.bgmPlay.PauseAndStopReading()
		session.bgmPlay = nil
	}
	for _, p := range session.effectPlayers {
		p.PauseAndStopReading()
	}
	session.effectPlayers = nil
	if session.bgm != nil {
		session.bgm.close()
	}
	session.bgm = nil
	session.rec = nil
	session.muted = false
	session.volume = 0.7
	session.bgmScale, session.seScale = 1, 1
	session.closeVoices()
	clear(session.effects)
	clear(session.effectDurations)
	session.effectsGen++
	session.varyMu.Lock()
	clear(session.lastPlayed)
	session.varyMu.Unlock()
	return err
}

// Session owns a game's audio players, effects, voices and recording state. The
// device context is shared by Ebitengine; closing a session never closes the device.
// Playback controls run on the game thread. Voice loading and effect synthesis
// retain this session when working in the background.
type Session struct {
	ctx                   *audio.Context
	bgm                   *Modular
	bgmPlay               *audio.Player
	muted                 bool
	volume                float64
	mu                    sync.Mutex
	bgmScale, seScale     float64
	voicePlayer           *audio.Player
	voiceCache            []voiceClip
	voiceScale            float64
	rec                   *recorder
	silentBuf             []byte
	varyMu                sync.Mutex
	varyPitch, varyVolume float64
	varyWindow            time.Duration
	lastPlayed            map[string]time.Time
	effectPlayers         []*audio.Player
	effects               map[string][]byte
	effectsGen            int
	effectDurations       map[string]float64
	synthPace             pace.Pacer
}

// NewSession creates isolated playback state, including when no device is used.
func NewSession() *Session {
	return &Session{volume: 0.7, bgmScale: 1, seScale: 1, voiceScale: 1, varyPitch: 0.03, varyVolume: 0.10, varyWindow: 400 * time.Millisecond, lastPlayed: map[string]time.Time{}, effects: map[string][]byte{}, effectDurations: map[string]float64{}}
}

// Suspend pauses this session's players without discarding their positions. Resume
// is supplied by the owning game after its embedded child finishes.
func (session *Session) Suspend() func() {
	session.mu.Lock()
	var playing []*audio.Player
	for _, p := range append([]*audio.Player{session.bgmPlay, session.voicePlayer}, session.effectPlayers...) {
		if p != nil && p.IsPlaying() {
			p.Pause()
			playing = append(playing, p)
		}
	}
	session.mu.Unlock()
	return func() {
		for _, p := range playing {
			p.Play()
		}
	}
}
