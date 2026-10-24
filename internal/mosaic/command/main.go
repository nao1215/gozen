package command

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/cliio"

	"github.com/nao1215/gozen/internal/diag"

	"github.com/spf13/pflag"
	_ "golang.org/x/image/webp" // decodes the WebP pictures a game may hold

	"github.com/nao1215/gozen/internal/mosaic"
	"github.com/nao1215/gozen/internal/mosaic/detect"
	"github.com/nao1215/gozen/internal/mosaic/segment"
	"github.com/nao1215/gozen/internal/mosaic/settings"
)

type options struct {
	outw, diag *cliio.Output
	report     *cliio.Report

	block, padBlocks, minBlocks, threads, quality int
	dilate                                        int
	out, model, lib, labels, tagsFile, fixFile    string
	manualFile, passes, taggerDir, faceModel      string
	shape, segModel, classes                      string
	yes                                           bool
	flags                                         *pflag.FlagSet
	minScore, sure, margin                        float64
}

func env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func (cli *cli) commonFlags(fs *pflag.FlagSet) *options {
	p := defaultParams
	o := &options{outw: cli.out, diag: cli.diag, flags: fs, report: cli.report}
	fs.StringVar(&o.fixFile, "fix", "", "read hand-written fixes from this YAML `FILE`: regions to add, detections to drop, and checked marks per image (env MOSAIC_FIX)")
	fs.StringVar(&o.manualFile, "manual", "", "read, and for manual write, the instructions YAML `FILE` that gozen mosaic keeps (env MOSAIC_MANUAL)")
	fs.StringVar(&o.tagsFile, "tags", "", "the image tag cache JSON `FILE`; missing or stale images are tagged with --tagger and added; empty uses no tags (env MOSAIC_TAGS)")
	fs.StringVar(&o.model, "model", "", "the detection ONNX model `FILE`")
	fs.BoolVar(&o.yes, "yes", false, "download the default model without asking when --model is omitted and it is not in the cache (see gozen models fetch)")
	fs.StringVar(&o.lib, "ort", "", "the ONNX Runtime shared library `FILE` (default: ONNXRUNTIME_LIB)")
	fs.StringVar(&o.faceModel, "face", "", "an optional face detection ONNX model `FILE`; failing to read it is an error")
	fs.StringVar(&o.segModel, "seg", "", "the segmentation ONNX model `FILE` that --shape mask needs")
	fs.StringVar(&o.taggerDir, "tagger", "", "a WD tagger model `DIR` holding model.onnx and selected_tags.csv (env MOSAIC_TAGGER)")
	fs.StringVar(&o.classes, "classes", "", "the class names of the --model output, in the order the model emits them, separated by commas (default: labels.json beside the model, then the ONNX metadata names, then "+strings.Join(detect.Labels, ",")+")")
	fs.StringVar(&o.labels, "labels", strings.Join(settings.Defaults.Targets, ","), "the detector `CLASSES` to pixelate, separated by commas, chosen from --classes (default: penis,pussy, since nipples are not pixelated by default; with --classes, or a model whose classes differ from the default, every class)")
	fs.Float64Var(&o.minScore, "threshold", float64(p.minScore), "pixelate detections scoring at least this `SCORE`; lower misses less and finds more false regions")
	fs.Float64Var(&o.sure, "sure", float64(p.sureScore), "pixelate, but also send to review, a region whose best score is below this `SCORE`")
	fs.StringVar(&o.passes, "passes", "default", "the detection `PASSES`, separated by commas: default, all, or full640, flip640, native, flipnative, tile2, tile2x, tile3x, fliptile2x (env MOSAIC_PASSES)")
	fs.IntVar(&o.threads, "threads", 0, "ONNX Runtime CPU threads; 0 uses half of the CPUs")
	fs.StringVar(&o.shape, "shape", "box", "the `SHAPE` of automatic regions: box (rectangles) or mask (the --seg model's mask) (env MOSAIC_SHAPE)")
	fs.IntVar(&o.block, "block", 0, "the block size in `PIXELS`; 0 uses max(4, ceil(long side / 100)), and a smaller value than that warns")
	fs.Float64Var(&o.margin, "margin", p.margin, "grow each detected box by this `FRACTION` of its sides; the touched blocks are filled whole, so the outline is covered")
	fs.IntVar(&o.padBlocks, "pad", p.padBlocks, "grow each region by at least this many `BLOCKS`")
	fs.IntVar(&o.minBlocks, "min", p.minBlocks, "make each side of a region at least this many `BLOCKS`")
	fs.IntVar(&o.dilate, "dilate", -1, "grow masks by these `PIXELS`; -1 is one block")
	fs.StringVarP(&o.out, "out", "o", "", "write the images and report.json to this `DIR` outside assets/ (default: <game-dir>/assets_mosaic; env MOSAIC_OUT)")
	fs.IntVar(&o.quality, "quality", 90, "the JPEG `QUALITY` from 1 to 100; it does not change the block size")
	return o
}

