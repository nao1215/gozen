package play

import (
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"sync"
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

	mu  sync.Mutex
	err error
}

type shotJob struct {
	path string
	img  *image.NRGBA
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
	w.jobs <- shotJob{path: path, img: img}
}

func (w *shotWriter) work() {
	for j := range w.jobs {
		err := writeShot(j.path, j.img)
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

func writeShot(path string, img *image.NRGBA) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.Create(path) //nolint:gosec // The output directory is explicit and the basename is validated by the caller.
	if err != nil {
		return err
	}
	return errors.Join(shotEncoder.Encode(f, img), f.Close())
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
