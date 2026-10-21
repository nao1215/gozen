package play

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

func sysPNG(t *testing.T, w, h int, f func(x, y int) color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, f(x, y))
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func sysPics(t *testing.T, files map[string][]byte) *pictures {
	t.Helper()
	fsys := fstest.MapFS{}
	for n, b := range files {
		fsys["assets/"+n] = &fstest.MapFile{Data: b}
	}
	p := newPictures(fsys)
	t.Cleanup(p.close)
	return p
}

var sysBlue = func(int, int) color.NRGBA { return color.NRGBA{B: 200, A: 255} }

// TestPicturesMissingAndCached: a missing or broken file is the magenta stand-in, an existing
// one is loaded once; the seen hook is told which.
func TestPicturesMissingAndCached(t *testing.T) {
	p := sysPics(t, map[string][]byte{"a.png": sysPNG(t, 8, 6, sysBlue), "broken.png": []byte("not a picture")})
	if p.get("") != nil {
		t.Fatal("an empty path is a picture")
	}
	seen := map[string]bool{}
	p.seen = func(path string, ok bool) { seen[path] = ok }
	a := p.get("a.png")
	if a == missing || a.Bounds().Dx() != 8 || a.Bounds().Dy() != 6 {
		t.Fatalf("a.png loaded as %v", a.Bounds())
	}
	if p.get("a.png") != a {
		t.Fatal("loaded twice")
	}
	if p.get("nowhere.png") != missing || p.get("broken.png") != missing {
		t.Fatal("missing and broken files should be the stand-in")
	}
	if !seen["a.png"] || seen["nowhere.png"] || seen["broken.png"] {
		t.Fatalf("seen hook: %v", seen)
	}
}

// TestPicturesTrimDropsTheLeastRecentlyUsed: over the budget the least recently used picture
// goes first; the newest stays.
func TestPicturesTrimDropsTheLeastRecentlyUsed(t *testing.T) {
	p := sysPics(t, map[string][]byte{
		"a.png": sysPNG(t, 10, 10, sysBlue), "b.png": sysPNG(t, 10, 10, sysBlue), "c.png": sysPNG(t, 10, 10, sysBlue),
	})
	p.budget = 10*10*4*2 + 1
	p.get("a.png")
	p.get("b.png")
	p.get("a.png") // a is newer than b now
	p.get("c.png")
	if p.imgs["b.png"] != nil {
		t.Fatal("the least recently used picture was kept")
	}
	if p.imgs["a.png"] == nil || p.imgs["c.png"] == nil {
		t.Fatalf("recent pictures dropped: %v", len(p.imgs))
	}
	if p.bytes > p.budget {
		t.Fatalf("%d bytes held over the budget %d", p.bytes, p.budget)
	}
}

// TestPicturesResetForgetsAndReloads: reset empties the caches; the next get loads again.
func TestPicturesResetForgetsAndReloads(t *testing.T) {
	p := sysPics(t, map[string][]byte{"a.png": sysPNG(t, 4, 4, sysBlue)})
	first := p.get("a.png")
	p.reset()
	if len(p.imgs) != 0 || p.bytes != 0 || p.gen != 1 {
		t.Fatalf("after reset: %d images %d bytes gen %d", len(p.imgs), p.bytes, p.gen)
	}
	if p.get("a.png") == first {
		t.Fatal("served the old image after a reset")
	}
}

// TestPicturesPreloadFeedsTheNextGet: a preloaded picture is decoded in the background and
// the next get takes it.
func TestPicturesPreloadFeedsTheNextGet(t *testing.T) {
	p := sysPics(t, map[string][]byte{"a.png": sysPNG(t, 4, 4, sysBlue), "b.png": sysPNG(t, 4, 4, sysBlue)})
	p.preload("a.png", "", "nowhere.png", "b.png")
	p.workers.Wait()
	p.mu.Lock()
	n := len(p.decoded)
	p.mu.Unlock()
	if n != 2 {
		t.Fatalf("%d pictures decoded ahead, want 2", n)
	}
	img := p.get("a.png")
	if img == missing || img.Bounds().Dx() != 4 {
		t.Fatal("preloaded picture not served")
	}
	p.mu.Lock()
	left := len(p.decoded)
	p.mu.Unlock()
	if left != 1 {
		t.Fatalf("the decoded copy was not consumed: %d left", left)
	}
	// a picture already loaded is not decoded again.
	p.preload("a.png")
	p.workers.Wait()
	if len(p.decoded) != 1 {
		t.Fatal("decoded a loaded picture again")
	}
	// the budget limits the decoded copies kept.
	q := sysPics(t, map[string][]byte{"c.png": sysPNG(t, 4, 4, sysBlue)})
	q.budget = 1
	q.preload("c.png")
	q.workers.Wait()
	if len(q.decoded) != 0 {
		t.Fatal("kept a decoded picture over the budget")
	}
}

