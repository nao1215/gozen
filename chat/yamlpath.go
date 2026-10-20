package chat

import (
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
)

// yamlAt returns, as YAML, the part of src at the YAML logical path ("people.host.rules", "people[0]"; "" is the whole).
// Within a sequence, select by index ([0]) or by the id value. A missing path wraps fs.ErrNotExist.
func yamlAt(src []byte, path string) ([]byte, error) {
	if path == "" {
		return append([]byte(nil), src...), nil
	}
	var v any
	if err := yaml.UnmarshalWithOptions(src, &v, yaml.UseOrderedMap()); err != nil {
		return nil, err
	}
	missing := func() error { return fmt.Errorf("%s: %w", path, fs.ErrNotExist) }
	for _, part := range strings.Split(path, ".") {
		name, rest, _ := strings.Cut(part, "[")
		if name != "" {
			switch x := v.(type) {
			case yaml.MapSlice:
				found := false
				for _, it := range x {
					if fmt.Sprint(it.Key) == name {
						v, found = it.Value, true
						break
					}
				}
				if !found {
					return nil, missing()
				}
			case []any:
				found := false
				for _, e := range x {
					if m, ok := e.(yaml.MapSlice); ok {
						for _, it := range m {
							if fmt.Sprint(it.Key) == "id" && fmt.Sprint(it.Value) == name {
								v, found = e, true
							}
						}
					}
				}
				if !found {
					return nil, missing()
				}
			default:
				return nil, missing()
			}
		}
		for rest != "" {
			idx, after, ok := strings.Cut(rest, "]")
			n, err := strconv.Atoi(idx)
			list, isList := v.([]any)
			if !ok || err != nil || !isList || n < 0 || n >= len(list) {
				return nil, errors.Join(missing(), err)
			}
			v, rest = list[n], strings.TrimPrefix(after, "[")
		}
	}
	return yaml.Marshal(v)
}
