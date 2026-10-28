package distribute

import (
	"bytes"
	"cmp"
	"fmt"
	"html"
	"image"
	_ "image/jpeg" // the link card's picture
	_ "image/png"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/layer2/gamedata"
)

// shareImage is the name of the link card's picture beside the page, before its extension.
const shareImage = "og-image"

// pageFor is the page of a browser export: titled with the game's title, in its language,
// and with game.yaml's web, the description and the tags of Open Graph and X cards; the
// card's picture is copied from the game src beside the page in stage as it is.
func pageFor(src, stage string, db *gamedata.DB) ([]byte, error) {
	title := html.EscapeString(cmp.Or(db.Game.Title, "Loading game"))
	page := bytes.Replace(webPage, []byte(`<html lang="en">`), fmt.Appendf(nil, `<html lang="%s">`, html.EscapeString(cmp.Or(db.Game.Language, "en"))), 1)
	var head strings.Builder
	fmt.Fprintf(&head, "<title>%s</title>", title)
	if w := db.Game.Web; w != nil {
		meta := func(attr, key, value string) {
			fmt.Fprintf(&head, "\n<meta %s=\"%s\" content=\"%s\">", attr, key, html.EscapeString(value))
		}
		if w.Description != "" {
			meta("name", "description", w.Description)
		}
		meta("property", "og:type", "website")
		meta("property", "og:title", db.Game.Title)
		if w.Description != "" {
			meta("property", "og:description", w.Description)
		}
		if w.URL != "" {
			meta("property", "og:url", w.URL)
		}
		image := ""
		if w.Image != "" && w.URL != "" {
			name, size, err := copyShareImage(src, stage, w.Image)
			if err != nil {
				return nil, err
			}
			image = w.URL + name
			meta("property", "og:image", image)
			meta("property", "og:image:width", strconv.Itoa(size.X))
			meta("property", "og:image:height", strconv.Itoa(size.Y))
		}
		meta("name", "twitter:card", map[bool]string{true: "summary_large_image", false: "summary"}[image != ""])
		meta("name", "twitter:title", db.Game.Title)
		if w.Description != "" {
			meta("name", "twitter:description", w.Description)
		}
		if image != "" {
			meta("name", "twitter:image", image)
		}
	}
	const loading = "<title>Loading game</title>"
	if bytes.Count(page, []byte(loading)) != 1 {
		return nil, fmt.Errorf("the page template has no single %s", loading)
	}
	return bytes.Replace(page, []byte(loading), []byte(head.String()), 1), nil
}

// copyShareImage copies the picture under the game's assets/ beside the page as
// og-image with its extension, as it is, and reports its name and size.
func copyShareImage(src, stage, file string) (string, image.Point, error) {
	b, err := os.ReadFile(filepath.Join(src, "assets", filepath.FromSlash(file))) //nolint:gosec // A picture of the game being exported, checked when it loaded.
	if err != nil {
		return "", image.Point{}, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return "", image.Point{}, fmt.Errorf("web.image assets/%s: %w", file, err)
	}
	name := shareImage + strings.ToLower(path.Ext(file))
	if err := cli.WriteNew(filepath.Join(stage, name), b); err != nil {
		return "", image.Point{}, err
	}
	return name, image.Pt(cfg.Width, cfg.Height), nil
}
