//go:build js

package play

import (
	"io/fs"
	"strings"
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

// speaking is the line being read: a phone's browser drops an utterance that nothing
// references, and then never says it has ended.
var speaking js.Value

// speak starts reading text aloud in the language (a BCP 47 tag) with the pitch and
// rate (0 is the browser's). The function returned reports whether it has ended (or
// failed); the caller still gives up after a while, should neither ever be reported.
// A phone reads aloud only after the page has spoken once inside a touch, which the
// page does (unlockSpeech in index.html), since the game speaks from its frame.
func speak(text, language string, pitch, rate float64) func() bool {
	synth := js.Global().Get("speechSynthesis")
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
	end = js.FuncOf(func(_ js.Value, args []js.Value) any {
		mu.Lock()
		defer mu.Unlock()
		if len(args) > 0 && args[0].Get("type").String() == "error" {
			if e := args[0].Get("error").String(); e != "interrupted" && e != "canceled" {
				js.Global().Get("console").Call("warn", "reading aloud: "+e)
			}
		}
		if !ended {
			ended = true
			u.Set("onend", js.Null())
			u.Set("onerror", js.Null())
			if speaking.Equal(u) {
				speaking = js.Undefined()
			}
			end.Release()
		}
		return nil
	})
	u.Set("onend", end)
	u.Set("onerror", end)
	start := func() {
		mu.Lock()
		gone := ended
		mu.Unlock()
		if gone {
			return
		}
		if v := voiceFor(synth, language); !v.IsUndefined() {
			u.Set("voice", v)
		}
		// A line left in the queue (one whose end never came) would hold this one back,
		// and a phone may keep the speech paused after the page was in the background.
		if synth.Get("speaking").Bool() || synth.Get("pending").Bool() {
			synth.Call("cancel")
		}
		synth.Call("resume")
		speaking = u
		synth.Call("speak", u)
	}
	if synth.Call("getVoices").Length() > 0 {
		start()
	} else {
		// The voices load after the page on some browsers: wait for them a second at most,
		// then speak in the language without choosing a voice.
		var ready js.Func
		var timer js.Value
		ready = js.FuncOf(func(js.Value, []js.Value) any {
			js.Global().Call("clearTimeout", timer)
			synth.Call("removeEventListener", "voiceschanged", ready)
			ready.Release()
			start()
			return nil
		})
		synth.Call("addEventListener", "voiceschanged", ready)
		timer = js.Global().Call("setTimeout", ready, 1000)
	}
	return func() bool {
		mu.Lock()
		defer mu.Unlock()
		return ended
	}
}

// voiceFor is a voice of the language: one named for it exactly (ja-JP, or ja_JP as
// Android names it), else one of its first part (ja). Some phones read in their own
// language's voice whatever lang says, so the voice is chosen too. Undefined when the
// browser has none.
func voiceFor(synth js.Value, language string) js.Value {
	voices := synth.Call("getVoices")
	var near js.Value
	for i := range voices.Length() {
		v := voices.Index(i)
		lang := strings.ReplaceAll(v.Get("lang").String(), "_", "-")
		if strings.EqualFold(lang, language) {
			return v
		}
		if near.IsUndefined() && strings.EqualFold(strings.SplitN(lang, "-", 2)[0], strings.SplitN(language, "-", 2)[0]) {
			near = v
		}
	}
	return near
}

// loadVoice loads a stream line's recorded voice in the background: a file the page is
// still fetching (the sounds come last, ADR 0051) would hold the frame until it came.
func loadVoice(fsys fs.FS, file string) *voiceLoad {
	v := &voiceLoad{file: file, done: make(chan struct{})}
	go v.read(fsys)
	return v
}

// wakeLock is the screen's wake lock held while a show is read aloud (undefined: none),
// asking while one is requested and awake while the watch view wants one.
var (
	wakeLock      js.Value
	asking, awake bool
)

// keepAwake asks for the screen's wake lock (Screen Wake Lock API) so the screen does not
// turn off while a show is read aloud, again when the page lost it (it does when hidden), or lets
// it go. A browser without the API, or one that refuses, keeps its screen as it would.
func keepAwake(on bool) {
	api := js.Global().Get("navigator").Get("wakeLock")
	if !api.Truthy() {
		return
	}
	held := wakeLock.Truthy() && !wakeLock.Get("released").Bool()
	awake = on
	if !on {
		if held {
			wakeLock.Call("release")
		}
		wakeLock = js.Undefined()
		return
	}
	if held || asking || js.Global().Get("document").Get("visibilityState").String() != "visible" {
		return
	}
	asking = true
	var got, failed js.Func
	done := func() {
		asking = false
		got.Release()
		failed.Release()
	}
	got = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if wakeLock = args[0]; !awake { // the reading stopped while it was asked for
			wakeLock.Call("release")
			wakeLock = js.Undefined()
		}
		done()
		return nil
	})
	failed = js.FuncOf(func(js.Value, []js.Value) any {
		done()
		return nil
	})
	api.Call("request", "screen").Call("then", got, failed)
}

// stopSpeaking stops reading aloud.
func stopSpeaking() { js.Global().Get("speechSynthesis").Call("cancel") }
