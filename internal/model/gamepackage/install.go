package gamepackage

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/nao1215/gozen/internal/base/savefile"
)

// Fetcher is supplied by desktop/browser startup. Fetch must enforce maxBytes
// while reading, honor cancellation, and reject redirects away from HTTPS.
// Returned bytes belong to the caller. No fetch happens during catalog loading.
type Fetcher interface {
	Fetch(ctx context.Context, url string, maxBytes int64) ([]byte, error)
}

// Store persists verified archives by digest. Save publishes bytes atomically or
// leaves existing content intact. Load returns caller-owned bytes, bounded by
// MaxArchiveBytes; a missing digest is fs.ErrNotExist. Implementations must allow
// simultaneous reads/installs. Browser startup may supply IndexedDB storage.
type Store interface {
	Load(ctx context.Context, digest string) ([]byte, error)
	Save(ctx context.Context, digest string, b []byte) error
}

// Installer is owned by the phone store. Engine is an explicit stable Gozen
// version, never an unconstrained "devel". Validate checks game definitions on
// the verified FS before publication; callers normally supply gamedata.Load.
// Calls are synchronous, cancellable and safe to schedule off the game thread;
// collaborators must not mutate presentation state. Nil collaborators fail when
// their capability is requested. Existing archives and active handles never change.
type Installer struct {
	Fetcher Fetcher
	Store   Store
	Engine  string
	// Browser selects the reviewed direct CORS mirror for new downloads.
	Browser  bool
	Validate func(fs.FS) error
}

func (i Installer) compatible(r Release) error {
	if i.Browser && (r.Size > MaxArchiveBytes || r.UnpackedSize > MaxUnpackedBytes) {
		return errors.New("package exceeds memory/browser limits")
	}
	if e := r.Validate(); e != nil {
		return e
	}
	allowed, e := Allows(r.Engine, i.Engine)
	if e != nil {
		return e
	}
	if !allowed {
		return errors.New("package is incompatible with this engine")
	}
	if i.Validate == nil {
		return errors.New("game definition validator is required")
	}
	return nil
}

func (i Installer) validate(ctx context.Context, game *Installed) (*Installed, error) {
	if e := i.Validate(game.FS); e != nil {
		return nil, errors.Join(fmt.Errorf("package game: %w", e), game.Close())
	}
	if e := ctx.Err(); e != nil {
		return nil, errors.Join(e, game.Close())
	}
	return game, nil
}

func (i Installer) verify(ctx context.Context, r Release, b []byte) (*Installed, error) {
	if e := i.compatible(r); e != nil {
		return nil, e
	}
	game, e := verify(ctx, r, b)
	if e != nil {
		return nil, e
	}
	return i.validate(ctx, game)
}

// Open verifies an already installed release and never accesses the network.
// Desktop FileStore archives are opened through ReaderAt, without cloning bytes.
func (i Installer) Open(ctx context.Context, r Release) (*Installed, error) {
	if i.Store == nil {
		return nil, errors.New("package storage is unavailable")
	}
	if e := i.compatible(r); e != nil {
		return nil, e
	}
	if store, ok := fileStore(i.Store); ok {
		name, e := store.name(r.SHA256)
		if e != nil {
			return nil, e
		}
		game, e := OpenArchive(ctx, cacheFS(filepath.Dir(name)), filepath.Base(name), r)
		if e != nil {
			return nil, e
		}
		return i.validate(ctx, game)
	}
	if r.Size > MaxArchiveBytes || r.UnpackedSize > MaxUnpackedBytes {
		return nil, errors.New("package exceeds memory/browser limits")
	}
	b, e := i.Store.Load(ctx, r.SHA256)
	if e != nil {
		return nil, e
	}
	return i.verify(ctx, r, b)
}

