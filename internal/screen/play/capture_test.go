package play

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/gif"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
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
		t.Fatalf("canceled capture: %v", err)
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

func TestCapturePreparationBurstPreservesTicksAndSamplingBoundary(t *testing.T) {
	g, err := New(Options{FS: gameFiles(t), Location: "camp", NoSound: true, ShotDir: t.TempDir(), Seed: 17, Script: "wait 1000\nwait 2147483647\n"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	b := &captureBatch{ctx: context.Background(), game: g, jobs: []CaptureJob{{ID: "neutral"}}, next: 1, prepare: 1}
	if err := b.Update(); err != nil {
		t.Fatal(err)
	}
	if b.tick != 60 || g.tick != 60 || b.sampling {
		t.Fatalf("preparation skipped ticks: %d/%d %v", b.tick, g.tick, b.sampling)
	}
	for !b.sampling {
		if err := b.Update(); err != nil {
			t.Fatal(err)
		}
	}
	tick := g.tick
	if !b.due {
		t.Fatal("sampling did not wait for a draw")
	}
	if err := b.Update(); err != nil {
		t.Fatal(err)
	}
	if g.tick != tick {
		t.Fatal("advanced across a pending sample")
	}
}

// Batch bookkeeping and publication are tested without entering a graphics loop.
func TestCaptureBatchFailedJobsContinueInOrder(t *testing.T) {
	b := &captureBatch{ctx: context.Background(), fsys: gameFiles(t), cfg: CaptureConfig{Output: t.TempDir(), Timeout: time.Second}, jobs: []CaptureJob{
		{ID: "bad-script", Script: "unknown-command"},
		{ID: "bad-location", Start: "absent"},
		{ID: "valid", Start: "camp", Seed: 17},
	}}
	for range 3 {
		if err := b.updateTick(); err != nil {
			t.Fatal(err)
		}
	}
	if len(b.results) != 2 || b.results[0].ID != "bad-script" || b.results[1].ID != "bad-location" {
		t.Fatalf("failed jobs lost their order: %+v", b.results)
	}
	for _, r := range b.results {
		if r.Err == nil || r.Artifact != "" || r.Seed != 1 {
			t.Errorf("invalid failed result: %+v", r)
		}
	}
	if b.game == nil || b.result.Seed != 17 {
		t.Fatal("failed jobs prevented the next job from starting")
	}
	b.game.cancel()
	if err := b.updateTick(); err != nil {
		t.Fatal(err)
	}
	if b.game != nil || len(b.results) != 3 || !errors.Is(b.results[2].Err, context.Canceled) {
		t.Fatalf("canceled job not finalized: %+v", b.results)
	}
	if err := b.updateTick(); !errors.Is(err, ebiten.Termination) {
		t.Fatalf("completed batch: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b.ctx = ctx
	if err := b.Update(); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled batch: %v", err)
	}
}

func TestCaptureBatchFinishPublication(t *testing.T) {
	for _, mode := range []string{"gif", "empty-gif", "write-failure", "failed-png", "cleanup-failure"} {
		t.Run(mode, func(t *testing.T) {
			out := t.TempDir()
			b := &captureBatch{ctx: context.Background(), fsys: gameFiles(t), cfg: CaptureConfig{Output: out, Timeout: time.Second}, jobs: []CaptureJob{{ID: "neutral", Start: "camp"}}}
			b.begin()
			if b.game == nil {
				t.Fatalf("begin: %+v", b.results)
			}
			w, h := b.Layout(0, 0)
			if w <= 0 || h <= 0 {
				t.Fatalf("layout %dx%d", w, h)
			}
			b.pictures = map[string]bool{"z.png": true, "a.png": true}
			var failure error
			name := filepath.Join(out, "neutral.png")
			if strings.Contains(mode, "gif") || mode == "write-failure" {
				var err error
				b.frames, err = newCaptureFrames(CaptureGIF{FPS: 2, Duration: time.Second, Width: 4}, 4, 3)
				if err != nil {
					t.Fatal(err)
				}
				if mode != "empty-gif" {
					for range b.frames.count {
						b.frames.add(image.NewRGBA(image.Rect(0, 0, 4, 3)))
					}
					b.result.Frames = len(b.frames.Image)
				}
				name = filepath.Join(out, "neutral.gif")
			}
			if mode == "write-failure" {
				b.cfg.Output = filepath.Join(out, "missing")
			}
			if mode == "failed-png" {
				if err := os.WriteFile(name, []byte("partial"), 0o600); err != nil {
					t.Fatal(err)
				}
				failure = errors.New("sampling failed")
			}
			if mode == "cleanup-failure" {
				if err := os.Mkdir(name, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(name, "child"), nil, 0o600); err != nil {
					t.Fatal(err)
				}
				failure = errors.New("sampling failed")
			}
			b.finish(failure)
			if b.game != nil || b.frames != nil || len(b.results) != 1 {
				t.Fatal("finished job retained resources or lost its result")
			}
			r := b.results[0]
			if !reflect.DeepEqual(r.Pictures, []string{"a.png", "z.png"}) {
				t.Fatalf("picture order: %v", r.Pictures)
			}
			if mode != "gif" {
				if r.Err == nil || r.Artifact != "" {
					t.Fatalf("failed output reported success: %+v", r)
				}
				if failure != nil && !errors.Is(r.Err, failure) {
					t.Fatalf("lost sampling error: %v", r.Err)
				}
				if mode != "cleanup-failure" {
					if _, err := os.Stat(name); !errors.Is(err, fs.ErrNotExist) {
						t.Fatalf("partial artifact remains: %v", err)
					}
				} else if !strings.Contains(r.Err.Error(), "remove") {
					t.Fatalf("lost cleanup error: %v", r.Err)
				}
				return
			}
			if r.Err != nil || r.Artifact != "neutral.gif" || r.Frames != 2 {
				t.Fatalf("GIF publication: %+v", r)
			}
			f, err := os.Open(name) //nolint:gosec // Test-owned artifact under t.TempDir.
			if err != nil {
				t.Fatal(err)
			}
			decoded, decodeErr := gif.DecodeAll(f)
			closeErr := f.Close()
			if decodeErr != nil || closeErr != nil {
				t.Fatalf("read GIF: %v, %v", decodeErr, closeErr)
			}
			if len(decoded.Image) != 2 || decoded.Config.Width != 4 || decoded.Config.Height != 3 {
				t.Fatalf("published wrong GIF: %+v", decoded.Config)
			}
		})
	}
}

func TestCaptureDrawOnlySamples(t *testing.T) {
	for _, state := range []struct {
		name                    string
		due, finished, sampling bool
	}{
		{name: "preparation"},
		{name: "between GIF samples", sampling: true},
		{name: "completed", due: true, finished: true},
	} {
		t.Run(state.name, func(t *testing.T) {
			// An unsampled callback must not enter Game.Draw at all. The empty
			// game deliberately has no scene or image resources to render.
			b := &captureBatch{game: &Game{}, due: state.due, finished: state.finished, sampling: state.sampling}
			b.Draw(nil)
			if b.game.rendering || b.due != state.due || b.finished != state.finished {
				t.Fatal("unsampled draw changed the capture state")
			}
		})
	}
}

// Preparation and sampled structure must not depend on how often the display draws.
func TestCaptureStructureIndependentOfDrawCadence(t *testing.T) {
	for _, job := range []CaptureJob{
		{ID: "same", Start: "camp", WaitTicks: 2},
		{ID: "edited", Event: "finish", Line: 1, WaitTicks: 2},
		{ID: "implicit-first-line", Event: "finish", WaitTicks: 2},
	} {
		t.Run(job.ID, func(t *testing.T) {
			var baseline []byte
			for _, draws := range []int{0, 1, 3} {
				b := &captureBatch{ctx: context.Background(), fsys: os.DirFS("../../../e2e/atago/fixtures/game"), cfg: CaptureConfig{Output: t.TempDir(), Timeout: time.Minute, Structure: true}, jobs: []CaptureJob{job}}
				b.begin()
				if b.game == nil {
					t.Fatalf("begin: %+v", b.results)
				}
				g := b.game
				for n := 0; !b.due && n < 1000; n++ {
					for range draws {
						g.Draw(covScreen())
					}
					if err := b.Update(); err != nil {
						t.Fatal(err)
					}
				}
				g.Draw(covScreen())
				screen := g.opts.capture.screen
				if !b.due || len(screen) == 0 {
					t.Fatal("sample not reached")
				}
				if baseline == nil {
					baseline = bytes.Clone(screen)
				} else if !bytes.Equal(baseline, screen) {
					t.Errorf("draws=%d tick=%d: old=%s new=%s", draws, g.tick, baseline, screen)
				}
				if err := g.Close(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
