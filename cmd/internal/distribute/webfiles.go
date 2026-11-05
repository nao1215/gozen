package distribute

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // the pictures a game holds
	_ "image/png"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/deepteams/webp"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/layer2/gamedata"
)

// webFile is an entry of a web export's game/files.json: the name the game reads, the
// file the page serves and its size, whether that file is gzip, and whether the page
// fetches it only when the game opens it (a recorded stream voice; internal/webfs).
// Source identifies the file it was made from (shrinkSource), so that a later export
// given this one (--reuse) takes the served file as it is when its source has not changed.
type webFile struct {
	Name   string `json:"name"`
	File   string `json:"file"`
	Size   int64  `json:"size"`
	Gzip   bool   `json:"gzip,omitempty"`
	Later  bool   `json:"later,omitempty"`
	Source string `json:"source,omitempty"`
}

// shrinkVersion names how shrinkFile makes a served file. A file of an earlier export is
// reused only when it was made the same way: change it when toWebP or gzipped change
// what they write (webQuality among them).
const shrinkVersion = "gozen-web-shrink/1 webp-q95-sharpyuv gzip-best"

// shrinkSource is the identity of the served file made from b: the SHA-256 of
// shrinkVersion and b.
func shrinkSource(b []byte) string {
	h := sha256.New()
	h.Write([]byte(shrinkVersion + "\x00"))
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

// maxShrinkJobs bounds the files shrinkWeb converts at once. At one per CPU (32) a 397 MB
// game of 4K pictures grew to 35 GB and was killed for lack of memory.
const maxShrinkJobs = 4

// shrinkJobs is how many files to convert at once with procs CPUs: at most maxShrinkJobs
// and at least one.
func shrinkJobs(procs int) int {
	return max(min(procs, maxShrinkJobs), 1)
}

// pixelMemory is about the memory converting a picture to WebP takes for each of its
// pixels (its decoded pixels, the encoder's planes and the output): at its peak a
// 4204×6144 RGBA picture took 1.3 GB (52 bytes a pixel), a 3840×2160 RGB one 0.18 GB.
const pixelMemory = 56

// maxShrinkMemory bounds the memory the pictures being converted at once may take, and
// minShrinkMemory is what they may take however little memory is free.
const (
	maxShrinkMemory = 2 << 30
	minShrinkMemory = 512 << 20
)

// shrinkMemory is the memory budget of the conversions with available bytes of free
// memory (0 when unknown): half of it, within minShrinkMemory and maxShrinkMemory.
func shrinkMemory(available uint64) int64 {
	if available == 0 {
		return maxShrinkMemory
	}
	return int64(min(max(available/2, minShrinkMemory), maxShrinkMemory))
}

// budget is memory that conversions take a part of and give back: a part waits until it
// is free, and a part larger than the whole takes the whole (it runs alone).
type budget struct {
	mu          sync.Mutex
	freed       *sync.Cond
	free, total int64
}

func newBudget(total int64) *budget {
	b := &budget{free: total, total: total}
	b.freed = sync.NewCond(&b.mu)
	return b
}

// take waits until n bytes (at most the whole budget) are free and takes them; it returns
// what it took, for give.
func (b *budget) take(n int64) int64 {
	n = min(max(n, 0), b.total)
	b.mu.Lock()
	defer b.mu.Unlock()
	for b.free < n {
		b.freed.Wait()
	}
	b.free -= n
	return n
}

func (b *budget) give(n int64) {
	b.mu.Lock()
	b.free += n
	b.mu.Unlock()
	b.freed.Broadcast()
}

// availableMemory is the memory the system can give without swapping (MemAvailable of
// /proc/meminfo), or 0 when it cannot be read (not Linux).
func availableMemory() uint64 {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for line := range strings.Lines(string(b)) {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "MemAvailable:" && fields[2] == "kB" {
			kb, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return 0
			}
			return kb << 10
		}
	}
	return 0
}

// earlierExport is a previous web export whose served files a new export may reuse: its
// files.json entries by name, and its game folder.
type earlierExport struct {
	dir   string
	files map[string]webFile
}

// readEarlier reads the web export at dir for reuse. An empty dir reuses nothing; a
// directory that is not a web export is an error, so that a mistyped --reuse is not a
// silent full conversion.
func readEarlier(dir string) (*earlierExport, error) {
	if dir == "" {
		return nil, nil
	}
	game := filepath.Join(dir, "game")
	b, err := os.ReadFile(filepath.Join(game, "files.json")) //nolint:gosec // The caller-selected previous export.
	if err != nil {
		return nil, fmt.Errorf("reuse: not a web export: %w", err)
	}
	var files []webFile
	if err := json.Unmarshal(b, &files); err != nil {
		return nil, fmt.Errorf("reuse: %s: %w", filepath.Join(game, "files.json"), err)
	}
	e := &earlierExport{dir: game, files: make(map[string]webFile, len(files))}
	for _, f := range files {
		if f.Source != "" && fs.ValidPath(f.Name) && fs.ValidPath(f.File) {
			e.files[f.Name] = f
		}
	}
	return e, nil
}

