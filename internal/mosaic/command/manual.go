package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nao1215/gozen/internal/base/cliio"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/spf13/pflag"

	"github.com/nao1215/gozen/internal/base/clihelp"
	"github.com/nao1215/gozen/internal/base/diag"
	"github.com/nao1215/gozen/internal/mosaic"
)

type coords [][]float64

func (c *coords) String() string { return fmt.Sprint(*c) }

// Type names the value kind for pflag.
func (c *coords) Type() string { return "coords" }

func (c *coords) Set(s string) error {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
	if len(fields) == 0 {
		return errors.New("coordinates cannot be empty")
	}
	var v []float64
	for _, field := range fields {
		f, err := strconv.ParseFloat(field, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > 1_000_000 {
			return fmt.Errorf("invalid coordinate %q", field)
		}
		v = append(v, f)
	}
	*c = append(*c, v)
	return nil
}

// manualOptions are the options of the manual commands beyond the common ones. A command
// registers only the ones it reads (manualAction.flags); the others keep their defaults.
type manualOptions struct {
	o                                     *options
	rects, boxes, polys, ellipses, region coords
	norm, checked                         bool
	note, regionID, src, viewOut          string
	by, left, top, right, bottom          int
	size, step                            int
}

// register adds one option of the manual commands to the flag set, by its name.
func (m *manualOptions) register(fs *pflag.FlagSet, name string) {
	switch name {
	case "rect":
		fs.Var(&m.rects, name, "a rectangle `X,Y,W,H` (top left, width, height); repeatable")
	case "box":
		fs.Var(&m.boxes, name, "a rectangle as a box `X0,Y0,X1,Y1` (top left and bottom right corners); repeatable")
	case "poly":
		fs.Var(&m.polys, name, "a polygon `X1,Y1,X2,Y2,...` of 3 points or more; repeatable")
	case "ellipse":
		fs.Var(&m.ellipses, name, "an ellipse `CX,CY,RX,RY` (center and radii); repeatable")
	case "norm":
		fs.BoolVar(&m.norm, name, false, "read coordinates as fractions from 0 to 1 of the width and the height")
	case "note":
		fs.StringVar(&m.note, name, "", "add this `TEXT` to the note of the image in the YAML")
	case "region-id":
		fs.StringVar(&m.regionID, name, "", "the automatic region `IDS` shown by view, such as A1, separated by commas; grow also takes all")
	case "by":
		fs.IntVar(&m.by, name, 0, "grow every side by these `PIXELS`; negative shrinks")
	case "left":
		fs.IntVar(&m.left, name, 0, "grow the left side by these `PIXELS`")
	case "top":
		fs.IntVar(&m.top, name, 0, "grow the top side by these `PIXELS`")
	case "right":
		fs.IntVar(&m.right, name, 0, "grow the right side by these `PIXELS`")
	case "bottom":
		fs.IntVar(&m.bottom, name, 0, "grow the bottom side by these `PIXELS`")
	case "src":
		fs.StringVar(&m.src, name, m.src, "draw the `SOURCE` image: original or masked")
	case "region":
		fs.Var(&m.region, name, "zoom into the area `X,Y,W,H` (default: the whole image)")
	case "size":
		fs.IntVar(&m.size, name, m.size, "the long side of the drawing in `PIXELS`")
	case "step":
		fs.IntVar(&m.step, name, 0, "the scale interval of the drawing in original `PIXELS`; 0 chooses one")
	case "view-out":
		fs.StringVar(&m.viewOut, name, "", "write the drawing to this `FILE` (default: <out>/manual/<image>.<source>.png)")
	case "checked":
		fs.BoolVar(&m.checked, name, false, "mark the image as reviewed; required")
	}
}

// manualCmd builds one manual command. An option that the command does not read is not
// registered, so giving it is an unknown option: a usage error, not a silent no-op.
func (cli *cli) manualCmd(sub string) clihelp.Operation {
	fs := newFlagSet("gozen mosaic " + sub)
	action := manualActions()[sub]
	mo := &manualOptions{o: cli.commonFlags(fs), src: "original", size: 1536}
	for _, name := range action.flags {
		mo.register(fs, name)
	}
	return clihelp.Operation{Flags: fs, Page: action.page(sub, fs), Run: func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		r := &manualRun{cli: cli, sub: sub, writes: action.writes, mo: mo, flags: fs}
		return r.run(ctx)
	}}
}

