package command

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nao1215/gozen/internal/diag"

	"github.com/nao1215/gozen/internal/mosaic"
	"github.com/nao1215/gozen/internal/mosaic/detect"
)

type inspection struct {
	Source      string     `json:"source"`
	File        string     `json:"file"`
	Path        string     `json:"path"`
	Size        [2]int     `json:"size"`
	NeedsMosaic bool       `json:"needs_mosaic"`
	Categories  []string   `json:"categories"`
	Evidence    []string   `json:"evidence"`
	Objects     []object   `json:"objects"`
	Faces       [][4]int   `json:"faces,omitempty"`
	Candidates  []object   `json:"candidates"`
	Mosaicked   mosaicked  `json:"mosaicked"`
	Status      string     `json:"status"`
	Review      []string   `json:"review"`
	Rating      string     `json:"rating"`
	Tags        tagSummary `json:"tags,omitzero"`
	Manual      *manualFix `json:"manual,omitzero"`
}

type object struct {
	Label    string  `json:"label"`
	Category string  `json:"category"`
	Score    float32 `json:"score"`
	Passes   int     `json:"passes"`
	Rect     [4]int  `json:"rect"`
	Mosaic   bool    `json:"mosaic"`
	Block    int     `json:"block,omitempty"`
}

type mosaicked struct {
	InImage       bool   `json:"in_image"`
	Output        string `json:"output,omitempty"`
	OutputCovers  bool   `json:"output_covers"`
	OutputBlock   int    `json:"output_block,omitempty"`
	StandardBlock int    `json:"standard_block"`
	BlockOK       bool   `json:"block_ok"`
	Note          string `json:"note,omitempty"`
}

type tagSummary map[string]float32

type manualFix struct {
	Add     [][4]int `json:"add,omitempty"`
	Drop    [][4]int `json:"drop,omitempty"`
	Poly    [][]int  `json:"poly,omitempty"`
	Skip    bool     `json:"skip,omitempty"`
	Checked bool     `json:"checked,omitempty"`
	Note    string   `json:"note,omitempty"`
}

var categoryJa = map[string]string{"penis": "男性器", "pussy": "女性器", "nipple_f": "乳首"}

func (cli *cli) inspectCmd(args []string) (err error) {
	fs := newFlagSet("gozen art mosaic inspect")
	o := cli.commonFlags(fs)
	pretty := fs.Bool("pretty", false, "write indented JSON instead of one line per image")
	redetect := fs.Bool("redetect", false, "detect every image with the model again instead of using report.json (slow)")
	if err := cli.parse(fs, args, inspectPage); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return argumentError{diag.Operands.Errorf("expected a game directory: gozen art mosaic inspect [OPTIONS] <game-dir> [IMAGE ...]")}
	}
	gameDir := fs.Arg(0)
	c, err := o.setup(gameDir)
	if err != nil {
		return err
	}
	pics := fs.Args()[1:]
	if len(pics) == 0 {
		if pics, _, err = mosaic.Pictures(gameDir); err != nil {
			return err
		}
	}
	c.redetect = *redetect
	c.showClasses = true
	c.printClasses()
	c.tags.readOnly = true
	defer func() { err = errors.Join(err, c.close()) }()
	report := map[string]entry{}
	for _, e := range readReport(c) {
		report[e.File] = e
	}
	enc := json.NewEncoder(c.outw)
	enc.SetEscapeHTML(false)
	if *pretty {
		enc.SetIndent("", "  ")
	}
	for _, p := range pics {
		in, err := inspect(&c, p, report)
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		if err := enc.Encode(in); err != nil {
			return err
		}
	}
	return nil
}

