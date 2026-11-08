//go:build graphics

package perf

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
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
		})
	}
}
