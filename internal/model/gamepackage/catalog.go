// Package gamepackage validates approved, immutable game archives for phone apps.
// It belongs to layer 2; callers supply network and storage capabilities explicitly.
package gamepackage

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/base/decodeguard"
)

// Catalog is a reviewed snapshot consumed by the phone store and export tooling.
// Schema must be 1. Each identity/version pair is unique.
type Catalog struct {
	Schema   int       `yaml:"schema" json:"schema"`
	Releases []Release `yaml:"releases" json:"releases"`
}

// Release names the exact bytes and presentation metadata of one approved game.
// Size is the ZIP size; UnpackedSize is its exact total regular-file size. Icon
// names a square PNG within the archive. Engine is a stable version constraint.
type Release struct {
	ID           string `yaml:"id" json:"id"`
	Version      string `yaml:"version" json:"version"`
	Title        string `yaml:"title" json:"title"`
	Author       string `yaml:"author" json:"author"`
	Description  string `yaml:"description" json:"description"`
	Icon         string `yaml:"icon" json:"icon"`
	IconData     string `yaml:"icon_data" json:"icon_data"`
	License      string `yaml:"license" json:"license"`
	Orientation  string `yaml:"orientation" json:"orientation"`
	URL          string `yaml:"url" json:"url"`
	BrowserURL   string `yaml:"browser_url,omitempty" json:"browser_url,omitempty"`
	SHA256       string `yaml:"sha256" json:"sha256"`
	Size         int64  `yaml:"size" json:"size"`
	UnpackedSize int64  `yaml:"unpacked_size" json:"unpacked_size"`
	Engine       string `yaml:"engine" json:"engine"`
}

const maxCatalogBytes = 1 << 20

// Parse validates a catalog supplied by an already trusted source. For a network
// snapshot, Authenticate must check its caller-owned trust anchor first.
func Parse(b []byte) (Catalog, error) {
	var c Catalog
	if len(b) > maxCatalogBytes {
		return c, errors.New("catalog exceeds size limit")
	}
	if err := decodeguard.Run(b, func() error { return yaml.UnmarshalWithOptions(b, &c, yaml.Strict()) }); err != nil {
		return Catalog{}, fmt.Errorf("catalog: %w", err)
	}
	if c.Schema != 1 {
		return Catalog{}, errors.New("unsupported catalog schema")
	}
	seen := map[string]bool{}
	for _, r := range c.Releases {
		if err := r.Validate(); err != nil {
			return Catalog{}, fmt.Errorf("release %s@%s: %w", r.ID, r.Version, err)
		}
		key := r.ID + "@" + r.Version
		if seen[key] {
			return Catalog{}, fmt.Errorf("duplicate release %s", key)
		}
		seen[key] = true
	}
	return c, nil
}

// Authenticate checks bytes against an independently trusted snapshot digest.
func Authenticate(b []byte, expectedSHA256 string) (Catalog, error) {
	if !digest(expectedSHA256) || checksum(b) != expectedSHA256 {
		return Catalog{}, errors.New("catalog checksum mismatch")
	}
	return Parse(b)
}

// Validate checks release metadata without loading its archive.
func (r Release) Validate() error {
	if !ValidID(r.ID) {
		return errors.New("invalid namespaced package id")
	}
	if _, err := parseVersion(r.Version); err != nil {
		return err
	}
	if _, err := parseConstraint(r.Engine); err != nil {
		return fmt.Errorf("engine: %w", err)
	}
	if r.Title == "" || r.Author == "" || r.Description == "" || r.License == "" {
		return errors.New("title, author, description and license are required")
	}
	if r.Orientation != "portrait" && r.Orientation != "landscape" {
		return errors.New("orientation must be portrait or landscape")
	}
	if !PortablePath(r.Icon) || !strings.HasSuffix(r.Icon, ".png") {
		return errors.New("icon must name a portable PNG path")
	}
	if _, err := r.IconBytes(); err != nil {
		return err
	}
	if !validURL(r.URL) {
		return errors.New("package URL must use HTTPS without credentials or fragment")
	}
	if r.BrowserURL != "" && (!validBrowserURL(r.BrowserURL) || r.Size > MaxArchiveBytes || r.UnpackedSize > MaxUnpackedBytes) {
		return errors.New("browser mirror must use an immutable raw GitHub ZIP within memory limits")
	}
	if !digest(r.SHA256) || r.Size <= 0 || r.Size > MaxStoredArchiveBytes || r.UnpackedSize <= 0 || r.UnpackedSize > MaxStoredUnpackedBytes {
		return errors.New("invalid checksum or package sizes")
	}
	return nil
}

// ValidID reports whether id has the portable owner/name shape used in catalogs.
func ValidID(id string) bool {
	parts := strings.Split(id, "/")
	if len(parts) != 2 {
		return false
	}
	for _, p := range parts {
		if p == "" || len(p) > 64 {
			return false
		}
		for _, c := range p {
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' && c != '_' {
				return false
			}
		}
	}
	return true
}

func validURL(s string) bool {
	u, e := url.Parse(s)
	return e == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.Fragment == ""
}

func validBrowserURL(address string) bool {
	u, e := url.Parse(address)
	if e != nil || u.Scheme != "https" || u.Host != "raw.githubusercontent.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) < 4 || !PortablePath(strings.TrimPrefix(u.Path, "/")) || !strings.HasSuffix(u.Path, ".zip") {
		return false
	}
	commit, e := hex.DecodeString(parts[2])
	return e == nil && len(commit) == 20 && strings.ToLower(parts[2]) == parts[2]
}

