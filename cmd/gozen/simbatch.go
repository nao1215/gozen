package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/cliio"
	"github.com/nao1215/gozen/internal/diag"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// batchCase is one line of gozen sim batch's input: the arguments of one gozen sim script
// run, and a name the result carries back.
type batchCase struct {
	ID   string   `json:"id"`
	Args []string `json:"args"`
}

// batchResult is one line of gozen sim batch's output: what the same gozen sim script
// would have exited with and printed.
type batchResult struct {
	ID     string `json:"id"`
	Exit   int    `json:"exit"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

// simBatch is gozen sim batch: many gozen sim script runs in one process, so a game is
// read and checked once instead of once a run. Each case starts a new game from the
// definitions read for its directory; nothing but those definitions passes from one case
// to the next (each has its own seed, saves, state and trace).
func (c *command) simBatch() clihelp.Operation {
	fs := clihelp.NewFlagSet("gozen sim batch")
	cases := fs.String("cases", "", "read the cases, as JSON Lines, from this `FILE`; - is the standard input; required")
	keep := fs.Int("games", 4, "keep the definitions of at most this many game `DIRS` read")
	return clihelp.Operation{Flags: fs, Run: func(ctx context.Context) (err error) {
		c.report = cliio.Report{Command: "sim batch", Format: "jsonl"}
		if fs.NArg() != 0 {
			return cliio.Invalid(diag.Operands, "expected no operands; the game directory of each case is in its args")
		}
		if *cases == "" {
			return cliio.Invalid(diag.OptionRequired, "--cases FILE is required (- reads the standard input)")
		}
		if *keep < 1 {
			return cliio.Invalid(diag.BadValue, "--games must be positive")
		}
		in := io.Reader(os.Stdin)
		if *cases != "-" {
			f, err := os.Open(*cases)
			if err != nil {
				return err
			}
			defer func() {
				if closeErr := f.Close(); closeErr != nil {
					err = errors.Join(err, closeErr)
				}
			}()
			in = f
		}
		games := &gameCache{keep: *keep}
		summary := struct {
			Succeeded int `json:"succeeded"`
			Failed    int `json:"failed"`
		}{}
		sc := bufio.NewScanner(in)
		sc.Buffer(make([]byte, 64<<10), 64<<20)
		for sc.Scan() {
			if err := ctx.Err(); err != nil {
				return err
			}
			line := bytes.TrimSpace(sc.Bytes())
			if len(line) == 0 {
				continue
			}
			res := runCase(line, games)
			// One write a result: a program streaming cases reads it before sending the next.
			status := "ok"
			if res.Exit != 0 {
				status = "failed"
				summary.Failed++
			} else {
				summary.Succeeded++
			}
			if err := c.report.Item(c.out, res.ID, status, res); err != nil {
				return err
			}
		}
		if err := sc.Err(); err != nil {
			return err
		}
		status := "ok"
		if summary.Failed > 0 {
			status = "failed"
		}
		if err := c.report.Result(c.out, status, summary); err != nil {
			return err
		}
		if summary.Failed > 0 {
			return cliio.ErrReported
		}
		return nil
	}}
}

// batchRun is runWith; set in init, since the command tree holds gozen sim batch.
var batchRun func(args []string, out, stderr io.Writer, games *gameCache) int

func init() { batchRun = runWith }

// runCase runs one case as gozen sim script would, with the definitions from games.
func runCase(line []byte, games *gameCache) batchResult {
	var bc batchCase
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&bc); err != nil {
		return batchResult{ID: bc.ID, Exit: 2, Stderr: fmt.Sprintf("gozen sim batch: case %q: %v\n", line, err)}
	}
	// A JSON Lines record contains exactly one value. Do not execute a partial
	// record when a producer accidentally concatenates cases or appends garbage.
	if err := dec.Decode(new(json.RawMessage)); err != io.EOF {
		return batchResult{ID: bc.ID, Exit: 2, Stderr: "gozen sim batch: expected exactly one JSON value per case\n"}
	}
	if slices.ContainsFunc(bc.Args, func(arg string) bool {
		return arg == "-" || arg == "--script=-" || arg == "--trace=-"
	}) {
		return batchResult{ID: bc.ID, Exit: 2, Stderr: "gozen sim batch: a case reads no script or trace on the standard streams (-)\n"}
	}
	var stdout bytes.Buffer
	// What the game logs goes to the standard error of gozen sim script, so it is
	// part of the case's stderr, in the order it was written.
	stderr := &lockedBuffer{}
	prev := log.Writer()
	log.SetOutput(stderr)
	defer log.SetOutput(prev)
	exit := batchRun(append([]string{"sim", "script"}, bc.Args...), &stdout, stderr, games)
	return batchResult{ID: bc.ID, Exit: exit, Stdout: stdout.String(), Stderr: stderr.String()}
}

// lockedBuffer is a buffer the log of the game's goroutines can share.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// gameCache keeps the definitions of the game directories read by gozen sim batch, so
// each is loaded and checked once. A directory whose files changed (in name, size or
// modification time) is read again.
type gameCache struct {
	keep    int
	entries []cachedGame // most recently used last
}

type cachedGame struct {
	dir   string
	stamp string
	db    *data.DB
}

// definitions returns the definitions of the game in dir, read from fsys, and sets the
// words of its language as reading it would. When it cannot, it returns nil, and the run
// reads the game itself and reports what is wrong exactly as gozen sim script does.
func (g *gameCache) definitions(dir string, fsys fs.FS) *data.DB {
	if g == nil {
		return nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}
	stamp, err := dirStamp(fsys)
	if err != nil {
		return nil
	}
	if err := data.UseWords(fsys); err != nil {
		return nil
	}
	for i, e := range g.entries {
		if e.dir == abs {
			g.entries = slices.Delete(g.entries, i, i+1)
			if e.stamp == stamp {
				g.entries = append(g.entries, e)
				return e.db
			}
			break
		}
	}
	db, err := data.Load(fsys)
	if err != nil {
		return nil
	}
	if len(g.entries) >= g.keep {
		g.entries = slices.Delete(g.entries, 0, 1)
	}
	g.entries = append(g.entries, cachedGame{abs, stamp, db})
	return db
}

// dirStamp names every file of a game directory with its size and modification time.
// A directory is named alone: its own time changes on some systems (Windows) when its
// files are only read, and an added or removed file changes the names anyway.
func dirStamp(fsys fs.FS) (string, error) {
	var b bytes.Buffer
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			fmt.Fprintf(&b, "%s/\n", p)
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s\x00%d\x00%d\x00%v\n", p, info.Size(), info.ModTime().UnixNano(), info.Mode())
		return nil
	})
	return b.String(), err
}
