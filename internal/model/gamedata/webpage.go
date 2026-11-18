package gamedata

import (
	"path"
	"strings"

	"github.com/nao1215/gozen/internal/base/diag"
)

// ShareImageMinW and ShareImageMinH are the smallest picture of a browser export's link card that the sites
// show as a large card.
const ShareImageMinW, ShareImageMinH = 1200, 630

// webIconMin is the smallest side of game.yaml's web.icon: the size phones ask a home
// screen icon to have at least.
const webIconMin = 192

// webPage reports the mistakes of game.yaml's web: an address that is not https:// or does
// not end in /, a picture that is missing, not a PNG or a JPEG, or without the address to
// make its own, a picture smaller than 1200x630 (a warning, GZN0121), and an icon that is
// missing, not a PNG, not square or smaller than 192x192.
func (c *checker) webPage(w *WebPage) {
	if w == nil || !c.needsVersion("game.yaml web", "web", SchemaVersion9) {
		return
	}
	if w.URL != "" && (!strings.HasPrefix(w.URL, "https://") || !strings.HasSuffix(w.URL, "/") || len(w.URL) <= len("https://")+1) {
		c.errorf(diag.Shape, "game.yaml web.url", "want the page's https:// address ending in /, not %q", w.URL)
	}
	c.webIcon(w.Icon)
	if w.Image == "" {
		return
	}
	if w.URL == "" {
		c.errorf(diag.Shape, "game.yaml web.image", "a picture needs web.url: the sites take its absolute address")
	}
	switch strings.ToLower(path.Ext(w.Image)) {
	case ".png", ".jpg", ".jpeg":
	default:
		c.errorf(diag.Shape, "game.yaml web.image", "want a PNG or a JPEG, not %q", w.Image)
		return
	}
	if !c.db.Exists("assets/" + w.Image) {
		c.errorf(diag.MissingAsset, "game.yaml web.image", "no picture assets/%s", w.Image)
		return
	}
	size, ok := c.pictureSize(w.Image)
	switch {
	case !ok:
		c.errorf(diag.Shape, "game.yaml web.image", "assets/%s is not a PNG or a JPEG picture", w.Image)
	case size.X < ShareImageMinW || size.Y < ShareImageMinH:
		c.warnf(diag.ShareImage, "game.yaml web.image", "assets/%s is %dx%d: the sites show a large card from %dx%d", w.Image, size.X, size.Y, ShareImageMinW, ShareImageMinH)
	}
}

// webIcon reports a web.icon that is missing, not a PNG, not square or smaller than
// webIconMin on a side.
func (c *checker) webIcon(icon string) {
	if icon == "" {
		return
	}
	const at = "game.yaml web.icon"
	if strings.ToLower(path.Ext(icon)) != ".png" {
		c.errorf(diag.Shape, at, "want a PNG, not %q", icon)
		return
	}
	if !c.db.Exists("assets/" + icon) {
		c.errorf(diag.MissingAsset, at, "no picture assets/%s", icon)
		return
	}
	size, ok := c.pictureSize(icon)
	switch {
	case !ok:
		c.errorf(diag.Shape, at, "assets/%s is not a PNG picture", icon)
	case size.X != size.Y || size.X < webIconMin:
		c.errorf(diag.Shape, at, "assets/%s is %dx%d: want a square of %dx%d or more", icon, size.X, size.Y, webIconMin, webIconMin)
	}
}