func digest(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == sha256.Size && strings.ToLower(s) == s
}
func checksum(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

type version [3]uint64

func parseVersion(s string) (version, error) {
	var v version
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return v, fmt.Errorf("invalid stable version %q", s)
	}
	for i, p := range parts {
		if p == "" || len(p) > 1 && p[0] == '0' {
			return v, fmt.Errorf("invalid stable version %q", s)
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return v, fmt.Errorf("invalid stable version %q", s)
			}
		}
		n, e := strconv.ParseUint(p, 10, 32)
		if e != nil {
			return v, fmt.Errorf("invalid stable version %q", s)
		}
		v[i] = n
	}
	return v, nil
}

func compare(a, b version) int {
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

type constraint struct {
	low, high version
	exact     bool
}

func parseConstraint(s string) (constraint, error) {
	if !strings.Contains(s, " ") {
		v, e := parseVersion(s)
		return constraint{low: v, exact: true}, e
	}
	p := strings.Split(s, " ")
	if len(p) != 2 || !strings.HasPrefix(p[0], ">=") || !strings.HasPrefix(p[1], "<") {
		return constraint{}, fmt.Errorf("constraint %q must be exact or >=x.y.z <x.y.z", s)
	}
	lo, e := parseVersion(strings.TrimPrefix(p[0], ">="))
	if e != nil {
		return constraint{}, e
	}
	hi, e := parseVersion(strings.TrimPrefix(p[1], "<"))
	if e != nil {
		return constraint{}, e
	}
	if compare(lo, hi) >= 0 {
		return constraint{}, errors.New("empty version interval")
	}
	return constraint{low: lo, high: hi}, nil
}

func (c constraint) includes(v version) bool {
	if c.exact {
		return compare(v, c.low) == 0
	}
	return compare(v, c.low) >= 0 && compare(v, c.high) < 0
}

// Allows validates the deliberately small stable-version constraint grammar and
// reports whether a release matches it. Unsupported syntax is an error.
func Allows(spec, release string) (bool, error) {
	c, e := parseConstraint(spec)
	if e != nil {
		return false, e
	}
	v, e := parseVersion(release)
	return e == nil && c.includes(v), e
}

// Resolve selects the highest allowed compatible version, independently of
// catalog order. Runtime receives this exact Release rather than a live range.
func (c Catalog) Resolve(id, spec, engine string) (Release, error) {
	wanted, e := parseConstraint(spec)
	if e != nil {
		return Release{}, e
	}
	ev, e := parseVersion(engine)
	if e != nil {
		return Release{}, e
	}
	var best Release
	var bv version
	for _, r := range c.Releases {
		if r.ID != id {
			continue
		}
		if e := r.Validate(); e != nil {
			return Release{}, e
		}
		v, err := parseVersion(r.Version)
		if err != nil {
			return Release{}, err
		}
		ec, err := parseConstraint(r.Engine)
		if err != nil {
			return Release{}, err
		}
		if wanted.includes(v) && ec.includes(ev) && (best.ID == "" || compare(v, bv) > 0) {
			best, bv = r, v
		}
	}
	if best.ID == "" {
		return Release{}, fmt.Errorf("no compatible approved release for %s %s", id, spec)
	}
	return best, nil
}

// IconBytes returns the reviewed icon for a store listing before downloading the
// archive. It is independently bounded and must match the archive icon exactly.
func (r Release) IconBytes() ([]byte, error) {
	if len(r.IconData) > base64.StdEncoding.EncodedLen(64<<10) {
		return nil, errors.New("catalog icon exceeds 64 KiB")
	}
	b, e := base64.StdEncoding.Strict().DecodeString(r.IconData)
	if e != nil {
		return nil, fmt.Errorf("catalog icon base64: %w", e)
	}
	if e = validatePNG(b); e != nil {
		return nil, e
	}
	return b, nil
}

// AuthenticateSigned verifies a detached Ed25519 signature over the exact catalog
// bytes using the host's independently pinned public key. Signature and key use
// standard base64. A signed snapshot is validated just like a bundled catalog.
func AuthenticateSigned(b []byte, signatureBase64, publicKeyBase64 string) (Catalog, error) {
	key, e := base64.StdEncoding.Strict().DecodeString(publicKeyBase64)
	if e != nil || len(key) != ed25519.PublicKeySize {
		return Catalog{}, errors.New("invalid trusted catalog public key")
	}
	sig, e := base64.StdEncoding.Strict().DecodeString(strings.TrimSpace(signatureBase64))
	if e != nil || len(sig) != ed25519.SignatureSize || !ed25519.Verify(key, b, sig) {
		return Catalog{}, errors.New("catalog signature verification failed")
	}
	return Parse(b)
}

// ValidateUpdate prevents a refreshed catalog from removing or changing any
// previously trusted release. New versions are additions; an installed version's
// identity, digest, icon and compatibility never silently change.
func ValidateUpdate(previous, next Catalog) error {
	releases := make(map[string]Release, len(next.Releases))
	for _, r := range next.Releases {
		if e := r.Validate(); e != nil {
			return e
		}
		key := r.ID + "@" + r.Version
		if _, ok := releases[key]; ok {
			return fmt.Errorf("duplicate release %s", key)
		}
		releases[key] = r
	}
	if next.Schema != 1 || previous.Schema != 1 {
		return errors.New("unsupported catalog schema")
	}
	for _, r := range previous.Releases {
		if got, ok := releases[r.ID+"@"+r.Version]; !ok || got != r {
			return fmt.Errorf("catalog update removes or mutates %s@%s", r.ID, r.Version)
		}
	}
	return nil
}
