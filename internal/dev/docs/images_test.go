package docs

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	goimage "image"
	"image/draw"
	"image/gif"
	_ "image/jpeg" // decode the committed JPEG pictures
	"image/png"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestDocImages remakes every picture of docs/images/manifest.yaml from the
// samples. It needs a display (xvfb-run on Linux), ffmpeg and python3, so it
// runs only when asked:
//
//	GOZEN_DOCS_IMAGES=update  write the pictures to docs/images (make docs-images)
//	GOZEN_DOCS_IMAGES=check   fail when a committed picture is stale (make docs-images-check)
//
// The fresh pictures stay in build/docs-images/out either way.
func TestDocImages(t *testing.T) {
	mode := os.Getenv("GOZEN_DOCS_IMAGES")
	switch mode {
	case "update", "check":
	case "":
		t.Skip("set GOZEN_DOCS_IMAGES=update or check (make docs-images, make docs-images-check); it needs a display")
	default:
		t.Fatalf("GOZEN_DOCS_IMAGES=%q: want update or check", mode)
	}
	if runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Fatal("no display: run it inside xvfb-run (make docs-images does)")
	}
	for _, tool := range []string{"ffmpeg", "python3"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("%s is needed to make the pictures: %v", tool, err)
		}
	}
	m := loadManifest(t)
	if bad := m.validate(func(p string) ([]byte, error) { return os.ReadFile(filepath.Join(root, p)) }); len(bad) > 0 { //nolint:gosec // Files of this repository named by the manifest.
		t.Fatalf("%s:\n%s", manifestPath, strings.Join(bad, "\n"))
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(rootAbs, "build", "docs-images")
	out := filepath.Join(work, "out")
	if err := os.RemoveAll(work); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(out, 0o750); err != nil {
		t.Fatal(err)
	}
	made := makeImages(t, m, rootAbs, work, out)
	committed := filepath.Join(rootAbs, "docs", "images")
	if mode == "update" {
		// A picture that still matches is kept, so a remake does not churn the binary files.
		for _, name := range made {
			if compareFiles(t.Context(), filepath.Join(committed, name), filepath.Join(out, name), m.Images[name].tolerance()) == nil {
				t.Logf("docs/images/%s is up to date", name)
				continue
			}
			b, err := os.ReadFile(filepath.Join(out, name)) //nolint:gosec // A picture this test wrote.
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(committed, name), b, 0o644); err != nil { //nolint:gosec // A committed picture, readable like the other files.
				t.Fatal(err)
			}
			t.Logf("wrote docs/images/%s", name)
		}
		return
	}
	var stale []string
	for _, name := range made {
		if err := compareFiles(t.Context(), filepath.Join(committed, name), filepath.Join(out, name), m.Images[name].tolerance()); err != nil {
			stale = append(stale, fmt.Sprintf("docs/images/%s: %v", name, err))
		}
	}
	if len(stale) > 0 {
		t.Fatalf("pictures are stale; run make docs-images and commit them (the fresh ones are in build/docs-images/out):\n%s", strings.Join(stale, "\n"))
	}
}

