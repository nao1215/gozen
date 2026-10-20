// Package detect finds the regions to mosaic in a picture with a
// user-supplied ONNX detection model.
package detect

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Where the class names of a model came from.
const (
	SourceFlag     = "--classes"
	SourceJSON     = "labels.json"
	SourceMetadata = "metadata"
	SourceDefault  = "default"
)

// ValidateClasses rejects empty and duplicate class names.
func ValidateClasses(names []string) error {
	if len(names) == 0 {
		return errors.New("the class list is empty")
	}
	for i, n := range names {
		if strings.TrimSpace(n) == "" {
			return fmt.Errorf("the class list has an empty class name (position %d)", i+1)
		}
		if slices.Contains(names[:i], n) {
			return fmt.Errorf("the class list has %q twice", n)
		}
	}
	return nil
}

// ParseLabelsJSON reads a labels.json: a JSON array of class names, in the
// order the model emits them.
func ParseLabelsJSON(data []byte) ([]string, error) {
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return nil, fmt.Errorf("labels.json must be a JSON array of strings: %w", err)
	}
	if err := ValidateClasses(names); err != nil {
		return nil, fmt.Errorf("labels.json: %w", err)
	}
	return names, nil
}

var namesEntry = regexp.MustCompile(`(\d+)\s*:\s*(?:'([^']*)'|"([^"]*)")`)

// ParseNamesMetadata reads the names entry that an Ultralytics export writes
// into the ONNX metadata, such as "{0: 'a', 1: 'b'}".
func ParseNamesMetadata(s string) ([]string, error) {
	ms := namesEntry.FindAllStringSubmatch(s, -1)
	if len(ms) == 0 {
		return nil, fmt.Errorf("no class names in the metadata %q", s)
	}
	names := make([]string, len(ms))
	seen := make([]bool, len(ms))
	for _, m := range ms {
		i, err := strconv.Atoi(m[1])
		if err != nil || i >= len(ms) || seen[i] {
			return nil, fmt.Errorf("the metadata class indexes must be 0..%d once each: %q", len(ms)-1, s)
		}
		seen[i] = true
		names[i] = m[2] + m[3]
	}
	if err := ValidateClasses(names); err != nil {
		return nil, fmt.Errorf("metadata: %w", err)
	}
	return names, nil
}

// ResolveClasses finds the class names of a model: labels.json beside the
// model, then the names entry of the ONNX metadata, then fallback. A labels.json
// that exists but is invalid is an error; unreadable metadata is not.
func ResolveClasses(model, lib string, fallback []string) ([]string, string, error) {
	if model != "" {
		if _, err := os.Stat(model); err == nil {
			p := filepath.Join(filepath.Dir(model), "labels.json")
			data, err := os.ReadFile(p) // #nosec G304 -- beside the user-supplied model
			switch {
			case err == nil:
				names, err := ParseLabelsJSON(data)
				if err != nil {
					return nil, "", fmt.Errorf("%s: %w", p, err)
				}
				return names, SourceJSON, nil
			case !errors.Is(err, fs.ErrNotExist):
				return nil, "", err
			}
			if lib != "" {
				if names := metadataClasses(model, lib); names != nil {
					return names, SourceMetadata, nil
				}
			}
		}
	}
	return slices.Clone(fallback), SourceDefault, nil
}
