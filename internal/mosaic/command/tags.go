package command

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/nao1215/gozen/internal/mosaic"
	"github.com/nao1215/gozen/internal/mosaic/tagger"
)

type tagCache struct {
	readOnly bool
	diag     *output

	path    string
	entries map[string]tagEntry
	dirty   bool
	t       *tagger.Tagger
	dir     string
	lib     string
	threads int
	noModel bool
}

type tagEntry struct {
	Stamp string             `json:"stamp"`
	Tags  map[string]float32 `json:"tags"`
}

func loadTagCache(path, dir, lib string, threads int) (*tagCache, error) {
	tc := &tagCache{path: path, entries: map[string]tagEntry{}, dir: dir, lib: lib, threads: threads}
	if path == "" {
		return tc, nil
	}
	b, err := os.ReadFile(path) //nolint:gosec // The caller explicitly selects this input file.
	if os.IsNotExist(err) {
		return tc, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &tc.entries); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return tc, nil
}

func stamp(path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d|%d", st.ModTime().Unix(), st.Size()), nil
}

func (tc *tagCache) get(rel, file string) (map[string]float32, error) {
	if tc == nil || tc.path == "" {
		return nil, nil
	}
	if rel == "" {
		abs, err := filepath.Abs(file)
		if err != nil {
			return nil, err
		}
		rel = abs
	}
	st, err := stamp(file)
	if err != nil {
		return nil, err
	}
	if e, ok := tc.entries[rel]; ok && e.Stamp == st {
		return e.Tags, nil
	}
	if tc.noModel || tc.readOnly {
		return nil, nil
	}
	if tc.t == nil {
		if _, err := os.Stat(filepath.Join(tc.dir, "model.onnx")); err != nil {
			if tc.dir != "" {
				return nil, fmt.Errorf("tagger model: %w", err)
			}
			tc.diag.printf("warning: タガーのモデル %s が無いので、タグ無しで続ける（-tagger で場所を変えられる）\n", tc.dir)
			tc.noModel = true
			return nil, nil
		}
		if tc.t, err = tagger.New(tc.dir, tc.lib, tc.threads); err != nil {
			return nil, err
		}
	}
	img, err := decode(file)
	if err != nil {
		return nil, err
	}
	tags, err := tc.t.Tags(img)
	if err != nil {
		return nil, err
	}
	tc.entries[rel] = tagEntry{Stamp: st, Tags: tags}
	tc.dirty = true
	return tags, nil
}

func (tc *tagCache) save() error {
	if tc == nil || tc.readOnly || !tc.dirty || tc.path == "" {
		return nil
	}
	b, err := json.Marshal(tc.entries)
	if err != nil {
		return err
	}
	if err := mosaic.WriteFile(tc.path, func(w io.Writer) error { _, err := w.Write(b); return err }); err != nil {
		return err
	}
	tc.dirty = false
	return nil
}

func (tc *tagCache) close() error {
	if tc != nil && tc.t != nil {
		return tc.t.Close()
	}
	return nil
}
