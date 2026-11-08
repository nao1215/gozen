package play

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"io/fs"
	"maps"
	"regexp"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/clihelp"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer2/gamepackage"
)

// Development builds target the next release's package contract. Release builds
// always use their actual version; unknown versions do not bypass constraints.
const phoneDevelopmentVersion = "0.23.0"

var phonePseudoVersion = regexp.MustCompile(`^(\d+\.\d+\.\d+)-(?:0\.)?(\d{14})-[0-9a-f]{12}$`)

func phoneEngineVersion() string {
	return phoneEngineTarget(clihelp.Version())
}

func phoneEngineTarget(version string) string {
	v := strings.TrimPrefix(version, "v")
	if v == "devel" || v == "(devel)" {
		return phoneDevelopmentVersion
	}
	base := strings.TrimSuffix(v, "+dirty")
	if valid, err := gamepackage.Allows(base, base); base != v && err == nil && valid {
		return phoneDevelopmentVersion
	}
	if parts := phonePseudoVersion.FindStringSubmatch(base); parts != nil {
		valid, versionErr := gamepackage.Allows(parts[1], parts[1])
		if _, err := time.Parse("20060102150405", parts[2]); err == nil && versionErr == nil && valid {
			return phoneDevelopmentVersion
		}
	}
	return v
}

type phonePackages struct {
	catalog      gamepackage.Catalog
	installer    gamepackage.Installer
	selected     map[string]gamepackage.Release
	opened       map[string]*gamepackage.Installed
	iconReleases map[string]gamepackage.Release
}

func (g *Game) preparePhonePackages() error {
	if g.packages != nil {
		return nil
	}
	cfg := g.db.Game.Phone
	if cfg == nil {
		return errors.New("this game has no phone apps")
	}
	f, err := g.opts.FS.Open(cfg.Catalog)
	if err != nil {
		return err
	}
	b, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	catalog, err := gamepackage.Authenticate(b, cfg.CatalogSHA256)
	if err != nil {
		return err
	}
	p := &phonePackages{catalog: catalog, selected: map[string]gamepackage.Release{}, opened: map[string]*gamepackage.Installed{}}
	archiveStore, err := defaultPhoneArchiveStore(g)
	if err != nil {
		return err
	}
	p.installer = gamepackage.Installer{Fetcher: phoneFetcher{}, Browser: phoneBrowser, Store: archiveStore, Engine: phoneEngineVersion(), Validate: func(fsys fs.FS) error { _, e := data.Load(fsys); return e }}
	// Selection is a lock, not a request to upgrade whenever the app is opened.
	if b, e := g.store.Read("phone-apps.json"); e == nil {
		if e = json.Unmarshal(b, &p.selected); e != nil {
			return fmt.Errorf("phone app selections: %w", e)
		}
	} else if !errors.Is(e, fs.ErrNotExist) {
		return e
	}
	if b, e := g.store.Read("phone-catalog.json"); e == nil {
		var saved phoneCatalogSnapshot
		if e = json.Unmarshal(b, &saved); e != nil {
			return e
		}
		updated, e := gamepackage.AuthenticateSigned(saved.Data, saved.Signature, cfg.CatalogPublicKey)
		if e != nil {
			return e
		}
		if e = gamepackage.ValidateUpdate(catalog, updated); e != nil {
			return e
		}
		p.catalog = updated
	} else if !errors.Is(e, fs.ErrNotExist) {
		return e
	}
	g.packages = p
	return nil
}

func (g *Game) phoneApp(id string) (data.PhoneApp, error) {
	if g.db.Game.Phone != nil {
		for _, a := range g.db.Game.Phone.Apps {
			if a.ID == id {
				if a.When.Set() {
					if g.st == nil {
						return a, errors.New("phone apps require an active game state")
					}
					ok, e := a.When.Bool(g.st.Env())
					if e != nil {
						return a, e
					}
					if !ok {
						return a, errors.New("app is locked")
					}
				}
				return a, nil
			}
		}
	}
	return data.PhoneApp{}, fmt.Errorf("unknown phone app %q", id)
}