// resolveEnvironment applies runtime defaults only to omitted options.
func (o *options) resolveEnvironment() {
	if o.flags == nil {
		return
	}
	for _, v := range []struct {
		name, key string
		target    *string
	}{
		{"fix", "MOSAIC_FIX", &o.fixFile}, {"manual", "MOSAIC_MANUAL", &o.manualFile}, {"tags", "MOSAIC_TAGS", &o.tagsFile}, {"tagger", "MOSAIC_TAGGER", &o.taggerDir}, {"out", "MOSAIC_OUT", &o.out}, {"passes", "MOSAIC_PASSES", &o.passes}, {"shape", "MOSAIC_SHAPE", &o.shape},
	} {
		if !o.flags.Changed(v.name) {
			*v.target = env(v.key, *v.target)
		}
	}
	if !o.flags.Changed("ort") {
		o.lib = detect.LibraryPath()
	}
	if o.threads == 0 {
		o.threads = min(1024, max(1, runtime.NumCPU()/2))
	}
}

func (o *options) setup(ctx context.Context, gameDir string) (config, error) {
	o.resolveEnvironment()
	if o.report != nil && o.report.Machine() {
		o.outw = &cliio.Output{Writer: io.Discard}
		o.diag = &cliio.Output{Writer: progress{*o.report, o.diag}}
	}
	if err := o.validate(); err != nil {
		return config{}, err
	}
	p := defaultParams
	p.minScore, p.sureScore = float32(o.minScore), float32(o.sure)
	p.margin, p.padBlocks, p.minBlocks = o.margin, o.padBlocks, o.minBlocks
	if o.model == "" {
		if p, err := (&cli{}).defaultModel(ctx, false, false); err == nil {
			o.model = p
		}
	}
	classes, source, err := o.resolveClasses()
	if err != nil {
		return config{}, err
	}
	labels := o.labels
	if (o.flags == nil || !o.flags.Changed("labels")) &&
		(source == detect.SourceFlag || !slices.Equal(classes, detect.Labels)) {
		labels = strings.Join(classes, ",")
	}
	p.labels = map[string]bool{}
	for l := range strings.SplitSeq(labels, ",") {
		if l = strings.TrimSpace(l); l != "" {
			if !slices.Contains(classes, l) {
				return config{}, fmt.Errorf("unknown label %q: --labels must be chosen from the classes %s", l, strings.Join(classes, ","))
			}
			p.labels[l] = true
		}
	}
	if len(p.labels) == 0 {
		return config{}, errors.New("--labels cannot be empty")
	}
	out := o.out
	if out == "" {
		out = filepath.Join(gameDir, "assets_mosaic")
	}
	c := config{classes: classes, classSource: source, outw: o.outw, diag: o.diag, gameDir: gameDir, out: out, block: o.block, quality: o.quality, p: p, opts: o}
	if o.shape != "mask" && o.shape != "box" {
		return c, fmt.Errorf("--shape は mask か box（%q）", o.shape)
	}
	if err = protectPaths(gameDir, out, o.fixFile, o.manualFile, o.tagsFile); err != nil {
		return c, err
	}
	if c.passes, err = detect.PassesByName(o.passes); err != nil {
		return c, err
	}
	if len(c.passes) == 0 {
		return c, errors.New("--passes cannot be empty")
	}
	human, err := mosaic.LoadFixes(o.fixFile)
	if err != nil {
		return c, err
	}
	if c.manual, err = mosaic.LoadFixes(o.manualFile); err != nil {
		return c, err
	}
	c.fixes = mosaic.Fixes{}
	maps.Copy(c.fixes, human)
	for k, f := range c.manual {
		c.fixes[k] = mosaic.Combine(c.fixes[k], f)
	}
	if c.tags, err = loadTagCache(o.tagsFile, o.taggerDir, o.lib, o.threads); err != nil {
		return c, err
	}
	c.tags.diag = o.diag
	if err := validateReport(c); err != nil {
		return c, err
	}
	return c, nil
}

