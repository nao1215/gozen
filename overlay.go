package gozen

import (
	"errors"
	"io/fs"
	"slices"
	"sort"
)

// layered overlays the files of the upper FS on the lower FS (the upper wins for the same name; directory contents are merged).
type layered struct{ layers []fs.FS } // later layers are higher

func (l layered) Open(name string) (fs.File, error) {
	var err error
	for _, v := range slices.Backward(l.layers) {
		var f fs.File
		if f, err = v.Open(name); err == nil || !errors.Is(err, fs.ErrNotExist) {
			return f, err // ReadDir merges directory contents from all layers
		}
	}
	return nil, err
}

func (l layered) ReadDir(name string) ([]fs.DirEntry, error) {
	seen := map[string]fs.DirEntry{}
	found := false
	for _, layer := range l.layers {
		entries, err := fs.ReadDir(layer, name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		found = true
		for _, e := range entries {
			seen[e.Name()] = e
		}
	}
	if !found {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	out := make([]fs.DirEntry, 0, len(seen))
	for _, e := range seen {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}
