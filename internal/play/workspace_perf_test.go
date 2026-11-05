package play

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestWorkspaceFrameTime measures the frames of a real game (Update and Draw,
// CPU side) while a run --script plays, and prints the frame times grouped by
// the script command each frame ran under. It defaults to the neutral twenty-card fixture; GOZEN_PERF_GAME
// can select another game directory; GOZEN_PERF_SCRIPT is the script (\n separates
// lines) and GOZEN_PERF_START the starting location.
//
//	GOZEN_PERF_GAME=../../examples/workshop/game GOZEN_PERF_SCRIPT='...' go test ./internal/play -run WorkspaceFrameTime -v -cpuprofile cpu.out
func TestWorkspaceFrameTime(t *testing.T) {
	dir := os.Getenv("GOZEN_PERF_GAME")
	if dir == "" {
		dir = "../../e2e/atago/fixtures/workboard"
	}
	script := strings.ReplaceAll(os.Getenv("GOZEN_PERF_SCRIPT"), `\n`, "\n")
	location := os.Getenv("GOZEN_PERF_START")
	if script == "" {
		script = "wait 20\ndo open: performance\nwait 120\npress x\nwait 10"
	}
	if location == "" {
		location = "workshop"
	}
	g := wldOpenWith(t, Options{FS: os.DirFS(dir), Location: location, Script: script + "\nquit\n"})
	screen := wldScreen(g)
	type group struct {
		name   string
		frames []time.Duration
	}
	var groups []*group
	completed := false
	for range 20000 {
		name := "start"
		if s := g.in.script; s != nil && s.pc < len(s.cmds) {
			c := s.cmds[s.pc]
			name = fmt.Sprintf("%02d %s %s%s", s.pc, c.name, c.verb, c.arg)
		}
		if len(groups) == 0 || groups[len(groups)-1].name != name {
			groups = append(groups, &group{name: name})
		}
		start := time.Now()
		err := g.Update()
		if s := g.in.script; s != nil {
			s.shot = ""
		}
		g.autoShot = ""
		if err != nil {
			if !errors.Is(err, errQuit) {
				t.Fatal(err)
			}
			completed = true
			break
		}
		g.Draw(screen)
		groups[len(groups)-1].frames = append(groups[len(groups)-1].frames, time.Since(start))
	}
	if !completed {
		t.Fatal("script did not finish within 20000 updates")
	}
	for _, gr := range groups {
		s := gr.frames
		if len(s) == 0 {
			continue
		}
		slices.Sort(s)
		var sum time.Duration
		for _, v := range s {
			sum += v
		}
		name := gr.name
		if len(name) > 40 {
			name = name[:40]
		}
		t.Logf("%-40s frames %5d  mean %7.2fms  p50 %7.2fms  max %7.2fms", name, len(s),
			perfMS(sum)/float64(len(s)), perfMS(s[len(s)/2]), perfMS(s[len(s)-1]))
	}
}

func perfMS(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