// phoneListed resolves reviewed metadata without opening an archive. Listing a
// home screen or store must not verify or retain every game's assets.
func (g *Game) phoneListed(id string) (gamepackage.Release, error) {
	if err := g.preparePhonePackages(); err != nil {
		return gamepackage.Release{}, err
	}
	a, err := g.phoneApp(id)
	if err != nil {
		return gamepackage.Release{}, err
	}
	p := g.packages
	if inst := p.opened[id]; inst != nil {
		return inst.Release, nil
	}
	if r, ok := p.selected[id]; ok {
		approved, e := p.catalog.Resolve(a.Package, r.Version, phoneEngineVersion())
		if e != nil {
			return r, e
		}
		if approved.SHA256 != r.SHA256 {
			return r, errors.New("installed app differs from approved release")
		}
		allowed := gamepackage.Catalog{Schema: 1, Releases: []gamepackage.Release{approved}}
		return allowed.Resolve(a.Package, a.Version, phoneEngineVersion())
	}
	if a.Install != "bundled" {
		return gamepackage.Release{}, fs.ErrNotExist
	}
	version := a.BundledVersion
	if version == "" {
		version = a.Version
	}
	return p.catalog.Resolve(a.Package, version, phoneEngineVersion())
}

func (g *Game) phoneInstalled(id string) (*gamepackage.Installed, error) {
	r, err := g.phoneListed(id)
	if err != nil {
		return nil, err
	}
	p := g.packages
	if inst := p.opened[id]; inst != nil {
		return inst, nil
	}
	if _, selected := p.selected[id]; selected {
		inst, e := p.installer.Open(g.context, r)
		if e == nil {
			p.opened[id] = inst
		}
		return inst, e
	}
	a, err := g.phoneApp(id)
	if err != nil {
		return nil, err
	}
	inst, err := openPhoneArchive(g.context, g.opts.FS, a.Path, r)
	if err != nil {
		return nil, err
	}
	if err = p.installer.Validate(inst.FS); err != nil {
		return nil, errors.Join(err, inst.Close())
	}
	p.opened[id] = inst
	return inst, nil
}

func (g *Game) selectPhonePackage(id string, inst *gamepackage.Installed) error {
	next := make(map[string]gamepackage.Release, len(g.packages.selected)+1)
	maps.Copy(next, g.packages.selected)
	next[id] = inst.Release
	b, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if err = g.store.Write("phone-apps.json", b); err != nil {
		return err
	}
	old := g.packages.opened[id]
	g.packages.selected = next
	clear(g.packages.iconReleases)
	delete(g.packages.opened, id)
	var errClose error
	if old != nil && old != inst {
		errClose = old.Close()
	}
	return errors.Join(errClose, inst.Close())
}

// Cache payloads are base64 because the browser save adapter stores UTF-8 text.
// Digest names are immutable; selection is published only after a successful save.
type phoneArchiveStore struct{ store Store }

func (s phoneArchiveStore) Load(ctx context.Context, digest string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b, err := s.store.Read("phone-package-" + digest + ".b64")
	if err != nil {
		return nil, err
	}
	if len(b) > base64.StdEncoding.EncodedLen(int(gamepackage.MaxArchiveBytes)) {
		return nil, errors.New("cached package exceeds limit")
	}
	return base64.StdEncoding.DecodeString(string(b))
}

func (s phoneArchiveStore) Save(ctx context.Context, digest string, b []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != digest {
		return errors.New("cache digest mismatch")
	}
	if int64(len(b)) > gamepackage.MaxArchiveBytes {
		return errors.New("package exceeds cache limit")
	}
	return s.store.Write("phone-package-"+digest+".b64", []byte(base64.StdEncoding.EncodeToString(b)))
}

type phoneCatalogSnapshot struct {
	Data      []byte `json:"data"`
	Signature string `json:"signature"`
}

