package trial

import (
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/goccy/go-yaml"
)

// Every text comes back as it was from what marshal writes, as a value, in a list
// and as a key: none reads as another type, as syntax, or with a character lost.
func TestMarshalRoundTripsEveryScalar(t *testing.T) {
	t.Parallel()
	texts := []string{
		"yes", "no", "on", "off", "y", "n", "null", "~", "true", "false", "Null", "TRUE",
		"123", "1.0", "0x1F", "0o17", "0b11", "1e3", "1_000", ".5", "+1", "-1", ".inf", "-.inf", ".nan",
		"2001-01-01", "12:30:00",
		" leading", "trailing ", "a: b", "x:", ":", "#x", "x #y", "- x", "-", "?", "? x",
		"@x", "*x", "&x", "!x", "%x", "{x}", "[x]", "'", "\"", "|", ">", "<<", "=", "`x`", ",",
		"multi\nline", "line1\n\nline3\n", "trail\n", "\nlead", "  \n", "a\rb", "a\r\nb", "tab\there", "\ttab",
		"", " ", "\u0085", "a\u2028b", "\ufeffbom", "é", "日本語", "a\\b", "a\\nb", "back\\", "\x00", "\x07bell", "\x1b[0m",
		"key: [a, b]", "x: {y: z}", "---", "...", "--- x", "? Who goes there", "HP\t10",
		"inf", "nan", "Infinity", "NaN", "e1", "_1", "x.y", "a-b", "a.", "a-", "plain_key",
	}
	for _, s := range texts {
		in := yaml.MapSlice{{Key: "say", Value: s}, {Key: "list", Value: []any{s}}, {Key: s, Value: uint64(1)}}
		b, err := marshal(in)
		if err != nil {
			t.Errorf("%q: marshal: %v", s, err)
			continue
		}
		var out any
		if err := yaml.UnmarshalWithOptions(b, &out, yaml.UseOrderedMap()); err != nil {
			t.Errorf("%q: written as %q, which does not read: %v", s, b, err)
			continue
		}
		if !reflect.DeepEqual(out, any(in)) {
			t.Errorf("%q: written as %q, read back as %#v", s, b, out)
		}
	}
}

// A data file read by readGame and written by marshal means what its source means
// to the decoder: merge keys are folded, and numbers, nulls and empty collections stay.
func TestDocumentRoundTrip(t *testing.T) {
	t.Parallel()
	for name, src := range map[string]string{
		"merge key":     "base: &b {when: flag.x, say: hi}\nevents:\n  a:\n    do:\n      - <<: *b\n        say: other\n",
		"merge after":   "base: &b {when: flag.x, say: hi}\nevents:\n  a:\n    do:\n      - say: other\n        <<: *b\n",
		"merge list":    "b: &b {when: x, say: hi}\nc: &c {say: c, z: 1}\na:\n  <<: [*b, *c]\n  q: other\n",
		"binary":        "a: !!binary aGVsbG8=\n",
		"float":         "v: 1.0\nw: 1e21\nx: -0.0\ny: 0.1\n",
		"big int":       "v: 18446744073709551615\nw: -9223372036854775808\n",
		"int keys":      "bosses:\n  5: troll\n  10: dragon\n",
		"null value":    "a:\nb: ~\n",
		"empty":         "",
		"two documents": "a: 1\n---\nb: 2\n",
		"block scalar":  "say: |\n  one\n  two\n\nnext: >-\n  folded\n  text\n",
		"empty seq map": "a: []\nb: {}\n",
	} {
		fsys := fstest.MapFS{"game.yaml": {Data: []byte("id: x\n")}, "data/a.yaml": {Data: []byte(src)}}
		fs, _, _, err := readGame(fsys)
		if err != nil {
			t.Errorf("%s: read: %v", name, err)
			continue
		}
		b, err := marshal(fs["data/a.yaml"])
		if err != nil {
			t.Errorf("%s: marshal: %v", name, err)
			continue
		}
		var want, got any
		if err := yaml.Unmarshal([]byte(src), &want); err != nil {
			t.Errorf("%s: source: %v", name, err)
			continue
		}
		if err := yaml.Unmarshal(b, &got); err != nil {
			t.Errorf("%s: written as %q, which does not read: %v", name, b, err)
			continue
		}
		if !reflect.DeepEqual(want, got) {
			t.Errorf("%s: source %q written as %q:\n want %#v\n got  %#v", name, src, b, want, got)
		}
	}
}