// Install explicitly downloads a selected approved release, validates it and
// publishes only after success. A retry can reuse a complete verified archive;
// interrupted bytes are discarded. Updating installs a different digest and
// leaves earlier installations and their saved progress intact.
func (i Installer) Install(ctx context.Context, r Release) (*Installed, error) {
	if i.Store == nil || i.Fetcher == nil {
		return nil, errors.New("package installation is unavailable")
	}
	if e := r.Validate(); e != nil {
		return nil, e
	}
	allowed, e := Allows(r.Engine, i.Engine)
	if e != nil {
		return nil, e
	}
	if !allowed {
		return nil, errors.New("package is incompatible with this engine")
	}
	game, e := i.Open(ctx, r)
	if e == nil {
		return game, nil
	}
	if !errors.Is(e, fs.ErrNotExist) {
		return nil, e
	}
	fetchURL := r.URL
	if i.Browser {
		if r.BrowserURL == "" {
			return nil, errors.New("this release has no approved browser download")
		}
		fetchURL = r.BrowserURL
	}
	if store, ok := fileStore(i.Store); ok && !i.Browser {
		if fetcher, ok := i.Fetcher.(StreamingFetcher); ok {
			return i.installStream(ctx, store, fetcher, r)
		}
	}
	if r.Size > MaxArchiveBytes || r.UnpackedSize > MaxUnpackedBytes {
		return nil, errors.New("package requires a streaming disk installer")
	}
	b, e := i.Fetcher.Fetch(ctx, fetchURL, r.Size)
	if e != nil {
		return nil, e
	}
	game, e = i.verify(ctx, r, b)
	if e != nil {
		return nil, e
	}
	if e = i.Store.Save(ctx, r.SHA256, b); e != nil {
		return nil, e
	}
	return game, nil
}

// FileStore is the desktop startup adapter. Root is an application-owned cache
// directory, never a game-provided path. An empty root is invalid. Each digest
// is immutable; publication uses an exclusive temporary file and same-directory
// hard-link publication. Filesystems without hard-link support return an error;
// existing cache files are never replaced. Only bounded verified archives are accepted.
type FileStore struct{ Root string }

func (s FileStore) name(sum string) (string, error) {
	if s.Root == "" || !digest(sum) {
		return "", fs.ErrInvalid
	}
	return filepath.Join(s.Root, sum+".zip"), nil
}

// cacheFS gives retained cache readers platform sharing compatible with deleting
// staging hard links after publication. It is used only under a caller-owned root.
type cacheFS string

func (root cacheFS) Open(name string) (fs.File, error) {
	if !PortablePath(name) {
		return nil, fs.ErrInvalid
	}
	return openCacheFile(filepath.Join(string(root), filepath.FromSlash(name)))
}

// Load returns a bounded copy of a cached ZIP and honors cancellation.
func (s FileStore) Load(ctx context.Context, sum string) ([]byte, error) {
	return s.Snapshot(ctx, sum, MaxArchiveBytes)
}

// ErrSizeLimit reports that an archive cannot fit the caller's bounded snapshot.
var ErrSizeLimit = errors.New("package exceeds snapshot byte limit")

