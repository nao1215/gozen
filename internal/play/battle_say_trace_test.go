package play

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// TestBattleLinesAreTraced: the lines, the notice and the choice of a troop event are written
// to the trace as say and notice lines, marked as said in a battle, as the lines of an event
// outside a battle are.
func TestBattleLinesAreTraced(t *testing.T) {
	g, out := btlOpen(t, Options{Battle: "chatty"}, "", "")
	btlTalk(t, g, "the choice made", func() bool { return g.st.Flags["left"] || g.st.Flags["right"] })
	var said []string
	for _, l := range traceLines(t, out.Bytes(), "say") {
		if l["in_battle"] != true {
			t.Errorf("a battle line is not marked: %v", l)
		}
		said = append(said, fmt.Sprint(l["text"]))
	}
	text := strings.Join(said, "|")
	if !strings.Contains(text, "Guide line") || !strings.Contains(text, "Lost line") {
		t.Fatalf("battle lines missing from the trace: %q", text)
	}
	if ls := traceLines(t, out.Bytes(), "say"); len(ls) < 3 || ls[len(ls)-1]["choices"] == nil {
		t.Fatalf("the battle choice is not traced: %v", ls)
	}
	notice := false
	for _, l := range traceLines(t, out.Bytes(), "notice") {
		notice = notice || l["text"] == "Heads up"
	}
	if !notice {
		t.Fatal("the battle notice is not traced")
	}
}

// TestBattleLinesStopSkip: skip mode stops before a battle line that was not read, as it does
// before such a line outside a battle, so the trace never shows one passed while skipping.
func TestBattleLinesStopSkip(t *testing.T) {
	g, out := btlOpen(t, Options{Battle: "chatty"}, "", "")
	g.skipping = true
	btlUntil(t, g, "the first battle line", 4000, func() bool { return g.talk.active })
	if g.skipping {
		t.Fatal("skip mode went on over a battle line not read")
	}
	if ls := traceLines(t, out.Bytes(), "skip"); len(ls) == 0 || ls[len(ls)-1]["why"] != "unread" {
		t.Fatalf("skip lines %v", ls)
	}
}

// TestBattleReplayChecksBattleLines: a battle with troop lines replays to a match that counts
// the lines; a changed line diverges; and a trace written before battle lines were traced
// (with none of them) still replays.
func TestBattleReplayChecksBattleLines(t *testing.T) {
	covSlow(t)
	trace := btlRecord(t, "chatty", "gap 4\npress ok 400\nquit\n")
	if len(traceLines(t, trace, "say")) == 0 {
		t.Fatal("the recorded battle has no traced line")
	}
	if report := btlReplay(t, btlWriteTrace(t, trace), 1); strings.Contains(report, "diverged") {
		t.Fatalf("report %q", report)
	}
	changed := bytes.Replace(trace, []byte("Lost line"), []byte("Lost lime"), 1)
	if report := btlReplay(t, btlWriteTrace(t, changed), 1); !strings.Contains(report, "diverged") {
		t.Fatalf("a changed battle line was accepted: %q", report)
	}
	var old [][]byte
	for ln := range bytes.SplitSeq(bytes.TrimSpace(trace), []byte("\n")) {
		if bytes.Contains(ln, []byte(`"in_battle":true`)) {
			continue
		}
		old = append(old, ln)
	}
	if report := btlReplay(t, btlWriteTrace(t, bytes.Join(old, []byte("\n"))), 1); strings.Contains(report, "diverged") {
		t.Fatalf("an older trace without battle lines diverged: %q", report)
	}
}
