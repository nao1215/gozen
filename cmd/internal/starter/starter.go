// Package starter creates small, editable games from bundled neutral templates.
// It belongs to command tooling; the engine does not depend on templates.
package starter

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	md "github.com/nao1215/markdown"

	"github.com/nao1215/gozen/cmd/internal/cli"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

//go:embed templates
var templates embed.FS

// Config identifies the template and the new game's persistent save identity.
type Config struct{ Template, ID, Title string }

// Has reports whether name is a bundled template.
func Has(name string) bool { return name == "rpg" || name == "novel" || name == "campaign" }

// Write creates and validates a template inside a caller-owned staging directory.
// The caller publishes it only after success; existing files are never replaced.
func Write(dir string, cfg Config) error {
	if !Has(cfg.Template) {
		return fmt.Errorf("unknown template %q", cfg.Template)
	}
	root := "templates/" + cfg.Template
	err := fs.WalkDir(templates, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := templates.ReadFile(path)
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(path, root+"/")
		switch name {
		case "game.yaml":
			id, err := json.Marshal(cfg.ID)
			if err != nil {
				return err
			}
			title, err := json.Marshal(cfg.Title)
			if err != nil {
				return err
			}
			b = []byte(strings.NewReplacer("{{ID}}", string(id), "{{TITLE}}", string(title)).Replace(string(b)))
		case "world.yaml":
			name = "data/world.yaml"
		}
		return cli.WriteNew(filepath.Join(dir, filepath.FromSlash(name)), b)
	})
	if err != nil {
		return err
	}
	if cfg.Template != "novel" {
		common, err := templates.ReadFile("templates/common.yaml")
		if err != nil {
			return err
		}
		if err := cli.WriteNew(filepath.Join(dir, "data", "rules.yaml"), common); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o750); err != nil {
		return err
	}
	if cfg.Template == "campaign" {
		picture, err := valley()
		if err != nil {
			return err
		}
		if err := cli.WriteNew(filepath.Join(dir, "assets", "valley.png"), picture); err != nil {
			return err
		}
	}
	if err := cli.WriteNew(filepath.Join(dir, "README.md"), []byte(readme())); err != nil {
		return err
	}
	if _, err := data.Load(os.DirFS(dir)); err != nil {
		return fmt.Errorf("generated game: %w", err)
	}
	return nil
}

// valley is original placeholder artwork, generated without external tools.
func valley() ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, 960, 540))
	for y := range 540 {
		for x := range 960 {
			c := color.RGBA{R: 184, G: 196, B: 151, A: 255}
			if x > 410+y/10 && x < 450+y/10 {
				c = color.RGBA{R: 98, G: 156, B: 181, A: 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func readme() string {
	return md.NewMarkdown(nil, md.WithBlockSpacing()).
		H1("Your game").
		PlainText("Open a terminal in this directory and run:").
		CodeBlocks(md.SyntaxHighlightShell, "gozen check .\ngozen run .").
		PlainText("Edit the title in game.yaml and the dialog or regions in data/world.yaml.\nNovel speakers need only names; add faces and voices when ready.\nRPG and campaign templates keep combat rules in data/rules.yaml.\nPut your pictures and sounds in assets/ and reference their filenames in YAML.").
		PlainText("The bundled placeholder artwork and template text are original Gozen material\nprovided under the MIT license: https://github.com/nao1215/gozen/blob/main/LICENSE.\nNo Go source, compiler or dependency installation is needed for these commands.").
		PlainText("For testing, run `gozen run . --dev --save-dir test-saves`.\nPress F5 after editing YAML to reload; use `gozen run --help` for all options.\nKeep game.yaml's id unchanged after sharing a game: it identifies its saves.\nThe built-in font supports Latin text. Add a suitable font for other scripts.").
		PlainText("Documentation: https://nao1215.github.io/gozen/").String() + "\n"
}
