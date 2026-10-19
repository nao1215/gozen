package main

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/data"
	"github.com/nao1215/gozen/headless"
	"github.com/nao1215/gozen/rpg"
	"github.com/nao1215/gozen/savefile"
)

func TestJourneyAndSaveRoundTrip(t *testing.T) {
	fsys, err := gameFS()
	if err != nil {
		t.Fatal(err)
	}
	db, err := data.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	state := rpg.New(db, rand.New(rand.NewPCG(7, 11))) //nolint:gosec // Fixed gameplay randomness for a reproducible journey.
	driver := headless.New(db, state)
	if err := driver.Start(); err != nil {
		t.Fatal(err)
	}
	if driver.Has("Walk to the ridge") || driver.Has("Light the marker") {
		t.Fatal("progression opened before practice")
	}
	if err := driver.Do("Courier"); err != nil {
		t.Fatal(err)
	}
	if !state.Flags["briefed"] || !strings.Contains(strings.Join(driver.Transcript, "\n"), "A little light") {
		t.Fatal("conversation did not run")
	}
	if err := driver.Do("Practice"); err != nil {
		t.Fatal(err)
	}
	if driver.Battles != 1 || driver.Wins != 1 || !state.Flags["trained"] || state.Gold != 3 {
		t.Fatalf("practice: battles=%d wins=%d gold=%d flags=%v", driver.Battles, driver.Wins, state.Gold, state.Flags)
	}
	if err := driver.Do("Light the marker"); err != nil {
		t.Fatal(err)
	}
	if driver.Has("Light the marker") || !driver.Has("Walk to the ridge") {
		t.Fatal("lighting the marker did not change available actions")
	}
	if err := driver.GoTo("ridge"); err != nil {
		t.Fatal(err)
	}
	if !state.Flags["complete"] || state.Gold != 7 {
		t.Fatalf("arrival reward: gold=%d flags=%v", state.Gold, state.Flags)
	}
	if got := state.Party; len(got) != 2 || got[1] != "sela" {
		t.Fatalf("Sela did not join at the ridge: party=%v", got)
	}
	// Use the same versioned save bytes as the screen, with isolated disk storage.
	encoded, err := state.Save()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := savefile.Write(filepath.Join(dir, "slot.json"), encoded); err != nil {
		t.Fatal(err)
	}
	encoded, err = savefile.Load(os.DirFS(dir), "slot.json")
	if err != nil {
		t.Fatal(err)
	}
	restored, err := rpg.Parse(db, encoded, nil)
	if err != nil {
		t.Fatal(err)
	}
	driver.Restore(restored)
	if restored.Location != "ridge" || !restored.Flags["complete"] || restored.Gold != 7 || len(restored.Party) != 2 {
		t.Fatalf("restored progress: location=%s gold=%d party=%v flags=%v", restored.Location, restored.Gold, restored.Party, restored.Flags)
	}
	for _, loc := range []string{"camp", "ridge", "camp", "town"} {
		if err := driver.GoTo(loc); err != nil {
			t.Fatal(err)
		}
	}
	if err := driver.Do("Bram"); err != nil {
		t.Fatal(err)
	}
	if got := restored.Party; len(got) != 3 || got[2] != "bram" || driver.Has("Bram") {
		t.Fatalf("Bram did not join in the village: party=%v", got)
	}
	for _, loc := range []string{"camp", "chamber"} {
		if err := driver.GoTo(loc); err != nil {
			t.Fatal(err)
		}
	}
	if restored.Gold != 7 {
		t.Fatal("arrival reward repeated after loading")
	}
	// The halls: a chest in a dead end, a sentry fight, a wheel that opens the gate to the lantern.
	if err := driver.Walk('T'); err != nil {
		t.Fatal(err)
	}
	if !restored.Flags["chest_opened"] || restored.Gold != 12 {
		t.Fatalf("chest: gold=%d flags=%v", restored.Gold, restored.Flags)
	}
	if err := driver.Walk('L'); err == nil {
		t.Fatal("the lantern was reachable before the sentries and the gate")
	}
	battles := driver.Battles
	if err := driver.Walk('P'); err != nil {
		t.Fatal(err)
	}
	if driver.Battles != battles+1 || !restored.Flags["sentries_passed"] || restored.Gold != 18 {
		t.Fatalf("sentries: battles=%d gold=%d flags=%v", driver.Battles, restored.Gold, restored.Flags)
	}
	if err := driver.Walk('L'); err == nil {
		t.Fatal("the lantern was reachable before the wheel")
	}
	if err := driver.Walk('K'); err != nil {
		t.Fatal(err)
	}
	if !restored.Flags["gate_open"] {
		t.Fatal("the wheel did not open the gate")
	}
	// Saving in the halls keeps the spent chest, the passed sentries and the open gate.
	encoded, err = restored.Save()
	if err != nil {
		t.Fatal(err)
	}
	inHalls, err := rpg.Parse(db, encoded, nil)
	if err != nil {
		t.Fatal(err)
	}
	driver.Restore(inHalls)
	if inHalls.Location != "chamber" || inHalls.Gold != 18 || !inHalls.Flags["gate_open"] {
		t.Fatalf("restored halls: location=%s gold=%d flags=%v", inHalls.Location, inHalls.Gold, inHalls.Flags)
	}
	if err := driver.Walk('T'); err == nil {
		t.Fatal("the opened chest was found again after loading")
	}
	if err := driver.Walk('L'); err != nil {
		t.Fatal(err)
	}
	if !inHalls.Flags["window_seen"] || inHalls.Pos.X != 13 || inHalls.Pos.Y != 9 || inHalls.Gold != 18 {
		t.Fatalf("halls walk did not reach the lantern: pos=%+v gold=%d flags=%v", inHalls.Pos, inHalls.Gold, inHalls.Flags)
	}
	if err := driver.Walk('C'); err != nil {
		t.Fatal(err)
	}
	if inHalls.Location != "camp" {
		t.Fatal("the halls have no working return route")
	}
}