// manualRun is one run of a manual command: the command, its options, and, once the
// operands are read, the image it works on.
type manualRun struct {
	cli    *cli
	sub    string
	writes bool
	mo     *manualOptions
	flags  *pflag.FlagSet

	c      config
	pic    string          // the path of the image below assets/, empty for a file elsewhere
	orig   string          // the file of the original image
	bounds image.Rectangle // the bounds of the original image
}

func (r *manualRun) run(ctx context.Context) error {
	if r.sub == "mark" && !r.mo.checked {
		return cliio.Invalid(diag.OptionRequired, "mark requires --checked")
	}
	if r.sub == "list" && r.flags.NArg() != 1 {
		return cliio.Parse(diag.Operands.Errorf("expected a game directory: gozen mosaic list [OPTIONS] <game-dir>"))
	}
	if r.sub != "list" && r.flags.NArg() != 2 {
		return cliio.Parse(diag.Operands.Errorf("expected a game directory and an image: gozen mosaic %s [OPTIONS] <game-dir> <IMAGE>", r.sub))
	}
	if r.mo.size < 1 || r.mo.size > 4096 || r.mo.step < 0 {
		return errors.New("view size must be 1..4096 and step >= 0")
	}
	var err error
	if r.c, err = r.mo.o.setup(ctx, r.flags.Arg(0)); err != nil {
		return err
	}
	if r.sub == "list" {
		return r.list()
	}
	r.pic, _, r.orig = resolve(r.c, r.flags.Arg(1))
	if r.pic == "" && r.sub != "view" {
		return fmt.Errorf("%s は assets の外の絵なので、view だけができる（指示は assets の絵にだけ書ける）", r.flags.Arg(1))
	}
	if err := validPicture(r.c, r.pic); err != nil && r.pic != "" {
		return err
	}
	img, err := decode(r.orig)
	if err != nil {
		return err
	}
	r.bounds = img.Bounds()
	if r.writes && r.mo.o.manualFile == "" {
		return errors.New("--manual（または環境変数 MOSAIC_MANUAL）に manual の YAML を指定する")
	}
	switch r.sub {
	case "show":
		return r.show()
	case "view":
		return r.view()
	}
	return r.write(ctx)
}

// list prints the images to review, one JSON line each.
func (r *manualRun) list() error {
	rows := manualList(r.c)
	if r.cli.report.Machine() {
		if r.cli.report.Format == "jsonl" {
			for _, row := range rows {
				if err := r.cli.report.Item(r.cli.out, fmt.Sprint(row["file"]), "ok", row); err != nil {
					return err
				}
			}
		}
		return r.cli.result(rows)
	}
	for _, row := range rows {
		if err := json.NewEncoder(r.c.outw).Encode(row); err != nil {
			return err
		}
	}
	return nil
}

