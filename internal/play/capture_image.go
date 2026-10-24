package play

import (
	"errors"
	"image"
	"image/color"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"time"

	xdraw "golang.org/x/image/draw"

	"github.com/nao1215/gozen/internal/layer1/picturelimit"
)

// CaptureGIF selects an animation instead of a PNG for a capture job.
// Zero fields select 10 FPS, three seconds and at most 640 pixels wide.
// Script contains inputs executed during the animation, after preparation.
type CaptureGIF struct {
	FPS      int
	Duration time.Duration
	Width    int
	Script   string
}

type captureFrames struct {
	gif.GIF
	fps, count int
	duration   time.Duration
}

func newCaptureFrames(cfg CaptureGIF, width, height int) (*captureFrames, error) {
	if err := picturelimit.Dimensions(width, height); err != nil {
		return nil, err
	}
	if cfg.FPS == 0 {
		cfg.FPS = 10
	}
	if cfg.Duration == 0 {
		cfg.Duration = 3 * time.Second
	}
	if cfg.Width == 0 {
		cfg.Width = min(width, 640)
	}
	if cfg.FPS < 1 || cfg.FPS > 30 || cfg.Duration <= 0 || cfg.Duration > 10*time.Second || cfg.Width < 1 || cfg.Width > 1280 {
		return nil, errors.New("GIF requires 1–30 FPS, a duration up to 10s, and width 1–1280")
	}
	// All operands are bounded before multiplication, including on 32-bit builds.
	h := max(1, height*cfg.Width/width)
	n := int(int64(cfg.Duration) * int64(cfg.FPS) / int64(time.Second))
	if h > 1280 || cfg.Width > (1280*720)/h || n < 2 || n > 300 || n > (64<<20)/(cfg.Width*h) {
		return nil, errors.New("GIF exceeds frame or pixel budget; reduce width, duration or FPS (2–300 frames, 921600 pixels/frame, 64 MiB total)")
	}
	return &captureFrames{GIF: gif.GIF{Config: image.Config{ColorModel: color.Palette(palette.Plan9), Width: cfg.Width, Height: h}}, fps: cfg.FPS, count: n, duration: cfg.Duration}, nil
}

// due returns the simulation tick of the next frame. The batch holds that tick
// until Draw samples it, even when Ebitengine requests consecutive Updates.
func (f *captureFrames) due() int {
	return len(f.Image) * 60 / f.fps
}

func (f *captureFrames) add(src image.Image) {
	bounds := image.Rect(0, 0, f.Config.Width, f.Config.Height)
	scaled := image.NewRGBA(bounds)
	xdraw.ApproxBiLinear.Scale(scaled, bounds, src, src.Bounds(), draw.Src, nil)
	p := image.NewPaletted(bounds, palette.Plan9)
	draw.FloydSteinberg.Draw(p, bounds, scaled, image.Point{})
	i := len(f.Image)
	// Round cumulative timestamps, so 30 FPS uses 3/4 centisecond delays.
	stamp := func(n int) int64 {
		t := min(int64(n)*int64(time.Second)/int64(f.fps), int64(f.duration))
		if n == f.count {
			t = int64(f.duration)
		}
		return (t + int64(5*time.Millisecond)) / int64(10*time.Millisecond)
	}
	f.Image = append(f.Image, p)
	f.Delay = append(f.Delay, int(stamp(i+1)-stamp(i)))
	f.Disposal = append(f.Disposal, gif.DisposalNone)
}
