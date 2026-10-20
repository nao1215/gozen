//go:build !js

package play

// canQuit: the title's last item ends the game.
const canQuit = true

// meterAsked: on the desktop the frame meter is shown with F8 under --dev.
func meterAsked() bool { return false }

func publishMeter(meterStats) {}