// makeImages builds the samples, plays every run and encodes every picture to out.
// It returns the names of the pictures it wrote. The samples build, the runs play and
// the pictures encode side by side, docsJobs at a time: every run is a process of its
// own with its own seed and script, so the frames do not depend on the order.
func makeImages(t *testing.T, m manifest, rootAbs, work, out string) []string {
	t.Helper()
	jobs := docsJobs()
	images, runNames := m.selected(t)
	bins := map[string]string{}
	var builds []func() error
	for _, name := range sortedKeys(m.Samples) {
		bin := filepath.Join(work, "bin", name)
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		bins[name] = bin
		pkg := m.Samples[name].Package
		builds = append(builds, func() error {
			if _, err := os.Stat(filepath.Join(rootAbs, filepath.FromSlash(pkg), "go.mod")); err == nil {
				// a sample that is a module of its own (examples/go-zen) is built in its directory
				return runCommand(t.Context(), filepath.Join(rootAbs, filepath.FromSlash(pkg)), 5*time.Minute, "go", "build", "-trimpath", "-buildvcs=false", "-o", bin, ".")
			}
			return runCommand(t.Context(), rootAbs, 5*time.Minute, "go", "build", "-trimpath", "-buildvcs=false", "-o", bin, pkg)
		})
	}
	if err := parallel(jobs, builds); err != nil {
		t.Fatal(err)
	}
	runs := map[string]*played{}
	var mu sync.Mutex
	var plays []func() error
	for _, name := range runNames {
		r := m.Runs[name]
		dir := filepath.Join(work, "runs", name)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		game := filepath.Join(work, "games", name)
		args := make([]string, len(r.Args))
		for i, a := range r.Args {
			a = strings.ReplaceAll(a, "{script}", filepath.Join(rootAbs, filepath.FromSlash(r.Script)))
			a = strings.ReplaceAll(a, "{game}", game)
			args[i] = strings.ReplaceAll(a, "{out}", dir)
		}
		plays = append(plays, func() error {
			began := time.Now()
			// A filmed run writes a 1920x1080 PNG every few ticks; on a busy machine that is slow.
			if r.Edit != nil {
				if err := os.CopyFS(game, os.DirFS(filepath.Join(rootAbs, filepath.FromSlash(r.Edit.Copy)))); err != nil {
					return fmt.Errorf("run %s: %w", name, err)
				}
				if err := commandEditing(t.Context(), rootAbs, 15*time.Minute, dir, game, *r.Edit, bins[r.Sample], args...); err != nil {
					return fmt.Errorf("run %s: %w", name, err)
				}
			} else if err := runCommand(t.Context(), rootAbs, 15*time.Minute, bins[r.Sample], args...); err != nil {
				return fmt.Errorf("run %s: %w", name, err)
			}
			if len(r.Check) > 0 {
				if err := runCommand(t.Context(), rootAbs, time.Minute, r.Check[0], append(slices.Clone(r.Check[1:]), dir)...); err != nil {
					return fmt.Errorf("run %s: %w", name, err)
				}
			}
			p, err := readPlayed(dir)
			if err != nil {
				return fmt.Errorf("run %s: %w", name, err)
			}
			t.Logf("run %s: %s %s (%s)", name, m.Samples[r.Sample].Package, strings.Join(r.Args, " "), time.Since(began).Round(time.Second))
			mu.Lock()
			runs[name] = p
			mu.Unlock()
			return nil
		})
	}
	if err := parallel(jobs, plays); err != nil {
		t.Fatal(err)
	}
	var encodes []func() error
	for _, name := range images {
		im := m.Images[name]
		p := runs[im.Run]
		dst := filepath.Join(out, name)
		encodes = append(encodes, func() error {
			var err error
			switch {
			case im.GIF != nil:
				var frames []string
				frames, err = montage(runs, im)
				if err == nil {
					err = encodeGIF(t, work, frames, *im.GIF, dst)
				}
			default:
				sel := pick{Shots: im.Shot}
				if im.Pick != nil {
					sel = *im.Pick
				}
				var frame string
				frame, err = p.one(sel)
				if err == nil {
					err = encodeJPEG(t, frame, *im.JPEG, dst)
				}
			}
			if err != nil {
				return fmt.Errorf("docs/images/%s: %w", name, err)
			}
			return nil
		})
	}
	if err := parallel(jobs, encodes); err != nil {
		t.Fatal(err)
	}
	return images
}

// docsJobs is how many samples build, runs play or pictures encode at once:
// GOZEN_DOCS_JOBS, or the number of CPUs.
func docsJobs() int {
	if n, err := strconv.Atoi(os.Getenv("GOZEN_DOCS_JOBS")); err == nil && n > 0 {
		return n
	}
	return max(1, runtime.NumCPU())
}

