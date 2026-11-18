package distribute

import (
	"bytes"
	"cmp"
	"encoding/json"
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
	"github.com/nao1215/gozen/internal/model/gamedata"
)

// shareImage is the name of the link card's picture beside the page, before its extension.
const shareImage = "og-image"

// homeManifest is the web app manifest beside the page and homeIcon its icon (web.icon):
// a phone that adds the page to its home screen opens it from there full screen, turned
// the way the game's screen is.
const (
	homeManifest = "manifest.webmanifest"
	homeIcon     = "icon.png"
	pageColor    = "#10181c"
)

// turnNotes are what the page says to a phone held the other way from the game's screen,
// and its button to play as it is, in the page's language (English for one without its own).
var turnNotes = map[string][2]string{
	"en-landscape": {"Turn your phone sideways to see the game larger.", "Play as it is"},
	"en-portrait":  {"Turn your phone upright to see the game larger.", "Play as it is"},
	"ja-landscape": {"スマホを横にすると、大きな画面で遊べます。", "このまま遊ぶ"},
	"ja-portrait":  {"スマホを縦にすると、大きな画面で遊べます。", "このまま遊ぶ"},
}

// pageFor is the page of a browser export: titled with the game's title, in its language,
// and with game.yaml's web, the description and the tags of Open Graph and X cards; the
// card's picture is copied from the game src beside the page in stage as it is.
//
// Every page can be added to a phone's home screen: the manifest names the game and the
// way its screen is turned, and web.icon, copied beside the page as icon.png, is its
// icon. A phone held the other way is asked to turn (turnNotes).
func pageFor(src, stage string, db *gamedata.DB) ([]byte, error) {
	title := html.EscapeString(cmp.Or(db.Game.Title, "Loading game"))
	language := cmp.Or(db.Game.Language, "en")
	turn := "landscape"
	if db.Game.Screen.Height > db.Game.Screen.Width {
		turn = "portrait"
	}
	page := bytes.Replace(webPage, []byte(`<html lang="en">`), fmt.Appendf(nil, `<html lang="%s" data-screen="%s">`, html.EscapeString(language), turn), 1)
	// The note is in the page's language, and the page shows it in the first of the
	// device's languages the game offers, as the game chooses its own.
	notes := map[string][2]string{}
	for _, code := range append([]string{language}, db.Game.Offered()...) {
		note, ok := turnNotes[code+"-"+turn]
		if !ok {
			note = turnNotes["en-"+turn]
		}
		notes[code] = note
	}
	others, err := json.Marshal(notes)
	if err != nil {
		return nil, err
	}
	for _, r := range [][2]string{
		{"Turn your phone to see the game larger.", html.EscapeString(notes[language][0])},
		{"Play as it is", html.EscapeString(notes[language][1])},
		{`data-notes="{}"`, `data-notes="` + html.EscapeString(string(others)) + `"`},
	} {
		if bytes.Count(page, []byte(r[0])) != 1 {
			return nil, fmt.Errorf("the page template has no single %q", r[0])
		}
		page = bytes.Replace(page, []byte(r[0]), []byte(r[1]), 1)
	}
	var head strings.Builder
	fmt.Fprintf(&head, "<title>%s</title>", title)
	meta := func(attr, key, value string) {
		fmt.Fprintf(&head, "\n<meta %s=\"%s\" content=\"%s\">", attr, key, html.EscapeString(value))
	}
	icon, err := writeHome(src, stage, db, turn)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(&head, "\n<link rel=\"manifest\" href=\"%s\">", homeManifest)
	meta("name", "theme-color", pageColor)
	meta("name", "mobile-web-app-capable", "yes")
	meta("name", "apple-mobile-web-app-capable", "yes")
	meta("name", "apple-mobile-web-app-status-bar-style", "black")
	meta("name", "apple-mobile-web-app-title", cmp.Or(db.Game.Title, "Game"))
	if icon {
		fmt.Fprintf(&head, "\n<link rel=\"icon\" href=\"%s\">\n<link rel=\"apple-touch-icon\" href=\"%s\">", homeIcon, homeIcon)
	}
	if w := db.Game.Web; w != nil {
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
	b, err := os.ReadFile(filepath.Join(src, "assets", filepath.FromSlash(file))) // #nosec G304 -- A picture of the game being exported, checked when it loaded.
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

// writeHome writes the web app manifest beside the page, and copies web.icon beside it as
// icon.png when the game has one (reported).
func writeHome(src, stage string, db *gamedata.DB, turn string) (bool, error) {
	type icon struct {
		Src     string `json:"src"`
		Sizes   string `json:"sizes"`
		Type    string `json:"type"`
		Purpose string `json:"purpose"`
	}
	m := struct {
		Name            string `json:"name"`
		ShortName       string `json:"short_name"`
		Lang            string `json:"lang"`
		StartURL        string `json:"start_url"`
		Scope           string `json:"scope"`
		Display         string `json:"display"`
		Orientation     string `json:"orientation"`
		BackgroundColor string `json:"background_color"`
		ThemeColor      string `json:"theme_color"`
		Icons           []icon `json:"icons,omitempty"`
	}{
		Name: cmp.Or(db.Game.Title, "Game"), Lang: cmp.Or(db.Game.Language, "en"),
		StartURL: "./", Scope: "./", Display: "fullscreen", Orientation: turn,
		BackgroundColor: pageColor, ThemeColor: pageColor,
	}
	m.ShortName = m.Name
	has := false
	if w := db.Game.Web; w != nil && w.Icon != "" {
		b, err := os.ReadFile(filepath.Join(src, "assets", filepath.FromSlash(w.Icon))) // #nosec G304 -- A picture of the game being exported, checked when it loaded.
		if err != nil {
			return false, err
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
		if err != nil {
			return false, fmt.Errorf("web.icon assets/%s: %w", w.Icon, err)
		}
		if err := cli.WriteNew(filepath.Join(stage, homeIcon), b); err != nil {
			return false, err
		}
		m.Icons = []icon{{Src: homeIcon, Sizes: fmt.Sprintf("%dx%d", cfg.Width, cfg.Height), Type: "image/png", Purpose: "any"}}
		has = true
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return false, err
	}
	return has, cli.WriteNew(filepath.Join(stage, homeManifest), append(b, '\n'))
}
