package play

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/nao1215/gozen/internal/battle"
)

// btlCaptureLine returns the start line, the first battle_capture (as a document) and the first
// result line of a trace.
func btlCaptureLine(t *testing.T, trace []byte) (start []byte, capture map[string]any, result []byte) {
	t.Helper()
	for ln := range bytes.SplitSeq(bytes.TrimSpace(trace), []byte("\n")) {
		switch {
		case bytes.Contains(ln, []byte(`"t":"start"`)):
			start = ln
		case bytes.Contains(ln, []byte(`"t":"battle_capture"`)) && capture == nil:
			if err := json.Unmarshal(ln, &capture); err != nil {
				t.Fatal(err)
			}
		case bytes.Contains(ln, []byte(`"t":"battle_result"`)) && result == nil:
			result = ln
		}
	}
	if start == nil || capture == nil || result == nil {
		t.Fatal("the trace lacks a start, a capture or a result")
	}
	return start, capture, result
}

// btlJoinLines writes lines (raw bytes or documents) as a trace.
func btlJoinLines(t *testing.T, lines ...any) []byte {
	t.Helper()
	var out []byte
	for _, l := range lines {
		if b, ok := l.([]byte); ok {
			out = append(out, b...)
		} else {
			b, err := json.Marshal(l)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, b...)
		}
		out = append(out, '\n')
	}
	return out
}

func btlCloneMap(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestBattleReplayRefusesDamagedCaptures: a capture whose random state or saved state cannot be
// restored, or whose own tick is not where it is written, is refused when the game is created.
func TestBattleReplayRefusesDamagedCaptures(t *testing.T) {
	trace := btlRecord(t, "one", "press ok 160\nquit\n")
	start, capture, result := btlCaptureLine(t, trace)
	for name, mutate := range map[string]func(c map[string]any, inner map[string]any){
		"random state": func(_, inner map[string]any) { inner["rand"] = "eA==" },
		"saved state":  func(_, inner map[string]any) { inner["save"] = "bm90IGEgc2F2ZQ==" },
		"no capture":   func(c, _ map[string]any) { delete(c, "capture") },
		"other tick":   func(_, inner map[string]any) { inner["tick"] = 5 },
	} {
		c := btlCloneMap(t, capture)
		inner := btlDoc(t, c["capture"])
		mutate(c, inner)
		path := btlWriteTrace(t, btlJoinLines(t, start, c, result))
		g, err := New(Options{FS: btlFiles(t, "", ""), NoSound: true, SaveDir: t.TempDir(), Replay: path, ReplayBattle: 1, Seed: 1})
		if err == nil {
			t.Errorf("%s: accepted", name)
			btlClose(t, g)
		}
	}
}

// TestBattleReplayReadsInputsAndActions: inputs after the capture are kept to feed the replay,
// inputs at or before it are left out, the auto command is kept as an action, and kinds a battle
// replay does not check are left out.
func TestBattleReplayReadsInputsAndActions(t *testing.T) {
	trace := btlRecord(t, "one", "press ok 160\nquit\n")
	start, capture, result := btlCaptureLine(t, trace)
	tick := int(btlNumber(t, capture["tick"]))
	lines := btlJoinLines(t, start, capture,
		map[string]any{"t": "input", "tick": tick, "press": []string{"ok"}},
		map[string]any{"t": "input", "tick": tick + 2, "press": []string{"ok"}, "held": []string{"ok"}},
		map[string]any{"t": "act", "tick": tick + 3, "name": "battle_auto"},
		map[string]any{"t": "scene", "tick": tick + 4, "id": "ignored"},
		result)
	g, err := New(Options{FS: btlFiles(t, "", ""), NoSound: true, SaveDir: t.TempDir(), Replay: btlWriteTrace(t, lines), ReplayBattle: 1, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer btlClose(t, g)
	r := g.rep
	if len(r.inputs) != 1 || r.inputs[tick+2] == nil || len(r.acts[tick+3]) != 1 {
		t.Fatalf("inputs %v acts %v", r.inputs, r.acts)
	}
	if len(r.expect["scene"]) != 0 {
		t.Fatal("a kind a battle replay does not check was kept")
	}
	if g.currentBattle() == nil || g.tick != tick {
		t.Fatalf("the battle did not start from the capture: tick %d, want %d", g.tick, tick)
	}
}

// TestBattleWithoutATrace: a battle with no trace writer plays through, and the trace helpers do nothing.
func TestBattleWithoutATrace(t *testing.T) {
	g := openGame(t, Options{Seed: 8, Location: "field", Party: []string{"scout", "guide"}})
	s := g.startBattle("one", func(int) {})
	g.traceBattleStep(battle.Step{Kind: battle.Message})
	g.traceBattleResult(s)
	g.traceBattleCapture("one")
	btlAttackUntilOver(t, g, s)
	if s.b.Result != battle.Won {
		t.Fatalf("result %v", s.b.Result)
	}
}
