//go:build !js

package play

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path/filepath"
	"time"

	"github.com/nao1215/gozen/internal/layer2/gamepackage"
	"github.com/nao1215/gozen/internal/savefile"
)

func defaultPhoneArchiveStore(g *Game) (gamepackage.Store, error) {
	if g.opts.SaveDir != "" {
		return gamepackage.FileStore{Root: filepath.Join(g.opts.SaveDir, "phone-packages")}, nil
	}
	if g.storeGiven || g.opts.Store != nil || g.opts.StoreFor != nil {
		return phoneArchiveStore{g.store}, nil
	}
	id := fmt.Sprintf("phone-cache-%x", sha256.Sum256([]byte(g.db.Game.ID)))
	dir, err := savefile.Prepare(id, savefile.Config{})
	if err != nil {
		return nil, err
	}
	return gamepackage.FileStore{Root: dir}, nil
}

type phoneFetcher struct{}

func (f phoneFetcher) Fetch(ctx context.Context, url string, maxBytes int64) ([]byte, error) {
	if maxBytes > gamepackage.MaxArchiveBytes {
		return nil, errors.New("download exceeds memory limit")
	}
	var b bytes.Buffer
	if err := f.FetchTo(ctx, url, maxBytes, &b); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// FetchTo lets native installation stream an archive into its staging file.
// The installer owns publication; a failed request only leaves temporary bytes.
func (phoneFetcher) FetchTo(ctx context.Context, url string, maxBytes int64, dst io.Writer) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}
	if req.URL.Scheme != "https" || req.URL.User != nil {
		return errors.New("package downloads require HTTPS")
	}
	if maxBytes <= 0 || maxBytes > gamepackage.MaxStoredArchiveBytes {
		return errors.New("invalid download limit")
	}
	client := &http.Client{Timeout: 10 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || req.URL.Scheme != "https" || req.URL.User != nil {
			return errors.New("invalid package redirect")
		}
		return nil
	}}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}
	n, err := io.Copy(dst, io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return err
	}
	if n > maxBytes {
		return errors.New("download exceeds limit")
	}
	return nil
}

const phoneBrowser = false

func openPhoneArchive(ctx context.Context, fsys fs.FS, path string, r gamepackage.Release) (*gamepackage.Installed, error) {
	return gamepackage.OpenArchive(ctx, fsys, path, r)
}