// selected returns the pictures to make, sorted, and the runs they take frames from.
// GOZEN_DOCS_IMAGES_ONLY narrows the pictures to the names (or path.Match patterns)
// it lists, separated by commas; empty, it is every picture.
func (m manifest) selected(t *testing.T) (images, runs []string) {
	t.Helper()
	var only []string
	for f := range strings.SplitSeq(os.Getenv("GOZEN_DOCS_IMAGES_ONLY"), ",") {
		if f = strings.TrimSpace(f); f != "" {
			only = append(only, f)
		}
	}
	need := map[string]bool{}
	for _, name := range sortedKeys(m.Images) {
		if len(only) > 0 && !slices.ContainsFunc(only, func(p string) bool { ok, err := path.Match(p, name); return err == nil && ok }) {
			continue
		}
		images = append(images, name)
		im := m.Images[name]
		need[im.Run] = true
		for _, f := range im.Frames {
			if f.Run != "" {
				need[f.Run] = true
			}
		}
	}
	if len(images) == 0 {
		t.Fatalf("GOZEN_DOCS_IMAGES_ONLY=%q matches no picture of %s", os.Getenv("GOZEN_DOCS_IMAGES_ONLY"), manifestPath)
	}
	return images, sortedKeys(need)
}

// parallel runs the functions, n at a time, and returns their errors.
func parallel(n int, fns []func() error) error {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
		sem  = make(chan struct{}, max(1, n))
	)
	for _, fn := range fns {
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			if err := fn(); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	return errors.Join(errs...)
}

func command(t *testing.T, dir string, limit time.Duration, name string, args ...string) {
	t.Helper()
	if err := runCommand(t.Context(), dir, limit, name, args...); err != nil {
		t.Fatal(err)
	}
}

// runCommand runs a command in dir and returns its error with the tails of its output.
func runCommand(ctx context.Context, dir string, limit time.Duration, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // The commands of the manifest and of the test itself.
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w\nstdout:\n%s\nstderr:\n%s", name, strings.Join(args, " "), err, tail(stdout.String()), tail(stderr.String()))
	}
	return nil
}

// commandEditing runs a command like runCommand, and once it has written the shot e.After
// under dir, changes e.File in the game's copy as e says (the hot reload's pictures).
func commandEditing(ctx context.Context, dir string, limit time.Duration, out, game string, e edit, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // The commands of the manifest.
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	shot := filepath.Join(out, e.After+".png")
	edited := false
	for {
		select {
		case err := <-done:
			if err == nil && !edited {
				err = fmt.Errorf("it ended before the shot %s", e.After)
			}
			if err != nil {
				return fmt.Errorf("%s %s: %w\nstdout:\n%s\nstderr:\n%s", name, strings.Join(args, " "), err, tail(stdout.String()), tail(stderr.String()))
			}
			return nil
		case <-time.After(20 * time.Millisecond):
		}
		if edited {
			continue
		}
		if _, err := os.Stat(shot); err != nil {
			continue
		}
		file := filepath.Join(game, filepath.FromSlash(e.File))
		b, err := os.ReadFile(file) //nolint:gosec // The copy of a sample's file this test made.
		if err == nil {
			err = os.WriteFile(file, []byte(strings.Replace(string(b), e.Old, e.New, 1)), 0o600) //nolint:gosec // The copy of a sample's file this test made, named by the manifest.
		}
		if err != nil {
			cancel()
			<-done
			return err
		}
		edited = true
	}
}

