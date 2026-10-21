package play

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// What a replay does when the record is not what the play makes: direct actions written in the
// record are done at their tick, an extra line or a missing one is reported, and a replay stopped
// early says it did not reach the end.

func wldSplitTrace(tr []byte) []string {
	return strings.Split(strings.TrimRight(string(tr), "\n"), "\n")
}

// wldJoinTrace joins lines into a trace, with the end line counting them (a record whose end
// does not count its lines is refused).
func wldJoinTrace(ls []string) []byte {
	last := len(ls) - 1
	i := strings.Index(ls[last], `"lines":`)
	j := i + len(`"lines":`)
	k := j
	for k < len(ls[last]) && ls[last][k] >= '0' && ls[last][k] <= '9' {
		k++
	}
	ls = append(append([]string{}, ls[:last]...), ls[last][:j]+strconv.Itoa(len(ls))+ls[last][k:])
	return []byte(strings.Join(ls, "\n") + "\n")
}

func TestWldReplayDoesTheDirectActionsOfTheRecord(t *testing.T) {
	tr := wldRecord(t, wldRecordScript)
	lines := wldSplitTrace(tr)
	var out []string
	done := false
	for _, l := range lines {
		var tick int
		at := strings.Index(l, `"tick":`)
		if at < 0 {
			t.Fatalf("no tick in %s", l)
		}
		if _, err := fmt.Sscanf(l[at:], `"tick":%d`, &tick); err != nil {
			t.Fatal(err)
		}
		if !done && tick > 20 {
			out = append(out, `{"t":"act","tick":20,"name":"buy","args":{"item":"potion","count":1,"price":4}}`)
			done = true
		}
		out = append(out, l)
	}
	g, err := wldReplayGame(t, wldJoinTrace(out), func(o *Options) { o.Until = 100 })
	if err != nil {
		t.Fatal(err)
	}
	potions, gold := 0, 0
	if g.st != nil {
		potions, gold = g.st.Items["potion"], g.st.Gold
	}
	report := wldReplay(t, g)
	if !strings.HasPrefix(report, "replay range matched:") {
		t.Fatalf("report %q", report)
	}
	if g.st.Gold != gold-4 || g.st.Items["potion"] != potions+1 {
		t.Fatalf("gold %d -> %d, potions %d -> %d: the act at tick 20 was not done", gold, g.st.Gold, potions, g.st.Items["potion"])
	}
}

func TestWldReplayReportsAnExtraLineAndALineNotReached(t *testing.T) {
	tr := wldRecord(t, wldRecordScript)
	lines := wldSplitTrace(tr)
	last := -1
	for i, l := range lines {
		if strings.HasPrefix(l, `{"t":"say"`) {
			last = i
		}
	}
	if last < 0 {
		t.Fatal("no say line")
	}
	// the record lacks the last line the play says
	g, err := wldReplayGame(t, wldJoinTrace(append(append([]string{}, lines[:last]...), lines[last+1:]...)), nil)
	if err != nil {
		t.Fatal(err)
	}
	if report := wldReplay(t, g); !strings.HasPrefix(report, "replay diverged: tick ") || !strings.Contains(report, "an extra say line (line 4)") {
		t.Fatalf("report %q", report)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	// the record has a line the play never says
	more := append(append([]string{}, lines[:last+1]...), lines[last])
	more = append(more, lines[last+1:]...)
	g, err = wldReplayGame(t, wldJoinTrace(more), nil)
	if err != nil {
		t.Fatal(err)
	}
	if report := wldReplay(t, g); !strings.HasPrefix(report, "replay diverged:") || !strings.Contains(report, "1 expected say lines were not reached") {
		t.Fatalf("report %q", report)
	}
}

func TestWldReplayStoppedEarlySaysItDidNotReachTheEnd(t *testing.T) {
	tr := wldRecord(t, wldRecordScript)
	g, err := wldReplayGame(t, tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	wldFrames(t, g, 30)
	g.traceEnd()
	if report := ReplayReport; !strings.HasPrefix(report, "replay diverged: did not reach the end (30 / ") {
		t.Fatalf("report %q", report)
	}
	// a report asked for before the final check says so
	r := &replay{}
	if got := r.report(); !strings.Contains(got, "the final check did not run") {
		t.Fatalf("report %q", got)
	}
	var buf bytes.Buffer
	_ = buf
}
