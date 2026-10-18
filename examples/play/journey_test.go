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
	if restored.Location != "ridge" || !restored.Flags["complete"] || restored.Gold != 7 {
		t.Fatalf("restored progress: location=%s gold=%d flags=%v", restored.Location, restored.Gold, restored.Flags)
	}
	for _, loc := range []string{"camp", "ridge", "camp", "chamber"} {
		if err := driver.GoTo(loc); err != nil {
			t.Fatal(err)
		}
	}
	if restored.Gold != 7 {
		t.Fatal("arrival reward repeated after loading")
	}
	if err := driver.Walk('L'); err != nil {
		t.Fatal(err)
	}
	if !restored.Flags["window_seen"] || restored.Pos.X != 5 || restored.Pos.Y != 1 {
		t.Fatalf("room walk did not reach the lantern: pos=%+v flags=%v", restored.Pos, restored.Flags)
	}
	if err := driver.Walk('C'); err != nil {
		t.Fatal(err)
	}
	if restored.Location != "camp" {
		t.Fatal("room has no working return route")
	}
}