func tail(s string) string {
	if len(s) > 4000 {
		return "..." + s[len(s)-4000:]
	}
	return s
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// played is what one run left: its frames and its trace.
type played struct {
	files  map[string]string // shot name -> PNG file
	order  []string          // shot names, in the order the game wrote them
	ticks  map[string]int    // shot name -> tick, from the trace
	events map[string]int    // trace key -> tick of the first line where it is set
}

func readPlayed(dir string) (*played, error) {
	p := &played{files: map[string]string{}, ticks: map[string]int{}, events: map[string]int{}}
	err := filepath.WalkDir(dir, func(f string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(f) != ".png" {
			return err
		}
		p.files[strings.TrimSuffix(d.Name(), ".png")] = f
		return nil
	})
	if err != nil {
		return nil, err
	}
	if b, err := os.ReadFile(filepath.Join(dir, "trace.jsonl")); err == nil { //nolint:gosec // The trace of the run this test made.
		sc := bufio.NewScanner(bytes.NewReader(b))
		sc.Buffer(make([]byte, 1<<20), 16<<20)
		for sc.Scan() {
			var line map[string]any
			if json.Unmarshal(sc.Bytes(), &line) != nil {
				continue
			}
			tick, ok := line["tick"].(float64)
			if !ok {
				tick = 0
			}
			if line["t"] == "shot" {
				if file, ok := line["file"].(string); ok {
					p.ticks[strings.TrimSuffix(filepath.Base(file), ".png")] = int(tick)
				}
			}
			for k, v := range line {
				if _, seen := p.events[k]; !seen && truthy(v) {
					p.events[k] = int(tick)
				}
			}
		}
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}
	for name := range p.files {
		p.order = append(p.order, name)
	}
	sort.Slice(p.order, func(i, j int) bool {
		a, b := p.order[i], p.order[j]
		ta, oka := p.ticks[a]
		tb, okb := p.ticks[b]
		if oka && okb && ta != tb {
			return ta < tb
		}
		return a < b
	})
	return p, nil
}

func truthy(v any) bool {
	switch v := v.(type) {
	case bool:
		return v
	case string:
		return v != ""
	case float64:
		return v != 0
	case nil:
		return false
	default:
		return true
	}
}

// matching returns the shot names that match the pick's pattern and its window, in order.
func (p *played) matching(s pick) ([]string, error) {
	var names []string
	for _, n := range p.order {
		if ok, err := path.Match(s.Shots, n); err != nil {
			return nil, fmt.Errorf("shots %q: %w", s.Shots, err)
		} else if ok {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no frame matches %q", s.Shots)
	}
	if s.Every > 1 {
		var kept []string
		for i, n := range names {
			if i%s.Every == 0 {
				kept = append(kept, n)
			}
		}
		names = kept
	}
	if s.Near != "" && (s.From != nil || s.To != nil) {
		at, err := p.event(s.Near)
		if err != nil {
			return nil, err
		}
		var kept []string
		for _, n := range names {
			tick, ok := p.ticks[n]
			if ok && (s.From == nil || tick >= at+*s.From) && (s.To == nil || tick <= at+*s.To) {
				kept = append(kept, n)
			}
		}
		names = kept
	}
	return names, nil
}

func (p *played) event(name string) (int, error) {
	at, ok := p.events[name]
	if !ok {
		return 0, fmt.Errorf("the trace never has %q", name)
	}
	return at, nil
}

// one returns the frame a still picture is made from.
func (p *played) one(s pick) (string, error) {
	names, err := p.matching(s)
	if err != nil {
		return "", err
	}
	switch {
	case s.Before != "":
		at, err := p.event(s.Before)
		if err != nil {
			return "", err
		}
		var last string
		for _, n := range names {
			if tick, ok := p.ticks[n]; ok && tick < at {
				last = n
			}
		}
		if last == "" {
			return "", fmt.Errorf("no %q frame before %s", s.Shots, s.Before)
		}
		return p.files[last], nil
	case s.At != nil:
		at, err := p.event(s.Near)
		if err != nil {
			return "", err
		}
		best, gap := "", -1
		for _, n := range names {
			tick, ok := p.ticks[n]
			if !ok {
				continue
			}
			d := tick - at - *s.At
			if d < 0 {
				d = -d
			}
			if gap < 0 || d < gap {
				best, gap = n, d
			}
		}
		if best == "" {
			return "", fmt.Errorf("no %q frame near %s", s.Shots, s.Near)
		}
		return p.files[best], nil
	case s.Middle:
		return p.files[names[len(names)/2]], nil
	default:
		return p.files[names[0]], nil
	}
}

// frames returns the frames of an animation: the union of the picks, in order.
func (p *played) frames(picks []pick) ([]string, error) {
	keep := map[string]bool{}
	for _, s := range picks {
		names, err := p.matching(s)
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			keep[n] = true
		}
	}
	var files []string
	for _, n := range p.order {
		if keep[n] {
			files = append(files, p.files[n])
		}
	}
	return files, nil
}

// montage returns the frames of an animation that may join several runs: the
// picks of each run, in the order the runs first appear in the list.
func montage(runs map[string]*played, im image) ([]string, error) {
	var order []string
	groups := map[string][]pick{}
	for _, s := range im.Frames {
		r := s.Run
		if r == "" {
			r = im.Run
		}
		if _, seen := groups[r]; !seen {
			order = append(order, r)
		}
		groups[r] = append(groups[r], s)
	}
	var files []string
	for _, r := range order {
		f, err := runs[r].frames(groups[r])
		if err != nil {
			return nil, fmt.Errorf("run %s: %w", r, err)
		}
		files = append(files, f...)
	}
	return files, nil
}

func ffmpeg(t *testing.T, args ...string) error {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "ffmpeg", append([]string{"-hide_banner", "-loglevel", "error", "-y"}, args...)...) //nolint:gosec // ffmpeg on the test's own files.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg %s: %w: %s", strings.Join(args, " "), err, stderr.String())
	}
	return nil
}

