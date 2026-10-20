// Package catalog holds the engine's built-in words by language. The public text package
// chooses among them; the loader and the CLI read them without touching the process-wide
// choice.
package catalog

import (
	"fmt"
	"maps"
)

// The built-in languages. The empty language is English.
const (
	English  = "en"
	Japanese = "ja"
)

// Words returns a copy of the built-in words of language ("" is English): the engine's
// key (its Japanese source text) -> the word shown. Japanese maps every key to itself.
// An unknown language is an error.
func Words(language string) (map[string]string, error) {
	out := make(map[string]string, len(english))
	switch language {
	case English, "":
		maps.Copy(out, english)
	case Japanese:
		for k := range english {
			out[k] = k
		}
	default:
		return nil, fmt.Errorf("unknown language %q (use %q or %q)", language, English, Japanese)
	}
	return out, nil
}
