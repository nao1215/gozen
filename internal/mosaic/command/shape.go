package command

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"

	"github.com/nao1215/gozen/internal/mosaic"
	"github.com/nao1215/gozen/internal/mosaic/segment"
	"github.com/nao1215/gozen/internal/mosaic/settings"
)

var (
	segMin      = settings.Defaults.SegMin
	segAlone    = settings.Defaults.SegAlone
	segMinCover = settings.Defaults.SegMinCover
)

var segGenital = map[string]bool{"penis": true, "vagina": true}

func (c *config) segObjects(img image.Image) ([]segment.Object, error) {
	if c.seg == nil {
		return nil, nil
	}
	var all []segment.Object
	for _, flip := range []bool{false, true} {
		objs, err := c.seg.Find(img, segMin, flip)
		if err != nil {
			return nil, err
		}
		all = append(all, objs...)
	}
	var out []segment.Object
	for _, o := range all {
		merged := false
		for i := range out {
			if out[i].Label == o.Label && out[i].Rect.Overlaps(o.Rect) {
				r := out[i].Rect.Union(o.Rect)
				m := unionMask(r, []*image.Alpha{out[i].Mask, o.Mask})
				out[i].Rect, out[i].Mask, out[i].Score = r, m.Alpha, max(out[i].Score, o.Score)
				merged = true
				break
			}
		}
		if !merged {
			out = append(out, o)
		}
	}
	return out, nil
}

func (c *config) segZoom(img image.Image, g image.Rectangle) ([]*image.Alpha, float32, error) {
	pad := max(g.Dx(), g.Dy())
	r := g.Inset(-pad).Intersect(img.Bounds())
	sub, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		return nil, 0, nil
	}
	var out []*image.Alpha
	var best float32
	for _, flip := range []bool{false, true} {
		objs, err := c.seg.Find(sub.SubImage(r), segMin, flip)
		if err != nil {
			return nil, 0, err
		}
		for _, o := range objs {
			if segGenital[o.Label] && o.Rect.Overlaps(g) {
				out = append(out, o.Mask)
				best = max(best, o.Score)
			}
		}
	}
	return out, best, nil
}

func unionMask(r image.Rectangle, masks []*image.Alpha) mosaic.Mask {
	m := image.NewAlpha(r)
	for _, src := range masks {
		c := src.Rect.Intersect(r)
		for y := c.Min.Y; y < c.Max.Y; y++ {
			for x := c.Min.X; x < c.Max.X; x++ {
				if src.Pix[src.PixOffset(x, y)] != 0 {
					m.Pix[m.PixOffset(x, y)] = 255
				}
			}
		}
	}
	return mosaic.Mask{Alpha: m}
}