func (c *config) open(ctx context.Context) error {
	if c.det != nil {
		return nil
	}
	if c.opts.model == "" {
		p, err := (&cli{diag: c.diag}).defaultModel(ctx, c.opts.yes, true)
		if err != nil {
			return err
		}
		c.opts.model = p
	}
	d, err := newDetector(c.opts.model, c.opts.lib, c.opts.threads, c.classes)
	if err != nil {
		return err
	}
	c.det = d
	if c.opts.faceModel != "" {
		fc, src, err := detect.ResolveClasses(c.opts.faceModel, c.opts.lib, detect.FaceLabels)
		if err != nil {
			return err
		}
		if c.showClasses {
			c.diag.Printf("face classes: %s (%s)\n", strings.Join(fc, ","), src)
		}
		fd, err := newDetector(c.opts.faceModel, c.opts.lib, c.opts.threads, fc)
		if err != nil {
			return err
		}
		c.face = fd
	}
	if c.opts.shape == "mask" {
		if c.opts.segModel == "" {
			return errors.New("--shape mask requires a user-supplied --seg model")
		}
		sg, err := newSegmenter(c.opts.segModel, c.opts.lib, c.opts.threads, 1280, segment.SegLabels)
		if err != nil {
			return err
		}
		c.seg = sg
	}

	return nil
}

var faceThreshold = settings.Defaults.FaceThreshold

func (c *config) faces(img image.Image) ([]image.Rectangle, error) {
	if c.face == nil {
		return nil, nil
	}
	boxes, err := c.face.Detect(img, detect.DefaultPasses[:2], faceThreshold)
	if err != nil {
		return nil, err
	}
	var out []image.Rectangle
	for _, g := range detect.Cluster(boxes, 0.4, 0.7) {
		out = append(out, g.Rect)
	}
	return out, nil
}

func (c *config) close() error {
	var err error
	if c.det != nil {
		err = errors.Join(err, c.det.Close())
	}
	if c.face != nil {
		err = errors.Join(err, c.face.Close())
	}
	if c.seg != nil {
		err = errors.Join(err, c.seg.Close())
	}
	return errors.Join(err, c.tags.close())
}

