package perf

import (
	"bytes"
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nao1215/gozen/internal/drive/headless/balance"
	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/screen/play"
)

func TestSpeedBudget(t *testing.T) {
	dir := t.TempDir()
	if err := Generate(dir, "large-sim", "../../../e2e/atago/fixtures/game/assets/regular.ttf"); err != nil {
		t.Fatal(err)
	}
	db, err := data.Load(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	result, err := balance.Sim(db, "lesson", 5000, 30, balance.FreshStart(db, 1, nil, balance.Prep{}), nil)
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if result.Battles != 5000 || result.Won != 5000 {
		t.Fatalf("large-sim did not exercise 5000 victories: %+v", result)
	}
	checkBudget(t, "large-sim", "allocations/battle", float64(after.Mallocs-before.Mallocs)/5000, 600)
	checkBudget(t, "large-sim", "heap bytes", float64(after.HeapInuse), 64<<20)
	checkBudget(t, "large-sim", "seconds", elapsed.Seconds(), 3)
}

func checkBudget(t *testing.T, section, metric string, got, limit float64) {
	t.Helper()
	t.Logf("section %s: %s = %.3f (limit %.3f)", section, metric, got, limit)
	if got > limit {
		t.Errorf("speed budget section %s: %s = %.3f exceeds %.3f", section, metric, got, limit)
	}
}

func TestConversationScriptFinishes(t *testing.T) {
	dir := t.TempDir()
	if err := Generate(dir, "long-conversation", "../../../e2e/atago/fixtures/game/assets/regular.ttf"); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	})
	script, err := root.ReadFile("input.txt")
	if err != nil {
		t.Fatal(err)
	}
	var trace bytes.Buffer
	result, err := play.Simulate(t.Context(), play.Options{FS: os.DirFS(dir), Location: "room", Event: "workload", Script: string(script), SaveDir: t.TempDir(), Trace: &trace, Seed: 1}, play.SimulateConfig{})
	if err != nil {
		t.Fatal(err)
	}
	says := 0
	completed := false
	for line := range strings.Lines(trace.String()) {
		var record struct {
			Kind  string `json:"t"`
			State struct {
				Gold int `json:"gold"`
			} `json:"state"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if record.Kind == "end" {
			completed = record.State.Gold == 1
		}
		if record.Kind == "say" {
			says++
		}
	}
	if !completed || says != 200 || result.Ticks >= 6000 {
		t.Fatalf("read %d lines in %d ticks, completed=%t", says, result.Ticks, completed)
	}
	t.Logf("read all %d lines in %d ticks", says, result.Ticks)
}
