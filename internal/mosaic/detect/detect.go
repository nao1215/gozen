package detect

import (
	"fmt"
	"image"
	"os"
	"sort"
	"strings"

	"github.com/nao1215/gozen/internal/mosaic/settings"
)

type Box struct {
	Rect  image.Rectangle
	Label string
	Score float32
	Pass  string
}

type Pass struct {
	Name    string
	Size    int
	Tiles   int
	Overlap float64
	Flip    bool
}

var DefaultPasses = []Pass{
	{Name: "full640", Size: 640},
	{Name: "flip640", Size: 640, Flip: true},
	{Name: "native", Size: 0},
	{Name: "flipnative", Size: 0, Flip: true},
	{Name: "tile2", Size: 0, Tiles: 2, Overlap: 1.0 / 3},
	{Name: "tile2x", Size: 1280, Tiles: 2, Overlap: 1.0 / 3},
	{Name: "tile3x", Size: 960, Tiles: 3, Overlap: 1.0 / 3},
}

var ExtraPasses = []Pass{
	{Name: "fliptile2x", Size: 1280, Tiles: 2, Overlap: 1.0 / 3, Flip: true},
}

func PassesByName(names string) ([]Pass, error) {
	all := append(append([]Pass{}, DefaultPasses...), ExtraPasses...)
	var out []Pass
	for n := range strings.SplitSeq(names, ",") {
		n = strings.TrimSpace(n)
		switch n {
		case "":
			continue
		case "default":
			out = append(out, DefaultPasses...)
			continue
		case "all":
			out = append(out, all...)
			continue
		}
		found := false
		for _, p := range all {
			if p.Name == n {
				out = append(out, p)
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("no pass named %q", n)
		}
	}
	return out, nil
}

var Labels = settings.Defaults.DetectionLabels

var FaceLabels = []string{"face"}

func LibraryPath() string { return os.Getenv("ONNXRUNTIME_LIB") }

func tiles(b image.Rectangle, n int, ov float64) []image.Rectangle {
	tw := int(float64(b.Dx()) / (float64(n) - ov*float64(n-1)))
	th := int(float64(b.Dy()) / (float64(n) - ov*float64(n-1)))
	var out []image.Rectangle
	for j := range n {
		for i := range n {
			x := b.Min.X + int(float64(i)*float64(tw)*(1-ov))
			y := b.Min.Y + int(float64(j)*float64(th)*(1-ov))
			r := image.Rect(x, y, x+tw, y+th)
			if i == n-1 {
				r.Max.X = b.Max.X
			}
			if j == n-1 {
				r.Max.Y = b.Max.Y
			}
			out = append(out, r.Intersect(b))
		}
	}
	return out
}

func Cluster(boxes []Box, iou, contain float64) []Group {
	sorted := append([]Box(nil), boxes...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Score > sorted[j].Score })
	var gs []Group
	for _, b := range sorted {
		joined := -1
		for i := range gs {
			if same(gs[i].Rect, b.Rect, iou, contain) {
				joined = i
				break
			}
		}
		if joined < 0 {
			gs = append(gs, Group{Box: b, passes: map[string]bool{}})
			joined = len(gs) - 1
		} else {
			gs[joined].Rect = gs[joined].Rect.Union(b.Rect)
		}
		gs[joined].Hits++
		gs[joined].passes[b.Pass] = true
	}

	for changed := true; changed; {
		changed = false
		for i := 0; i < len(gs) && !changed; i++ {
			for j := i + 1; j < len(gs); j++ {
				if same(gs[i].Rect, gs[j].Rect, iou, contain) {
					gs[i].Rect = gs[i].Rect.Union(gs[j].Rect)
					gs[i].Hits += gs[j].Hits
					for p := range gs[j].passes {
						gs[i].passes[p] = true
					}
					gs = append(gs[:j], gs[j+1:]...)
					changed = true
					break
				}
			}
		}
	}
	for i := range gs {
		gs[i].Passes = len(gs[i].passes)
	}
	return gs
}

type Group struct {
	Box
	Hits   int
	Passes int
	passes map[string]bool
}

func area(r image.Rectangle) float64 { return float64(r.Dx()) * float64(r.Dy()) }

func same(a, b image.Rectangle, iou, contain float64) bool {
	in := area(a.Intersect(b))
	if in == 0 {
		return false
	}
	if in/(area(a)+area(b)-in) > iou {
		return true
	}
	return in/min(area(a), area(b)) >= contain
}