func (r *manualRun) show() error {
	data := manualShow(r.c, r.pic)
	if r.cli.report.Machine() {
		return r.cli.result(data)
	}
	enc := json.NewEncoder(r.c.outw)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

func (r *manualRun) view() error {
	var region image.Rectangle
	if len(r.mo.region) > 0 {
		v, err := r.conv(r.mo.region[0], 4, "--region")
		if err != nil {
			return err
		}
		region = mosaic.Rect([4]int{v[0], v[1], v[2], v[3]})
	}
	path, err := manualView(r.c, r.pic, r.orig, r.mo.src, region, r.mo.size, r.mo.step, r.mo.viewOut)
	if err != nil {
		return err
	}
	return r.cli.result(map[string]any{"file": r.pic, "output": path})
}

// conv reads the n numbers of one option value as pixels of the image; n below 0 is a
// polygon (three points or more). With --norm the numbers are fractions of its size.
func (r *manualRun) conv(v []float64, n int, what string) ([]int, error) {
	if len(v) != n && (n >= 0 || len(v) < 6 || len(v)%2 != 0) {
		return nil, fmt.Errorf("%s の数が合わない: %v", what, v)
	}
	out := make([]int, len(v))
	for i, f := range v {
		if r.mo.norm {
			if f < 0 || f > 1 {
				return nil, errors.New("normalized coordinates must be in [0,1]")
			}
			if i%2 == 0 {
				f *= float64(r.bounds.Dx())
			} else {
				f *= float64(r.bounds.Dy())
			}
		}
		out[i] = int(math.Round(f))
	}
	return out, nil
}

// rectangles reads --rect and, for the commands that have it, --box, clipped to the image.
func (r *manualRun) rectangles() ([][4]int, error) {
	var out [][4]int
	for _, group := range []struct {
		what   string
		values coords
		box    bool
	}{{"--rect", r.mo.rects, false}, {"--box", r.mo.boxes, true}} {
		for _, v := range group.values {
			x, err := r.conv(v, 4, group.what)
			if err != nil {
				return nil, err
			}
			if group.box {
				x = []int{x[0], x[1], x[2] - x[0], x[3] - x[1]}
			}
			if x[2] <= 0 || x[3] <= 0 {
				return nil, errors.New("rectangle width and height must be positive")
			}
			clipped := clipRect(x, r.bounds)
			if clipped[2] <= 0 || clipped[3] <= 0 {
				return nil, errors.New("rectangle does not intersect the image")
			}
			out = append(out, clipped)
		}
	}
	return out, nil
}

// regions gives the automatic regions of the last run that --region-id names.
func (r *manualRun) regions(all bool, hint string) ([][4]int, error) {
	e, ok := reportEntry(r.c, r.pic)
	if !ok {
		return nil, fmt.Errorf("%s は report.json に無い（先に %s を動かす）", r.pic, hint)
	}
	ids, err := regionIDs(r.mo.regionID, len(e.Regions), all)
	if err != nil {
		return nil, err
	}
	out := make([][4]int, len(ids))
	for i, id := range ids {
		out[i] = e.Regions[id]
	}
	return out, nil
}

func (r *manualRun) add(m *mosaic.Fix) error {
	rects, err := r.rectangles()
	if err != nil {
		return err
	}
	m.Add = append(m.Add, rects...)
	for _, v := range r.mo.polys {
		x, err := r.conv(v, -1, "--poly")
		if err != nil {
			return err
		}
		m.Poly = append(m.Poly, x)
	}
	for _, v := range r.mo.ellipses {
		x, err := r.conv(v, 4, "--ellipse")
		if err != nil {
			return err
		}
		if x[2] <= 0 || x[3] <= 0 {
			return errors.New("ellipse radii must be positive")
		}
		m.Poly = append(m.Poly, ellipse(x[0], x[1], x[2], x[3], 48))
	}
	if len(rects)+len(r.mo.polys)+len(r.mo.ellipses) == 0 {
		return errors.New("add には --rect・--box・--poly・--ellipse のどれかが要る")
	}
	return nil
}

func (r *manualRun) drop(m *mosaic.Fix) error {
	rects, err := r.rectangles()
	if err != nil {
		return err
	}
	if r.mo.regionID != "" {
		named, err := r.regions(false, "auto か view")
		if err != nil {
			return err
		}
		rects = append(rects, named...)
	}
	if len(rects) == 0 {
		return errors.New("drop には --region-id か --rect・--box が要る")
	}
	m.Drop = append(m.Drop, rects...)
	return nil
}

func (r *manualRun) grow(m *mosaic.Fix) error {
	margin := r.flags.Changed("margin")
	if margin {
		if r.mo.o.margin < 0 {
			return errors.New("--margin は 0 以上")
		}
		m.Margin = r.mo.o.margin
	}
	mo := r.mo
	d := [4]int{mo.left + mo.by, mo.top + mo.by, mo.right + mo.by, mo.bottom + mo.by}
	if mo.regionID == "" && len(mo.rects) == 0 {
		if !margin {
			return errors.New("grow には --region-id か --rect（と広げる px）か --margin が要る")
		}
		return nil
	}
	if d == [4]int{} {
		return errors.New("grow には --by か --left・--top・--right・--bottom が要る")
	}
	// --region-id comes before --rect, as it always has: with both, the rectangles are not read.
	var rects [][4]int
	var err error
	if mo.regionID != "" {
		rects, err = r.regions(true, "auto")
	} else {
		rects, err = r.rectangles()
	}
	if err != nil {
		return err
	}
	for _, x := range rects {
		m.Grow = append(m.Grow, [8]int{x[0], x[1], x[2], x[3], d[0], d[1], d[2], d[3]})
	}
	return nil
}

// write changes the instructions of the image, rebuilds it, and saves the manual file.
func (r *manualRun) write(ctx context.Context) error {
	m := r.c.manual[r.pic]
	var err error
	switch r.sub {
	case "add":
		err = r.add(&m)
		m.Checked = false
	case "drop":
		err = r.drop(&m)
		m.Checked = false
	case "grow":
		err = r.grow(&m)
		m.Checked = false
	case "skip":
		m.Skip = true
		m.Checked = false
	case "mark":
		m.Checked = true
	case "clear":
		m = mosaic.Fix{}
	default:
		err = cliio.Parse(diag.UnknownCommand.Errorf("unknown manual command %q", r.sub))
	}
	if err != nil {
		return err
	}
	if r.mo.note != "" {
		m.Note = strings.TrimSpace(m.Note + " " + r.mo.note)
	}
	for _, p := range m.Polygons() {
		if !p.Hits(p.Bounds().Intersect(r.bounds)) {
			return errors.New("polygon does not intersect the image")
		}
	}
	o := r.mo.o
	if r.flags.Changed("block") {
		m.Block = o.block
	}
	r.c.manual[r.pic] = m
	if err := mosaic.ValidateFixes(r.c.manual); err != nil {
		return err
	}
	human, err := mosaic.LoadFixes(o.fixFile)
	if err != nil {
		return err
	}
	r.c.fixes[r.pic] = mosaic.Combine(human[r.pic], m)
	if err := publishManual(ctx, r.c, r.pic, r.orig, r.sub, r.mo.size, r.mo.step); err != nil {
		return err
	}
	return r.cli.result(map[string]any{"file": r.pic, "manual": o.manualFile, "output": r.c.out, "fix": m})
}

func clipRect(x []int, b image.Rectangle) [4]int {
	r := image.Rect(x[0], x[1], x[0]+x[2], x[1]+x[3]).Canon().Intersect(b)
	return xywh(r)
}

func ellipse(cx, cy, rx, ry, n int) []int {
	out := make([]int, 0, 2*n)
	for i := range n {
		a := 2 * math.Pi * float64(i) / float64(n)

		k := 1 / math.Cos(math.Pi/float64(n))
		out = append(out, int(math.Round(float64(cx)+float64(rx)*k*math.Cos(a))), int(math.Round(float64(cy)+float64(ry)*k*math.Sin(a))))
	}
	return out
}

func readReport(c config) []entry {
	b, err := os.ReadFile(filepath.Join(c.out, "report.json"))
	if err != nil {
		return nil
	}
	var es []entry
	if json.Unmarshal(b, &es) != nil {
		return nil
	}
	return es
}

func reportEntry(c config, pic string) (entry, bool) {
	for _, e := range readReport(c) {
		if e.File == pic {
			return e, true
		}
	}
	return entry{}, false
}

// regionIDs reads --region-id: the names view gives the automatic regions (A1, A2, ...),
// separated by commas, as indexes into the n regions of the report. With all allowed, the
// word all names every region.
func regionIDs(ids string, n int, all bool) ([]int, error) {
	var out []int
	if all && strings.TrimSpace(ids) == "all" {
		for i := range n {
			out = append(out, i)
		}
		return out, nil
	}
	for id := range strings.SplitSeq(ids, ",") {
		i, err := strconv.Atoi(strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(id)), "A"))
		if err != nil || i < 1 || i > n {
			return nil, fmt.Errorf("--region-id %s が無い（自動の範囲は A1〜A%d）", id, n)
		}
		out = append(out, i-1)
	}
	return out, nil
}

