package play

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestCaptureSetupRunsAfterGalleryDefaults(t *testing.T) {
	for _, scene := range []bool{false, true} {
		t.Run(map[bool]string{false: "location", true: "gallery"}[scene], func(t *testing.T) {
			fsys := gameFiles(t)
			fsys["game.yaml"].Data = append(append([]byte("schema_version: 2\n"), fsys["game.yaml"].Data...), []byte("\ngallery:\n  scenes:\n    - title: Opening\n      event: greet\n      flag: seen\n      set: {gold: 9}\n")...)
			opts := Options{FS: fsys, NoSound: true, Seed: 7, Location: "camp", capture: &captureStartup{setup: "- set: {gold: 42}"}}
			if scene {
				opts.Location, opts.Scene = "", "Opening"
			}
			g, err := New(opts)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := g.Close(); err != nil {
					t.Error(err)
				}
			}()
			if g.st.Gold != 42 {
				t.Fatalf("setup lost: gold = %d", g.st.Gold)
			}
		})
	}
}

func TestCaptureSetupRejectsInputAndInvalidCommands(t *testing.T) {
	for _, src := range []string{"- say: Wait for input", "- unknown: true", "- set: {gold: 'missing_name + 1'}"} {
		g, err := New(Options{FS: gameFiles(t), NoSound: true, Location: "camp", capture: &captureStartup{setup: src}})
		if g != nil {
			if err := g.Close(); err != nil {
				t.Error(err)
			}
		}
		if err == nil {
			t.Errorf("accepted setup %q", src)
		}
	}
}

func TestCaptureRejectsInvalidJobsBeforeWriting(t *testing.T) {
	for _, job := range []CaptureJob{
		{ID: "../escape", Start: "camp"},
		{ID: "shot", Start: "camp", Script: "shot extra"},
		{ID: "note", Start: "camp", Script: "press memo"},
		{ID: "wait", Start: "camp", Script: "wait -1"},
		{ID: "syntax", Start: "camp", Script: "missing-command"},
		{ID: "size", Start: "camp", GIF: &CaptureGIF{Width: 1281}},
		{ID: "setup", Start: "camp", Setup: "- unknown: true"},
	} {
		t.Run(job.ID, func(t *testing.T) {
			out := t.TempDir()
			_, err := Capture(context.Background(), gameFiles(t), []CaptureJob{job}, CaptureConfig{Output: out, Timeout: time.Second})
			if err == nil {
				t.Fatal("accepted invalid job")
			}
			entries, err := os.ReadDir(out)
			if err != nil || len(entries) != 0 {
				t.Fatalf("invalid job wrote output: %v, %v", entries, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Capture(ctx, nil, nil, CaptureConfig{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled capture: %v", err)
	}
}

func TestCaptureScriptPreparationBoundary(t *testing.T) {
	j := CaptureJob{ID: "test", Script: "press ok", WaitTicks: 2, GIF: &CaptureGIF{Script: "press right"}}
	s, err := parseScript(captureScript(j))
	if err != nil {
		t.Fatal(err)
	}
	prep, err := parseScript(capturePreparation(j))
	if err != nil {
		t.Fatal(err)
	}
	var in input
	for range 20 {
		if s.pc >= len(prep.cmds) && s.wait == 0 {
			break
		}
		s.step(&in)
	}
	if s.pc != len(prep.cmds) || s.quit || s.cmds[s.pc].arg != "right" {
		t.Fatalf("animation started during preparation: %+v", s)
	}
}