func (cli *cli) autoCmd() clihelp.Operation {
	fs := newFlagSet("gozen mosaic auto")
	o := cli.commonFlags(fs)
	dry := fs.BoolP("dry-run", "n", false, "write nothing; print the regions found and the reasons to review")
	return clihelp.Operation{Flags: fs, Page: autoPage, Run: func(ctx context.Context) (err error) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if fs.NArg() < 1 {
			return cliio.Parse(diag.Operands.Errorf("expected a game directory: gozen mosaic auto [OPTIONS] <game-dir> [IMAGE ...]"))
		}
		gameDir := fs.Arg(0)
		c, err := o.setup(ctx, gameDir)
		if err != nil {
			return err
		}
		c.dry = *dry
		c.showClasses = *dry
		if *dry {
			c.printClasses()
		}
		c.tags.readOnly = *dry
		pics := fs.Args()[1:]
		if len(pics) == 0 {
			var missing []string
			if pics, missing, err = mosaic.Pictures(gameDir); err != nil {
				return err
			}
			for _, m := range missing {
				c.diag.Printf("warning: YAML に書かれた絵 assets/%s が無い\n", m)
			}
		}
		for rel := range c.fixes {
			if _, err := os.Stat(filepath.Join(gameDir, "assets", filepath.FromSlash(rel))); err != nil {
				c.diag.Printf("warning: 手直しの絵 assets/%s が無い\n", rel)
			}
		}
		rows, err := run(ctx, c, pics)
		if err != nil {
			return err
		}
		if cli.report.Format == "jsonl" {
			for _, row := range rows {
				if err := cli.report.Item(cli.out, row.File, "ok", row); err != nil {
					return err
				}
			}
		}
		return cli.result(rows)
	}}
}

type config struct {
	outw, diag *cliio.Output

	gameDir, out string
	classes      []string
	classSource  string
	showClasses  bool
	displayOut   string
	block        int
	dry          bool
	redetect     bool
	quality      int
	p            params
	fixes        mosaic.Fixes
	manual       mosaic.Fixes
	opts         *options
	passes       []detect.Pass
	tags         *tagCache
	det          boxDetector
	face         boxDetector
	seg          segmenter
}

type entry struct {
	File    string   `json:"file"`
	Size    [2]int   `json:"size"`
	Block   int      `json:"block"`
	Offset  [2]int   `json:"offset,omitzero"`
	Found   []found  `json:"found,omitempty"`
	Skipped []found  `json:"skipped,omitempty"`
	Regions [][4]int `json:"regions,omitempty"`
	Adds    [][4]int `json:"adds,omitempty"`
	Polys   [][]int  `json:"polys,omitempty"`
	Faces   [][4]int `json:"faces,omitempty"`
	Shapes  []string `json:"shapes,omitempty"`
	Review  []string `json:"review,omitempty"`
	Tagged  bool     `json:"tagged,omitempty"`
	Fixed   bool     `json:"fixed,omitempty"`
	Output  string   `json:"output,omitempty"`
	Source  string   `json:"source,omitempty"`
	FixSum  string   `json:"fix_sum,omitempty"`
}

type found struct {
	Label  string  `json:"label"`
	Score  float32 `json:"score"`
	Rect   [4]int  `json:"rect"`
	Hits   int     `json:"hits"`
	Passes int     `json:"passes"`
	Shape  string  `json:"shape,omitempty"`
}

func xywh(r image.Rectangle) [4]int { return [4]int{r.Min.X, r.Min.Y, r.Dx(), r.Dy()} }

func run(ctx context.Context, c config, pics []string) (report []entry, err error) {
	defer func() { err = errors.Join(err, c.close()) }()
	report = []entry{}
	var masked, review int
	for i, rel := range pics {
		e, err := one(ctx, &c, rel)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		report = append(report, e)
		mark := "-"
		if n := len(e.Regions) + len(e.Adds) + len(e.Polys); n > 0 {
			mark = fmt.Sprintf("%d 箇所（自動 %d・手 %d）", n, len(e.Regions), len(e.Adds)+len(e.Polys))
			masked++
		}
		if len(e.Review) > 0 {
			mark += " 要確認: " + strings.Join(e.Review, " / ")
			review++
		}
		c.outw.Printf("[%d/%d] %s %s\n", i+1, len(pics), rel, mark)
		if !c.dry && i%20 == 19 {
			if err := c.tags.save(); err != nil {
				return nil, err
			}
		}
	}
	if !c.dry {
		if err := c.tags.save(); err != nil {
			return nil, err
		}
	}
	c.outw.Printf("%d 枚中 %d 枚にモザイク、要確認 %d 枚\n", len(pics), masked, review)
	if c.dry {
		return report, nil
	}
	all, err := writeReport(c.gameDir, c.out, report)
	if err != nil {
		return nil, err
	}
	return report, writeReview(c, all)
}