func resolve(c config, arg string) (rel, path, orig string) {
	assets := filepath.Join(c.gameDir, "assets")
	if st, err := os.Stat(arg); err == nil && !st.IsDir() {
		abs, err := filepath.Abs(arg)
		if err != nil {
			return "", arg, arg
		}
		for _, root := range []string{assets, c.out} {
			r, err := filepath.Abs(root)
			if err != nil {
				continue
			}
			if rr, err := filepath.Rel(r, abs); err == nil && !strings.HasPrefix(rr, "..") {
				rel = filepath.ToSlash(rr)
				return rel, arg, filepath.Join(assets, filepath.FromSlash(rel))
			}
		}
		return "", arg, arg
	}
	rel = filepath.ToSlash(strings.TrimPrefix(arg, "assets/"))
	p := filepath.Join(assets, filepath.FromSlash(rel))
	return rel, p, p
}

func inspect(c *config, arg string, report map[string]entry) (inspection, error) {
	rel, path, origPath := resolve(*c, arg)
	in := inspection{File: rel, Path: path, Categories: []string{}, Evidence: []string{}, Objects: []object{}, Candidates: []object{}, Review: []string{}}
	img, err := decode(path)
	if err != nil {
		return in, err
	}
	orig := img
	if origPath != path {
		if orig, err = decode(origPath); err != nil {
			return in, err
		}
	}
	b := orig.Bounds()
	in.Size = [2]int{b.Dx(), b.Dy()}
	in.Source, err = fileSum(origPath)
	if err != nil {
		return in, err
	}
	std := mosaic.DefaultBlock(b.Dx(), b.Dy())
	in.Mosaicked.StandardBlock = std

	fx := c.fixes[rel]
	tags, err := c.tags.get(rel, origPath)
	if err != nil {
		return in, err
	}
	in.Rating = rating(tags)
	if len(tags) > 0 {
		in.Tags = tagSummary{}
		for _, names := range [][]string{penisTags, pussyTags, sexTags, {"anus", "nipples", "mosaic_censoring", "censored", "uncensored"}} {
			for _, n := range names {
				if v := tags[n]; v >= tagVague {
					in.Tags[n] = v
				}
			}
		}
	}
	if m, ok := c.manual[rel]; ok && !m.Empty() {
		in.Manual = &manualFix{m.Add, m.Drop, m.Poly, m.Skip, m.Checked, m.Note}
	}

	e, inReport := report[rel]
	if rel != "" && inReport && !c.redetect && e.FixSum == fixSum(fx) {
		if sum, err := fileSum(origPath); err == nil && sum == e.Source {
			fromReport(c, &in, e, fx, tags, std)
			return in, nil
		}
	}

	var boxes []detect.Box
	if !fx.Skip {
		if err := c.open(); err != nil {
			return in, err
		}
		if boxes, err = c.det.Detect(orig, c.passes, c.p.minScore); err != nil {
			return in, err
		}
	}
	block := std
	if c.block > 0 {
		block = c.block
	}
	faces, err := c.faces(orig)
	if err != nil {
		return in, err
	}
	for _, f := range faces {
		in.Faces = append(in.Faces, xywh(f))
	}
	groups, skipped, regions, reasons, _ := plan(b, boxes, faces, fx, tags, block, c.p)
	in.Review = append(in.Review, reasons...)
	cats := map[string]bool{}
	allIn := len(groups) > 0
	for _, g := range groups {
		f := mosaic.Find(img, g.Rect)
		in.Objects = append(in.Objects, object{g.Label, categoryJa[g.Label], g.Score, g.Passes, xywh(g.Rect), f.Mosaic, f.Block})
		cats[g.Label] = true
		in.Evidence = append(in.Evidence, fmt.Sprintf("検出 %s %.2f（見方 %d）", g.Label, g.Score, g.Passes))
		allIn = allIn && f.Mosaic
	}
	for _, g := range skipped {
		f := mosaic.Find(img, g.Rect)
		in.Candidates = append(in.Candidates, object{g.Label, categoryJa[g.Label], g.Score, g.Passes, xywh(g.Rect), f.Mosaic, f.Block})
	}
	nManual := len(fx.Add) + len(fx.Poly)
	if nManual > 0 {
		in.Evidence = append(in.Evidence, fmt.Sprintf("手の指示 %d 箇所", nManual))
	}
	if tags != nil {
		if v := tagMax(tags, penisTags); v >= tagSure {
			cats["penis"] = true
			in.Evidence = append(in.Evidence, fmt.Sprintf("タグ 男性器 %.2f", v))
		}
		if v := tagMax(tags, pussyTags); v >= tagSure {
			cats["pussy"] = true
			in.Evidence = append(in.Evidence, fmt.Sprintf("タグ 女性器 %.2f", v))
		}
	}
	for l := range cats {
		if c.p.labels[l] {
			in.Categories = append(in.Categories, categoryJa[l])
		}
	}
	sort.Strings(in.Categories)
	in.NeedsMosaic = len(groups) > 0 || nManual > 0 || len(in.Categories) > 0

	in.Mosaicked.InImage = allIn
	if allIn {
		bmin := 0
		for _, o := range in.Objects {
			if bmin == 0 || o.Block < bmin {
				bmin = o.Block
			}
		}
		in.Mosaicked.BlockOK = bmin >= std
		in.Mosaicked.Note = fmt.Sprintf("この絵の升目の目安 %dpx（推定。JPEG では小さく出ることがある）", bmin)
	}

	outPath := ""
	if rel != "" {
		p := filepath.Join(c.out, filepath.FromSlash(rel))
		if _, err := os.Stat(p); err == nil {
			outPath = p
		}
	}
	if outPath != "" {
		in.Mosaicked.Output = outPath
		out, err := decode(outPath)
		if err != nil {
			return in, err
		}
		covers := true
		shapes := make([]mosaic.Shape, 0, len(regions)+nManual)
		if inReport {
			for _, r := range e.Regions {
				shapes = append(shapes, mosaic.Box(mosaic.Rect(r)))
			}
			for _, a := range e.Adds {
				shapes = append(shapes, mosaic.Box(mosaic.Rect(a)))
			}
			for _, pg := range (mosaic.Fix{Poly: e.Polys}).Polygons() {
				shapes = append(shapes, pg)
			}
			in.Mosaicked.OutputBlock = e.Block
		}
		painted := readMask(maskPath(c.out, rel))
		for _, o := range in.Objects {
			r := mosaic.Rect(o.Rect)
			if painted != nil {

				if frac := float64(mosaic.Mask{Alpha: painted}.Area(r)) / float64(max(1, r.Dx()*r.Dy())); frac < 0.30 {
					covers = false
				}
				continue
			}
			if !coveredBy(r, shapes) || !mosaic.Find(out, r).Mosaic {
				covers = false
			}
		}
		in.Mosaicked.OutputCovers = covers && inReport
		if !inReport {
			in.Mosaicked.Note = "出力はあるが report.json に無い（auto を動かし直す）"
		}
		if in.Mosaicked.OutputBlock > 0 {
			in.Mosaicked.BlockOK = in.Mosaicked.OutputBlock >= std
		}
		if inReport && e.Output == "" {
			in.Mosaicked.Note = "report.json では出力が無いことになっている（古い出力。auto を動かし直す）"
		}
	} else if in.NeedsMosaic && rel != "" {
		in.Mosaicked.Note = "出力が無い（auto を動かす）"
	}
	stale := rel != "" && in.NeedsMosaic
	if stale {
		in.Mosaicked.OutputCovers = false
		in.Mosaicked.Note = "report.json が今の元の絵か手直しと合わない（auto を動かし直す）"
	}

	switch {
	case len(in.Review) > 0 && !fx.Checked:
		in.Status = "review"
	case !in.NeedsMosaic:
		in.Status = "none"
	case in.Mosaicked.OutputCovers && in.Mosaicked.BlockOK || in.Mosaicked.InImage && in.Mosaicked.BlockOK:
		in.Status = "done"
	default:
		in.Status = "todo"
	}
	if fx.Checked {
		in.Review = []string{}
	}
	return in, nil
}

