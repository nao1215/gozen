package distribute

import (
	"bytes"
	"compress/gzip"
	"errors"
	"image"
	_ "image/jpeg" // the pictures a game holds
	_ "image/png"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/deepteams/webp"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/layer2/gamedata"
)

// webFile is an entry of a web export's game/files.json: the name the game reads, the
// file the page serves and its size, whether that file is gzip, and whether the page
// fetches it only when the game opens it (a recorded stream voice; internal/webfs).
type webFile struct {
	Name  string `json:"name"`
	File  string `json:"file"`
	Size  int64  `json:"size"`
	Gzip  bool   `json:"gzip,omitempty"`
	Later bool   `json:"later,omitempty"`
}

// voiceDir is the folder of the game's recorded stream voices ("assets/VOICES/", ADR 0052),
// whose files a web export fetches only when the watch view comes to their lines, or ""
// when the game has none.
func voiceDir(db *gamedata.DB) string {
	if db.Streams == nil || db.Streams.Voices == "" {
		return ""
	}
	return "assets/" + db.Streams.Voices + "/"
}

// webQuality is the quality of the lossy WebP the pictures become. At 85 a painted
// 1024×1536 puppet keeps a PSNR near 45 dB (no visible change) at a fifth of its PNG.
const webQuality = 85

// shrinkWeb rewrites the staged game files under dir (as CopyGame listed them) to
// download less: an 8-bit PNG or JPEG becomes a WebP of the same pixels (lossy, its
// alpha lossless; a paletted PNG lossless), and another file that gzip makes at least a
// tenth smaller is served gzipped. A file stays as it is when that does not make it
// smaller. The game reads a picture by its content, not by its name, so it is served as
// name.webp under its own name; the game files under the source do not change.
func shrinkWeb(dir string, names []string, later string) ([]webFile, error) {
	taken := make(map[string]bool, len(names))
	for _, name := range names {
		taken[name] = true
	}
	files := make([]webFile, len(names))
	errs := make([]error, len(names))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		wg.Go(func() {
			for i := range jobs {
				files[i], errs[i] = shrinkFile(dir, names[i], taken)
			}
		})
	}
	for i := range names {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	for i := range files {
		files[i].Later = later != "" && strings.HasPrefix(files[i].Name, later)
	}
	return files, errors.Join(errs...)
}

func shrinkFile(dir, name string, taken map[string]bool) (webFile, error) {
	src := filepath.Join(dir, filepath.FromSlash(name))
	b, err := os.ReadFile(src) //nolint:gosec // A file of the newly staged game.
	if err != nil {
		return webFile{}, err
	}
	f := webFile{Name: name, File: name, Size: int64(len(b))}
	var out []byte
	switch strings.ToLower(path.Ext(name)) {
	case ".png", ".jpg", ".jpeg":
		out, f.File = toWebP(b), name+".webp"
	case ".gif", ".webp", ".ogg", ".mp3":
		// compressed already
	default:
		out, f.File, f.Gzip = gzipped(b), name+".gz", true
	}
	if out == nil || taken[f.File] {
		return webFile{Name: name, File: name, Size: int64(len(b))}, nil
	}
	if err := cli.WriteNew(filepath.Join(dir, filepath.FromSlash(f.File)), out); err != nil {
		return webFile{}, err
	}
	f.Size = int64(len(out))
	return f, os.Remove(src)
}

// toWebP returns the picture b as WebP, or nil when it is not an 8-bit picture (a 16-bit
// depth map, which WebP would cut to 8 bits) or the WebP would not be smaller.
func toWebP(b []byte) []byte {
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil
	}
	opts := webp.DefaultOptions()
	opts.Quality = webQuality
	switch img.(type) {
	case *image.NRGBA, *image.RGBA, *image.YCbCr, *image.Gray, *image.CMYK:
	case *image.Paletted:
		// drawn in few colours (pixel art, icons): lossy would blur its edges
		opts.Lossless = true
	default:
		return nil
	}
	var buf bytes.Buffer
	if err := webp.Encode(&buf, img, opts); err != nil || buf.Len() >= len(b) {
		return nil
	}
	if cfg, err := webp.DecodeConfig(bytes.NewReader(buf.Bytes())); err != nil || cfg.Width != img.Bounds().Dx() || cfg.Height != img.Bounds().Dy() {
		return nil
	}
	return buf.Bytes()
}

// gzipped returns b gzipped, or nil when that saves less than a tenth.
func gzipped(b []byte) []byte {
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil
	}
	if _, err := w.Write(b); err != nil || w.Close() != nil || buf.Len() > len(b)*9/10 {
		return nil
	}
	return buf.Bytes()
}