func (c *config) autoShapes(img image.Image, regions []image.Rectangle, areas []area, tags map[string]float32, fx mosaic.Fix, block int) (shapes []mosaic.Shape, kinds, extra []string, err error) {
	if c.seg == nil || c.opts.shape != "mask" {
		for _, r := range regions {
			shapes = append(shapes, mosaic.Box(r))
		}
		return shapes, nil, nil, nil
	}
	b := img.Bounds()
	objs, err := c.segObjects(img)
	if err != nil {
		return nil, nil, nil, err
	}
	used := make([]bool, len(objs))
	for _, a := range areas {
		if a.Box {
			shapes = append(shapes, mosaic.Box(a.Rect))
			kinds = append(kinds, "box（grow）")
			continue
		}
		g := a.Group.Rect
		d := c.dilate(block, a.Label)
		limit := a.Rect.Union(g.Inset(-d))
		var masks []*image.Alpha
		for i, o := range objs {
			if segGenital[o.Label] && o.Rect.Overlaps(g) {
				masks = append(masks, o.Mask)
				used[i] = true
			}
		}
		m := unionMask(limit, masks)
		cover := float64(m.Area(g)) / float64(max(1, g.Dx()*g.Dy()))
		if len(masks) == 0 || cover < segMinCover {

			zm, zs, err := c.segZoom(img, g)
			if err != nil {
				return nil, nil, nil, err
			}
			if len(zm) > 0 {
				m = unionMask(limit, append(masks, zm...))
				if zc := float64(m.Area(g)) / float64(max(1, g.Dx()*g.Dy())); zc >= segMinCover {
					shapes = append(shapes, dryDilate(img, m, d, b).Clip(limit))
					kinds = append(kinds, fmt.Sprintf("mask（拡大して分割モデルの性器 %.2f が枠の %.0f%%）", zs, 100*zc))
					continue
				}
			}
			shapes = append(shapes, mosaic.Box(a.Rect))
			kinds = append(kinds, fmt.Sprintf("box（分割モデルの性器が枠の %.0f%%）", 100*cover))
			continue
		}
		shapes = append(shapes, dryDilate(img, m, d, b).Clip(limit))
		kinds = append(kinds, fmt.Sprintf("mask（分割モデルの性器が枠の %.0f%%）", 100*cover))
	}
	if len(shapes) == 0 && len(areas) > 0 && (tagMax(tags, penisTags) >= tagSure || tagMax(tags, pussyTags) >= tagSure) {
		extra = append(extra, "検出した枠を全部誤検出とみなしたが、タグでは性器がある（見落としていないか）")
	}
	if fx.Skip {
		return shapes, kinds, extra, nil
	}
	anal := tagMax(tags, anusTags) >= tagSure

	tagged := map[string]bool{"penis": tagMax(tags, penisTags) >= tagSure, "vagina": tagMax(tags, pussyTags) >= tagSure}
	rate := rating(tags)
next:
	for i, o := range objs {
		if used[i] {
			continue
		}
		for _, dr := range fx.Drop {
			if mosaic.Rect(dr).Overlaps(o.Rect) {
				continue next
			}
		}
		switch {
		case segGenital[o.Label] && o.Score >= segAlone && rate != "general" && tagged[o.Label]:
			extra = append(extra, fmt.Sprintf("検出モデルは見落としたが分割モデルが %s を見つけた（%.2f、%v）。誤検出なら drop", o.Label, o.Score, xywh(o.Rect)))
		case segGenital[o.Label] && o.Score >= segAlone && rate != "general" && tags == nil:
			extra = append(extra, fmt.Sprintf("分割モデルだけが %s を見つけた（%.2f、%v。タグが無いので掛けていない。性器なら manual で足す）", o.Label, o.Score, xywh(o.Rect)))
			continue
		case o.Label == "anus" && anal && o.Score >= segMin:
			extra = append(extra, fmt.Sprintf("アナルの挿入か排泄のタグがあり、分割モデルがアヌスを見つけた（%.2f、%v）", o.Score, xywh(o.Rect)))
		default:
			continue
		}

		if r, ok := growRect(o.Rect, fx, b); ok {
			shapes = append(shapes, mosaic.Box(r))
			kinds = append(kinds, fmt.Sprintf("box（分割モデルだけ、%s、grow）", o.Label))
			continue
		}
		m := mosaic.Mask{Alpha: o.Mask}
		d := c.dilate(block, o.Label)
		shapes = append(shapes, dryDilate(img, m, d, b).Clip(o.Rect.Inset(-d)))
		kinds = append(kinds, fmt.Sprintf("mask（分割モデルだけ、%s）", o.Label))
	}
	return shapes, kinds, extra, nil
}

func growRect(r image.Rectangle, fx mosaic.Fix, bounds image.Rectangle) (image.Rectangle, bool) {
	grown := false
	for _, gr := range fx.Grow {
		if mosaic.Rect([4]int{gr[0], gr[1], gr[2], gr[3]}).Overlaps(r) {

			r = image.Rectangle{Min: image.Pt(r.Min.X-gr[4], r.Min.Y-gr[5]), Max: image.Pt(r.Max.X+gr[6], r.Max.Y+gr[7])}.Intersect(bounds)
			grown = true
		}
	}
	return r, grown
}

func writeMask(path string, b image.Rectangle, shapes []mosaic.Shape, block int, off image.Point) error {
	m := image.NewGray(b)
	for _, cell := range mosaic.CellsAt(b, shapes, block, off) {
		for y := cell.Min.Y; y < cell.Max.Y; y++ {
			for x := cell.Min.X; x < cell.Max.X; x++ {
				m.SetGray(x, y, color.Gray{255})
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return mosaic.WriteFile(path, func(w io.Writer) error { return png.Encode(w, m) })
}

func readMask(path string) *image.Alpha {
	img, err := decode(path)
	if err != nil {
		return nil
	}
	b := img.Bounds()
	a := image.NewAlpha(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if r, _, _, _ := img.At(x, y).RGBA(); r > 0x8000 {
				a.Pix[a.PixOffset(x, y)] = 255
			}
		}
	}
	return a
}

func maskPath(out, rel string) string {
	return filepath.Join(out, "masks", filepath.FromSlash(rel)+".png")
}

func (c *config) dilate(block int, _ string) int {
	if c.opts.dilate >= 0 {
		return c.opts.dilate
	}
	return block
}

func dryDilate(_ image.Image, m mosaic.Mask, d int, b image.Rectangle) mosaic.Mask {
	return m.Dilate(d, b)
}
