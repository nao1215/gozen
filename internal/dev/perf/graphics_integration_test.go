//go:build graphics

package perf

import (
	"bufio"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// Each workload runs a fresh process. No graphics initialization is shared by
// parallel tests; CI splits the cases into independent jobs.
func TestGraphicsSpeedBudget(t *testing.T) {
	selected := os.Getenv("GOZEN_PERF_CASE")
	if selected != "" && (selected == "large-sim" || !slices.Contains(Cases(), selected)) {
		t.Fatalf("unknown graphics speed budget section %q", selected)
	}
	root := t.TempDir()
	bin := filepath.Join(root, "gozen")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-buildvcs=false", "-o", bin, "../../../cmd/gozen") //nolint:gosec // Fixed Go executable building the tested CLI.
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		t.Fatal(err)
	}
	for _, name := range Cases() {
		if name == "large-sim" {
			continue
		}
		if selected != "" && selected != name {
			continue
		}
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(root, name)
			if err := Generate(dir, name, "../../../e2e/atago/fixtures/game/assets/regular.ttf"); err != nil {
				t.Fatal(err)
			}
			trace := filepath.Join(dir, "trace.jsonl")
			args := []string{"run", dir, "--dev", "--mute", "--seed", "1", "--start", "room", "--script", filepath.Join(dir, "input.txt"), "--shots", filepath.Join(dir, "shots"), "--save-dir", filepath.Join(dir, "saves"), "--trace", trace}
			if name != "huge-map" {
				args = append(args, "--event", "workload")
			}
			start := time.Now()
			cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // Locally built CLI with generated fixtures.
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			if err := cmd.Run(); err != nil {
				t.Fatal(err)
			}
			t.Logf("section %s: whole CLI seconds = %.3f", name, time.Since(start).Seconds())
			file, err := os.Open(trace) //nolint:gosec // Trace from this test process in its temporary directory.
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := file.Close(); err != nil {
					t.Error(err)
				}
			}()
			scanner := bufio.NewScanner(file)
			scanner.Buffer(make([]byte, 65536), 16<<20)
			found, startup := false, false
			for scanner.Scan() {
				var record struct {
					Kind             string   `json:"t"`
					Name             string   `json:"name"`
					Scope            string   `json:"scope"`
					Frames           uint64   `json:"frames"`
					Ticks            uint64   `json:"ticks"`
					Allocations      *float64 `json:"allocations_per_frame"`
					TotalAllocations *float64 `json:"allocations"`
					Heap             float64  `json:"heap_inuse_bytes"`
					Seconds          float64  `json:"elapsed_seconds"`
				}
				if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
					t.Fatal(err)
				}
				if record.Kind != "section" || record.Scope != "mark" {
					continue
				}
				if record.Name == "run" {
					startup = true
				}
				if record.Frames == 0 || record.Ticks == 0 {
					t.Fatalf("section %s not exercised: %+v", record.Name, record)
				}
				if record.Name == name {
					found = true
					if record.Frames < 1 || record.Ticks < 60 {
						t.Fatalf("section %s not exercised: %+v", name, record)
					}
				}
				if record.TotalAllocations == nil {
					t.Fatalf("section %s: missing allocation count", record.Name)
				}
				t.Logf("section %s/%s: frames=%d ticks=%d allocations=%g", name, record.Name, record.Frames, record.Ticks, *record.TotalAllocations)
				// Startup and the named steady section are both guarded, so decoding
				// regressions cannot hide before the mark.
				allocLimit, heapLimit, secondsLimit := 20000.0, float64(256<<20), 120.0
				switch name {
				case "huge-map":
					// Measured under Xvfb (llvmpipe): about 9,960 startup and 371 warm
					// allocations/frame, 290 MB heap; limits are about 1.3-2x of that.
					allocLimit, heapLimit, secondsLimit = 13000, 512<<20, 15
					if record.Name == name {
						allocLimit = 600
					}
				case "pictures-4k":
					// Measured: about 818 startup and 162 warm allocations/frame.
					allocLimit, secondsLimit = 1700, 5
					if record.Name == name {
						allocLimit = 350
					}
				case "standing-pictures":
					allocLimit, secondsLimit = 1900, 5
				case "phone-video":
					allocLimit, secondsLimit = 1300, 5
				}
				if record.Allocations == nil {
					t.Fatalf("section %s: missing allocations/frame", record.Name)
				}
				if name == "standing-pictures" && record.Name == "run" {
					// Loading the 32 figures is fixed work. The loop can render
					// only 11 startup frames rather than 30, inflating the
					// ratio even when total allocations decrease (24k vs 32k).
					// Keep the original 1,900 x 30 nominal-frame allowance.
					checkBudget(t, name+"/run", "total allocations", *record.TotalAllocations, 1900*30)
				} else {
					checkBudget(t, name+"/"+record.Name, "allocations/frame", *record.Allocations, allocLimit)
				}
				if record.Heap <= 0 {
					t.Fatalf("section %s: missing heap sample", record.Name)
				}
				checkBudget(t, name+"/"+record.Name, "heap bytes", record.Heap, heapLimit)
				checkBudget(t, name+"/"+record.Name, "seconds", record.Seconds, secondsLimit)
			}
			if err := scanner.Err(); err != nil {
				t.Fatal(err)
			}
			if !found || !startup {
				t.Fatalf("missing section: startup=%t, %s=%t", startup, name, found)
			}
			if name == "phone-video" {
				verifyPhonePlayback(ctx, t, bin, dir, args)
			}
		})
	}
}

// Check actual decoded frames separately so PNG capture does not change the budget.
func verifyPhonePlayback(ctx context.Context, t *testing.T, bin, dir string, args []string) {
	t.Helper()
	path := filepath.Join(dir, "input.txt")
	body, err := os.ReadFile(path) //nolint:gosec // Generated fixture under the test's temporary directory.
	if err != nil {
		t.Fatal(err)
	}
	script := strings.Replace(string(body), "mark phone-video", "shot phone-before\nmark phone-video", 1)
	script = strings.Replace(script, "quit", "shot phone-after\nquit", 1)
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil { //nolint:gosec // Rewrites only the generated temporary fixture script.
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // Built CLI and generated fixture paths.
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	read := func(name string) image.Image {
		t.Helper()
		file, err := os.Open(filepath.Join(dir, "shots", name+".png")) //nolint:gosec // Fixed image names under test-owned output.
		if err != nil {
			t.Fatal(err)
		}
		img, decodeErr := png.Decode(file)
		closeErr := file.Close()
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		return img
	}
	before, after := read("phone-before"), read("phone-after")
	if before.Bounds() != after.Bounds() {
		t.Fatal("phone capture dimensions changed")
	}
	changed := 0
	for y := before.Bounds().Min.Y; y < before.Bounds().Max.Y; y++ {
		for x := before.Bounds().Min.X; x < before.Bounds().Max.X; x++ {
			r, g, b, _ := before.At(x, y).RGBA()
			r2, g2, b2, _ := after.At(x, y).RGBA()
			// The generated frames have a changing red/green gradient and a
			// constant blue channel. Phone chrome and text cannot satisfy this.
			if b == 128*257 && b2 == b && r < 80*257 && g < 80*257 && (r != r2 || g != g2) {
				changed++
			}
		}
	}
	if changed < 1000 {
		t.Fatalf("video did not advance: only %d changing frame pixels", changed)
	}
}
