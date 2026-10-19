package command

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/spf13/pflag"

	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/mosaic"
)

type coords [][]float64

func (c *coords) String() string { return fmt.Sprint(*c) }

// Type names the value kind for pflag.
func (c *coords) Type() string { return "coords" }

func (c *coords) Set(s string) error {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
	if len(fields) == 0 {
		return fmt.Errorf("coordinates cannot be empty")
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

// manualOptions are the options of gozen mosaic manual beyond the common ones.
type manualOptions struct {
	o                                     *options
	rects, boxes, polys, ellipses, region *coords
	norm                                  *bool
	note, regionID, src, viewOut          *string
	by, left, top, right, bottom          *int
	size, step                            *int
}

func manualFlags(cli *cli, fs *pflag.FlagSet) manualOptions {
	m := manualOptions{o: cli.commonFlags(fs), rects: &coords{}, boxes: &coords{}, polys: &coords{}, ellipses: &coords{}, region: &coords{}}
	fs.Var(m.rects, "rect", "add a rectangle `X,Y,W,H` (top left, width, height); repeatable")
	fs.Var(m.boxes, "box", "add a box `X0,Y0,X1,Y1` (top left and bottom right corners); repeatable")
	fs.Var(m.polys, "poly", "add a polygon `X1,Y1,X2,Y2,...` of 3 points or more; repeatable")
	fs.Var(m.ellipses, "ellipse", "add an ellipse `CX,CY,RX,RY` (center and radii); repeatable")
	m.norm = fs.Bool("norm", false, "read coordinates as fractions from 0 to 1 of the width and the height")
	m.note = fs.String("note", "", "add this `TEXT` to the note of the image in the YAML")
	m.regionID = fs.String("region-id", "", "for drop and grow, the automatic region `IDS` shown by view, such as A1, separated by commas; grow also takes all")
	m.by = fs.Int("by", 0, "for grow, grow every side by these `PIXELS`; negative shrinks")
	m.left = fs.Int("left", 0, "for grow, grow the left side by these `PIXELS`")
	m.top = fs.Int("top", 0, "for grow, grow the top side by these `PIXELS`")
	m.right = fs.Int("right", 0, "for grow, grow the right side by these `PIXELS`")
	m.bottom = fs.Int("bottom", 0, "for grow, grow the bottom side by these `PIXELS`")
	m.src = fs.String("src", "original", "for view, draw the `SOURCE` image: original or masked")
	fs.Var(m.region, "region", "for view, zoom into the area `X,Y,W,H` (default: the whole image)")
	m.size = fs.Int("size", 1536, "for view, the long side of the drawing in `PIXELS`")
	m.step = fs.Int("step", 0, "for view, the scale interval in original `PIXELS`; 0 chooses one")
	m.viewOut = fs.String("view-out", "", "for view, write the drawing to this `FILE` (default: <out>/manual/<image>.<source>.png)")
	return m
}

func (cli *cli) manualCmd(args []string) (err error) {
	if len(args) == 0 {
		return argumentError{fmt.Errorf("a manual command is required: list, show, view, add, drop, grow, skip, check, or clear; use --help")}
	}
	if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		if len(args) > 1 {
			return argumentError{fmt.Errorf("help takes no arguments")}
		}
		fs := newFlagSet("gozen mosaic manual")
		manualFlags(cli, fs)
		cli.out.write(manualPage.String(fs))
		return clihelp.ErrHelp
	}
	sub := args[0]
	if !slices.Contains([]string{"list", "show", "view", "add", "drop", "grow", "skip", "check", "clear"}, sub) {
		return argumentError{fmt.Errorf("unknown manual command %q: use list, show, view, add, drop, grow, skip, check, or clear", sub)}
	}
	fs := newFlagSet("gozen mosaic manual " + sub)
	mo := manualFlags(cli, fs)
	if err := cli.parse(fs, args[1:], manualPage); err != nil {
		return err
	}
	o, rects, boxes, polys, ellipses, region := mo.o, *mo.rects, *mo.boxes, *mo.polys, *mo.ellipses, *mo.region
	norm, note, regionID, by, left, top, right, bottom := mo.norm, mo.note, mo.regionID, mo.by, mo.left, mo.top, mo.right, mo.bottom
	src, size, step, viewOut := mo.src, mo.size, mo.step, mo.viewOut
	need := 2
	if sub == "list" {
		need = 1
	}
	if fs.NArg() != need {
		if need == 1 {
			return argumentError{fmt.Errorf("expected a game directory: gozen mosaic manual list [OPTIONS] <game-dir>")}
		}
		return argumentError{fmt.Errorf("expected a game directory and an image: gozen mosaic manual %s [OPTIONS] <game-dir> <IMAGE>", sub)}
	}
	if *size < 1 || *size > 4096 || *step < 0 {
		return fmt.Errorf("view size must be 1..4096 and step >= 0")
	}
	if sub != "add" && sub != "drop" && (len(boxes)+len(polys)+len(ellipses) > 0) {
		return fmt.Errorf("box, polygon, and ellipse flags require add or drop")
	}
	if sub == "drop" && (len(polys)+len(ellipses) > 0) {
		return fmt.Errorf("drop requires rectangles")
	}
	gameDir := fs.Arg(0)
	c, err := o.setup(gameDir)
	if err != nil {
		return err
	}
	if sub == "list" {
		return manualList(c)
	}
	pic, _, origPath := resolve(c, fs.Arg(1))
	if pic == "" && sub != "view" {
		return fmt.Errorf("%s は assets の外の絵なので、view だけができる（指示は assets の絵にだけ書ける）", fs.Arg(1))
	}
	if err := validPicture(c, pic); err != nil && pic != "" {
		return err
	}
	img, err := decode(origPath)
	if err != nil {
		return err
	}
	b := img.Bounds()
	conv := func(v []float64, n int, what string) ([]int, error) {
		if len(v) != n && (n >= 0 || len(v) < 6 || len(v)%2 != 0) {
			return nil, fmt.Errorf("%s の数が合わない: %v", what, v)
		}
		out := make([]int, len(v))
		for i, f := range v {
			if *norm {
				if f < 0 || f > 1 {
					return nil, fmt.Errorf("normalized coordinates must be in [0,1]")
				}
				if i%2 == 0 {
					f *= float64(b.Dx())
				} else {
					f *= float64(b.Dy())
				}
			}
			out[i] = int(math.Round(f))
		}
		return out, nil
	}
	if sub != "add" && sub != "drop" && sub != "grow" && len(rects)+len(boxes)+len(polys)+len(ellipses) > 0 {
		return fmt.Errorf("geometry flags require add, drop, or grow")
	}
	needsManual := sub == "add" || sub == "drop" || sub == "grow" || sub == "skip" || sub == "check" || sub == "clear"
	if needsManual && o.manualFile == "" {
		return errors.New("--manual（または環境変数 MOSAIC_MANUAL）に manual の YAML を指定する")
	}
	var geometryErr error
	clip := func(x []int) [4]int {
		if x[2] <= 0 || x[3] <= 0 {
			geometryErr = fmt.Errorf("rectangle width and height must be positive")
			return [4]int{}
		}
		r := clipRect(x, b)
		if r[2] <= 0 || r[3] <= 0 {
			geometryErr = fmt.Errorf("rectangle does not intersect the image")
		}
		return r
	}
	m := c.manual[pic]
	switch sub {
	case "show":
		return manualShow(c, pic)
	case "view":
		var r image.Rectangle
		if len(region) > 0 {
			v, err := conv(region[0], 4, "--region")
			if err != nil {
				return err
			}
			r = mosaic.Rect([4]int{v[0], v[1], v[2], v[3]})
		}
		_, err := manualView(c, pic, origPath, *src, r, *size, *step, *viewOut)
		return err
	case "add":
		n := 0
		for _, v := range rects {
			x, err := conv(v, 4, "--rect")
			if err != nil {
				return err
			}
			m.Add = append(m.Add, clip(x))
			n++
		}
		for _, v := range boxes {
			x, err := conv(v, 4, "--box")
			if err != nil {
				return err
			}
			m.Add = append(m.Add, clip([]int{x[0], x[1], x[2] - x[0], x[3] - x[1]}))
			n++
		}
		for _, v := range polys {
			x, err := conv(v, -1, "--poly")
			if err != nil {
				return err
			}
			m.Poly = append(m.Poly, x)
			n++
		}
		for _, v := range ellipses {
			x, err := conv(v, 4, "--ellipse")
			if err != nil {
				return err
			}
			if x[2] <= 0 || x[3] <= 0 {
				return fmt.Errorf("ellipse radii must be positive")
			}
			m.Poly = append(m.Poly, ellipse(x[0], x[1], x[2], x[3], 48))
			n++
		}
		if n == 0 {
			return errors.New("add には --rect・--box・--poly・--ellipse のどれかが要る")
		}
		m.Checked = false
	case "drop":
		for _, v := range rects {
			x, err := conv(v, 4, "--rect")
			if err != nil {
				return err
			}
			m.Drop = append(m.Drop, clip(x))
		}
		for _, v := range boxes {
			x, err := conv(v, 4, "--box")
			if err != nil {
				return err
			}
			m.Drop = append(m.Drop, clip([]int{x[0], x[1], x[2] - x[0], x[3] - x[1]}))
		}
		if *regionID != "" {
			e, ok := reportEntry(c, pic)
			if !ok {
				return fmt.Errorf("%s は report.json に無い（先に auto か view を動かす）", pic)
			}
			for id := range strings.SplitSeq(*regionID, ",") {
				i, err := strconv.Atoi(strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(id)), "A"))
				if err != nil || i < 1 || i > len(e.autoRegions()) {
					return fmt.Errorf("--region-id %s が無い（自動の範囲は A1〜A%d）", id, len(e.autoRegions()))
				}
				m.Drop = append(m.Drop, e.autoRegions()[i-1])
			}
		}
		if len(m.Drop) == len(c.manual[pic].Drop) {
			return errors.New("drop には --region-id か --rect・--box が要る")
		}
		m.Checked = false
	case "grow":
		set := map[string]bool{}
		fs.Visit(func(f *pflag.Flag) { set[f.Name] = true })
		if set["margin"] {
			if o.margin < 0 {
				return errors.New("--margin は 0 以上")
			}
			m.Margin = o.margin
		}
		d := [4]int{*left + *by, *top + *by, *right + *by, *bottom + *by}
		if *regionID != "" {
			if d == [4]int{} {
				return errors.New("grow には --by か --left・--top・--right・--bottom が要る")
			}
			e, ok := reportEntry(c, pic)
			if !ok {
				return fmt.Errorf("%s は report.json に無い（先に auto を動かす）", pic)
			}
			regs := e.autoRegions()
			var ids []int
			if strings.TrimSpace(*regionID) == "all" {
				for i := range regs {
					ids = append(ids, i)
				}
			} else {
				for id := range strings.SplitSeq(*regionID, ",") {
					i, err := strconv.Atoi(strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(id)), "A"))
					if err != nil || i < 1 || i > len(regs) {
						return fmt.Errorf("--region-id %s が無い（自動の範囲は A1〜A%d）", id, len(regs))
					}
					ids = append(ids, i-1)
				}
			}
			for _, i := range ids {
				r := regs[i]
				m.Grow = append(m.Grow, [8]int{r[0], r[1], r[2], r[3], d[0], d[1], d[2], d[3]})
			}
		} else if len(rects) > 0 {

			if d == [4]int{} {
				return errors.New("grow には --by か --left・--top・--right・--bottom が要る")
			}
			for _, v := range rects {
				x, err := conv(v, 4, "--rect")
				if err != nil {
					return err
				}
				r := clip(x)
				m.Grow = append(m.Grow, [8]int{r[0], r[1], r[2], r[3], d[0], d[1], d[2], d[3]})
			}
		} else if !set["margin"] {
			return errors.New("grow には --region-id か --rect（と広げる px）か --margin が要る")
		}
		m.Checked = false
	case "skip":
		m.Skip = true
		m.Checked = false
	case "check":
		m.Checked = true
	case "clear":
		m = mosaic.Fix{}
	default:
		return argumentError{fmt.Errorf("unknown manual command %q", sub)}
	}
	if *note != "" {
		m.Note = strings.TrimSpace(m.Note + " " + *note)
	}
	for _, p := range m.Polygons() {
		if !p.Hits(p.Bounds().Intersect(b)) {
			return fmt.Errorf("polygon does not intersect the image")
		}
	}
	if geometryErr != nil {
		return geometryErr
	}
	fs.Visit(func(f *pflag.Flag) {
		if f.Name == "block" {
			m.Block = o.block
		}
	})
	c.manual[pic] = m
	if err := mosaic.ValidateFixes(c.manual); err != nil {
		return err
	}
	human, err := mosaic.LoadFixes(o.fixFile)
	if err != nil {
		return err
	}
	c.fixes[pic] = mosaic.Combine(human[pic], m)
	return publishManual(c, pic, origPath, sub, *size, *step)

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

func (e entry) autoRegions() [][4]int { return e.Regions }

func manualList(c config) error {
	enc := json.NewEncoder(c.outw)
	enc.SetEscapeHTML(false)
	for _, e := range readReport(c) {
		if len(e.Review) == 0 {
			continue
		}
		if err := enc.Encode(map[string]any{"file": e.File, "review": e.Review, "masked": e.Output != ""}); err != nil {
			return err
		}
	}
	return nil
}

func manualShow(c config, pic string) error {
	e, ok := reportEntry(c, pic)
	out := map[string]any{"file": pic, "manual": c.manual[pic], "fix": c.fixes[pic]}
	if ok {
		out["report"] = e
	} else {
		out["report"] = "report.json に無い（auto を動かす）"
	}
	enc := json.NewEncoder(c.outw)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
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
		return "", fmt.Errorf("--src は original か masked")
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
		return "", fmt.Errorf("--region が絵の外")
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
	c.outw.printf("view: %s（%s、範囲 %v、元の絵 %dx%d）\n", display, src, xywh(region), b.Dx(), b.Dy())
	return dst, nil
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
