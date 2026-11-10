package trial

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io/fs"
	"math"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/base/decodeguard"
)

type files map[string]any

// readGame reads what of a game tree ships with the game, as cli.WalkRuntime
// selects it: the decoded game.yaml and data files, which the trial prunes; the
// names under assets/, which it selects from; and the other files (translations,
// stream episodes, license notices), which it carries unchanged.
func readGame(fsys fs.FS) (out files, assets, carried []string, err error) {
	if _, err := fs.Stat(fsys, "game.yaml"); err != nil {
		return nil, nil, nil, err
	}
	out = files{}
	err = cli.WalkRuntime(fsys, func(name string, d fs.DirEntry) error {
		if d.IsDir() {
			return nil
		}
		top, rest, _ := strings.Cut(name, "/")
		switch {
		case name == "game.yaml" || top == "data":
			b, err := fs.ReadFile(fsys, name)
			if err != nil {
				return err
			}
			var v any
			if err := decodeguard.Run(func() error { return yaml.UnmarshalWithOptions(b, &v, yaml.UseOrderedMap()) }); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			out[name] = mergeKeys(v)
		case top == "assets" && !cli.Notice(name):
			assets = append(assets, rest)
		default:
			carried = append(carried, name)
		}
		return nil
	})
	if err != nil {
		return nil, nil, nil, err
	}
	sort.Strings(assets)
	return out, assets, carried, nil
}

// mergeKeys gives each mapping one pair per key. The decoder lists the pairs of
// a merge key (<<) and the mapping's own in document order, and the loader takes
// the later one, so the later value wins here, at the place of the first.
func mergeKeys(v any) any {
	switch x := v.(type) {
	case yaml.MapSlice:
		out := make(yaml.MapSlice, 0, len(x))
		at := map[string]int{}
		for _, it := range x {
			id := fmt.Sprintf("%T %v", it.Key, it.Key)
			value := mergeKeys(it.Value)
			if i, ok := at[id]; ok {
				out[i].Value = value
				continue
			}
			at[id] = len(out)
			out = append(out, yaml.MapItem{Key: it.Key, Value: value})
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = mergeKeys(e)
		}
		return out
	}
	return v
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

// marshal writes a decoded document in block style. It writes every string in
// double quotes with its control characters escaped, so that each value reads
// back as it was: a plain or a block scalar loses tabs and carriage returns, and
// a plain one may read as another type or as syntax. Comments and the source's
// flow style are not kept.
func marshal(v any) ([]byte, error) {
	var b bytes.Buffer
	if err := emit(&b, v, 0, atStart); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Where a node starts: at the start of a line, after "key:" or after "-".
const (
	atStart = iota
	afterKey
	afterDash
)

// emit writes v, whose mapping pairs and sequence entries stand at indent.
func emit(b *bytes.Buffer, v any, indent, at int) error {
	// lead brings the cursor to the first pair or entry of a collection.
	lead := func(i int) {
		switch {
		case i > 0:
			b.WriteString(strings.Repeat(" ", indent))
		case at == afterKey:
			b.WriteString("\n" + strings.Repeat(" ", indent))
		case at == afterDash:
			b.WriteByte(' ')
		}
	}
	switch x := v.(type) {
	case yaml.MapSlice:
		for i, it := range x {
			lead(i)
			key, err := scalar(it.Key, true)
			if err != nil {
				return err
			}
			b.WriteString(key + ":")
			if err := emit(b, it.Value, indent+2, afterKey); err != nil {
				return fmt.Errorf("%s: %w", keyOf(it.Key), err)
			}
		}
		if len(x) > 0 {
			return nil
		}
		v = emptyMapping
	case []any:
		for i, e := range x {
			lead(i)
			b.WriteByte('-')
			if err := emit(b, e, indent+2, afterDash); err != nil {
				return fmt.Errorf("[%d]: %w", i, err)
			}
		}
		if len(x) > 0 {
			return nil
		}
		v = emptySequence
	}
	s, err := scalar(v, false)
	if err != nil {
		return err
	}
	if at != atStart {
		b.WriteByte(' ')
	}
	b.WriteString(s + "\n")
	return nil
}

// flow is a collection without pairs or entries, which has no block form.
type flow string

const (
	emptyMapping  flow = "{}"
	emptySequence flow = "[]"
)

// scalar writes one value on a line. A key that is a plain word stays bare;
// every other string is quoted.
func scalar(v any, key bool) (string, error) {
	switch x := v.(type) {
	case flow:
		return string(x), nil
	case nil:
		return "null", nil
	case string:
		if !utf8.ValidString(x) {
			return "", fmt.Errorf("the text %q is not UTF-8", x)
		}
		if key && plainKey(x) {
			return x, nil
		}
		// A Go quoted string is a YAML double-quoted scalar: the escapes Go
		// writes (\n, \t, \xNN, \uNNNN, \UNNNNNNNN and the like) are YAML's.
		return strconv.Quote(x), nil
	case bool:
		return strconv.FormatBool(x), nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", x), nil
	case float32:
		return float(float64(x)), nil
	case float64:
		return float(x), nil
	case []byte:
		return "!!binary " + strconv.Quote(base64.StdEncoding.EncodeToString(x)), nil
	}
	return "", fmt.Errorf("a %T value cannot be written to the trial", v)
}

// float writes a number so that it reads back as a float: with a fraction, and
// without an exponent, which the decoder reads as text.
func float(f float64) string {
	switch {
	case math.IsNaN(f):
		return ".nan"
	case math.IsInf(f, 1):
		return ".inf"
	case math.IsInf(f, -1):
		return "-.inf"
	}
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

// plainKey reports whether a key is a word that needs no quotes: letters,
// digits, "_", "-" and "." after a letter or "_", and not a YAML constant.
func plainKey(s string) bool {
	switch strings.ToLower(s) {
	case "", "null", "true", "false":
		return false
	}
	for i, r := range s {
		letter := r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
		if !letter && (i == 0 || !strings.ContainsRune("0123456789-.", r)) {
			return false
		}
	}
	return !strings.HasSuffix(s, ".") && !strings.HasSuffix(s, "-")
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
