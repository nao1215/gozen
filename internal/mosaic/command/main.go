package command

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	_ "golang.org/x/image/webp"

	"github.com/nao1215/gozen/internal/mosaic"
	"github.com/nao1215/gozen/internal/mosaic/detect"
	"github.com/nao1215/gozen/internal/mosaic/segment"
	"github.com/nao1215/gozen/internal/mosaic/settings"
)

type options struct {
	outw, diag *output

	block, padBlocks, minBlocks, threads, quality int
	dilate                                        int
	out, model, lib, labels, tagsFile, fixFile    string
	manualFile, passes, taggerDir, faceModel      string
	shape, segModel                               string
	minScore, sure, margin                        float64
}

func env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func (cli *cli) commonFlags(fs *flag.FlagSet) *options {
	p := defaultParams
	o := &options{outw: cli.out, diag: cli.diag}
	fs.IntVar(&o.block, "block", 0, "モザイクの 1 ブロックの大きさ（px）。0 なら 既定のブロック幅（max(4, 長辺/100 の切り上げ)）。基準より小さい値は警告する")
	fs.StringVar(&o.out, "out", env("MOSAIC_OUT", ""), "書き出すディレクトリ（既定は <game dir>/assets_mosaic）")
	fs.StringVar(&o.model, "model", "", "検出モデル（ONNX）")
	fs.StringVar(&o.faceModel, "face", "", "任意の顔検出 ONNX モデル。明示したファイルが読めない場合は失敗する")
	fs.StringVar(&o.shape, "shape", env("MOSAIC_SHAPE", "box"), "auto の形状。box は矩形、mask は -seg で指定したモデルのマスク")
	fs.IntVar(&o.dilate, "dilate", -1, "mask を太らせる px。-1 は 1 ブロック分")
	fs.StringVar(&o.segModel, "seg", "", "-shape mask に必要な利用者指定の ONNX 分割モデル")
	fs.StringVar(&o.lib, "ort", detect.LibraryPath(), "ONNX Runtime 共有ライブラリの場所（ONNXRUNTIME_LIB でも指定可）")
	fs.Float64Var(&o.minScore, "threshold", float64(p.minScore), "このスコア以上の枠にモザイクを掛ける（低いほど漏れが減り、誤検出が増える）")
	fs.Float64Var(&o.sure, "sure", float64(p.sureScore), "最高のスコアがこれより低い枠は、掛けた上で要確認に出す")
	fs.StringVar(&o.labels, "labels", strings.Join(settings.Defaults.Targets, ","), "モザイクを掛ける物（nipple_f,penis,pussy から）")
	fs.Float64Var(&o.margin, "margin", p.margin, "枠の辺のこの割合だけ広げる（既定 0。掛かったブロックはまるごと塗るので輪郭線は入る。足りない絵だけ広げる）")
	fs.IntVar(&o.padBlocks, "pad", p.padBlocks, "枠を広げる幅の最小（ブロックの数。既定 0）")
	fs.IntVar(&o.minBlocks, "min", p.minBlocks, "範囲の一辺の最小（ブロックの数。既定 0）")
	fs.StringVar(&o.tagsFile, "tags", env("MOSAIC_TAGS", ""), "絵のタグのキャッシュの JSON（無い・古い絵は -tagger で付けて書き足す。空ならタグを使わない）")
	fs.StringVar(&o.taggerDir, "tagger", env("MOSAIC_TAGGER", ""), "WD タガーのモデルのディレクトリ（model.onnx と selected_tags.csv）")
	fs.StringVar(&o.fixFile, "fix", env("MOSAIC_FIX", ""), "人が書く手直しの YAML（絵ごとに範囲を足す・誤検出を捨てる・確かめた印）")
	fs.StringVar(&o.manualFile, "manual", env("MOSAIC_MANUAL", ""), "mosaic manual が書く手直しの YAML（auto も読む）")
	fs.StringVar(&o.passes, "passes", env("MOSAIC_PASSES", "default"), "絵の見方（カンマ区切り）。default、all、または full640・flip640・native・flipnative・tile2・tile2x・tile3x・fliptile2x")
	fs.IntVar(&o.threads, "threads", max(1, runtime.NumCPU()/2), "onnxruntime の CPU のスレッド数")
	fs.IntVar(&o.quality, "quality", 90, "JPEG の画質")
	return o
}

