package play

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/event"
)

const sysChain = `events:
  a:
    do:
      - say: "A1"
      - say: "A2"
      - set: {flag.a: true}
  b:
    trigger: auto
    once: true
    when: "flag.a"
    do:
      - say: "B1"
      - say: "B2"
      - set: {flag.b: true}
  c:
    trigger: auto
    once: true
    when: "flag.b"
    do:
      - say: "C1"
      - say: "C2"
`

// TestRollbackRingKeepsOnlyTheSizeAsked: with ui.rollback: 1 only the last event before the one
// running can be rolled back into.
func TestRollbackRingKeepsOnlyTheSizeAsked(t *testing.T) {
	g := sysOpen4(t, Options{Seed: 1, Location: "camp", Event: "a", Line: 1, Script: "read 3\nread 3\nread 3\nread 3\nread 3\nwait 20\nquit"},
		map[string]string{"data/chain.yaml": sysChain, "game.yaml+": "ui: {rollback: 1}\n"})
	got := sysPlayLines(t, g, 4000)
	if _, ok := got["C1"]; !ok {
		t.Fatalf("never reached C1: %v", got)
	}
	if len(g.backRing) > 1 {
		t.Fatalf("the ring kept %d events with ui.rollback: 1", len(g.backRing))
	}
	if len(g.backRing) == 1 && g.backRing[0].name != "b" {
		t.Fatalf("the ring kept %q, want the event just before", g.backRing[0].name)
	}
}

// TestRollBackRefusesWhenItCannot: nothing happens, nothing is traced, when there is no line to
// go back to or the saved start cannot be read.
func TestRollBackRefusesWhenItCannot(t *testing.T) {
	var trace bytes.Buffer
	g := sysOpen4(t, Options{Seed: 1, Location: "camp", Event: "a", Line: 1, Trace: &trace}, map[string]string{"data/chain.yaml": sysChain})
	sysRunFast(t, g, 500)
	before := g.evLines
	g.rollBack()
	if g.evLines != before {
		t.Fatal("rolled back from the first line")
	}
	if g.replayEvent(backEvent{snap: []byte("junk")}) {
		t.Fatal("replayed an event from a damaged state")
	}
	if rb := traceLines(t, trace.Bytes(), "rollback"); len(rb) != 0 {
		t.Fatalf("rollback traced %v", rb)
	}
}

// TestSaveInAnArrivalEventResumesTheArrival: a save made in the lines of an arrival goes on in the
// arrival again, at that line.
func TestSaveInAnArrivalEventResumesTheArrival(t *testing.T) {
	g := sysOpen4(t, Options{Seed: 1, Location: "camp"}, nil)
	g.Move("field")
	for range 300 {
		sysFrames(t, g, 1)
		if g.talk.active {
			break
		}
	}
	if !g.talk.active || g.talk.say.Text != "The trail opens." {
		t.Fatalf("arrival line %q (active %v)", g.talk.say.Text, g.talk.active)
	}
	if err := g.save(0); err != nil {
		t.Fatal(err)
	}
	b, err := g.store.Read(g.slotPath(0))
	if err != nil || !bytes.Contains(b, []byte(`"arrival": true`)) {
		t.Fatalf("no arrival cursor in %s (%v)", b, err)
	}
	g.Move("camp")
	st, err := g.load(0)
	if err != nil || st.Resume == nil || !st.Resume.Cursor.Arrival {
		t.Fatalf("load: %v %+v", err, st.Resume)
	}
	g.loadSavedGame(st)
	for range 600 {
		sysFrames(t, g, 1)
		if g.talk.active {
			break
		}
	}
	if g.st.Location != "field" || !g.talk.active || g.talk.say.Text != "The trail opens." {
		t.Fatalf("resumed at %s with %q", g.st.Location, g.talk.say.Text)
	}
}

// TestSaveWithAVanishedEventStartsWhereItBegan: if the data changed and the event of the cursor
// is gone, the game loads without it.
func TestSaveWithAVanishedEventStartsWhereItBegan(t *testing.T) {
	g := sysOpen4(t, Options{Seed: 1, Location: "camp", Event: "a", Line: 2}, map[string]string{"data/chain.yaml": sysChain})
	sysRunFast(t, g, 500)
	if err := g.save(0); err != nil {
		t.Fatal(err)
	}
	b, err := g.store.Read(g.slotPath(0))
	if err != nil || !bytes.Contains(b, []byte(`"event": "a"`)) {
		t.Fatalf("save %s (%v)", b, err)
	}
	if err := g.store.Write(g.slotPath(1), bytes.Replace(b, []byte(`"event": "a"`), []byte(`"event": "gone"`), 1)); err != nil {
		t.Fatal(err)
	}
	st, err := g.load(1)
	if err != nil {
		t.Fatal(err)
	}
	g.loadSavedGame(st)
	sysFrames(t, g, 5)
	if g.talk.active && g.evName == "a" {
		t.Fatal("the vanished event was resumed")
	}
}

