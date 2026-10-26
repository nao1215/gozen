package play

import (
	"bufio"
	"context"
	"errors"
	"io"
)

// SimulateConfig is what Simulate needs beyond the game's Options.
type SimulateConfig struct {
	// Lines, when not nil, supplies more script lines after Options.Script runs out, read
	// one at a time when the game is ready for the next command. Before each read the
	// trace gets a ready line and is flushed, so a program reading the trace can decide
	// what to send. A line that does not parse is written to the trace as script_error
	// and skipped. The end of Lines, or the end of the context, ends the run.
	Lines io.Reader
}

// SimulateResult tells how far a simulated run went.
type SimulateResult struct {
	Ticks int // game ticks run (60 a second of play)
	Lines int // trace lines written
}

// Simulate runs a development script the way Run does with --dev --script, without a
// window: the same game is updated tick by tick, as fast as it can, and never drawn.
// The command gozen sim script calls it. With the same options and seed the game goes
// the same way as in Run, so the trace has the same lines except the ones that come from
// drawing (shot, view, stand, menu); shot and the screenshots of lines and play take no
// pictures. opts.Development and opts.NoSound are set; opts.Trace, when given, is written
// through a buffer that is flushed at each ready line and at the end.
func Simulate(ctx context.Context, opts Options, cfg SimulateConfig) (res SimulateResult, err error) {
	if err := ctx.Err(); err != nil {
		return res, err
	}
	if opts.Script == "" && cfg.Lines == nil {
		return res, &ArgumentError{Err: errors.New("play: a simulated run needs a script")}
	}
	if opts.Autopilot != nil || opts.Replay != "" || opts.Video != nil || opts.Audio != nil || opts.TraceShots {
		return res, &ArgumentError{Err: errors.New("play: a simulated run takes no autoplay, replay, recording, or trace shots")}
	}
	opts.Development, opts.NoSound, opts.headless = true, true, true
	var buf *bufio.Writer
	if opts.Trace != nil {
		buf = bufio.NewWriterSize(opts.Trace, 1<<16)
		opts.Trace = buf
	}
	flush := func() error {
		if buf == nil {
			return nil
		}
		return buf.Flush()
	}
	g, err := New(opts)
	if err != nil {
		return res, errors.Join(err, flush())
	}
	g.cancel()
	g.context, g.cancel = context.WithCancel(ctx)
	defer func() {
		if g.tr != nil {
			res.Lines = g.tr.lines
		}
		res.Ticks = g.tick
		err = errors.Join(err, g.Close(), flush())
	}()
	if cfg.Lines != nil {
		g.in.script.next = g.simulateLines(ctx, cfg.Lines, flush)
		g.in.script.rejected = func(line string, err error) {
			g.emit("script_error", traceLine{"line": line, "error": err.Error()})
		}
	}
	for {
		err = g.Update()
		if errors.Is(err, errQuit) {
			err = nil
			break
		}
		if err != nil {
			break
		}
	}
	if err == nil && g.tr != nil {
		err = g.tr.err
	}
	return res, errors.Join(err, g.failure)
}

// simulateLines reads r on its own goroutine and returns the script's next: it writes a
// ready line, flushes the trace, and waits for a line, the end of r, or the end of ctx.
func (g *Game) simulateLines(ctx context.Context, r io.Reader, flush func() error) func() (string, bool) {
	lines := make(chan string)
	done := make(chan error, 1)
	go func() {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64<<10), 16<<20)
		for sc.Scan() {
			select {
			case lines <- sc.Text():
			case <-ctx.Done():
				done <- nil
				return
			}
		}
		done <- sc.Err()
	}()
	return func() (string, bool) {
		what, _ := g.waiting()
		l := traceLine{"busy": g.busy(), "waiting": what}
		if g.st != nil {
			l["location"] = g.st.Location
		}
		g.emit("ready", l)
		if err := flush(); err != nil {
			g.failure = errors.Join(g.failure, err)
			return "", false
		}
		select {
		case line := <-lines:
			return line, true
		case err := <-done:
			g.failure = errors.Join(g.failure, err)
		case <-ctx.Done():
		}
		return "", false
	}
}
