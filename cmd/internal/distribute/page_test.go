package distribute

import (
	"bytes"
	"encoding/json"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/testgame"
)

// pageGame writes a small game whose game.yaml ends with more, a 1280x720 JPEG at
// assets/share/card.jpg and a 192x192 PNG at assets/share/icon.png; it returns the game's
// folder and the JPEG.
func pageGame(t *testing.T, more string) (string, []byte) {
	t.Helper()
	src := t.TempDir()
	var card, icon bytes.Buffer
	if err := jpeg.Encode(&card, image.NewRGBA(image.Rect(0, 0, 1280, 720)), nil); err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(&icon, image.NewRGBA(image.Rect(0, 0, 192, 192))); err != nil {
		t.Fatal(err)
	}
	for name, b := range map[string][]byte{
		"game.yaml":             []byte("schema_version: 9\nid: page-test\n" + strings.Replace(testgame.Game, "Camp expedition", "Camp & <expedition>", 1) + more),
		"data/main.yaml":        []byte(testgame.Data),
		"assets/share/card.jpg": card.Bytes(),
		"assets/share/icon.png": icon.Bytes(),
		"assets/x.png":          []byte("x"),
	} {
		if err := cli.WriteNew(filepath.Join(src, filepath.FromSlash(name)), b); err != nil {
			t.Fatal(err)
		}
	}
	return src, card.Bytes()
}

// TestWebPageShareTags: the page is titled with the game's title, and game.yaml's web gives
// it a description and the Open Graph and X card tags, the picture copied beside the page as
// it is (not made WebP) and named by its absolute address with its size.
func TestWebPageShareTags(t *testing.T) {
	src, card := pageGame(t, "web:\n  url: https://example.org/camp/\n  description: 'A camp \"under\" the stars'\n  image: share/card.jpg\n")
	out := filepath.Join(t.TempDir(), "site")
	if err := Web(t.Context(), src, out, webBundle(t), "devel", ""); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "index.html")) //nolint:gosec // Test-owned exported page.
	if err != nil {
		t.Fatal(err)
	}
	page := string(b)
	for _, want := range []string{
		"<title>Camp &amp; &lt;expedition&gt;</title>",
		`<meta name="description" content="A camp &#34;under&#34; the stars">`,
		`<meta property="og:type" content="website">`,
		`<meta property="og:title" content="Camp &amp; &lt;expedition&gt;">`,
		`<meta property="og:description" content="A camp &#34;under&#34; the stars">`,
		`<meta property="og:url" content="https://example.org/camp/">`,
		`<meta property="og:image" content="https://example.org/camp/og-image.jpg">`,
		`<meta property="og:image:width" content="1280">`,
		`<meta property="og:image:height" content="720">`,
		`<meta name="twitter:card" content="summary_large_image">`,
		`<meta name="twitter:title" content="Camp &amp; &lt;expedition&gt;">`,
		`<meta name="twitter:description" content="A camp &#34;under&#34; the stars">`,
		`<meta name="twitter:image" content="https://example.org/camp/og-image.jpg">`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page has no %s", want)
		}
	}
	if strings.Contains(page, "Loading game</title>") {
		t.Error("the page keeps the loading title")
	}
	if got, err := os.ReadFile(filepath.Join(out, "og-image.jpg")); err != nil || !bytes.Equal(got, card) { //nolint:gosec // Test-owned exported picture.
		t.Errorf("the picture beside the page is not the game's as it is (%v)", err)
	}
}

// TestWebPageWithoutWeb: without web the page has only the game's title: no tags, no picture.
func TestWebPageWithoutWeb(t *testing.T) {
	src, _ := pageGame(t, "")
	out := filepath.Join(t.TempDir(), "site")
	if err := Web(t.Context(), src, out, webBundle(t), "devel", ""); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "index.html")) //nolint:gosec // Test-owned exported page.
	if err != nil {
		t.Fatal(err)
	}
	if page := string(b); !strings.Contains(page, "<title>Camp &amp; &lt;expedition&gt;</title>") || strings.Contains(page, "<meta name=\"description\"") || strings.Contains(page, "og:") || strings.Contains(page, "twitter:") {
		t.Errorf("the page without web:\n%s", page[:min(len(page), 800)])
	}
	if _, err := os.Stat(filepath.Join(out, "og-image.jpg")); !os.IsNotExist(err) {
		t.Errorf("a picture is copied without web (%v)", err)
	}
}

// TestWebPageHomeScreen: every page can be added to a phone's home screen: the manifest
// names the game and the way its screen is turned, web.icon is copied beside the page as
// icon.png with its size, and a phone held the other way is asked to turn in the page's
// language, with the note of each language the game offers for the device to choose.
func TestWebPageHomeScreen(t *testing.T) {
	src, _ := pageGame(t, "language: ja\nscreen: {width: 720, height: 1280}\nweb: {icon: share/icon.png}\n")
	out := filepath.Join(t.TempDir(), "site")
	if err := Web(t.Context(), src, out, webBundle(t), "devel", ""); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "index.html")) //nolint:gosec // Test-owned exported page.
	if err != nil {
		t.Fatal(err)
	}
	page := string(b)
	for _, want := range []string{
		`<html lang="ja" data-screen="portrait">`,
		`<link rel="manifest" href="manifest.webmanifest">`,
		`<meta name="apple-mobile-web-app-capable" content="yes">`,
		`<meta name="apple-mobile-web-app-title" content="Camp &amp; &lt;expedition&gt;">`,
		`<link rel="apple-touch-icon" href="icon.png">`,
		`<p id="turn-note">スマホを縦にすると、大きな画面で遊べます。</p>`,
		`<button type="button" id="keep">このまま遊ぶ</button>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page has no %s", want)
		}
	}
	var m struct {
		Name, Display, Orientation string
		Start                      string `json:"start_url"`
		Icons                      []struct{ Src, Sizes, Type string }
	}
	b, err = os.ReadFile(filepath.Join(out, "manifest.webmanifest")) //nolint:gosec // Test-owned exported manifest.
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m.Name != "Camp & <expedition>" || m.Display != "fullscreen" || m.Orientation != "portrait" || m.Start != "./" ||
		len(m.Icons) != 1 || m.Icons[0].Src != "icon.png" || m.Icons[0].Sizes != "192x192" || m.Icons[0].Type != "image/png" {
		t.Errorf("manifest:\n%s", b)
	}
	if _, err := os.Stat(filepath.Join(out, "icon.png")); err != nil {
		t.Errorf("the icon is not beside the page: %v", err)
	}

	// Without web, a landscape game in English: the manifest has no icon, the page asks
	// to turn sideways.
	src, _ = pageGame(t, "")
	out = filepath.Join(t.TempDir(), "site")
	if err := Web(t.Context(), src, out, webBundle(t), "devel", ""); err != nil {
		t.Fatal(err)
	}
	b, err = os.ReadFile(filepath.Join(out, "index.html")) //nolint:gosec // Test-owned exported page.
	if err != nil {
		t.Fatal(err)
	}
	if page := string(b); !strings.Contains(page, `data-screen="landscape"`) || !strings.Contains(page, "Turn your phone sideways") || strings.Contains(page, "apple-touch-icon") {
		t.Errorf("the page of a landscape game without web:\n%s", page[:min(len(page), 1500)])
	}
	if b, err := os.ReadFile(filepath.Join(out, "manifest.webmanifest")); err != nil || strings.Contains(string(b), "icons") { //nolint:gosec // Test-owned exported manifest.
		t.Errorf("manifest without web.icon (%v):\n%s", err, b)
	}
}
