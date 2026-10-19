package command

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"sort"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/nao1215/gozen/internal/mosaic"
)

func sortEntries(es []entry) {
	sort.Slice(es, func(i, j int) bool { return es[i].File < es[j].File })
}

const (
	thumbW, thumbH = 480, 270
	sheetCols      = 3
	sheetRows      = 4
)

func writeReview(c config, all []entry) error {
	var rev []entry
	for _, e := range all {
		if len(e.Review) > 0 {
			rev = append(rev, e)
		}
	}
	dir := filepath.Join(c.out, "review")
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	var md strings.Builder
	fmt.Fprintf(&md, "# モザイクの要確認（%d 枚）\n\n", len(rev))
	md.WriteString("review/sheet_NN.jpg の番号と対応する。黄色の枠が自動の範囲、水色・桃色が手で足した範囲。確かめたら `gozen mosaic manual check`（人は --fix の YAML に `checked: true`）。\n")
	md.WriteString("漏れは `add: [[x, y, 幅, 高さ]]`（元の絵の px）、誤検出は `drop: [[x, y, 幅, 高さ]]` で直す。\n\n")
	if len(rev) == 0 {
		return os.WriteFile(filepath.Join(c.out, "review.md"), []byte(md.String()), 0o600)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	per := sheetCols * sheetRows
	for s := 0; s*per < len(rev); s++ {
		sheet := image.NewRGBA(image.Rect(0, 0, sheetCols*thumbW, sheetRows*thumbH))
		draw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.RGBA{40, 40, 40, 255}), image.Point{}, draw.Src)
		name := fmt.Sprintf("sheet_%02d.jpg", s+1)
		fmt.Fprintf(&md, "## %s\n\n", name)
		for k := 0; k < per && s*per+k < len(rev); k++ {
			e := rev[s*per+k]
			n := s*per + k + 1
			fmt.Fprintf(&md, "- #%d `%s`: %s\n", n, e.File, strings.Join(e.Review, "。"))
			src := filepath.Join(c.gameDir, "assets", filepath.FromSlash(e.File))
			if e.Output != "" {
				src = filepath.Join(c.out, filepath.FromSlash(e.Output))
			}
			img, err := decode(src)
			if err != nil {
				c.diag.printf("warning: 一覧に %s を描けない: %v\n", e.File, err)
				continue
			}
			cell := image.Rect(k%sheetCols*thumbW, k/sheetCols*thumbH, (k%sheetCols+1)*thumbW, (k/sheetCols+1)*thumbH)
			thumb(sheet, cell, img, e, fmt.Sprintf("#%d", n))
		}
		md.WriteString("\n")
		if err := encode(filepath.Join(dir, name), sheet, 88); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(c.out, "review.md"), []byte(md.String()), 0o600)
}

func thumb(dst *image.RGBA, cell image.Rectangle, img image.Image, e entry, label string) {
	b := img.Bounds()
	scale := min(float64(cell.Dx()-4)/float64(b.Dx()), float64(cell.Dy()-4)/float64(b.Dy()))
	w, h := int(float64(b.Dx())*scale), int(float64(b.Dy())*scale)
	at := image.Rect(cell.Min.X+(cell.Dx()-w)/2, cell.Min.Y+(cell.Dy()-h)/2, 0, 0)
	at.Max = at.Min.Add(image.Pt(w, h))
	xdraw.ApproxBiLinear.Scale(dst, at, img, b, draw.Src, nil)
	frame := func(r [4]int, c color.RGBA) {
		x0 := at.Min.X + int(float64(r[0])*scale)
		y0 := at.Min.Y + int(float64(r[1])*scale)
		x1 := at.Min.X + int(float64(r[0]+r[2])*scale)
		y1 := at.Min.Y + int(float64(r[1]+r[3])*scale)
		outline(dst, image.Rect(x0, y0, x1, y1).Intersect(at), c)
	}
	for _, r := range e.Regions {
		frame(r, color.RGBA{255, 220, 0, 255})
	}
	for _, r := range e.Adds {
		frame(r, color.RGBA{0, 220, 255, 255})
	}
	for _, p := range e.Polys {
		r := mosaic.Fix{Poly: [][]int{p}}.Polygons()[0].Bounds()
		frame(xywh(r), color.RGBA{255, 80, 200, 255})
	}
	tag := image.Rect(at.Min.X, at.Min.Y, at.Min.X+8*len(label)+8, at.Min.Y+18)
	draw.Draw(dst, tag, image.NewUniform(color.RGBA{0, 0, 0, 255}), image.Point{}, draw.Src)
	d := font.Drawer{Dst: dst, Src: image.NewUniform(color.White), Face: basicfont.Face7x13,
		Dot: fixed.P(tag.Min.X+4, tag.Min.Y+14)}
	d.DrawString(label)
}

func outline(dst *image.RGBA, r image.Rectangle, c color.RGBA) {
	for t := range 2 {
		for x := r.Min.X; x < r.Max.X; x++ {
			dst.SetRGBA(x, r.Min.Y+t, c)
			dst.SetRGBA(x, r.Max.Y-1-t, c)
		}
		for y := r.Min.Y; y < r.Max.Y; y++ {
			dst.SetRGBA(r.Min.X+t, y, c)
			dst.SetRGBA(r.Max.X-1-t, y, c)
		}
	}
}
