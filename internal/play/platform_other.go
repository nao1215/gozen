//go:build !js

package play

import "io/fs"

// canQuit: the title's last item ends the game.
const canQuit = true

// meterAsked: on the desktop the frame meter is shown with F8 under --dev.
func meterAsked() bool { return false }

func publishMeter(meterStats) {}

// canSpeak: only a browser build reads stream lines aloud.
func canSpeak() bool { return false }

func speak(string, string, float64, float64) func() bool { return nil }

func stopSpeaking() {}

// voicesAhead: a desktop build reads a line's voice from the disk when the line begins.
const voicesAhead = 0

// loadVoice loads a stream line's recorded voice at once (the files are on the disk), so
// a run and its replay time the lines the same.
func loadVoice(fsys fs.FS, file string) *voiceLoad {
	v := &voiceLoad{file: file, done: make(chan struct{})}
	v.read(fsys)
	return v
}

// whenHidden: a desktop build writes what it keeps when it closes.
func whenHidden(func()) {}

// keepAwake: only a browser build holds the screen awake while the lines are read aloud.
func keepAwake(bool) {}