// frameSlots bounds the frames converted at once over every picture: a frame takes
// 8 MB or more while it is converted.
var frameSlots = make(chan struct{}, docsJobs())

// opaqueFrame writes the frame src as an RGB PNG at dst: the colors kept, the alpha
// dropped, as ffmpeg's -pix_fmt rgb24 does. An opaque frame (what the samples write) is
// copied and an 8-bit RGBA one converted here; any other kind goes through ffmpeg.
func opaqueFrame(t *testing.T, src, dst string) error {
	frameSlots <- struct{}{}
	defer func() { <-frameSlots }()
	b, err := os.ReadFile(src) //nolint:gosec // A frame the sample wrote.
	if err != nil {
		return err
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("%s: %w", src, err)
	}
	var pix []byte
	switch m := img.(type) {
	case *goimage.NRGBA:
		pix = m.Pix
	case *goimage.RGBA:
		if !m.Opaque() { // premultiplied: the colors under a partial alpha are not the stored ones
			return ffmpeg(t, "-i", src, "-pix_fmt", "rgb24", dst)
		}
		// Already RGB (what the samples write): the file as it is.
		return os.WriteFile(dst, b, 0o600) //nolint:gosec // A file in the test's work directory.
	default:
		return ffmpeg(t, "-i", src, "-pix_fmt", "rgb24", dst)
	}
	for i := 3; i < len(pix); i += 4 {
		pix[i] = 0xff
	}
	f, err := os.Create(dst) //nolint:gosec // A file in the test's work directory.
	if err != nil {
		return err
	}
	// Opaque, it is written as RGB; BestSpeed, as it is read once by ffmpeg.
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	return errors.Join(enc.Encode(f, img), f.Close())
}

func encodeJPEG(t *testing.T, frame string, e jpegEncoding, dst string) error {
	args := []string{"-i", frame}
	if cfg, err := decodeConfig(frame); err != nil {
		return err
	} else if cfg.Width != e.Width || cfg.Height != e.Height {
		args = append(args, "-vf", fmt.Sprintf("scale=%d:%d:flags=lanczos", e.Width, e.Height))
	}
	if err := ffmpeg(t, append(args, "-q:v", strconv.Itoa(max(e.Quality, 1)), dst)...); err != nil {
		return err
	}
	frames, err := decodeFrames(t.Context(), dst)
	if err != nil {
		return err
	}
	if b := frames[0].Bounds(); b.Dx() != e.Width || b.Dy() != e.Height {
		return fmt.Errorf("wrote %dx%d, want %dx%d", b.Dx(), b.Dy(), e.Width, e.Height)
	}
	return nil
}

func decodeConfig(file string) (goimage.Config, error) {
	b, err := os.ReadFile(file) //nolint:gosec // A frame the sample wrote.
	if err != nil {
		return goimage.Config{}, err
	}
	cfg, _, err := goimage.DecodeConfig(bytes.NewReader(b))
	return cfg, err
}