func sysReadGame(t *testing.T, dir string) *Game {
	t.Helper()
	return sysOpen4(t, Options{Seed: 1, Location: "camp", SaveDir: dir}, nil)
}

// TestReadRecordIsNeverOverwrittenWhenItCannotBeUnderstood: another game's record, a newer
// format, damaged JSON and unknown fields all stay on disk as they are, and the game plays on.
func TestReadRecordIsNeverOverwrittenWhenItCannotBeUnderstood(t *testing.T) {
	for name, content := range map[string]string{
		"damaged":        "{broken",
		"null":           "null",
		"other game":     `{"format_version": 1, "game_id": "someone-else", "keys": [1, 2]}`,
		"newer format":   `{"format_version": 9, "game_id": "screen-test", "keys": [1]}`,
		"unknown field":  `{"format_version": 1, "game_id": "screen-test", "keys": [1], "extra": true}`,
		"no game id":     `{"format_version": 1, "keys": [1]}`,
		"no keys":        `{"format_version": 1, "game_id": "screen-test"}`,
		"damaged legacy": `[1, 2,`,
	} {
		dir := t.TempDir()
		path := filepath.Join(dir, "read.json")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		g := sysReadGame(t, dir)
		if g.readErr == nil {
			t.Errorf("%s: understood a record it should not", name)
		}
		g.markRead(event.Say{Text: "a new line"})
		g.flushRead()
		if b := sysRead(t, path); string(b) != content {
			t.Errorf("%s: the record was overwritten with %q", name, b)
		}
		if g.isRead(event.Say{Text: "a new line"}) != true {
			t.Errorf("%s: the line is not remembered during this play", name)
		}
		if err := g.Close(); err != nil {
			t.Errorf("%s: close: %v", name, err)
		}
	}
}

// TestLegacyReadRecordIsKeptAsABackup: a bare list of keys is read, and the next write keeps
// the old file beside the new one.
func TestLegacyReadRecordIsKeptAsABackup(t *testing.T) {
	dir := t.TempDir()
	line := event.Say{Name: "Courier", Text: "Hello"}
	legacy := "[" + strconv.FormatUint(legacyKey(line), 10) + "]"
	if err := os.WriteFile(filepath.Join(dir, "read.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	g := sysReadGame(t, dir)
	if g.readErr != nil || !g.isRead(line) {
		t.Fatalf("legacy record not read: %v", g.readErr)
	}
	g.markRead(event.Say{Text: "another"})
	g.flushRead()
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	entries := sysReadDir(t, dir)
	var backups int
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "read.json.before-") {
			backups++
		}
	}
	if backups != 1 {
		t.Fatalf("%d backups in %v", backups, entries)
	}
	b := sysRead(t, filepath.Join(dir, "read.json"))
	if !strings.Contains(string(b), `"format_version":1`) {
		t.Fatalf("the record was not written in the new format: %s", b)
	}
}

// TestReadRecordSurvivesARestart: lines marked read in one play are read in the next.
func TestReadRecordSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	g := sysReadGame(t, dir)
	s := event.Say{Name: "Courier", Text: "Gold 100.", Key: 12345}
	if g.isRead(s) {
		t.Fatal("read before it was shown")
	}
	g.markRead(s)
	g.markKey(777)
	g.flushRead()
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	g2 := sysReadGame(t, dir)
	if !g2.isRead(s) || !g2.read[777] {
		t.Fatal("the record was lost on restart")
	}
	if g2.isRead(event.Say{Text: "never shown", Key: 99}) {
		t.Fatal("an unseen line counts as read")
	}
}

// TestTrialRunKeepsNoReadRecord: a script run without a save directory neither reads nor writes
// the player's record.
func TestTrialRunKeepsNoReadRecord(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	g := sysOpen(t, Options{Location: "camp", SaveDir: "", Script: "wait 1\nquit", Store: nil}, nil)
	g.storeGiven = false
	if g.keepsRead() {
		t.Fatal("a trial run keeps a read record")
	}
	g.loadRead()
	if len(g.read) != 0 {
		t.Fatal("read lines in a trial run")
	}
}
