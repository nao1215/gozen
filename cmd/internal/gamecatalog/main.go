// Command gamecatalog validates and signs the reviewed game catalog. It is
// maintainer tooling, not an author-facing gozen subcommand.
package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer2/gamepackage"
)

type (
	source struct {
		ID            string `json:"id"`
		Repository    string `json:"repository"`
		ManifestAsset string `json:"manifest_asset"`
	}
	sources struct {
		Sources []source `json:"sources"`
	}
)

func main() {
	if e := run(context.Background(), os.Args[1:]); e != nil {
		if _, writeErr := fmt.Fprintln(os.Stderr, e); writeErr != nil {
			os.Exit(1)
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return errors.New("usage: gamecatalog check CATALOG SOURCES [PREVIOUS] | sign CATALOG SIGNATURE")
	}
	b, e := os.ReadFile(args[1]) //nolint:gosec // Explicit maintainer-selected catalog path.
	if e != nil {
		return e
	}
	c, e := gamepackage.Parse(b)
	if e != nil {
		return e
	}
	switch args[0] {
	case "check":
		if len(args) != 3 && len(args) != 4 {
			return errors.New("check requires catalog, sources and optional previous catalog")
		}
		cfg, e := readSources(args[2])
		if e != nil {
			return e
		}
		if len(args) == 4 {
			old, e := os.ReadFile(args[3]) //nolint:gosec // Explicit maintainer-selected comparison snapshot.
			if e != nil {
				return e
			}
			prev, e := gamepackage.Parse(old)
			if e != nil {
				return e
			}
			if e = gamepackage.ValidateUpdate(prev, c); e != nil {
				return e
			}
		}
		return check(ctx, c, cfg, verifyRelease)
	case "sign":
		if len(args) != 3 {
			return errors.New("sign requires catalog and output signature")
		}
		trusted, e := os.ReadFile("docs/registry/public-key.txt")
		if e != nil {
			return e
		}
		sig, e := sign(b, os.Getenv("GAME_CATALOG_SIGNING_KEY"), strings.TrimSpace(string(trusted)))
		if e != nil {
			return e
		}
		return os.WriteFile(args[2], []byte(sig+"\n"), 0o600) //nolint:gosec // Explicit maintainer-selected signature output.
	default:
		return errors.New("unknown catalog operation")
	}
}

func readSources(name string) (sources, error) {
	var cfg sources
	b, e := os.ReadFile(name) //nolint:gosec // Explicit maintainer-selected source allowlist.
	if e != nil {
		return cfg, e
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if e = d.Decode(&cfg); e != nil {
		return cfg, e
	}
	return cfg, nil
}

func check(ctx context.Context, c gamepackage.Catalog, cfg sources, verify func(context.Context, gamepackage.Release) error) error {
	allowed := map[string]source{}
	for _, s := range cfg.Sources {
		if !gamepackage.ValidID(s.ID) || !repository(s.Repository) || s.ManifestAsset != "gozen-package.json" {
			return errors.New("invalid registry source")
		}
		if _, ok := allowed[s.ID]; ok {
			return fmt.Errorf("duplicate source %s", s.ID)
		}
		allowed[s.ID] = s
	}
	for _, r := range c.Releases {
		s, ok := allowed[r.ID]
		if !ok {
			return fmt.Errorf("release %s has no approved source", r.ID)
		}
		if r.BrowserURL != "" && !strings.HasPrefix(r.BrowserURL, "https://raw.githubusercontent.com/"+s.Repository+"/") {
			return fmt.Errorf("release %s browser mirror leaves its approved repository", r.ID)
		}
		if !releaseURL(r.URL, s.Repository) {
			return fmt.Errorf("release %s URL leaves its approved repository", r.ID)
		}
		if e := verify(ctx, r); e != nil {
			return fmt.Errorf("%s@%s: %w", r.ID, r.Version, e)
		}
	}
	return nil
}

func repository(s string) bool {
	p := strings.Split(s, "/")
	if len(p) != 2 {
		return false
	}
	for _, part := range p {
		if part == "" || part == "." || part == ".." {
			return false
		}
		for _, r := range part {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' && r != '.' {
				return false
			}
		}
	}
	return true
}

func releaseURL(raw, repo string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return false
	}
	prefix := "/" + repo + "/releases/download/"
	if !strings.HasPrefix(u.Path, prefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, prefix), "/")
	if len(parts) != 2 {
		return false
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func verifyRelease(ctx context.Context, r gamepackage.Release) (err error) {
	dir, e := os.MkdirTemp("", "gozen-catalog-")
	if e != nil {
		return e
	}
	defer func() { err = errors.Join(err, os.RemoveAll(dir)) }()
	name := filepath.Join(dir, "game.zip")
	f, e := os.Create(name) //nolint:gosec // Fixed filename in a newly created private temporary directory.
	if e != nil {
		return e
	}
	downloadErr := fetch(ctx, r.URL, r.Size, f)
	if e = errors.Join(downloadErr, f.Close()); e != nil {
		return e
	}
	game, e := gamepackage.OpenArchive(ctx, os.DirFS(dir), "game.zip", r)
	if e != nil {
		return e
	}
	defer func() { err = errors.Join(err, game.Close()) }()
	if _, e = gamedata.Load(game.FS); e != nil {
		return e
	}
	if r.BrowserURL != "" {
		// Browser metadata is an alternate location for the exact same archive, never
		// a transformed build. Re-verify bytes and the stricter memory entry budget.
		var mirror bytes.Buffer
		if e = fetch(ctx, r.BrowserURL, r.Size, &mirror); e != nil {
			return e
		}
		browser, e := gamepackage.VerifyContext(ctx, r, mirror.Bytes())
		if e != nil {
			return e
		}
		return browser.Close()
	}
	return nil
}

func fetch(ctx context.Context, url string, maxBytes int64, dst io.Writer) (err error) {
	client := http.Client{Timeout: 10 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" || len(via) >= 10 {
			return errors.New("unsafe package redirect")
		}
		return nil
	}}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if e != nil {
		return e
	}
	resp, e := client.Do(req)
	if e != nil {
		return e
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("package HTTP status %d", resp.StatusCode)
	}
	if resp.ContentLength > maxBytes {
		return errors.New("download exceeds limit")
	}
	n, e := io.Copy(dst, io.LimitReader(resp.Body, maxBytes+1))
	if e != nil {
		return e
	}
	if n > maxBytes {
		return errors.New("download exceeds limit")
	}
	return nil
}

func sign(b []byte, seed, publicKey string) (string, error) {
	key, e := base64.StdEncoding.Strict().DecodeString(seed)
	if e != nil || len(key) != ed25519.SeedSize {
		return "", errors.New("GAME_CATALOG_SIGNING_KEY must be a base64 Ed25519 seed")
	}
	private := ed25519.NewKeyFromSeed(key)
	expected, e := base64.StdEncoding.Strict().DecodeString(publicKey)
	if e != nil || len(expected) != ed25519.PublicKeySize || !bytes.Equal(private[ed25519.SeedSize:], expected) {
		return "", errors.New("signing key does not match reviewed registry public key")
	}
	return base64.StdEncoding.EncodeToString(ed25519.Sign(private, b)), nil
}
