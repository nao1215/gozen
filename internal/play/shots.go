package play

import (
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/nao1215/gozen/internal/layer1/picturelimit"
)

// shotWriter writes the screenshots off the game loop. Draw only copies the screen's
// pixels; workers encode them as PNG and write the files, so a filmed run (a shot every
// few ticks at 1920x1080) does not stall on the encoder. A shot taken in Draw already
// holds the loop (Update waits for it), so the frames are the same; only the files come
// a moment later. wait writes the shots still queued before Run returns.
type shotWriter struct {
	once sync.Once
	jobs chan shotJob
	wg   sync.WaitGroup
	// pool keeps the pixel buffers of written shots for the next ones (8 MB each at 1920x1080).
	pool sync.Pool

	mu      sync.Mutex
	err     error
	pending map[string]chan struct{}
}

type shotJob struct {
	path     string
	img      *image.NRGBA
	previous <-chan struct{}
	finished chan struct{}
}

// shotEncoder trades a little size for speed: BestSpeed is about five times faster than
// the default level on a game screen, for files about 15% larger.
var shotEncoder = png.Encoder{CompressionLevel: png.BestSpeed}

// pixels returns an image of the bounds to copy a screen into, reusing a written one's buffer.
func (w *shotWriter) pixels(b image.Rectangle) *image.NRGBA {
	if img, ok := w.pool.Get().(*image.NRGBA); ok && img.Rect == b {
		return img
	}
	return image.NewNRGBA(b)
}

// write queues the image to be written to path. It blocks while every worker is busy
// and the queue is full, so the memory held stays bounded.
func (w *shotWriter) write(path string, img *image.NRGBA) {
	w.once.Do(func() {
		n := min(max(1, runtime.GOMAXPROCS(0)-1), 4)
		w.jobs = make(chan shotJob, n)
		for range n {
			w.wg.Go(w.work)
		}
	})
	if w.jobs == nil { // after wait: write it here
		w.done(writeShot(path, img))
		return
	}
	path = filepath.Clean(path)
	w.mu.Lock()
	if w.pending == nil {
		w.pending = make(map[string]chan struct{})
	}
	previous := w.pending[path]
	finished := make(chan struct{})
	w.pending[path] = finished
	w.mu.Unlock()
	w.jobs <- shotJob{path: path, img: img, previous: previous, finished: finished}
}

func (w *shotWriter) work() {
	for j := range w.jobs {
		if j.previous != nil {
			<-j.previous
		}
		err := writeShot(j.path, j.img)
		w.mu.Lock()
		if w.pending[j.path] == j.finished {
			delete(w.pending, j.path)
		}
		close(j.finished)
		w.mu.Unlock()
		w.pool.Put(j.img)
		w.done(err)
	}
}

func (w *shotWriter) done(err error) {
	if err != nil {
		w.mu.Lock()
		w.err = errors.Join(w.err, err)
		w.mu.Unlock()
	}
}

func writeShot(path string, img *image.NRGBA) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".shot-*.png")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if removeErr := os.Remove(tmp); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
			err = errors.Join(err, removeErr)
		}
	}()
	if err := errors.Join(shotEncoder.Encode(f, img), f.Close()); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// failed returns the errors of the shots written since it was last asked, once.
func (w *shotWriter) failed() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	err := w.err
	w.err = nil
	return err
}

// wait writes the queued shots and returns the errors failed has not returned. A shot
// taken after it is written at once.
func (w *shotWriter) wait() error {
	w.once.Do(func() {}) // never started: nothing to wait for
	if w.jobs != nil {
		close(w.jobs)
		w.wg.Wait()
		w.jobs = nil
	}
	return w.failed()
}

// letterboxShot includes the native target's outer bars without resampling pixels.
func letterboxShot(src *image.NRGBA, width, height int) (*image.NRGBA, error) {
	if err := picturelimit.Dimensions(width, height); err != nil {
		return nil, err
	}
	if width < src.Bounds().Dx() || height < src.Bounds().Dy() || width == src.Bounds().Dx() && height == src.Bounds().Dy() {
		return src, nil
	}
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
	at := image.Pt((width-src.Bounds().Dx())/2, (height-src.Bounds().Dy())/2)
	draw.Draw(dst, src.Bounds().Add(at), src, src.Bounds().Min, draw.Src)
	return dst, nil
}
