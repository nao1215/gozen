package gamedata_test

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

// webGame is the fixture game with game.yaml's web and a 1280x720 JPEG, a 600x315 PNG and
// a GIF under assets/.
func webGame(t *testing.T, web string) map[string]string {
	t.Helper()
	var big, small bytes.Buffer
	if err := jpeg.Encode(&big, image.NewRGBA(image.Rect(0, 0, 1280, 720)), nil); err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(&small, image.NewRGBA(image.Rect(0, 0, 600, 315))); err != nil {
		t.Fatal(err)
	}
	return map[string]string{
		"game.yaml":      "schema_version: 9\nid: web\n" + testgame.Game + web,
		"assets/big.jpg": big.String(), "assets/small.png": small.String(), "assets/card.gif": "GIF89a",
	}
}

// TestCheckWebPage: game.yaml's web needs schema version 9, an https:// address ending in
// /, and a PNG or a JPEG picture under assets/ with the address; a picture under 1200x630
// is warned of (GZN0121).
func TestCheckWebPage(t *testing.T) {
	for _, tt := range []struct{ name, web, want string }{
		{"http", "web: {url: 'http://example.org/'}\n", "want the page's https:// address ending in /"},
		{"no slash", "web: {url: 'https://example.org/game'}\n", "want the page's https:// address ending in /"},
		{"missing picture", "web: {url: 'https://example.org/', image: gone.png}\n", "no picture assets/gone.png"},
		{"gif", "web: {url: 'https://example.org/', image: card.gif}\n", "want a PNG or a JPEG"},
		{"picture without address", "web: {image: big.jpg}\n", "a picture needs web.url"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := testgame.TryLoad(webGame(t, tt.web)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("want an error with %q, got %v", tt.want, err)
			}
		})
	}
	old := webGame(t, "web: {url: 'https://example.org/'}\n")
	old["game.yaml"] = strings.Replace(old["game.yaml"], "schema_version: 9", "schema_version: 8", 1)
	if _, err := testgame.TryLoad(old); err == nil || !strings.Contains(err.Error(), "web needs schema_version: 9") {
		t.Fatalf("web under schema version 8: %v", err)
	}
	warned := func(web string) string {
		var b strings.Builder
		for _, w := range testgame.Load(t, webGame(t, web)).SceneWarnings() {
			b.WriteString(w.Error() + "\n")
		}
		return b.String()
	}
	if w := warned("web: {url: 'https://example.org/game/', description: A game., image: small.png}\n"); !strings.Contains(w, "GZN0121") || !strings.Contains(w, "600x315") {
		t.Errorf("a small picture is not warned of:\n%s", w)
	}
	if w := warned("web: {url: 'https://example.org/game/', description: A game., image: big.jpg}\n"); strings.Contains(w, "GZN0121") {
		t.Errorf("a picture of 1280x720 is warned of:\n%s", w)
	}
}
