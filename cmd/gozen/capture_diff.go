package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"reflect"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/layer1/picturelimit"
)

// ShotDiff holds optional capture report facts. It is exported so encoding/json
// can allocate the embedded pointer when reading a previous report; it does not
// determine capture success or failure.
type ShotDiff struct {
	Changed          bool                   `json:"changed"`
	Change           string                 `json:"change"`
	DiffPixels       int                    `json:"diff_pixels"`
	DiffRatio        float64                `json:"diff_ratio"`
	DiffBBox         *[4]int                `json:"diff_bbox"`
	Previous         string                 `json:"previous,omitempty"`
	Highlight        string                 `json:"diff_image,omitempty"`
	AnimationChanged bool                   `json:"animation_changed,omitempty"`
	Structure        map[string]elementDiff `json:"structure_diff,omitempty"`
}
type elementDiff struct {
	Added   []json.RawMessage `json:"added"`
	Removed []json.RawMessage `json:"removed"`
	Changed []elementChange   `json:"changed"`
}
type elementChange struct {
	Old json.RawMessage `json:"old"`
	New json.RawMessage `json:"new"`
}
type captureDiffSummary struct {
	Changed   int `json:"changed"`
	Unchanged int `json:"unchanged"`
	New       int `json:"new"`
	Removed   int `json:"removed"`
}

func readPreviousShots(dir string) ([]shotResult, error) {
	b, err := os.ReadFile(filepath.Join(dir, "shots.json")) //nolint:gosec // Explicit caller-selected capture directory.
	if err != nil {
		return nil, err
	}
	var rows []shotResult
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if !cli.ID(r.ID) || seen[r.ID] || (r.Artifact != "" && r.Artifact != r.ID+".png" && r.Artifact != r.ID+".gif") {
			return nil, fmt.Errorf("invalid previous capture job %q", r.ID)
		}
		seen[r.ID] = true
	}
	return rows, nil
}

// compareRGBA counts spatial pixels once, including pixels present on only one side.
// The loop reads RGBA bytes directly; conversion and decoding happen outside it.
func compareRGBA(old, next *image.RGBA) (ShotDiff, *image.RGBA) {
	bounds := old.Bounds().Union(next.Bounds())
	highlight := image.NewRGBA(bounds)
	d := ShotDiff{}
	box := image.Rectangle{}
	total := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			p := image.Pt(x, y)
			a, b := p.In(old.Bounds()), p.In(next.Bounds())
			if !a && !b {
				continue
			}
			total++
			different := a != b
			if a && b {
				i, j := old.PixOffset(x, y), next.PixOffset(x, y)
				different = !bytes.Equal(old.Pix[i:i+4], next.Pix[j:j+4])
			}
			if different {
				d.DiffPixels++
				box = box.Union(image.Rect(x, y, x+1, y+1))
				i := highlight.PixOffset(x, y)
				copy(highlight.Pix[i:i+4], []byte{255, 0, 128, 255})
			}
		}
	}
	d.Changed = d.DiffPixels > 0
	if total > 0 {
		d.DiffRatio = float64(d.DiffPixels) / float64(total)
	}
	if d.Changed {
		d.DiffBBox = &[4]int{box.Min.X, box.Min.Y, box.Dx(), box.Dy()}
	}
	return d, highlight
}

type captureImage struct {
	frames []*image.RGBA
	delay  []int
	loop   int
}

func decodeShot(b []byte) (captureImage, error) {
	var out captureImage
	if err := picturelimit.Header(b); err != nil {
		return out, err
	}
	convert := func(src image.Image, bounds image.Rectangle) {
		dst := image.NewRGBA(bounds)
		draw.Draw(dst, bounds, src, bounds.Min, draw.Src)
		out.frames = append(out.frames, dst)
	}
	if bytes.HasPrefix(b, []byte("GIF")) {
		g, err := gif.DecodeAll(bytes.NewReader(b))
		if err != nil {
			return out, err
		}
		// Capture GIFs contain complete frames with DisposalNone. Reject partial or
		// disposing frames rather than silently comparing incorrect composed pixels.
		bounds := image.Rect(0, 0, g.Config.Width, g.Config.Height)
		for i, f := range g.Image {
			if f.Bounds() != bounds || (len(g.Disposal) > i && g.Disposal[i] > gif.DisposalNone) {
				return out, errors.New("previous GIF is not a full-frame capture")
			}
			convert(f, bounds)
		}
		out.delay, out.loop = g.Delay, g.LoopCount
	} else {
		src, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			return out, err
		}
		convert(src, src.Bounds())
	}
	return out, nil
}