// served returns the file the earlier export served for name when it was made from the
// same source the same way, and whether there is one. A file kept as it was has no bytes
// to take (out is nil); a file that is missing or not of its listed size is not reused.
func (e *earlierExport) served(name, source string) (f webFile, out []byte, ok bool) {
	if e == nil {
		return webFile{}, nil, false
	}
	f, ok = e.files[name]
	if !ok || f.Source != source {
		return webFile{}, nil, false
	}
	if f.File == name {
		return f, nil, true
	}
	out, err := os.ReadFile(filepath.Join(e.dir, filepath.FromSlash(f.File)))
	if err != nil || int64(len(out)) != f.Size {
		return webFile{}, nil, false
	}
	return f, out, true
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

// webQuality is the quality of the lossy WebP the pictures become, made with sharp RGB to
// YUV: at 85 with the default conversion the thin lines of a painted puppet (eyes, lips,
// strands) were smeared on a phone's screen; at 95 a 1024×1536 puppet keeps a PSNR of
// 33 dB on its lines (31 before) at a fifth of its PNG (a seventh before).
const webQuality = 95

// shrinkWeb rewrites the staged game files under dir (as CopyGame listed them) to
// download less: an 8-bit PNG or JPEG becomes a WebP of the same pixels (lossy, its
// alpha lossless; a paletted PNG lossless), and another file that gzip makes at least a
// tenth smaller is served gzipped. A file stays as it is when that does not make it
// smaller. The game reads a picture by its content, not by its name, so it is served as
// name.webp under its own name; the game files under the source do not change.
// A file whose source has not changed since the earlier export (which may be nil) is
// taken from it instead of being converted again. A large picture takes much memory to
// convert, so few files are converted at once (shrinkJobs) and the pictures being
// converted share a budget of memory by their pixels (shrinkMemory).
func shrinkWeb(dir string, names []string, later string, earlier *earlierExport) ([]webFile, error) {
	s := shrinker{dir: dir, taken: make(map[string]bool, len(names)), earlier: earlier, memory: newBudget(shrinkMemory(availableMemory()))}
	for _, name := range names {
		s.taken[name] = true
	}
	files := make([]webFile, len(names))
	errs := make([]error, len(names))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range shrinkJobs(runtime.GOMAXPROCS(0)) {
		wg.Go(func() {
			for i := range jobs {
				files[i], errs[i] = s.file(names[i])
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

// shrinker makes the served files of a staged game: dir is its folder, taken the names
// of its files, earlier the export whose files it may reuse (or nil) and memory the
// budget of the pictures it converts at once.
type shrinker struct {
	dir     string
	taken   map[string]bool
	earlier *earlierExport
	memory  *budget
}

func (s *shrinker) file(name string) (webFile, error) {
	dir, taken, earlier := s.dir, s.taken, s.earlier
	src := filepath.Join(dir, filepath.FromSlash(name))
	b, err := os.ReadFile(src) //nolint:gosec // A file of the newly staged game.
	if err != nil {
		return webFile{}, err
	}
	source := shrinkSource(b)
	kept := webFile{Name: name, File: name, Size: int64(len(b)), Source: source}
	if f, out, ok := earlier.served(name, source); ok && (f.File == name || !taken[f.File]) {
		if out == nil {
			return kept, nil
		}
		f.Later = false
		if err := cli.WriteNew(filepath.Join(dir, filepath.FromSlash(f.File)), out); err != nil {
			return webFile{}, err
		}
		return f, os.Remove(src)
	}
	f := webFile{Name: name, File: name, Size: int64(len(b)), Source: source}
	var out []byte
	switch strings.ToLower(path.Ext(name)) {
	case ".png", ".jpg", ".jpeg":
		out, f.File = s.webP(b), name+".webp"
	case ".gif", ".webp", ".ogg", ".mp3":
		// compressed already
	default:
		out, f.File, f.Gzip = gzipped(b), name+".gz", true
	}
	if out == nil || taken[f.File] {
		return kept, nil
	}
	if err := cli.WriteNew(filepath.Join(dir, filepath.FromSlash(f.File)), out); err != nil {
		return webFile{}, err
	}
	f.Size = int64(len(out))
	return f, os.Remove(src)
}

// webP is toWebP within the memory budget: it waits until the memory of b's pixels is free.
func (s *shrinker) webP(b []byte) []byte {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil
	}
	// Keep an oversized source unchanged rather than decode outside the budget.
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width) > s.memory.total/pixelMemory/int64(cfg.Height) {
		return nil
	}
	defer s.memory.give(s.memory.take(int64(cfg.Width) * int64(cfg.Height) * pixelMemory))
	return toWebP(b)
}

// toWebP returns the picture b as WebP, or nil when it is not an 8-bit picture (a 16-bit
// depth map, which WebP would cut to 8 bits) or the WebP would not be smaller.
func toWebP(b []byte) []byte {
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil
	}
	opts := webp.DefaultOptions()
	opts.Quality, opts.UseSharpYUV = webQuality, true
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
