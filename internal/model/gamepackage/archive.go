package gamepackage

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image/png"
	"io"
	"io/fs"
	"path"
	"strings"
	"sync"
	"unicode"
)

// MaxArchiveBytes and MaxUnpackedBytes bound memory/browser installations.
// MaxStoredArchiveBytes and MaxStoredUnpackedBytes bound disk-backed installs,
// whose archive is read through ReaderAt instead of retained in memory.
const (
	MaxArchiveBytes        int64 = 64 << 20
	MaxUnpackedBytes       int64 = 256 << 20
	maxEntries                   = 4096
	MaxStoredArchiveBytes  int64 = 1 << 30
	MaxStoredUnpackedBytes int64 = 2 << 30
	maxStoredEntries             = 32768
)

// Installed is an immutable verified release and its read-only game filesystem.
// The FS owns its archive bytes or an open archive file. Call Close after all
// readers and game resources stop using it. Updating never changes this handle.
type Installed struct {
	Release  Release
	FS       fs.FS
	closer   io.Closer
	once     sync.Once
	closeErr error
}

// VerifyContext validates an archive for browser installation and registry checks.
// Its input may be reused after return. Every file CRC and expansion limit is
// checked before returning an owned filesystem; callers validate game definitions.
func VerifyContext(ctx context.Context, r Release, b []byte) (*Installed, error) {
	return verify(ctx, r, b)
}

// Close releases an owned archive file once; memory-backed archives need no work.
func (i *Installed) Close() error {
	if i == nil {
		return nil
	}
	i.once.Do(func() {
		if i.closer != nil {
			i.closeErr = i.closer.Close()
		}
	})
	return i.closeErr
}

func verify(ctx context.Context, r Release, b []byte) (*Installed, error) {
	if r.Size > MaxArchiveBytes || r.UnpackedSize > MaxUnpackedBytes {
		return nil, errors.New("package exceeds memory/browser limits; use disk-backed installation")
	}
	if e := r.Validate(); e != nil {
		return nil, e
	}
	if int64(len(b)) != r.Size {
		return nil, errors.New("package size mismatch")
	}
	owned := bytes.Clone(b)
	return verifyReader(ctx, r, bytes.NewReader(owned), int64(len(owned)), MaxUnpackedBytes, maxEntries)
}

// OpenArchive verifies a local bundled or cached ZIP without cloning it when its
// file supports ReaderAt. Otherwise it uses the bounded memory path (for browser
// filesystems). The returned Installed owns the file; failures close it here.
func OpenArchive(ctx context.Context, fsys fs.FS, name string, r Release) (*Installed, error) {
	if fsys == nil || !PortablePath(name) {
		return nil, fs.ErrInvalid
	}
	f, e := fsys.Open(name)
	if e != nil {
		return nil, e
	}
	info, e := f.Stat()
	if e != nil {
		return nil, errors.Join(e, f.Close())
	}
	if !info.Mode().IsRegular() {
		return nil, errors.Join(errors.New("archive is not a regular file"), f.Close())
	}
	if reader, ok := f.(io.ReaderAt); ok {
		game, e := verifyReader(ctx, r, reader, info.Size(), MaxStoredUnpackedBytes, maxStoredEntries)
		if e != nil {
			return nil, errors.Join(e, f.Close())
		}
		game.closer = f
		return game, nil
	}
	if r.Size > MaxArchiveBytes || r.UnpackedSize > MaxUnpackedBytes {
		return nil, errors.Join(errors.New("archive filesystem requires memory/browser limits"), f.Close())
	}
	b, e := readBounded(contextReader{ctx, f}, MaxArchiveBytes)
	e = errors.Join(e, f.Close())
	if e != nil {
		return nil, e
	}
	return verify(ctx, r, b)
}