func one(ctx context.Context, c *config, rel string) (result entry, err error) {
	if err := validPicture(*c, rel); err != nil {
		return entry{}, err
	}
	src := filepath.Join(c.gameDir, "assets", filepath.FromSlash(rel))
	img, err := decode(src)
	if err != nil {
		return entry{}, err
	}
	b := img.Bounds()
	e := entry{File: rel, Size: [2]int{b.Dx(), b.Dy()}}
	fx, hasFix := c.fixes[rel]
	e.Fixed = hasFix
	if e.Source, err = fileSum(src); err != nil {
		return e, err
	}
	e.FixSum = fixSum(fx)
	e.Block = c.block
	if fx.Block > 0 {
		e.Block = fx.Block
	}
	std := mosaic.DefaultBlock(b.Dx(), b.Dy())
	if e.Block <= 0 {
		e.Block = std
	} else if e.Block < std {
		c.diag.Printf("warning: %s のブロック %dpx は 既定のブロック幅（%dpx）より小さい\n", rel, e.Block, std)
	}
	var boxes []detect.Box
	if !fx.Skip {
		if err := c.open(ctx); err != nil {
			return entry{}, err
		}
		if boxes, err = c.det.Detect(img, c.passes, c.p.minScore); err != nil {
			return e, err
		}
	}
	tags, err := c.tags.get(rel, src)
	if err != nil {
		return e, err
	}
	tagged := tags != nil
	e.Tagged = tagged
	faces, err := c.faces(img)
	if err != nil {
		return e, err
	}
	for _, f := range faces {
		e.Faces = append(e.Faces, xywh(f))
	}
	groups, skipped, regions, reasons, areas := plan(b, boxes, faces, fx, tags, e.Block, c.p)
	if c.tags.path != "" && !tagged && !fx.Checked {
		reasons = append(reasons, "タグが無い（タガーのモデルが無い）")
	}
	for _, g := range groups {
		e.Found = append(e.Found, found{g.Label, g.Score, xywh(g.Rect), g.Hits, g.Passes, ""})
	}
	for _, g := range skipped {
		e.Skipped = append(e.Skipped, found{g.Label, g.Score, xywh(g.Rect), g.Hits, g.Passes, ""})
	}
	for _, r := range regions {
		e.Regions = append(e.Regions, xywh(r))
	}
	polys := fx.Polygons()
	e.Adds, e.Polys = fx.Add, fx.Poly
	var shapes []mosaic.Shape
	if !c.dry {
		var extra []string
		if shapes, e.Shapes, extra, err = c.autoShapes(img, regions, areas, tags, fx, e.Block); err != nil {
			return e, err
		}

		for i, a := range areas {
			if i >= len(e.Shapes) {
				break
			}
			for j := range e.Found {
				if e.Found[j].Rect == xywh(a.Group.Rect) {
					e.Found[j].Shape = e.Shapes[i]
				}
			}
		}
		if !fx.Checked {
			reasons = append(reasons, extra...)
		}
	}
	for _, a := range fx.Add {
		shapes = append(shapes, mosaic.Box(mosaic.Rect(a).Intersect(b)))
	}
	for _, pg := range polys {
		shapes = append(shapes, pg)
	}
	e.Review = reasons
	if c.dry || len(shapes) == 0 {
		return e, nil
	}
	outputRel := rel
	if strings.EqualFold(filepath.Ext(rel), ".webp") {
		outputRel = rel + ".png"
	}
	dst := filepath.Join(c.out, filepath.FromSlash(outputRel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return e, err
	}

	off := mosaic.BestOffset(b, shapes, e.Block)
	e.Offset = [2]int{off.X, off.Y}
	if err := encode(dst, mosaic.ApplyShapesAt(img, shapes, e.Block, off), c.quality); err != nil {
		return e, err
	}
	if err := writeMask(maskPath(c.out, rel), b, shapes, e.Block, off); err != nil {
		return e, err
	}
	e.Output = outputRel
	return e, nil
}

func decode(path string) (image.Image, error) {
	f, err := os.Open(path) //nolint:gosec // The caller explicitly selects this input file.
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(f)
	return img, errors.Join(err, f.Close())
}

func encode(path string, img image.Image, quality int) (err error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		return errors.New("output extension must be PNG or JPEG")
	}
	return mosaic.WriteFile(path, func(w io.Writer) error {
		if ext == ".png" {
			return png.Encode(w, img)
		}
		return jpeg.Encode(w, img, &jpeg.Options{Quality: quality})
	})
}

