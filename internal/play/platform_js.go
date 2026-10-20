//go:build js

package play

import "syscall/js"

// canQuit is false in a browser: the game cannot close its tab, and ending it would
// only freeze the page, so the title has no item to quit.
const canQuit = false

// meterAsked reports whether the page was opened with ?fps (https://host/game/?fps).
func meterAsked() bool {
	search := js.Global().Get("location").Get("search")
	if search.Type() != js.TypeString {
		return false
	}
	return js.Global().Get("URLSearchParams").New(search).Call("has", "fps").Bool()
}

// publishMeter puts the meter's numbers on the page as window.gozenStats, for the
// browser's console or a test to read.
func publishMeter(s meterStats) {
	js.Global().Set("gozenStats", map[string]any{
		"tps": s.TPS, "fps": s.FPS,
		"updateMaxMs": s.UpdateMax, "updateAvgMs": s.UpdateAvg,
		"drawMaxMs": s.DrawMax, "drawAvgMs": s.DrawAvg,
	})
}