func coveredBy(r image.Rectangle, shapes []mosaic.Shape) bool {
	for y := r.Min.Y; y < r.Max.Y; y += 2 {
		for x := r.Min.X; x < r.Max.X; x += 2 {
			cell := image.Rect(x, y, x+1, y+1)
			in := false
			for _, s := range shapes {
				if s.Bounds().Overlaps(cell) && s.Hits(cell) {
					in = true
					break
				}
			}
			if !in {
				return false
			}
		}
	}
	return true
}

func fromReport(c *config, in *inspection, e entry, fx mosaic.Fix, tags map[string]float32, std int) {
	cats := map[string]bool{}
	for _, f := range e.Found {
		o := object{Label: f.Label, Category: categoryJa[f.Label], Score: f.Score, Passes: f.Passes, Rect: f.Rect}
		if strings.HasPrefix(f.Shape, "skip") {
			in.Candidates = append(in.Candidates, o)
			continue
		}
		o.Mosaic, o.Block = e.Output != "", e.Block
		in.Objects = append(in.Objects, o)
		cats[f.Label] = true
		in.Evidence = append(in.Evidence, fmt.Sprintf("検出 %s %.2f（見方 %d）", f.Label, f.Score, f.Passes))
	}
	for _, f := range e.Skipped {
		in.Candidates = append(in.Candidates, object{Label: f.Label, Category: categoryJa[f.Label], Score: f.Score, Passes: f.Passes, Rect: f.Rect})
	}
	segOnly := 0
	for _, k := range e.Shapes {
		if strings.Contains(k, "分割モデルだけ") {
			segOnly++
		}
	}
	if segOnly > 0 {
		in.Evidence = append(in.Evidence, fmt.Sprintf("分割モデルだけで見つけた物 %d 箇所", segOnly))
	}
	if n := len(e.Adds) + len(e.Polys); n > 0 {
		in.Evidence = append(in.Evidence, fmt.Sprintf("手の指示 %d 箇所", n))
	}
	if tags != nil {
		if v := tagMax(tags, penisTags); v >= tagSure {
			cats["penis"] = true
			in.Evidence = append(in.Evidence, fmt.Sprintf("タグ 男性器 %.2f", v))
		}
		if v := tagMax(tags, pussyTags); v >= tagSure {
			cats["pussy"] = true
			in.Evidence = append(in.Evidence, fmt.Sprintf("タグ 女性器 %.2f", v))
		}
	}
	for l := range cats {
		if c.p.labels[l] {
			in.Categories = append(in.Categories, categoryJa[l])
		}
	}
	sort.Strings(in.Categories)
	in.Faces = e.Faces
	painted := e.Output != ""
	in.NeedsMosaic = painted || len(in.Categories) > 0
	if fx.Checked && !painted {
		in.NeedsMosaic = false
	}
	in.Review = append(in.Review, e.Review...)
	if painted {
		p := filepath.Join(c.out, filepath.FromSlash(e.Output))
		if _, err := os.Stat(p); err == nil {
			in.Mosaicked.Output = p
			in.Mosaicked.OutputCovers = true
			in.Mosaicked.OutputBlock = e.Block
			in.Mosaicked.BlockOK = e.Block >= std
		} else {
			in.Mosaicked.Note = "report.json に出力があるのにファイルが無い（auto を動かし直す）"
		}
	} else if in.NeedsMosaic {
		in.Mosaicked.Note = "auto が塗った物が無い（性器が写っていれば manual で足す。写っていなければ check）"
	}
	switch {
	case len(in.Review) > 0 && !fx.Checked:
		in.Status = "review"
	case !in.NeedsMosaic:
		in.Status = "none"
	case in.Mosaicked.OutputCovers && in.Mosaicked.BlockOK:
		in.Status = "done"
	default:
		in.Status = "todo"
	}
	if fx.Checked {
		in.Review = []string{}
	}
}
