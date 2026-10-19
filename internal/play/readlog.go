package play

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"

	"github.com/nao1215/gozen/event"
	"github.com/nao1215/gozen/savefile"
)

func lineKey(s event.Say) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s.Name))
	h.Write([]byte{0})
	h.Write([]byte(s.Text))
	return h.Sum64()
}

func (g *Game) readPath() string { return filepath.Join(g.saveDir(), "read.json") }

func (g *Game) keepsRead() bool { return g.opts.SaveDir != "" || !g.testRun() }

func (g *Game) loadRead() {
	g.read = map[uint64]bool{}
	if !g.keepsRead() {
		return
	}
	b, err := os.ReadFile(g.readPath())
	if err != nil {
		return
	}
	var keys []uint64
	if json.Unmarshal(b, &keys) == nil {
		for _, k := range keys {
			g.read[k] = true
		}
	}
}

func (g *Game) isRead(s event.Say) bool { return g.read[lineKey(s)] }

func (g *Game) markRead(s event.Say) {
	if g.read == nil {
		g.read = map[uint64]bool{}
	}
	k := lineKey(s)
	if !g.read[k] {
		g.read[k] = true
		g.readDirty = true
	}
}

func (g *Game) flushRead() {
	if !g.readDirty || !g.keepsRead() {
		return
	}
	keys := make([]uint64, 0, len(g.read))
	for k := range g.read {
		keys = append(keys, k)
	}
	b, err := json.Marshal(keys)
	if err == nil {
		if err = os.MkdirAll(g.saveDir(), 0o700); err == nil {
			err = savefile.Write(g.readPath(), b)
		}
	}
	if err != nil {
		g.failure = errors.Join(g.failure, fmt.Errorf("read progress: %w", err))
		return
	}
	g.readDirty = false
}

func (g *Game) testRun() bool {
	return g.auto != nil || g.in.script != nil || g.rep != nil || g.opts.Location != "" || len(g.opts.Party) > 0
}
