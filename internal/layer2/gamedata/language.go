package gamedata

import (
	"io/fs"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/layer1/decodeguard"
	"github.com/nao1215/gozen/internal/text/catalog"
	lang "github.com/nao1215/gozen/text"
)

// UseWords chooses the process-wide words (text.Use) from game.yaml's language and text in
// fsys, reading nothing else, so that Load reports its problems in the game's language.
// A missing or unreadable game.yaml or an unknown language gives the English words; Load
// reports why. Hosts call it before Load and use db.Game after a successful Load.
func UseWords(fsys fs.FS) error {
	var g struct {
		Language string            `yaml:"language"`
		Text     map[string]string `yaml:"text"`
	}
	if b, err := fs.ReadFile(fsys, "game.yaml"); err == nil {
		if decodeguard.Run(func() error { return yaml.Unmarshal(b, &g) }) != nil {
			g.Text = nil
		}
	}
	if _, err := catalog.Words(g.Language); err != nil {
		g.Language = lang.English
	}
	return lang.Use(g.Language, g.Text)
}
