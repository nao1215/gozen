package play

import (
	"strings"
	"testing"
	"time"
)

// The meter keeps the longest Update and Draw of a second and, after the second,
// shows them and starts over.
func TestFrameMeter(t *testing.T) {
	var m frameMeter
	m.toggle()
	if !m.on {
		t.Fatal("toggle did not turn the meter on")
	}
	m.drawn(time.Now().Add(-30 * time.Millisecond))
	m.drawn(time.Now().Add(-10 * time.Millisecond))
	m.updated(time.Now().Add(-5 * time.Millisecond))
	if m.text != "" || m.updates != 1 || m.draws != 2 {
		t.Fatalf("within the first second: text %q, %d updates, %d draws", m.text, m.updates, m.draws)
	}
	if m.drawMax < 30*time.Millisecond || m.updMax < 5*time.Millisecond {
		t.Fatalf("longest draw %v, update %v", m.drawMax, m.updMax)
	}
	m.since = time.Now().Add(-2 * time.Second)
	m.updated(time.Now())
	if !strings.HasPrefix(m.text, "TPS ") || !strings.Contains(m.text, "Draw") {
		t.Fatalf("meter text %q", m.text)
	}
	if m.drawMax != 0 || m.draws != 0 || !m.on {
		t.Fatalf("the next second did not start afresh: %+v", m)
	}
	m.toggle()
	if m.on || m.text != "" {
		t.Fatalf("toggled off but still %+v", m)
	}
}
