//go:build js

package play

import (
	"sync"
	"syscall/js"
)

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

// canSpeak reports whether the browser can read lines aloud (speechSynthesis).
func canSpeak() bool {
	return js.Global().Get("speechSynthesis").Truthy() && js.Global().Get("SpeechSynthesisUtterance").Truthy()
}

// speak starts reading text aloud in the language (a BCP 47 tag) with the pitch and
// rate (0 is the browser's). The function returned reports whether it has ended.
func speak(text, language string, pitch, rate float64) func() bool {
	u := js.Global().Get("SpeechSynthesisUtterance").New(text)
	u.Set("lang", language)
	if pitch > 0 {
		u.Set("pitch", pitch)
	}
	if rate > 0 {
		u.Set("rate", rate)
	}
	var mu sync.Mutex
	ended := false
	var end js.Func
	end = js.FuncOf(func(js.Value, []js.Value) any {
		mu.Lock()
		defer mu.Unlock()
		if !ended {
			ended = true
			u.Set("onend", js.Null())
			u.Set("onerror", js.Null())
			end.Release()
		}
		return nil
	})
	u.Set("onend", end)
	u.Set("onerror", end)
	js.Global().Get("speechSynthesis").Call("speak", u)
	return func() bool {
		mu.Lock()
		defer mu.Unlock()
		return ended
	}
}

// stopSpeaking stops reading aloud.
func stopSpeaking() { js.Global().Get("speechSynthesis").Call("cancel") }
