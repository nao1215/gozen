package distribute

import (
	"bytes"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/testgame"
)

// pageGame writes a small game whose game.yaml ends with more, and a 1280x720 JPEG at
// assets/share/card.jpg; it returns the game's folder.
func pageGame(t *testing.T, more string) (string, []byte) {
	t.Helper()
	src := t.TempDir()
	var card bytes.Buffer
	if err := jpeg.Encode(&card, image.NewRGBA(image.Rect(0, 0, 1280, 720)), nil); err != nil {
		t.Fatal(err)
	}
	for name, b := range map[string][]byte{
		"game.yaml":             []byte("schema_version: 9\nid: page-test\n" + strings.Replace(testgame.Game, "Camp expedition", "Camp & <expedition>", 1) + more),
		"data/main.yaml":        []byte(testgame.Data),
		"assets/share/card.jpg": card.Bytes(),
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
	if err := Web(src, out, webBundle(t), "devel"); err != nil {
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
	if err := Web(src, out, webBundle(t), "devel"); err != nil {
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