// Snapshot reads cached bytes for a bounded consumer such as an exact-replay
// initial save. It checks the actual file size before allocating archive bytes.
func (s FileStore) Snapshot(ctx context.Context, sum string, maxBytes int64) ([]byte, error) {
	if maxBytes < 0 || maxBytes > MaxStoredArchiveBytes {
		return nil, fs.ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	name, e := s.name(sum)
	if e != nil {
		return nil, e
	}
	f, e := openCacheFile(name)
	if e != nil {
		return nil, e
	}
	info, e := f.Stat()
	if e != nil {
		return nil, errors.Join(e, f.Close())
	}
	if info.Size() > maxBytes {
		return nil, errors.Join(ErrSizeLimit, f.Close())
	}
	b, e := readBounded(contextReader{ctx, f}, maxBytes)
	return b, errors.Join(e, f.Close())
}

// Save publishes a checksum-verified archive without modifying earlier releases.
// Cancellation before promotion leaves the destination untouched.
func (s FileStore) Save(ctx context.Context, sum string, b []byte) (err error) {
	name, e := s.name(sum)
	if e != nil {
		return e
	}
	if int64(len(b)) > MaxArchiveBytes || checksum(b) != sum {
		return errors.New("invalid cached archive")
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	if e = os.MkdirAll(s.Root, 0o700); e != nil {
		return e
	}
	old, e := s.Load(ctx, sum)
	if e == nil {
		if checksum(old) != sum {
			return errors.New("existing package cache is corrupt")
		}
		return nil
	}
	if !errors.Is(e, fs.ErrNotExist) {
		return e
	}
	temp, e := savefile.Stage(s.Root, ".install-", b)
	if e != nil {
		return e
	}
	defer func() { err = errors.Join(err, savefile.Unstage(temp)) }()
	if e = ctx.Err(); e != nil {
		return e
	}
	// Publish without replacing another installer's immutable file or open handle.
	// The deferred cleanup removes the staging link after successful publication.
	if e = os.Link(temp, name); e != nil {
		winner, loadErr := s.Load(ctx, sum)
		if loadErr == nil && checksum(winner) == sum {
			return nil
		}
		return e
	}
	return nil
}

// SignedCatalog retains the exact authenticated envelope for atomic offline
// storage. Callers must authenticate Data and Signature again when restoring it.
type SignedCatalog struct {
	Catalog   Catalog
	Data      []byte
	Signature string
}

// RefreshSignedCatalog explicitly fetches an approved catalog and its detached
// signature. The host supplies the trusted public key; downloaded metadata never
// supplies a trust anchor. Call ValidateUpdate before replacing a prior snapshot.
func (i Installer) RefreshSignedCatalog(ctx context.Context, url, signatureURL, publicKey string) (SignedCatalog, error) {
	if i.Fetcher == nil || !validURL(url) || !validURL(signatureURL) {
		return SignedCatalog{}, errors.New("signed catalog refresh requires HTTPS")
	}
	key, e := base64.StdEncoding.Strict().DecodeString(publicKey)
	if e != nil || len(key) != ed25519.PublicKeySize {
		return SignedCatalog{}, errors.New("invalid trusted catalog public key")
	}
	b, e := i.Fetcher.Fetch(ctx, url, maxCatalogBytes)
	if e != nil {
		return SignedCatalog{}, e
	}
	sig, e := i.Fetcher.Fetch(ctx, signatureURL, 256)
	if e != nil {
		return SignedCatalog{}, e
	}
	if e = ctx.Err(); e != nil {
		return SignedCatalog{}, e
	}
	c, e := AuthenticateSigned(b, string(sig), publicKey)
	if e != nil {
		return SignedCatalog{}, e
	}
	return SignedCatalog{Catalog: c, Data: b, Signature: string(sig)}, nil
}

// StreamingFetcher is the optional desktop transfer capability. FetchTo writes
// at most maxBytes to dst, observes cancellation, and enforces the same HTTPS
// redirect policy as Fetcher. It must not close dst. FileStore installs select
// this capability to avoid retaining whole archives in memory.
type StreamingFetcher interface {
	FetchTo(ctx context.Context, url string, maxBytes int64, dst io.Writer) error
}

func fileStore(store Store) (FileStore, bool) {
	switch s := store.(type) {
	case FileStore:
		return s, true
	case *FileStore:
		if s != nil {
			return *s, true
		}
	}
	return FileStore{}, false
}

func (i Installer) installStream(ctx context.Context, store FileStore, fetcher StreamingFetcher, r Release) (game *Installed, err error) {
	name, e := store.name(r.SHA256)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(store.Root, 0o700); e != nil {
		return nil, e
	}
	f, e := os.CreateTemp(store.Root, ".install-")
	if e != nil {
		return nil, e
	}
	temp := f.Name()
	closed := false
	defer func() {
		if !closed {
			err = errors.Join(err, f.Close())
		}
		if e := os.Remove(temp); e != nil && !errors.Is(e, fs.ErrNotExist) {
			err = errors.Join(err, e)
		}
		if err != nil && game != nil {
			err = errors.Join(err, game.Close())
			game = nil
		}
	}()
	dst := &limitedWriter{dst: f, left: r.Size}
	if e = fetcher.FetchTo(ctx, r.URL, r.Size, dst); e != nil {
		return nil, e
	}
	if dst.left != 0 {
		return nil, errors.New("incomplete package download")
	}
	if e = f.Sync(); e != nil {
		return nil, e
	}
	game, e = verifyReader(ctx, r, f, r.Size, MaxStoredUnpackedBytes, maxStoredEntries)
	if e != nil {
		return nil, e
	}
	if _, e = i.validate(ctx, game); e != nil {
		return nil, e
	}
	// Close the validated staging file before publishing its immutable name.
	// A hard link creates only an absent destination; concurrent installs never
	// replace each other's open files. Unsupported filesystems fail explicitly.
	e = f.Close()
	closed = true
	if e != nil {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	if e = os.Link(temp, name); e != nil {
		// A concurrent identical winner may already own the destination.
		winner, openErr := i.Open(ctx, r)
		if openErr == nil {
			return winner, nil
		}
		return nil, errors.Join(e, openErr)
	}
	return i.Open(ctx, r)
}

type limitedWriter struct {
	dst  io.Writer
	left int64
}

func (w *limitedWriter) Write(b []byte) (int, error) {
	if int64(len(b)) > w.left {
		return 0, errors.New("package download exceeds declared size")
	}
	n, e := w.dst.Write(b)
	w.left -= int64(n)
	return n, e
}