// TestFindHeadLocatesTheHeadOfAStandingPicture: for a figure on a transparent background the
// head square is 42% of the width, centred on the figure and starting just above it.
func TestFindHeadLocatesTheHeadOfAStandingPicture(t *testing.T) {
	const w, h = 100, 200
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 30; y < 190; y++ {
		for x := 60; x < 80; x++ { // a figure standing right of centre
			img.SetNRGBA(x, y, color.NRGBA{R: 200, A: 255})
		}
	}
	r := findHead(img)
	if r.Dx() != 42 || r.Dy() != 42 {
		t.Fatalf("head square %v, want 42x42", r)
	}
	if cx := (r.Min.X + r.Max.X) / 2; cx < 65 || cx > 75 {
		t.Fatalf("head centre x %d, want about 70", cx)
	}
	if r.Min.Y > 30 || r.Min.Y < 25 {
		t.Fatalf("head starts at y %d, want just above the figure at 30", r.Min.Y)
	}
	// a picture filled to its top corners has no head to find, and an empty one neither.
	if r := findHead(image.NewNRGBA(image.Rect(0, 0, 10, 10))); !r.Empty() {
		t.Fatalf("empty picture: %v", r)
	}
	full := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	for i := 3; i < len(full.Pix); i += 4 {
		full.Pix[i] = 255
	}
	if r := findHead(full); !r.Empty() {
		t.Fatalf("picture with opaque corners: %v", r)
	}
}

// TestHeadOfAPicture: the face box beside the picture wins, then the one gozen art faces
// found, then the detected head, then the top middle of the picture.
func TestHeadOfAPicture(t *testing.T) {
	fig := func(x, y int) color.NRGBA {
		if x >= 30 && x < 50 && y >= 10 {
			return color.NRGBA{R: 200, A: 255}
		}
		return color.NRGBA{}
	}
	faces := `{"version": 1, "model": "t", "pictures": {"found/f.png": {"box": [10, 10, 30, 30], "size": [100, 100], "sha256": "x"}}}`
	p := sysPics(t, map[string][]byte{
		"set/a.png": sysPNG(t, 100, 100, fig), "found/f.png": sysPNG(t, 100, 100, fig),
		"flat/b.png": sysPNG(t, 100, 100, sysBlue), "faces.json": []byte(faces),
	})
	p.boxes["set"] = []float64{0.5, 0.25, 0.5}
	if r := p.head("set/a.png"); r != image.Rect(25, 0, 75, 50) {
		t.Fatalf("explicit box: %v", r)
	}
	r := p.head("found/f.png")
	// box (10..30) centre 20, half = 20*1.4/2 = 14.
	if r != image.Rect(6, 6, 34, 34) {
		t.Fatalf("found face: %v", r)
	}
	p2 := sysPics(t, map[string][]byte{"x/a.png": sysPNG(t, 100, 100, fig)})
	if r := p2.head("x/a.png"); r.Empty() || r.Dx() != 42 {
		t.Fatalf("detected head: %v", r)
	}
	if r := p.head("flat/b.png"); r != image.Rect(25, 0, 75, 50) {
		t.Fatalf("fallback head: %v", r)
	}
	if r := p.foundHead("flat/b.png"); r != image.Rect(25, 0, 75, 50) {
		t.Fatalf("foundHead fallback: %v", r)
	}
	if r := p2.foundHead("x/a.png"); r.Empty() || r.Dx() != 42 {
		t.Fatalf("foundHead: %v", r)
	}
	// face boxes with a broken file are ignored with the default result.
	p3 := sysPics(t, map[string][]byte{"flat/b.png": sysPNG(t, 100, 100, sysBlue), "faces.json": []byte("{")})
	if r := p3.head("flat/b.png"); r != image.Rect(25, 0, 75, 50) {
		t.Fatalf("head with broken faces.json: %v", r)
	}
}

// TestBlurredPictureIsSmallerAndCached: the blurred copy used behind a picture is smaller than
// the picture and made once.
func TestBlurredPictureIsSmallerAndCached(t *testing.T) {
	p := sysPics(t, map[string][]byte{"a.png": sysPNG(t, 640, 360, sysBlue)})
	b := p.blurred("a.png")
	if b == nil || b.Bounds().Dx() >= 640 || b.Bounds().Dx() == 0 {
		t.Fatalf("blurred copy %v", b.Bounds())
	}
	if p.blurred("a.png") != b {
		t.Fatal("blurred twice")
	}
}

// TestTitleFiguresAreLoadedAndDrawn: the title's standing figures are fetched when the title
// is drawn, and the frame goes on for the figures' rise.
func TestTitleFiguresAreLoadedAndDrawn(t *testing.T) {
	g := sysOpen4(t, Options{}, map[string]string{"game.yaml+": "title_figures: [{file: x.png, x: 0.7, height: 0.8}]\ntitle_text: {size: 80}\nsubtitle: Part one\n"})
	sysFrames(t, g, 50)
	if g.pics.imgs["x.png"] == nil {
		t.Fatal("the figure was never loaded")
	}
	g.drawStanding(ebiten.NewImage(1280, 720), "x.png", 600, 700, 400, 1, true, nil)
	g.drawFace(ebiten.NewImage(1280, 720), "x.png", 10, 10, 64, 0.5, true)
	g.drawFace(ebiten.NewImage(1280, 720), "", 10, 10, 64, 0.5, false)
	time.Sleep(time.Millisecond)
}

// TestTitleArtAndBackground: title_art and title_bg pictures are loaded for the title.
func TestTitleArtAndBackground(t *testing.T) {
	g := sysOpen(t, Options{}, map[string]string{"game.yaml+": "title_art: {file: x.png}\ntitle_background: {file: x.png}\n"})
	sysFrames(t, g, 40)
	if g.pics.imgs["x.png"] == nil {
		t.Fatal("the picture was never loaded")
	}
	if g.titleCentered() {
		t.Fatal("a title with art is not centred")
	}
}