func compareShotImages(a, b captureImage) (ShotDiff, *image.RGBA) {
	d := ShotDiff{}
	var mask *image.RGBA
	empty := image.NewRGBA(image.Rectangle{})
	total := 0
	box := image.Rectangle{}
	for i := range max(len(a.frames), len(b.frames)) {
		old, next := empty, empty
		if i < len(a.frames) {
			old = a.frames[i]
		}
		if i < len(b.frames) {
			next = b.frames[i]
		}
		frame, m := compareRGBA(old, next)
		d.DiffPixels += frame.DiffPixels
		area := func(r image.Rectangle) int { return r.Dx() * r.Dy() }
		total += area(old.Bounds()) + area(next.Bounds()) - area(old.Bounds().Intersect(next.Bounds()))
		if frame.DiffBBox != nil {
			b := *frame.DiffBBox
			box = box.Union(image.Rect(b[0], b[1], b[0]+b[2], b[1]+b[3]))
		}
		if mask == nil {
			mask = m
		} else {
			if !m.Bounds().In(mask.Bounds()) {
				merged := image.NewRGBA(mask.Bounds().Union(m.Bounds()))
				draw.Draw(merged, mask.Bounds(), mask, mask.Bounds().Min, draw.Src)
				mask = merged
			}
			draw.Draw(mask, m.Bounds(), m, m.Bounds().Min, draw.Over)
		}
	}
	if mask == nil {
		mask = empty
	}
	d.AnimationChanged = len(a.frames) != len(b.frames) || !reflect.DeepEqual(a.delay, b.delay) || a.loop != b.loop
	// The union highlight supplies a spatial box; counts/ratio cover all frames.
	if !box.Empty() {
		d.DiffBBox = &[4]int{box.Min.X, box.Min.Y, box.Dx(), box.Dy()}
	}
	if total > 0 {
		d.DiffRatio = float64(d.DiffPixels) / float64(total)
	}
	d.Changed = d.DiffPixels > 0 || d.AnimationChanged
	return d, mask
}

func diffStructure(a, b json.RawMessage) map[string]elementDiff {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	// Only compare the instrumented texts and controls, ignoring other field types.
	var old, next map[string]json.RawMessage
	if json.Unmarshal(a, &old) != nil || json.Unmarshal(b, &next) != nil {
		return nil
	}
	out := map[string]elementDiff{}
	for _, kind := range []string{"texts", "controls"} {
		var av, bv []json.RawMessage
		if json.Unmarshal(old[kind], &av) != nil || json.Unmarshal(next[kind], &bv) != nil {
			continue
		}
		out[kind] = diffElements(av, bv)
	}
	return out
}

// Match exact facts first, preserving duplicate entries. Remaining entries with
// the same box are changes; unmatched entries are additions/removals.
func diffElements(old, next []json.RawMessage) elementDiff {
	d := elementDiff{Added: []json.RawMessage{}, Removed: []json.RawMessage{}, Changed: []elementChange{}}
	used := make([]bool, len(next))
	match := func(v json.RawMessage, equal func([]byte, []byte) bool) int {
		for j, w := range next {
			if !used[j] && equal(v, w) {
				used[j] = true
				return j
			}
		}
		return -1
	}
	var pending []json.RawMessage
	for _, v := range old {
		if match(v, jsonEqual) < 0 {
			pending = append(pending, v)
		}
	}
	for _, v := range pending {
		if j := match(v, sameElementRect); j >= 0 {
			d.Changed = append(d.Changed, elementChange{v, next[j]})
		} else {
			d.Removed = append(d.Removed, v)
		}
	}
	for j, v := range next {
		if !used[j] {
			d.Added = append(d.Added, v)
		}
	}
	return d
}

