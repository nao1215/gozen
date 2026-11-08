package play

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestTracePerformance(t *testing.T) {
	p := newTracePerformance(0)
	if n := testing.AllocsPerRun(120, func() {
		p.updated(time.Now())
		p.drawn(time.Now())
	}); n != 0 {
		t.Fatalf("section meter allocates %.2f objects/frame", n)
	}
	p.section.pending = time.Second
	p.drawn(time.Now())
	if p.section.slow != 1 {
		t.Fatalf("slow frames = %d", p.section.slow)
	}
	var out bytes.Buffer
	g := &Game{opts: Options{Trace: &out}, tr: newTracer(&out)}
	g.tr.performance = newTracePerformance(0)
	g.traceMark("first")
	g.tr.performance.updated(time.Now())
	g.tr.performance.drawn(time.Now())
	g.traceMark("second")
	g.tr.performance.updated(time.Now())
	g.tr.performance.drawn(time.Now())
	g.tr.performance.drawn(time.Now())
	g.tracePerformanceEnd()
	var lines []traceLine
	for line := range strings.Lines(out.String()) {
		var l traceLine
		if err := json.Unmarshal([]byte(line), &l); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, l)
	}
	var sections []traceLine
	for _, l := range lines {
		if l["t"] == "section" {
			sections = append(sections, l)
		}
	}
	if len(sections) != 4 {
		t.Fatalf("sections: %v", sections)
	}
	if sections[1]["name"] != "first" || sections[2]["name"] != "second" || sections[3]["scope"] != "run" {
		t.Fatal(sections)
	}
	if sections[1]["frames"] != float64(1) || sections[2]["frames"] != float64(2) || sections[3]["frames"] != float64(3) {
		t.Fatal("incorrect frame partition", sections)
	}
	if _, ok := sections[0]["allocations_per_frame"]; ok {
		t.Fatal("empty section has frame ratio")
	}
	var ticks float64
	for _, s := range sections[:3] {
		n, ok := s["ticks"].(float64)
		if !ok {
			t.Fatal("invalid tick count", s)
		}
		ticks += n

	}
	if ticks != sections[3]["ticks"] {
		t.Fatalf("section ticks %v != total %v", ticks, sections[3]["ticks"])
	}
	out.Reset()
	g.tr.performance = newTracePerformance(0)
	g.tr.performance.updated(time.Now())
	g.tracePerformanceEnd()
	if strings.Count(out.String(), `"t":"section"`) != 1 || !strings.Contains(out.String(), `"scope":"run"`) {
		t.Fatal("unmarked run must have exactly one whole-run section", out.String())
	}
	for _, src := range []string{"mark", "mark one two", "mark one 2"} {
		if _, err := parseScript(src); err == nil {
			t.Fatalf("accepted %q", src)
		}
	}
}

func BenchmarkTracePerformance(b *testing.B) {
	p := newTracePerformance(0)
	b.ReportAllocs()
	for b.Loop() {
		p.updated(time.Now())
		p.drawn(time.Now())
	}
}

func TestDurationWindow(t *testing.T) {
	var w durationWindow
	if w.average() != 0 {
		t.Fatal("empty average")
	}
	for range 60 {
		w.add(time.Millisecond)
	}
	w.add(61 * time.Millisecond)
	if w.average() != 2*time.Millisecond || w.last != 61*time.Millisecond {
		t.Fatalf("rolling window: %+v", w)
	}
	if n := testing.AllocsPerRun(100, func() { w.add(time.Millisecond); _ = w.average() }); n != 0 {
		t.Fatalf("allocations: %g", n)
	}
}