func writeReport(gameDir, out string, report []entry) ([]entry, error) {
	path := filepath.Join(out, "report.json")
	now := map[string]entry{}
	for _, e := range report {
		now[e.File] = e
	}
	all := append([]entry(nil), report...)
	if b, err := os.ReadFile(path); err == nil { //nolint:gosec // The caller explicitly selects this input file.
		var old []entry
		if json.Unmarshal(b, &old) == nil {
			for _, e := range old {
				n, seen := now[e.File]
				if !seen {
					all = append(all, e)
					continue
				}
				if e.Output != "" && n.Output == "" {
					if err := removeTracked(out, e.Output); err != nil {
						return nil, err
					}
					if err := removeTracked(out, "masks/"+e.File+".png"); err != nil {
						return nil, err
					}
				}
			}
		}
	}

	kept := all[:0]
	for _, e := range all {
		if _, err := os.Stat(filepath.Join(gameDir, "assets", filepath.FromSlash(e.File))); err != nil {
			if !os.IsNotExist(err) {
				return nil, err
			}
			if e.Output != "" {
				if err := removeTracked(out, e.Output); err != nil {
					return nil, err
				}
				if err := removeTracked(out, "masks/"+e.File+".png"); err != nil {
					return nil, err
				}
			}
			continue
		}
		kept = append(kept, e)
	}
	all = kept

	sortEntries(all)
	if err := os.MkdirAll(out, 0o750); err != nil {
		return nil, err
	}
	b, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return nil, err
	}
	return all, mosaic.WriteFile(path, func(w io.Writer) error { _, err := w.Write(b); return err })
}

func fileSum(path string) (string, error) {
	b, err := os.ReadFile(path) //nolint:gosec // The caller explicitly selects this input file.
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(b)), nil
}

func fixSum(fx mosaic.Fix) string {
	fx.Checked, fx.Note = false, ""
	b, err := json.Marshal(fx)
	if err != nil {
		return "invalid"
	}
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

func removeTracked(out, rel string) error {
	if !relativePicture(rel) {
		return fmt.Errorf("invalid report output %q", rel)
	}
	root, err := canonical(out)
	if err != nil {
		return err
	}
	p := filepath.Join(out, filepath.FromSlash(rel))
	dst, err := canonical(p)
	if err != nil {
		return err
	}
	if !within(dst, root) {
		return errors.New("report output escapes destination")
	}
	if err = os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// resolveClasses finds the class names of --model: --classes, labels.json
// beside the model, the ONNX metadata, then the default order.
func (o *options) resolveClasses() ([]string, string, error) {
	if strings.TrimSpace(o.classes) != "" {
		names, err := parseClasses(o.classes)
		return names, detect.SourceFlag, err
	}
	return detect.ResolveClasses(o.model, o.lib, detect.Labels)
}

func (c *config) printClasses() {
	var ls []string
	for _, l := range c.classes {
		if c.p.labels[l] {
			ls = append(ls, l)
		}
	}
	c.diag.Printf("classes: %s (%s)\npixelated: %s\n", strings.Join(c.classes, ","), c.classSource, strings.Join(ls, ","))
}