func manualList(c config) []map[string]any {
	rows := []map[string]any{}
	for _, e := range readReport(c) {
		if len(e.Review) > 0 {
			rows = append(rows, map[string]any{"file": e.File, "review": e.Review, "masked": e.Output != ""})
		}
	}
	return rows
}

func manualShow(c config, pic string) map[string]any {
	e, ok := reportEntry(c, pic)
	out := map[string]any{"file": pic, "manual": c.manual[pic], "fix": c.fixes[pic]}
	if ok {
		out["report"] = e
	} else {
		out["report"] = "report.json に無い（auto を動かす）"
	}
	return out
}

func manualView(c config, pic, orig, src string, region image.Rectangle, size, step int, dst string) (string, error) {
	var img image.Image
	var err error
	e, ok := reportEntry(c, pic)
	switch src {
	case "original":
		img, err = decode(orig)
	case "masked":
		if ok && e.Output != "" {
			img, err = decode(filepath.Join(c.out, filepath.FromSlash(e.Output)))
		} else {
			img, err = decode(orig)
		}
	default:
		return "", errors.New("--src は original か masked")
	}
	if err != nil {
		return "", err
	}
	b := img.Bounds()
	if region.Empty() {
		region = b
	}
	region = region.Intersect(b)
	if region.Empty() {
		return "", errors.New("--region が絵の外")
	}
	var ov overlay
	if ok {
		ov = overlay{auto: e.Regions, add: e.Adds, poly: e.Polys}
	}
	if pic != "" {
		ov.painted = readMask(maskPath(c.out, pic))
	}
	view := renderView(img, region, size, step, ov)
	if dst == "" {
		name := strings.ReplaceAll(pic, "/", "__")
		if pic == "" {
			name = strings.TrimSuffix(filepath.Base(orig), filepath.Ext(orig))
		}
		dst = filepath.Join(c.out, "manual", name+"."+src+".png")
	}
	if err := protectView(c, orig, dst); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return "", err
	}
	if err := encode(dst, view, 95); err != nil {
		return "", err
	}
	display := dst
	if c.displayOut != "" {
		if rel, e := filepath.Rel(c.out, dst); e == nil {
			display = filepath.Join(c.displayOut, rel)
		}
	}
	c.outw.Printf("view: %s（%s、範囲 %v、元の絵 %dx%d）\n", display, src, xywh(region), b.Dx(), b.Dy())
	return display, nil
}