func encodeGIF(t *testing.T, work string, frames []string, e gifEncoding, dst string) error {
	if len(frames) < 2 {
		return fmt.Errorf("only %d frames", len(frames))
	}
	dir := filepath.Join(work, "gif-"+strings.TrimSuffix(filepath.Base(dst), ".gif"))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	// Some frames have an alpha channel. A mixed sequence breaks ffmpeg's single
	// palette (every GIF frame alike, or false colors), so every frame becomes RGB first.
	conversions := make([]func() error, len(frames))
	for i, f := range frames {
		conversions[i] = func() error { return opaqueFrame(t, f, filepath.Join(dir, fmt.Sprintf("frame-%04d.png", i))) }
	}
	if err := parallel(docsJobs(), conversions); err != nil {
		return err
	}
	filter := "format=rgb24,"
	if e.Crop != "" {
		filter += "crop=" + e.Crop + ","
	}
	filter += fmt.Sprintf("scale=%d:%d:flags=lanczos,split[a][b];[a]palettegen=max_colors=%d:stats_mode=diff[p];"+
		"[b][p]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle", e.Width, e.Height, max(e.Colors, 2))
	if err := ffmpeg(t, "-framerate", strconv.Itoa(e.FPS), "-i", filepath.Join(dir, "frame-%04d.png"),
		"-fps_mode", "cfr", "-filter_complex", filter, "-loop", "0", dst); err != nil {
		return err
	}
	if err := slimGIF(dst, slimTolerance); err != nil {
		return err
	}
	got, err := decodeFrames(t.Context(), dst)
	if err != nil {
		return err
	}
	if len(got) != len(frames) {
		return fmt.Errorf("the GIF has %d frames, want %d", len(got), len(frames))
	}
	if b := got[0].Bounds(); b.Dx() != e.Width || b.Dy() != e.Height {
		return fmt.Errorf("wrote %dx%d, want %dx%d", b.Dx(), b.Dy(), e.Width, e.Height)
	}
	// A GIF whose frames all decode alike still has the right size and length.
	distinct := map[uint64]bool{}
	for _, f := range got {
		h := fnv.New64a()
		rgba, ok := f.(*goimage.RGBA)
		if !ok {
			return fmt.Errorf("frame %T", f)
		}
		_, _ = h.Write(rgba.Pix) // a hash never fails to write
		distinct[h.Sum64()] = true
	}
	if len(distinct) < len(got)/3 {
		return fmt.Errorf("the GIF frames are too alike (%d distinct of %d)", len(distinct), len(got))
	}
	return nil
}

// decodeFrames reads a picture, a GIF or a video as full frames.
func decodeFrames(ctx context.Context, file string) ([]goimage.Image, error) {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".gif":
		b, err := os.ReadFile(file) //nolint:gosec // A picture of this repository or of the test.
		if err != nil {
			return nil, err
		}
		g, err := gif.DecodeAll(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		return composeGIF(g), nil
	case ".mp4", ".webm", ".mov":
		return decodeVideo(ctx, file)
	default:
		b, err := os.ReadFile(file) //nolint:gosec // A picture of this repository or of the test.
		if err != nil {
			return nil, err
		}
		img, _, err := goimage.Decode(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		return []goimage.Image{img}, nil
	}
}

// composeGIF draws every frame of a GIF over the ones before it, as a viewer shows them.
func composeGIF(g *gif.GIF) []goimage.Image {
	bounds := goimage.Rect(0, 0, g.Config.Width, g.Config.Height)
	canvas := goimage.NewRGBA(bounds)
	out := make([]goimage.Image, 0, len(g.Image))
	for i, frame := range g.Image {
		var before *goimage.RGBA
		disposal := byte(0)
		if i < len(g.Disposal) {
			disposal = g.Disposal[i]
		}
		if disposal == gif.DisposalPrevious {
			before = goimage.NewRGBA(bounds)
			copy(before.Pix, canvas.Pix)
		}
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
		shown := goimage.NewRGBA(bounds)
		copy(shown.Pix, canvas.Pix)
		out = append(out, shown)
		switch disposal {
		case gif.DisposalBackground:
			draw.Draw(canvas, frame.Bounds(), goimage.Transparent, goimage.Point{}, draw.Src)
		case gif.DisposalPrevious:
			canvas = before
		}
	}
	return out
}

// decodeVideo extracts every frame of a video with ffmpeg.
func decodeVideo(ctx context.Context, file string) (frames []goimage.Image, err error) {
	dir, err := os.MkdirTemp("", "gozen-video-")
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(dir)) }()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-i", file, "-pix_fmt", "rgb24", filepath.Join(dir, "%05d.png")) //nolint:gosec // ffmpeg on a picture of this repository or of the test.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg could not read %s: %w: %s", file, err, stderr.String())
	}
	names, err := filepath.Glob(filepath.Join(dir, "*.png"))
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	for _, n := range names {
		f, err := decodeFrames(ctx, n)
		if err != nil {
			return nil, err
		}
		frames = append(frames, f...)
	}
	return frames, nil
}

