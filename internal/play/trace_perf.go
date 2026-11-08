package play

import (
	"runtime/metrics"
	"time"
)

// sectionMeter is owned by the game thread. Sampling uses a fixed slice and is
// limited to section boundaries and every 60 updates, never every draw.
// Counters include all goroutines in this process, including trace writing.
type sectionMeter struct {
	name                string
	start               time.Time
	firstTick           int
	frames, ticks, slow uint64
	pending             time.Duration
	heap                uint64
}

type tracePerformance struct {
	section, whole    sectionMeter
	samples           [3]metrics.Sample
	base, sectionBase uint64
	marked            bool
}

func newTracePerformance(tick int) *tracePerformance {
	p := &tracePerformance{samples: [3]metrics.Sample{
		{Name: "/gc/heap/allocs:objects"},
		{Name: "/memory/classes/heap/objects:bytes"},
		{Name: "/memory/classes/heap/unused:bytes"},
	}}
	now := time.Now()
	p.section = sectionMeter{name: "run", start: now, firstTick: tick}
	p.whole = sectionMeter{name: "run", start: now, firstTick: tick}
	p.base = p.sample()
	p.sectionBase = p.base
	return p
}

func (p *tracePerformance) sample() uint64 {
	metrics.Read(p.samples[:])
	heap := p.samples[1].Value.Uint64() + p.samples[2].Value.Uint64()
	p.section.heap = max(p.section.heap, heap)
	p.whole.heap = max(p.whole.heap, heap)
	return p.samples[0].Value.Uint64()
}

func (p *tracePerformance) updated(start time.Time) {
	elapsed := time.Since(start)
	p.section.ticks++
	p.whole.ticks++
	p.section.pending += elapsed
	p.whole.pending += elapsed
	if p.whole.ticks%60 == 0 {
		p.sample()
	}
}

func (p *tracePerformance) drawn(start time.Time) {
	elapsed := time.Since(start)
	for _, s := range []*sectionMeter{&p.section, &p.whole} {
		s.frames++
		if s.pending+elapsed > time.Second/60 {
			s.slow++
		}
		s.pending = 0
	}
}

func (g *Game) emitSection(s sectionMeter, scope string, allocations uint64, now time.Time) {
	seconds := now.Sub(s.start).Seconds()
	l := traceLine{
		"name": s.name, "scope": scope, "from_tick": s.firstTick,
		"frames": s.frames, "ticks": s.ticks, "elapsed_seconds": seconds,
		"heap_inuse_bytes": s.heap, "allocations": allocations,
	}
	if seconds > 0 {
		l["fps"] = float64(s.frames) / seconds
		l["tps"] = float64(s.ticks) / seconds
	}
	// With no completed Draw, frame ratios are undefined and omitted.
	if s.frames > 0 {
		l["slow_frame_share"] = float64(s.slow) / float64(s.frames)
		l["allocations_per_frame"] = float64(allocations) / float64(s.frames)
	}
	g.emit("section", l)
}

func (g *Game) traceMark(name string) {
	if g.tr == nil || g.opts.Trace == nil || g.opts.headless || g.tr.ended {
		return
	}
	if g.tr.performance == nil {
		g.tr.performance = newTracePerformance(g.tick)
	}
	p := g.tr.performance
	p.marked = true
	allocations := p.sample()
	now := time.Now()
	g.emitSection(p.section, "mark", allocations-p.sectionBase, now)
	p.section = sectionMeter{name: name, start: now, firstTick: g.tick}
	p.sectionBase = p.sample()
}

func (g *Game) tracePerformanceEnd() {
	if g.tr.performance == nil {
		return
	}
	p := g.tr.performance
	allocations := p.sample()
	now := time.Now()
	if p.marked {
		g.emitSection(p.section, "mark", allocations-p.sectionBase, now)
	}
	g.emitSection(p.whole, "run", allocations-p.base, now)
}