type overlay struct {
	auto, add [][4]int
	poly      [][]int
	painted   *image.Alpha
}

const (
	rulerTop  = 34
	rulerLeft = 64
)

func renderView(img image.Image, region image.Rectangle, size, step int, ov overlay) *image.RGBA {
	scale := float64(size) / float64(max(region.Dx(), region.Dy()))
	w, h := int(float64(region.Dx())*scale), int(float64(region.Dy())*scale)
	out := image.NewRGBA(image.Rect(0, 0, w+rulerLeft, h+rulerTop))
	draw.Draw(out, out.Bounds(), image.NewUniform(color.RGBA{24, 24, 24, 255}), image.Point{}, draw.Src)
	at := image.Rect(rulerLeft, rulerTop, rulerLeft+w, rulerTop+h)
	if scale >= 2 {
		xdraw.NearestNeighbor.Scale(out, at, img, region, draw.Src, nil)
	} else {
		xdraw.CatmullRom.Scale(out, at, img, region, draw.Src, nil)
	}
	if ov.painted != nil {
		for py := at.Min.Y; py < at.Max.Y; py++ {
			for px := at.Min.X; px < at.Max.X; px++ {
				x := region.Min.X + int(float64(px-at.Min.X)/scale)
				y := region.Min.Y + int(float64(py-at.Min.Y)/scale)
				if (image.Point{x, y}).In(ov.painted.Rect) && ov.painted.Pix[ov.painted.PixOffset(x, y)] != 0 {
					c := out.RGBAAt(px, py)
					out.SetRGBA(px, py, color.RGBA{uint8((int(c.R)*3 + 255) / 4), uint8((int(c.G)*3 + 220) / 4), uint8(int(c.B) * 3 / 4), 255}) //nolint:gosec // Channel averages remain within 0..255.
				}
			}
		}
	}
	toX := func(x int) int { return at.Min.X + int(math.Round(float64(x-region.Min.X)*scale)) }
	toY := func(y int) int { return at.Min.Y + int(math.Round(float64(y-region.Min.Y)*scale)) }
	if step <= 0 {
		step = niceStep(max(region.Dx(), region.Dy()) / 12)
	}
	white := color.RGBA{255, 255, 255, 255}
	for x := (region.Min.X + step - 1) / step * step; x < region.Max.X; x += step {
		px := toX(x)
		gridLine(out, image.Rect(px, at.Min.Y, px+1, at.Max.Y))
		text(out, px-3*len(strconv.Itoa(x)), 4, strconv.Itoa(x), white, 2)
		outline(out, image.Rect(px, rulerTop-6, px+1, rulerTop), white)
	}
	for y := (region.Min.Y + step - 1) / step * step; y < region.Max.Y; y += step {
		py := toY(y)
		gridLine(out, image.Rect(at.Min.X, py, at.Max.X, py+1))
		text(out, 2, py-12, strconv.Itoa(y), white, 2)
	}
	draws := func(rs [][4]int, c color.RGBA, prefix string) {
		for i, r := range rs {
			rr := image.Rect(toX(r[0]), toY(r[1]), toX(r[0]+r[2]), toY(r[1]+r[3]))
			if !rr.Overlaps(at) {
				continue
			}
			outline(out, rr.Intersect(at), c)
			label(out, rr.Min.Add(image.Pt(3, 3)).Add(at.Min).Sub(at.Min), fmt.Sprintf("%s%d", prefix, i+1), c, at)
		}
	}
	draws(ov.auto, color.RGBA{255, 220, 0, 255}, "A")
	draws(ov.add, color.RGBA{0, 220, 255, 255}, "M")
	for i, p := range ov.poly {
		c := color.RGBA{255, 80, 200, 255}
		var pts []image.Point
		for k := 0; k+1 < len(p); k += 2 {
			pts = append(pts, image.Pt(toX(p[k]), toY(p[k+1])))
		}
		for k := range pts {
			line(out, pts[k], pts[(k+1)%len(pts)], c, at)
		}
		if len(pts) > 0 && mosaic.Polygon(pts).Bounds().Overlaps(at) {
			label(out, pts[0], fmt.Sprintf("P%d", i+1), c, at)
		}
	}
	return out
}

