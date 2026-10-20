package trial

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/layer1/decodeguard"
)

type files map[string]any

func readFiles(fsys fs.FS) (files, error) {
	out := files{}
	var paths []string
	if _, err := fs.Stat(fsys, "game.yaml"); err != nil {
		return nil, err
	}
	paths = append(paths, "game.yaml")
	err := fs.WalkDir(fsys, "data", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && (strings.HasSuffix(p, ".yaml") || strings.HasSuffix(p, ".yml")) {
			paths = append(paths, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, p := range paths {
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return nil, err
		}
		var v any
		if err := decodeguard.Run(func() error { return yaml.UnmarshalWithOptions(b, &v, yaml.UseOrderedMap()) }); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		out[p] = v
	}
	return out, nil
}

func (f files) names() []string {
	out := make([]string, 0, len(f))
	for p := range f {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func cloneNode(v any) any {
	switch x := v.(type) {
	case yaml.MapSlice:
		out := make(yaml.MapSlice, len(x))
		for i, it := range x {
			out[i] = yaml.MapItem{Key: it.Key, Value: cloneNode(it.Value)}
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = cloneNode(e)
		}
		return out
	}
	return v
}

func marshal(v any) ([]byte, error) {
	return yaml.MarshalWithOptions(v, yaml.Indent(2), yaml.IndentSequence(true))
}

func keyOf(k any) string {
	if s, ok := k.(string); ok {
		return s
	}
	return fmt.Sprint(k)
}

func get(m yaml.MapSlice, k string) (any, bool) {
	for _, it := range m {
		if keyOf(it.Key) == k {
			return it.Value, true
		}
	}
	return nil, false
}

func set(m yaml.MapSlice, k string, v any) yaml.MapSlice {
	for i, it := range m {
		if keyOf(it.Key) == k {
			m[i].Value = v
			return m
		}
	}
	return append(m, yaml.MapItem{Key: k, Value: v})
}

func del(m yaml.MapSlice, k string) yaml.MapSlice {
	out := m[:0:0]
	for _, it := range m {
		if keyOf(it.Key) != k {
			out = append(out, it)
		}
	}
	return out
}

func str(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case nil:
		return "", false
	case yaml.MapSlice, []any:
		return "", false
	case bool:
		if x {
			return "true", true
		}
		return "false", true
	}
	return fmt.Sprint(v), true
}

func (f files) section(name string) []sectionPart {
	var out []sectionPart
	for _, p := range f.names() {
		if p == "game.yaml" {
			continue
		}
		m, ok := f[p].(yaml.MapSlice)
		if !ok {
			continue
		}
		if v, ok := get(m, name); ok {
			if sm, ok := v.(yaml.MapSlice); ok {
				out = append(out, sectionPart{file: p, m: sm})
			}
		}
	}
	return out
}

type sectionPart struct {
	file string
	m    yaml.MapSlice
}

func (f files) setSection(p, name string, v any) {
	m := mapping(f[p])
	f[p] = set(m, name, v)
}

func (f files) game() yaml.MapSlice {
	m := mapping(f["game.yaml"])
	return m
}

func cleanPath(s string) string {
	return strings.TrimPrefix(path.Clean(strings.TrimSpace(s)), "assets/")
}

func listAssets(fsys fs.FS) ([]string, error) {
	var out []string
	err := fs.WalkDir(fsys, "assets", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			out = append(out, strings.TrimPrefix(p, "assets/"))
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

func mapping(v any) yaml.MapSlice {
	m, ok := v.(yaml.MapSlice)
	if !ok {
		return nil
	}
	return m
}

func sequence(v any) []any {
	s, ok := v.([]any)
	if !ok {
		return nil
	}
	return s
}
