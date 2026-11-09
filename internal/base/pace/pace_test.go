package pace

import (
	"testing"
	"time"
)

// Outside a browser the goroutines have their own threads: a Pacer never sleeps and keeps no clock.
func TestPacerDoesNothingOffTheBrowser(t *testing.T) {
	if OneThread {
		t.Skip("a browser build paces for real")
	}
	var p Pacer
	start := time.Now()
	for range 1000 {
		p.Breathe()
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("1000 breaths took %v; a Pacer must not sleep here", d)
	}
	if !p.since.IsZero() {
		t.Error("a Pacer off the browser keeps no clock")
	}
}