func (o *options) setup(gameDir string) (config, error) {
	if err := o.validate(); err != nil {
		return config{}, err
	}
	p := defaultParams
	p.minScore, p.sureScore = float32(o.minScore), float32(o.sure)
	p.margin, p.padBlocks, p.minBlocks = o.margin, o.padBlocks, o.minBlocks
	p.labels = map[string]bool{}
	for l := range strings.SplitSeq(o.labels, ",") {
		if l = strings.TrimSpace(l); l != "" {
			if !knownLabel(l) {
				return config{}, fmt.Errorf("unknown label %q", l)
			}
			p.labels[l] = true
		}
	}
	if len(p.labels) == 0 {
		return config{}, fmt.Errorf("-labels cannot be empty")
	}
	out := o.out
	if out == "" {
		out = filepath.Join(gameDir, "assets_mosaic")
	}
	c := config{outw: o.outw, diag: o.diag, gameDir: gameDir, out: out, block: o.block, quality: o.quality, p: p, opts: o}
	if o.shape != "mask" && o.shape != "box" {
		return c, fmt.Errorf("-shape は mask か box（%q）", o.shape)
	}
	var err error
	if err = protectPaths(gameDir, out, o.fixFile, o.manualFile, o.tagsFile); err != nil {
		return c, err
	}
	if c.passes, err = detect.PassesByName(o.passes); err != nil {
		return c, err
	}
	if len(c.passes) == 0 {
		return c, fmt.Errorf("-passes cannot be empty")
	}
	human, err := mosaic.LoadFixes(o.fixFile)
	if err != nil {
		return c, err
	}
	if c.manual, err = mosaic.LoadFixes(o.manualFile); err != nil {
		return c, err
	}
	c.fixes = mosaic.Fixes{}
	for k, f := range human {
		c.fixes[k] = f
	}
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

func (c *config) open() error {
	if c.det != nil {
		return nil
	}
	d, err := detect.New(c.opts.model, c.opts.lib, c.opts.threads)
	if err != nil {
		return err
	}
	c.det = d
	if c.opts.faceModel != "" {
		if c.face, err = detect.NewWithLabels(c.opts.faceModel, c.opts.lib, c.opts.threads, detect.FaceLabels); err != nil {
			return err
		}
	}
	if c.opts.shape == "mask" {
		if c.opts.segModel == "" {
			return fmt.Errorf("-shape mask requires a user-supplied -seg model")
		}
		if c.seg, err = segment.NewYoloSeg(c.opts.segModel, c.opts.lib, c.opts.threads, 1280, segment.SegLabels); err != nil {
			return err
		}
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

func (cli *cli) autoCmd(args []string) (err error) {
	fs := flag.NewFlagSet("auto", flag.ContinueOnError)
	fs.SetOutput(cli.diag)
	o := cli.commonFlags(fs)
	dry := fs.Bool("dry", false, "絵を書かずに、見つかった範囲と要確認の理由だけを表示する")
	fs.Usage = func() {
		cli.diag.println("usage: mosaic auto [flags] <game dir> [assets/ からの相対パス...]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return argumentError{err}
	}
	if fs.NArg() < 1 {
		fs.Usage()
		return argumentError{fmt.Errorf("expected a game directory")}
	}
	gameDir := fs.Arg(0)
	c, err := o.setup(gameDir)
	if err != nil {
		return err
	}
	c.dry = *dry
	c.tags.readOnly = *dry
	pics := fs.Args()[1:]
	if len(pics) == 0 {
		var missing []string
		if pics, missing, err = mosaic.Pictures(gameDir); err != nil {
			return err
		}
		for _, m := range missing {
			c.diag.printf("warning: YAML に書かれた絵 assets/%s が無い\n", m)
		}
	}
	for rel := range c.fixes {
		if _, err := os.Stat(filepath.Join(gameDir, "assets", filepath.FromSlash(rel))); err != nil {
			c.diag.printf("warning: 手直しの絵 assets/%s が無い\n", rel)
		}
	}
	return run(c, pics)
}

type config struct {
	outw, diag *output

	gameDir, out string
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
	det          *detect.Detector
	face         *detect.Detector
	seg          *segment.YoloSeg
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

func run(c config, pics []string) (err error) {
	defer func() { err = errors.Join(err, c.close()) }()
	var report []entry
	var masked, review int
	for i, rel := range pics {
		e, err := one(&c, rel)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
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
		c.outw.printf("[%d/%d] %s %s\n", i+1, len(pics), rel, mark)
		if !c.dry && i%20 == 19 {
			if err := c.tags.save(); err != nil {
				return err
			}
		}
	}
	if !c.dry {
		if err := c.tags.save(); err != nil {
			return err
		}
	}
	c.outw.printf("%d 枚中 %d 枚にモザイク、要確認 %d 枚\n", len(pics), masked, review)
	if c.dry {
		return nil
	}
	all, err := writeReport(c.gameDir, c.out, report)
	if err != nil {
		return err
	}
	return writeReview(c, all)
}

func one(c *config, rel string) (result entry, err error) {
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
		c.diag.printf("warning: %s のブロック %dpx は 既定のブロック幅（%dpx）より小さい\n", rel, e.Block, std)
	}
	var boxes []detect.Box
	if !fx.Skip {
		if err := c.open(); err != nil {
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
		return fmt.Errorf("output extension must be PNG or JPEG")
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
		return fmt.Errorf("report output escapes destination")
	}
	if err = os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