// pixelThreshold is how far apart (0-255, in any channel) two pixels must be to count as
// different. JPEG noise and a different software renderer stay below it.
const pixelThreshold = 48

// diffRatio is the share of pixels that differ between two frames of the same size.
func diffRatio(a, b goimage.Image) float64 {
	ab, bb := a.Bounds(), b.Bounds()
	if ab.Dx() != bb.Dx() || ab.Dy() != bb.Dy() {
		return 1
	}
	differ := 0
	for y := range ab.Dy() {
		for x := range ab.Dx() {
			r1, g1, b1, a1 := a.At(ab.Min.X+x, ab.Min.Y+y).RGBA()
			r2, g2, b2, a2 := b.At(bb.Min.X+x, bb.Min.Y+y).RGBA()
			if far(r1, r2) || far(g1, g2) || far(b1, b2) || far(a1, a2) {
				differ++
			}
		}
	}
	return float64(differ) / float64(ab.Dx()*ab.Dy())
}

func far(a, b uint32) bool {
	a, b = a>>8, b>>8
	if a > b {
		return a-b > pixelThreshold
	}
	return b-a > pixelThreshold
}

// representative are the frame indexes compared for an animation of n frames.
func representative(n int) []int {
	idx := []int{0, n / 4, n / 2, 3 * n / 4, n - 1}
	slices.Sort(idx)
	return slices.Compact(idx)
}

// compareFrames reports how the fresh frames differ from the committed ones beyond the tolerance.
func compareFrames(committed, fresh []goimage.Image, tolerance float64) error {
	if len(committed) != len(fresh) {
		return fmt.Errorf("%d frames, now %d", len(committed), len(fresh))
	}
	if len(fresh) == 0 {
		return errors.New("no frames")
	}
	cb, fb := committed[0].Bounds(), fresh[0].Bounds()
	if cb.Dx() != fb.Dx() || cb.Dy() != fb.Dy() {
		return fmt.Errorf("%dx%d, now %dx%d", cb.Dx(), cb.Dy(), fb.Dx(), fb.Dy())
	}
	var bad []string
	for _, i := range representative(len(fresh)) {
		if r := diffRatio(committed[i], fresh[i]); r > tolerance {
			bad = append(bad, fmt.Sprintf("frame %d differs in %.1f%% of its pixels (allowed %.1f%%)", i, r*100, tolerance*100))
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("%s", strings.Join(bad, "; "))
	}
	return nil
}

func compareFiles(ctx context.Context, committed, fresh string, tolerance float64) error {
	old, err := decodeFrames(ctx, committed)
	if err != nil {
		return fmt.Errorf("the committed picture: %w", err)
	}
	now, err := decodeFrames(ctx, fresh)
	if err != nil {
		return fmt.Errorf("the fresh picture: %w", err)
	}
	return compareFrames(old, now, tolerance)
}
