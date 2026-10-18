package command

import (
	"fmt"
	"image"
	"math"
	"sort"

	"github.com/nao1215/gozen/internal/mosaic"
	"github.com/nao1215/gozen/internal/mosaic/detect"
	"github.com/nao1215/gozen/internal/mosaic/settings"
)

type params struct {
	labels               map[string]bool
	minScore, sureScore  float32
	margin               float64
	padBlocks, minBlocks int
}

var defaultParams = params{labels: targetLabels(), minScore: settings.Defaults.MinScore, sureScore: settings.Defaults.SureScore}

func targetLabels() map[string]bool {
	m := map[string]bool{}
	for _, l := range settings.Defaults.Targets {
		m[l] = true
	}
	return m
}

var (
	penisTags = []string{"penis"}
	pussyTags = []string{"pussy"}
	anusTags  = []string{"anal"}
	sexTags   = []string{"sex"}
	tagSure   = settings.Defaults.TagSure
	tagVague  = settings.Defaults.TagVague
)

func rating(tags map[string]float32) string {
	best, r := float32(-1), ""
	for _, k := range []string{"general", "sensitive", "questionable", "explicit"} {
		if v, ok := tags[k]; ok && v > best {
			best, r = v, k
		}
	}
	return r
}
func tagMax(tags map[string]float32, names []string) float32 {
	var m float32
	for _, n := range names {
		m = max(m, tags[n])
	}
	return m
}

type area struct {
	detect.Group
	Rect image.Rectangle
	Box  bool
}

// plan applies confidence, explicit drops, and geometry without private
// calibration rules. Auxiliary evidence can request review, never clear it.
func plan(bounds image.Rectangle, boxes []detect.Box, faces []image.Rectangle, fx mosaic.Fix, tags map[string]float32, block int, p params) (groups, skipped []detect.Group, regions []image.Rectangle, reasons []string, areas []area) {
	var wanted []detect.Box
	if !fx.Skip {
	next:
		for _, b := range boxes {
			if !p.labels[b.Label] || b.Score < p.minScore {
				continue
			}
			for _, d := range fx.Drop {
				if mosaic.Rect(d).Overlaps(b.Rect) {
					continue next
				}
			}
			wanted = append(wanted, b)
		}
	}
	for _, g := range detect.Cluster(wanted, 0.5, 1) {
		r := g.Rect.Intersect(bounds)
		if r.Empty() {
			continue
		}
		contained := false
		for _, f := range faces {
			if r.In(f) {
				contained = true
			}
		}
		if contained && g.Label != "penis" {
			skipped = append(skipped, g)
			reasons = append(reasons, "face-contained detection requires manual review")
			continue
		}
		groups = append(groups, g)
		margin := p.margin
		if fx.Margin > 0 {
			margin = fx.Margin
		}
		mx := max(p.padBlocks*block, int(math.Ceil(float64(r.Dx())*margin)))
		my := max(p.padBlocks*block, int(math.Ceil(float64(r.Dy())*margin)))
		r = mosaic.Pad(r, mx, my, bounds)
		r = mosaic.Grow(r, p.minBlocks*block, p.minBlocks*block, bounds)
		r, grown := growRect(r, fx, bounds)
		if r.Empty() {
			continue
		}
		regions = append(regions, r)
		areas = append(areas, area{Group: g, Rect: r, Box: grown})
		if g.Score < p.sureScore {
			reasons = append(reasons, fmt.Sprintf("low confidence: %s %.2f", g.Label, g.Score))
		}
	}
	regions = mosaic.Merge(regions, 0)
	sort.Slice(regions, func(i, j int) bool {
		a, b := regions[i], regions[j]
		return a.Min.Y < b.Min.Y || a.Min.Y == b.Min.Y && a.Min.X < b.Min.X
	})
	if len(groups) == 0 && !fx.Skip && len(tags) > 0 {
		reasons = append(reasons, "tag evidence requires manual review")
	}
	if fx.Checked {
		reasons = nil
	}
	return
}
