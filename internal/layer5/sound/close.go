package sound

// Close detaches the current session; the process-wide audio context is reusable.
func Close() error {
	mu.Lock()
	defer mu.Unlock()
	var err error
	if bgmPlay != nil {
		bgmPlay.PauseAndStopReading()
		bgmPlay = nil
	}
	for _, p := range effectPlayers {
		p.PauseAndStopReading()
	}
	effectPlayers = nil
	bgm = nil
	rec = nil
	muted = false
	volume = 0.7
	bgmScale, seScale = 1, 1
	clear(effects)
	varyMu.Lock()
	clear(lastPlayed)
	varyMu.Unlock()
	return err
}