func sameElementRect(a, b []byte) bool {
	var old, next map[string]json.RawMessage
	return json.Unmarshal(a, &old) == nil && json.Unmarshal(b, &next) == nil && jsonEqual(old["rect"], next["rect"])
}

func jsonEqual(a, b []byte) bool {
	var x, y any
	return json.Unmarshal(a, &x) == nil && json.Unmarshal(b, &y) == nil && reflect.DeepEqual(x, y)
}

func compareShots(out, previous string, current, old []shotResult) ([]shotResult, *captureDiffSummary, error) {
	if err := checkComparisonNames(current, old); err != nil {
		return nil, nil, err
	}
	summary := &captureDiffSummary{}
	byID := map[string]shotResult{}
	for _, r := range old {
		if r.Status != "removed" {
			byID[r.ID] = r
		}
	}
	for i := range current {
		r := &current[i]
		before, exists := byID[r.ID]
		delete(byID, r.ID)
		if err := compareShot(out, previous, r, before, exists); err != nil {
			return nil, nil, err
		}
		if r.Changed {
			r.Change = "changed"
			summary.Changed++
		} else {
			summary.Unchanged++
		}
		if !exists {
			r.Change = "new"
			summary.New++
		}
	}
	for _, before := range old {
		if _, ok := byID[before.ID]; !ok {
			continue
		}
		// Removed jobs are facts in the manifest, never failed capture jobs.
		removed, _, err := compareShots(out, previous, []shotResult{{ID: before.ID, Status: "removed", item: shotItem{ID: before.ID}}}, []shotResult{before})
		if err != nil {
			return nil, nil, err
		}
		removed[0].Change = "removed"
		current = append(current, removed[0])
		summary.Removed++
		summary.Changed++
	}
	return current, summary, nil
}

func checkComparisonNames(current, old []shotResult) error {
	// IDs may contain dots. Reserve report filenames before copying anything so
	// a job named "view.diff" cannot be overwritten by another job's highlight.
	artifacts := map[string]bool{}
	ids := map[string]bool{}
	for _, r := range current {
		artifacts[r.Artifact] = true
		ids[r.ID] = true
	}
	for _, r := range old {
		if r.Status != "removed" {
			ids[r.ID] = true
		}
	}
	for id := range ids {
		for _, suffix := range []string{".diff.png", ".previous.png", ".previous.gif"} {
			if artifacts[id+suffix] {
				return fmt.Errorf("capture comparison filename %q conflicts with a capture job; rename that job", id+suffix)
			}
		}
	}
	return nil
}

func compareShot(out, previous string, r *shotResult, before shotResult, exists bool) error {
	d := ShotDiff{Change: "unchanged"}
	r.ShotDiff = &d
	var a, b captureImage
	if exists && before.Artifact != "" {
		data, err := picturelimit.Read(os.DirFS(previous), before.Artifact)
		if err != nil {
			return err
		}
		a, err = decodeShot(data)
		if err != nil {
			return err
		}
		d.Previous = r.ID + ".previous" + filepath.Ext(before.Artifact)
		if err := os.WriteFile(filepath.Join(out, d.Previous), data, 0o600); err != nil {
			return err
		}
	}
	if r.Artifact != "" {
		data, err := picturelimit.Read(os.DirFS(out), r.Artifact)
		if err != nil {
			return err
		}
		b, err = decodeShot(data)
		if err != nil {
			return err
		}
	}
	pixels, mask := compareShotImages(a, b)
	d.DiffPixels, d.DiffRatio, d.DiffBBox, d.AnimationChanged = pixels.DiffPixels, pixels.DiffRatio, pixels.DiffBBox, pixels.AnimationChanged
	d.Structure = diffStructure(before.Screen, r.Screen)
	d.Changed = pixels.Changed || !exists || before.Status != r.Status || before.Reason != r.Reason
	for _, v := range d.Structure {
		d.Changed = d.Changed || len(v.Added)+len(v.Removed)+len(v.Changed) > 0
	}
	if d.Changed && !mask.Bounds().Empty() {
		d.Highlight = r.ID + ".diff.png"
		var buf bytes.Buffer
		if err := png.Encode(&buf, mask); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, d.Highlight), buf.Bytes(), 0o600); err != nil {
			return err
		}
	}
	return nil
}
