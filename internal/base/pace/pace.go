// Package pace lets long work in the background (baking a doll, decoding music) give the
// frames a turn. In a browser (GOOS=js) a program has one thread and a goroutine runs until
// it blocks, so such work would hold the screen still until it ends; elsewhere the
// goroutines run on their own threads and a Pacer does nothing.
package pace

import (
	"runtime"
	"time"
)

// slice is how long the work runs before it lets the browser draw a frame.
const slice = 4 * time.Millisecond

// OneThread reports whether the goroutines share one thread and run until they block (a
// browser).
const OneThread = runtime.GOOS == "js"

// Pacer is one piece of background work. Its zero value is ready to use.
type Pacer struct {
	since time.Time
}

// Breathe is called between small steps of the work: in a browser, once the work has run
// for a slice, it sleeps a little so that the browser can draw a frame.
func (p *Pacer) Breathe() {
	if !OneThread {
		return
	}
	now := time.Now()
	if p.since.IsZero() {
		p.since = now
		return
	}
	if now.Sub(p.since) < slice {
		return
	}
	time.Sleep(time.Millisecond) // blocks: the browser gets its turn
	p.since = time.Now()
}
