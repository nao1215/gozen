package distribute

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/cmd/internal/starter"
)

// TestShrinkJobs: the files converted at once are bounded by the CPUs and a fixed limit,
// and are never none; their memory is half the free memory within fixed bounds.
func TestShrinkJobs(t *testing.T) {
	for procs, want := range map[int]int{32: maxShrinkJobs, 2: 2, 1: 1, 0: 1} {
		if got := shrinkJobs(procs); got != want {
			t.Errorf("shrinkJobs(%d) = %d, want %d", procs, got, want)
		}
	}
	for available, want := range map[uint64]int64{0: maxShrinkMemory, 64 << 30: maxShrinkMemory, 4 << 30: 2 << 30, 100 << 20: minShrinkMemory} {
		if got := shrinkMemory(available); got != want {
			t.Errorf("shrinkMemory(%d) = %d, want %d", available, got, want)
		}
	}
	if got := shrinkMemory(availableMemory()); got < minShrinkMemory || got > maxShrinkMemory {
		t.Errorf("shrinkMemory with this machine's memory = %d", got)
	}
}

// TestBudgetBoundsWhatIsTaken: parts taken at once never exceed the budget, a part larger
// than the budget still runs (alone), and every part is given back.
func TestBudgetBoundsWhatIsTaken(t *testing.T) {
	b := newBudget(100)
	var mu sync.Mutex
	held, peak := int64(0), int64(0)
	var wg sync.WaitGroup
	for i := range 64 {
		wg.Go(func() {
			n := b.take(int64(10 + i*7%150))
			mu.Lock()
			held += n
			peak = max(peak, held)
			mu.Unlock()
			runtime.Gosched()
			mu.Lock()
			held -= n
			mu.Unlock()
			b.give(n)
		})
	}
	wg.Wait()
	if peak > 100 || b.free != 100 {
		t.Errorf("peak %d of 100, %d free at the end", peak, b.free)
	}
}

// picture is a PNG of w×h noisy pixels (from seed), which WebP makes smaller.
func picture(t *testing.T, w, h int, seed uint32) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	state := seed | 1
	for i := 0; i < len(img.Pix); i += 4 {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		binary.LittleEndian.PutUint32(img.Pix[i:], state)
		img.Pix[i+3] = 255
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func manifest(t *testing.T, site string) map[string]webFile {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(site, "game", "files.json")) //nolint:gosec // Test-owned exported file.
	if err != nil {
		t.Fatal(err)
	}
	var entries []webFile
	if err := json.Unmarshal(b, &entries); err != nil {
		t.Fatal(err)
	}
	m := make(map[string]webFile, len(entries))
	for _, e := range entries {
		m[e.Name] = e
	}
	return m
}

// TestWebReusesUnchangedFiles: an export given an earlier one (--reuse) takes the served
// file of every file whose content has not changed from it, converting only the changed
// ones, and its site is the one a full export makes.
func TestWebReusesUnchangedFiles(t *testing.T) {
	src, bundle := t.TempDir(), webBundle(t)
	if err := starter.Write(src, starter.Config{Template: "rpg", ID: "reuse-test", Title: "Reuse test"}); err != nil {
		t.Fatal(err)
	}
	for name, b := range map[string][]byte{"assets/kept.png": picture(t, 64, 48, 1), "assets/changed.png": picture(t, 64, 48, 2)} {
		if err := cli.WriteNew(filepath.Join(src, filepath.FromSlash(name)), b); err != nil {
			t.Fatal(err)
		}
	}
	parent := t.TempDir()
	first := filepath.Join(parent, "first")
	if err := Web(src, first, bundle, "devel", ""); err != nil {
		t.Fatal(err)
	}
	before := manifest(t, first)
	for _, name := range []string{"assets/kept.png", "assets/changed.png"} {
		if before[name].File != name+".webp" || before[name].Source == "" {
			t.Fatalf("%s: %+v", name, before[name])
		}
	}
	// Mark the earlier WebP of the unchanged picture (same size): a reused file keeps the mark.
	kept := filepath.Join(first, "game", "assets", "kept.png.webp")
	served, err := os.ReadFile(kept) //nolint:gosec // Test-owned exported file.
	if err != nil {
		t.Fatal(err)
	}
	marked := bytes.Clone(served)
	marked[len(marked)-1] ^= 0xff
	if err := os.WriteFile(kept, marked, 0o600); err != nil { //nolint:gosec // Test-owned exported file.
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "assets", "changed.png"), picture(t, 80, 40, 3), 0o600); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(parent, "second")
	if err := Web(src, second, bundle, "devel", first); err != nil {
		t.Fatal(err)
	}
	full := filepath.Join(parent, "full")
	if err := Web(src, full, bundle, "devel", ""); err != nil {
		t.Fatal(err)
	}
	got, want := manifest(t, second), manifest(t, full)
	if len(got) != len(want) {
		t.Fatalf("%d entries, a full export has %d", len(got), len(want))
	}
	for name, w := range want {
		g := got[name]
		if g != w {
			t.Errorf("%s: %+v, a full export lists %+v", name, g, w)
			continue
		}
		a, errA := os.ReadFile(filepath.Join(second, "game", filepath.FromSlash(g.File))) //nolint:gosec // Test-owned exported file.
		b, errB := os.ReadFile(filepath.Join(full, "game", filepath.FromSlash(w.File)))   //nolint:gosec // Test-owned exported file.
		if errA != nil || errB != nil {
			t.Fatal(errA, errB)
		}
		switch {
		case name == "assets/kept.png":
			if !bytes.Equal(a, marked) {
				t.Errorf("%s was converted again, not taken from the earlier export", name)
			}
		case !bytes.Equal(a, b):
			t.Errorf("%s: served file differs from a full export", name)
		}
	}
	if c := got["assets/changed.png"]; c.Source == before["assets/changed.png"].Source {
		t.Errorf("changed picture kept its earlier source %s", c.Source)
	}
	// a file of the earlier export that is missing is converted again
	if err := os.Remove(kept); err != nil {
		t.Fatal(err)
	}
	third := filepath.Join(parent, "third")
	if err := Web(src, third, bundle, "devel", first); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(third, "game", "assets", "kept.png.webp")); err != nil || !bytes.Equal(b, served) { //nolint:gosec // Test-owned exported file.
		t.Errorf("missing earlier file was not converted again: %v", err)
	}
}

// TestWebReuseNeedsAnExport: --reuse of a folder that is not a web export, or of the
// output itself, is an error and leaves no output.
func TestWebReuseNeedsAnExport(t *testing.T) {
	src, bundle := t.TempDir(), webBundle(t)
	if err := starter.Write(src, starter.Config{Template: "rpg", ID: "reuse-test", Title: "Reuse test"}); err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	out := filepath.Join(parent, "site")
	if err := Web(src, out, bundle, "devel", t.TempDir()); err == nil || !strings.Contains(err.Error(), "not a web export") {
		t.Fatalf("reuse of an empty folder: %v", err)
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("output written")
	}
	if err := Web(src, out, bundle, "devel", src); err == nil {
		t.Fatal("reuse of the game itself was accepted")
	}
}

func TestOversizedImageIsKeptWithoutConversion(t *testing.T) {
	source := picture(t, 32, 32, 17)
	s := shrinker{memory: newBudget(32*32*pixelMemory - 1)}
	if out := s.webP(source); out != nil {
		t.Fatal("converted an image outside the memory budget")
	}
	if s.memory.free != s.memory.total {
		t.Fatal("budget leaked")
	}
}
