package gamepackage

import (
	"crypto/ed25519"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

func TestCatalogTrustAndResolution(t *testing.T) {
	r, _ := archiveFixture(t, nil)
	newer := r
	newer.Version = "1.2.0"
	incompatible := r
	incompatible.Version = "1.3.0"
	incompatible.Engine = ">=9.0.0 <10.0.0"
	c := Catalog{Schema: 1, Releases: []Release{incompatible, newer, r}}
	b, e := yaml.Marshal(c)
	if e != nil {
		t.Fatal(e)
	}
	got, e := Authenticate(b, checksum(b))
	if e != nil {
		t.Fatal(e)
	}
	selected, e := got.Resolve(r.ID, ">=1.0.0 <2.0.0", "0.23.0")
	if e != nil || selected.Version != "1.2.0" {
		t.Fatalf("resolve: %+v %v", selected, e)
	}
	selected, e = got.Resolve(r.ID, "1.0.0", "0.23.0")
	if e != nil || selected.Version != "1.0.0" {
		t.Fatalf("pin: %+v %v", selected, e)
	}
	if _, e = got.Resolve(r.ID, "1.0.0", "devel"); e == nil {
		t.Fatal("accepted unknown engine")
	}
	if _, e = got.Resolve(r.ID, "1.3.0", "0.23.0"); e == nil {
		t.Fatal("accepted incompatible release")
	}
	if _, e = Authenticate(append(b, ' '), checksum(b)); e == nil {
		t.Fatal("accepted changed catalog")
	}
	if _, e = Authenticate(b, ""); e == nil {
		t.Fatal("accepted untrusted snapshot")
	}
	c.Releases = append(c.Releases, r)
	b, e = yaml.Marshal(c)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Parse(b); e == nil {
		t.Fatal("accepted duplicate version")
	}
}

func TestCatalogRejectsMalformedMetadata(t *testing.T) {
	r, _ := archiveFixture(t, nil)
	for _, tc := range []struct {
		name string
		edit func(*Release)
	}{
		{"id", func(r *Release) { r.ID = "../escape" }},
		{"version", func(r *Release) { r.Version = "01.0.0" }},
		{"engine", func(r *Release) { r.Engine = "*" }},
		{"URL", func(r *Release) { r.URL = "http://example.com/game.zip" }},
		{"credentials", func(r *Release) { r.URL = "https://user:pass@example.com/game.zip" }},
		{"digest", func(r *Release) { r.SHA256 = "broken" }},
		{"size", func(r *Release) { r.Size = MaxStoredArchiveBytes + 1 }},
		{"expansion", func(r *Release) { r.UnpackedSize = MaxStoredUnpackedBytes + 1 }},
		{"icon", func(r *Release) { r.Icon = "../icon.png" }},
		{"description", func(r *Release) { r.Description = "" }},
		{"orientation", func(r *Release) { r.Orientation = "square" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := r
			tc.edit(&changed)
			if e := changed.Validate(); e == nil {
				t.Fatal("accepted malformed release")
			}
		})
	}
	if _, e := Parse([]byte("schema: 1\nunknown: true\n")); e == nil {
		t.Fatal("unknown catalog field")
	}
	if _, e := Parse([]byte(strings.Repeat(" ", maxCatalogBytes+1))); e == nil {
		t.Fatal("oversized catalog")
	}
}

func TestVersionConstraints(t *testing.T) {
	for _, spec := range []string{"*", "^1.0.0", ">=1.0.0", ">=2.0.0 <1.0.0", ">=1.0.0  <2.0.0", "v1.0.0", "1.0.0-beta", "1.0", "1.0.+0", "4294967296.0.0"} {
		if _, e := Allows(spec, "1.0.0"); e == nil {
			t.Errorf("accepted %q", spec)
		}
	}
	for _, tc := range []struct {
		version string
		want    bool
	}{{"1.0.0", true}, {"1.99.0", true}, {"2.0.0", false}, {"0.99.0", false}} {
		got, e := Allows(">=1.0.0 <2.0.0", tc.version)
		if e != nil || got != tc.want {
			t.Errorf("%s: %v,%v", tc.version, got, e)
		}
	}
}

func TestSignedCatalogAndImmutableUpdates(t *testing.T) {
	public, private, e := ed25519.GenerateKey(nil)
	if e != nil {
		t.Fatal(e)
	}
	r, _ := archiveFixture(t, nil)
	previous := Catalog{Schema: 1, Releases: []Release{r}}
	b, e := yaml.Marshal(previous)
	if e != nil {
		t.Fatal(e)
	}
	key := base64.StdEncoding.EncodeToString(public)
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(private, b))
	if _, e = AuthenticateSigned(b, sig, key); e != nil {
		t.Fatal(e)
	}
	if _, e = AuthenticateSigned(append(b, ' '), sig, key); e == nil {
		t.Fatal("accepted changed signed bytes")
	}
	other, _, e := ed25519.GenerateKey(nil)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = AuthenticateSigned(b, sig, base64.StdEncoding.EncodeToString(other)); e == nil {
		t.Fatal("accepted another key")
	}
	next := r
	next.Version = "1.1.0"
	if e = ValidateUpdate(previous, Catalog{Schema: 1, Releases: []Release{r, next}}); e != nil {
		t.Fatal(e)
	}
	if e = ValidateUpdate(previous, Catalog{Schema: 1, Releases: []Release{next}}); e == nil {
		t.Fatal("accepted rollback/removal")
	}
	changed := r
	changed.Description = "Changed immutable metadata"
	if e = ValidateUpdate(previous, Catalog{Schema: 1, Releases: []Release{changed}}); e == nil {
		t.Fatal("accepted mutated version")
	}
}

func TestBrowserMirrorRequiresImmutableRawURL(t *testing.T) {
	r, _ := archiveFixture(t, nil)
	for _, address := range []string{
		"https://raw.githubusercontent.com/sample/game/main/game.zip",
		"https://raw.githubusercontent.com/sample/game/0000000000000000000000000000000000000000/../game.zip",
		"https://raw.githubusercontent.com/sample/game/0000000000000000000000000000000000000000/game.zip?ref=main",
		"https://github.com/sample/game/releases/download/v1/game.zip",
		"http://raw.githubusercontent.com/sample/game/0000000000000000000000000000000000000000/game.zip",
	} {
		bad := r
		bad.BrowserURL = address
		if e := bad.Validate(); e == nil {
			t.Errorf("accepted browser mirror %q", address)
		}
	}
	r.BrowserURL = "https://raw.githubusercontent.com/sample/game/0000000000000000000000000000000000000000/game.zip"
	if e := r.Validate(); e != nil {
		t.Fatal(e)
	}
	r.Size = MaxArchiveBytes + 1
	if e := r.Validate(); e == nil {
		t.Fatal("advertised oversized browser package")
	}
}