func niceStep(v int) int {
	for p := 1; ; p *= 10 {
		for _, m := range []int{1, 2, 5} {
			if m*p >= max(v, 1) {
				return m * p
			}
		}
	}
}

func gridLine(dst *image.RGBA, r image.Rectangle) {
	r = r.Intersect(dst.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := dst.RGBAAt(x, y)
			v := uint8(255)
			if (x+y)/4%2 == 1 {
				v = 0
			}
			mix := func(a uint8) uint8 { return uint8((int(a)*2 + int(v)) / 3) } //nolint:gosec // Channel averages remain within 0..255.
			dst.SetRGBA(x, y, color.RGBA{mix(c.R), mix(c.G), mix(c.B), 255})
		}
	}
}

func line(dst *image.RGBA, a, b image.Point, c color.RGBA, clip image.Rectangle) {
	n := max(abs(b.X-a.X), abs(b.Y-a.Y), 1)
	for i := 0; i <= n; i++ {
		x := a.X + (b.X-a.X)*i/n
		y := a.Y + (b.Y-a.Y)*i/n
		for _, d := range []image.Point{{0, 0}, {1, 0}, {0, 1}} {
			if p := image.Pt(x, y).Add(d); p.In(clip) {
				dst.SetRGBA(p.X, p.Y, c)
			}
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func label(dst *image.RGBA, at image.Point, s string, c color.RGBA, clip image.Rectangle) {
	at.X = min(max(at.X, clip.Min.X), clip.Max.X-16*len(s)-4)
	at.Y = min(max(at.Y, clip.Min.Y), clip.Max.Y-28)
	draw.Draw(dst, image.Rect(at.X, at.Y, at.X+14*len(s)+6, at.Y+28), image.NewUniform(color.RGBA{0, 0, 0, 255}), image.Point{}, draw.Src)
	text(dst, at.X+3, at.Y+1, s, c, 2)
}

func text(dst *image.RGBA, x, y int, s string, c color.RGBA, k int) {
	small := image.NewRGBA(image.Rect(0, 0, 7*len(s)+1, 13))
	d := font.Drawer{Dst: small, Src: image.NewUniform(c), Face: basicfont.Face7x13, Dot: fixed.P(0, 11)}
	d.DrawString(s)
	r := image.Rect(x, y, x+small.Bounds().Dx()*k, y+13*k)
	xdraw.NearestNeighbor.Scale(dst, r, small, small.Bounds(), draw.Over, nil)
}
