package main

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/headless/balance"
	"github.com/nao1215/gozen/internal/rpg"
	data "github.com/nao1215/gozen/schema"
)

// gozen sim battle --state counts from the memo's .save with the same function and seed as the dev balance display (F7).
func TestSimFromSavedStateMatchesTheShared(t *testing.T) {
	dir := fixture(t)
	db, err := data.Load(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	st := rpg.New(db, rand.New(rand.NewPCG(4, 4))) //nolint:gosec // Deterministic test.
	st.Members()[0].HP = 1
	save, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "note.save")
	if err := os.WriteFile(path, save, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		level int
		asIs  bool
	}{{0, false}, {3, false}, {0, true}} {
		args := []string{"sim", "battle", dir, "lesson", "--state", path, "-n", "40", "--seed", "6", "--level", strconv.Itoa(tc.level), "--hp", "--streak", "5"}
		if tc.asIs {
			args = append(args, "--as-is")
		}
		out, _ := invoke(t, 0, args...)
		start := balance.SavedStart(db, save, 6, balance.Prep{Level: tc.level, Tree: true, AsIs: tc.asIs})
		o, err := balance.Sim(db, "lesson", 40, 60, start, nil)
		if err != nil {
			t.Fatal(err)
		}
		want := fmt.Sprintf("win %.0f%%  lose %.0f%%", 100*o.WinRate(), pct(o.Lost, 40))
		if !strings.Contains(out, want) || !strings.Contains(out, fmt.Sprintf("avg turns %.1f", o.AvgTurns())) {
			t.Fatalf("%+v: %q want %q", tc, out, want)
		}
		if lo, mid, hi, ok := o.HPBand(); ok && !strings.Contains(out, fmt.Sprintf("p10 %.0f%%  median %.0f%%  p90 %.0f%%", 100*lo, 100*mid, 100*hi)) {
			t.Fatalf("band missing: %q", out)
		}
		wins, err := balance.Streak(db, "lesson", 40, 60, 5, start)
		if err != nil {
			t.Fatal(err)
		}
		if line := fmt.Sprintf("(up to 5): p10 %d  median %d  p90 %d", balance.Percentile(wins, 0.1), balance.Percentile(wins, 0.5), balance.Percentile(wins, 0.9)); !strings.Contains(out, line) {
			t.Fatalf("%+v: %q want %q", tc, out, line)
		}
	}
	invoke(t, 2, "sim", "battle", dir, "lesson", "--as-is")
	invoke(t, 2, "sim", "battle", dir, "lesson", "--state", path, "--streak", "-1")
	invoke(t, 2, "sim", "battle", dir, "lesson", "--state", path, "--party", "scout")
	invoke(t, 1, "sim", "battle", dir, "lesson", "--state", filepath.Join(t.TempDir(), "absent.save"))
}
