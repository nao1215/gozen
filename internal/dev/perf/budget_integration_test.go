package perf

import (
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/nao1215/gozen/internal/headless/balance"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
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
