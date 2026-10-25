package trial

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/expr"
	"github.com/nao1215/gozen/internal/layer1/decodeguard"
	lang "github.com/nao1215/gozen/internal/text"
)

type Config struct {
	Assume map[string]any `yaml:"assume"`

	DiveMax map[string]int `yaml:"dive_max"`

	Blocked []any `yaml:"blocked"`

	Cut []string `yaml:"cut"`

	Unknown []string `yaml:"unknown"`

	Game yaml.MapSlice `yaml:"game"`

	Keep []string `yaml:"keep"`

	// Forbid matches retained definition paths (events.ending) or assets paths (assets/cg/*).
	Forbid []string `yaml:"forbid"`
}

func Load(fsys fs.FS, name string) (*Config, error) {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

func Parse(b []byte) (*Config, error) {
	var c Config
	if err := decodeguard.Run(func() error { return yaml.UnmarshalWithOptions(b, &c, yaml.Strict(), yaml.UseOrderedMap()) }); err != nil {
		return nil, fmt.Errorf("trial: %s", yaml.FormatError(err, false, true))
	}
	for k, v := range c.Assume {
		if _, err := assumed(v); err != nil {
			return nil, fmt.Errorf("trial: assume %s: %w", k, err)
		}
		if !strings.HasPrefix(k, "var.") && !strings.HasPrefix(k, "flag.") {
			return nil, fmt.Errorf(lang.T("trial: assume %s: var.名前 か flag.名前 で書く"), k)
		}
	}
	if len(c.DiveMax) > 0 && len(c.Blocked) == 0 {
		return nil, errors.New(lang.T("trial: dive_max には blocked（範囲の外へ出ようとしたときのコマンド）が要る"))
	}
	for i, b := range c.Blocked {
		if _, ok := b.(yaml.MapSlice); !ok {
			return nil, fmt.Errorf("trial: blocked: each entry is a command (a mapping such as {say: ...}); entry %d is %v", i, b)
		}
	}
	for _, pattern := range c.Forbid {
		valid := strings.HasPrefix(pattern, "assets/")
		for _, section := range deletable {
			valid = valid || strings.HasPrefix(pattern, section+".")
		}
		if !valid {
			return nil, fmt.Errorf("trial: forbid %q must start with assets/ or a removable definition section", pattern)
		}
	}
	for _, patterns := range [][]string{c.Cut, c.Unknown, c.Keep, c.Forbid} {
		for _, p := range patterns {
			if _, err := pathMatch(p, ""); err != nil {
				return nil, fmt.Errorf("trial pattern %q: %w", p, err)
			}
		}
	}
	return &c, nil
}

func assumed(v any) (expr.Range, error) {
	switch x := v.(type) {
	case []any:
		if len(x) != 2 {
			return expr.Range{}, errors.New(lang.T("幅は [下, 上] の 2 つ"))
		}
		lo, ok1 := number(x[0])
		hi, ok2 := number(x[1])
		if !ok1 || !ok2 {
			return expr.Range{}, errors.New(lang.T("幅は数で書く"))
		}
		return expr.Between(lo, hi), nil
	case bool, string, nil:
		return expr.Of(x), nil
	}
	if f, ok := number(v); ok {
		return expr.Number(f), nil
	}
	return expr.Range{}, fmt.Errorf(lang.T("%v は前提に書けない"), v)
}

func number(v any) (float64, bool) {
	switch x := v.(type) {
	case string:
		switch x {
		case "inf", "+inf":
			return math.Inf(1), true
		case "-inf":
			return math.Inf(-1), true
		}
		return 0, false
	case uint64:
		return float64(x), true
	case uint:
		return float64(x), true
	case int32:
		return float64(x), true
	case float32:
		return float64(x), true
	}
	return expr.ToNumber(v)
}

func match(patterns []string, name string) bool {
	for _, p := range patterns {
		if ok, err := pathMatch(p, name); err == nil && ok {
			return true
		}
	}
	return false
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
