package checkdata

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/base/diag"
	"github.com/nao1215/gozen/internal/model/gamedata"
)

func complexityWarnings(fsys fs.FS, db *gamedata.DB) ([]error, error) {
	const name = "check-complexity.yaml"
	var cfg gamedata.ComplexityConfig
	b, err := fs.ReadFile(fsys, name)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if err == nil {
		if err := yaml.UnmarshalWithOptions(b, &cfg, yaml.Strict()); err != nil {
			return nil, diag.Shape.Errorf("%s: %w", name, err)
		}
	}
	return db.ComplexityWarnings(cfg)
}