func verifyReader(ctx context.Context, r Release, reader io.ReaderAt, size, expanded int64, entries int) (*Installed, error) {
	if e := r.Validate(); e != nil {
		return nil, e
	}
	if size != r.Size || r.UnpackedSize > expanded {
		return nil, errors.New("package size exceeds installation limit")
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	h := sha256.New()
	if _, e := io.Copy(h, contextReader{ctx, io.NewSectionReader(reader, 0, size)}); e != nil {
		return nil, e
	}
	if hex.EncodeToString(h.Sum(nil)) != r.SHA256 {
		return nil, errors.New("package checksum mismatch")
	}
	if e := directoryBounds(reader, size, entries); e != nil {
		return nil, e
	}
	z, e := zip.NewReader(reader, size)
	if e != nil {
		return nil, fmt.Errorf("package ZIP: %w", e)
	}
	if len(z.File) > entries {
		return nil, errors.New("too many archive entries")
	}
	return verifyEntries(ctx, r, z, expanded)
}

func verifyEntries(ctx context.Context, r Release, z *zip.Reader, expanded int64) (*Installed, error) {
	seen := map[string]bool{}
	spellings := map[string]string{}
	types := map[string]bool{}
	var total int64
	for _, f := range z.File {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		name := strings.TrimSuffix(f.Name, "/")
		dir := f.FileInfo().IsDir()
		if !PortablePath(name) || !runtimePath(name, dir) || f.Mode()&fs.ModeSymlink != 0 || !dir && !f.Mode().IsRegular() || f.Mode().Perm()&0o111 != 0 && !dir {
			return nil, fmt.Errorf("unsafe archive entry %q", f.Name)
		}
		if e := recordSpelling(spellings, name); e != nil {
			return nil, e
		}

		key := strings.ToLower(name)
		if seen[key] {
			return nil, fmt.Errorf("duplicate or case-colliding archive entry %q", name)
		}
		seen[key] = true
		types[key] = dir
		if dir {
			if f.UncompressedSize64 != 0 {
				return nil, fmt.Errorf("nonempty directory %q", name)
			}
			continue
		}
		if f.UncompressedSize64 > uint64(expanded-total) { //nolint:gosec // Total starts at zero and each preceding entry is bounded by the remaining positive budget.
			return nil, errors.New("package expansion exceeds limit")
		}
		size := int64(f.UncompressedSize64) //nolint:gosec // Bounded above by the installation expansion limit minus total.
		total += size
		if e := verifyFile(ctx, f, size); e != nil {
			return nil, fmt.Errorf("entry %s: %w", name, e)
		}
	}
	// Reject a regular file that also acts as another entry's directory, including
	// case-folded paths on Windows/macOS. Implicit directories need no ZIP entries.
	for name := range types {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if dir, ok := types[parent]; ok && !dir {
				return nil, fmt.Errorf("file/directory collision at %s", parent)
			}
		}
	}
	if total != r.UnpackedSize {
		return nil, errors.New("package unpacked size mismatch")
	}
	if dir, ok := types["game.yaml"]; !ok || dir {
		return nil, errors.New("package requires root game.yaml")
	}
	if e := verifyIcon(z, r); e != nil {
		return nil, e
	}
	return &Installed{Release: r, FS: z}, nil
}

func verifyFile(ctx context.Context, f *zip.File, size int64) error {
	rd, e := f.Open()
	if e != nil {
		return e
	}
	n, e := io.Copy(io.Discard, io.LimitReader(contextReader{ctx, rd}, size+1))
	e = errors.Join(e, rd.Close())
	if e != nil {
		return e
	}
	if n != size {
		return errors.New("entry size mismatch")
	}
	return nil
}

func verifyIcon(fsys fs.FS, r Release) error {
	f, e := fsys.Open(r.Icon)
	if e != nil {
		return fmt.Errorf("package icon: %w", e)
	}
	b, e := readBounded(f, 64<<10)
	e = errors.Join(e, f.Close())
	if e != nil {
		return fmt.Errorf("package icon: %w", e)
	}
	expected, e := r.IconBytes()
	if e != nil {
		return e
	}
	if !bytes.Equal(b, expected) {
		return errors.New("archive icon differs from reviewed catalog icon")
	}
	return nil
}

func validatePNG(b []byte) error {
	cfg, e := png.DecodeConfig(bytes.NewReader(b))
	if e != nil {
		return fmt.Errorf("package icon PNG: %w", e)
	}
	if cfg.Width != cfg.Height || cfg.Width < 32 || cfg.Width > 1024 {
		return errors.New("package icon must be square, 32 to 1024 pixels")
	}
	// Reject APNG animation chunks even though image/png accepts its first frame.
	for pos := 8; pos+12 <= len(b); {
		n := int64(binary.BigEndian.Uint32(b[pos : pos+4]))
		if n > int64(len(b)-pos-12) {
			return errors.New("truncated icon PNG")
		}
		kind := string(b[pos+4 : pos+8])
		if kind == "acTL" || kind == "fcTL" || kind == "fdAT" {
			return errors.New("icon must be a static PNG")
		}
		pos += int(n) + 12
	}
	if _, e = png.Decode(bytes.NewReader(b)); e != nil {
		return fmt.Errorf("package icon PNG: %w", e)
	}
	return nil
}