func (g *Game) acceptPhoneCatalog(snapshot gamepackage.SignedCatalog) error {
	if err := gamepackage.ValidateUpdate(g.packages.catalog, snapshot.Catalog); err != nil {
		return err
	}
	b, err := json.Marshal(phoneCatalogSnapshot{snapshot.Data, snapshot.Signature})
	if err != nil {
		return err
	}
	if err = g.store.Write("phone-catalog.json", b); err != nil {
		return err
	}
	g.packages.catalog = snapshot.Catalog
	clear(g.packages.iconReleases)
	return nil
}

// phoneIconRelease caches reviewed metadata between catalog/selection changes.
// Drawing an existing GPU icon must not revalidate and decode every catalog PNG.
func (g *Game) phoneIconRelease(key string) (gamepackage.Release, error) {
	if g.packages == nil {
		return gamepackage.Release{}, fs.ErrNotExist
	}
	id, installed := strings.CutPrefix(key, "installed:")
	if !installed {
		id = strings.TrimPrefix(key, "package:")
	}
	app, err := g.phoneApp(id)
	if err != nil {
		return gamepackage.Release{}, err
	}
	if release, ok := g.packages.iconReleases[key]; ok {
		return release, nil
	}
	var release gamepackage.Release
	if installed {
		release, err = g.phoneListed(id)
	} else {
		release, err = g.packages.catalog.Resolve(app.Package, app.Version, phoneEngineVersion())
	}
	if err != nil {
		return gamepackage.Release{}, err
	}
	if g.packages.iconReleases == nil {
		g.packages.iconReleases = map[string]gamepackage.Release{}
	}
	g.packages.iconReleases[key] = release
	return release, nil
}

// phonePackageIcon decodes reviewed catalog bytes only when sizing a new icon.
// The caller owns and releases the temporary image.

func (g *Game) phonePackageIcon(key string) *ebiten.Image {
	r, err := g.phoneIconRelease(key)
	if err != nil {
		return nil
	}
	b, err := r.IconBytes()
	if err != nil {
		return nil
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return nil
	}
	return ebiten.NewImageFromImage(img)
}

func (g *Game) closePhonePackages() error {
	if g.packages == nil {
		return nil
	}
	var err error
	for _, inst := range g.packages.opened {
		err = errors.Join(err, inst.Close())
	}
	g.packages = nil
	return err
}

func (g *Game) releasePhonePackage(id string) error {
	if g.packages == nil {
		return nil
	}
	inst := g.packages.opened[id]
	delete(g.packages.opened, id)
	if inst == nil {
		return nil
	}
	return inst.Close()
}

// tracePhoneArchiveNames adds virtual native-cache entries to replay snapshots.
// The player store is never rewritten just to record a trace.
func (g *Game) tracePhoneArchiveNames() ([]string, error) {
	if g.db.Game.Phone == nil {
		return nil, nil
	}
	if e := g.preparePhonePackages(); e != nil {
		return nil, e
	}
	names := make([]string, 0, len(g.packages.selected))
	for _, r := range g.packages.selected {
		if e := r.Validate(); e != nil {
			return nil, e
		}
		names = append(names, "phone-package-"+r.SHA256+".b64")
	}
	return names, nil
}

func (g *Game) traceSaveRead(name string) ([]byte, error) {
	b, e := g.storeRead(name)
	if !errors.Is(e, fs.ErrNotExist) || g.packages == nil {
		return b, e
	}
	sum, ok := strings.CutPrefix(name, "phone-package-")
	if !ok {
		return nil, e
	}
	sum, ok = strings.CutSuffix(sum, ".b64")
	if !ok {
		return nil, e
	}
	var store gamepackage.FileStore
	switch s := g.packages.installer.Store.(type) {
	case gamepackage.FileStore:
		store = s
	case *gamepackage.FileStore:
		if s == nil {
			return nil, e
		}
		store = *s
	default:
		return nil, e
	}
	// Standard base64 consumes four bytes for each three source bytes.
	b, e = store.Snapshot(g.context, sum, 3*(initialSaveLimit/4))
	if e != nil {
		return nil, e
	}
	return []byte(base64.StdEncoding.EncodeToString(b)), nil
}