func readBounded(r io.Reader, n int64) ([]byte, error) {
	b, e := io.ReadAll(io.LimitReader(r, n+1))
	if e != nil {
		return nil, e
	}
	if int64(len(b)) > n {
		return nil, errors.New("size limit exceeded")
	}
	return b, nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.r.Read(b)
}

// PortablePath validates an archive or host-relative package path for all supported OSes.
// It rejects hidden components, reserved names and paths outside the supplied FS.
func PortablePath(name string) bool {
	if name == "." || !fs.ValidPath(name) || strings.ContainsAny(name, "\\:") || strings.Count(name, "/") > 16 {
		return false
	}
	for p := range strings.SplitSeq(name, "/") {
		if strings.HasPrefix(p, ".") || strings.TrimRight(p, ". ") != p {
			return false
		}
		base := strings.ToUpper(strings.SplitN(p, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
			return false
		}
		for _, r := range p {
			if unicode.IsControl(r) || strings.ContainsRune(`<>"|?*`, r) {
				return false
			}
		}
	}
	return true
}

func runtimePath(name string, dir bool) bool {
	top, _, nested := strings.Cut(name, "/")
	switch top {
	case "data", "lang", "streams":
		return dir || strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
	case "assets":
		if dir {
			return true
		}
		switch strings.ToLower(path.Ext(name)) {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".ogg", ".wav", ".mp3", ".ttf", ".otf", ".ttc", ".yaml", ".yml", ".txt", ".md":
			return true
		case ".json":
			return name == "assets/faces.json"
		}
	case "LICENSES":
		return dir || strings.HasSuffix(name, ".txt") || strings.HasSuffix(name, ".md") || strings.HasPrefix(path.Base(name), "LICENSE") || strings.HasPrefix(path.Base(name), "NOTICE")
	default:
		return !nested && !dir && (name == "game.yaml" || name == "LICENSE" || name == "LICENSE.txt" || name == "LICENSE.md" || name == "NOTICE" || name == "NOTICE.txt" || name == "ASSETS.md")
	}
	return false
}

func recordSpelling(spellings map[string]string, name string) error {
	for prefix := name; prefix != "."; prefix = path.Dir(prefix) {
		key := strings.ToLower(prefix)
		if old, ok := spellings[key]; ok && old != prefix {
			return fmt.Errorf("case-colliding archive path %q and %q", old, prefix)
		}
		spellings[key] = prefix
	}
	return nil
}

// directoryBounds caps central-directory allocation before archive/zip parses it.
// Under the supported disk limits ZIP64 is unnecessary and deliberately rejected.
func directoryBounds(reader io.ReaderAt, size int64, entries int) error {
	if size < 22 {
		return errors.New("truncated ZIP directory")
	}
	n := min(size, int64(22+65535))
	tail := make([]byte, n)
	if _, e := reader.ReadAt(tail, size-n); e != nil {
		return e
	}
	for end := len(tail) - 22; end >= 0; end-- {
		if !bytes.Equal(tail[end:end+4], []byte{'P', 'K', 5, 6}) {
			continue
		}
		record := tail[end:]
		if int(binary.LittleEndian.Uint16(record[20:22]))+22 != len(record) {
			continue
		}
		count := int(binary.LittleEndian.Uint16(record[10:12]))
		if count > entries || count == 65535 {
			return errors.New("too many archive entries or unsupported ZIP64")
		}
		if binary.LittleEndian.Uint16(record[4:6]) != 0 || binary.LittleEndian.Uint16(record[6:8]) != 0 || int(binary.LittleEndian.Uint16(record[8:10])) != count {
			return errors.New("multi-volume ZIP is unsupported")
		}
		dirSize := int64(binary.LittleEndian.Uint32(record[12:16]))
		offset := int64(binary.LittleEndian.Uint32(record[16:20]))
		if dirSize > 16<<20 || offset+dirSize > size {
			return errors.New("ZIP directory exceeds limit")
		}
		return nil
	}
	return errors.New("missing ZIP directory")
}
