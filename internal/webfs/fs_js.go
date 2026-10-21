//go:build js

// Package webfs provides the browser examples with a filesystem backed by game files
// served next to the page.
package webfs

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"sync"
	"testing/fstest"
)

const workers = 20

type file struct {
	Name string `json:"name"`
	File string `json:"file"`
	Size int64  `json:"size"`
}

// Load reads files.json, downloads its files with bounded concurrency, and returns an
// in-memory filesystem. Open can still fetch an entry that was not included in the cache.
func Load(base string) (fs.FS, error) {
	b, err := get(base + "files.json")
	if err != nil {
		return nil, err
	}
	var files []file
	if err := json.Unmarshal(b, &files); err != nil {
		return nil, fmt.Errorf("files.json: %w", err)
	}
	served := make(map[string]string, len(files))
	entries := fstest.MapFS{}
	for _, f := range files {
		if !fs.ValidPath(f.Name) || !fs.ValidPath(f.File) || f.Size < 0 {
			return nil, fmt.Errorf("files.json: invalid entry %q", f.Name)
		}
		if _, exists := served[f.Name]; exists {
			return nil, fmt.Errorf("files.json: duplicate entry %q", f.Name)
		}
		served[f.Name] = f.File
		entries[f.Name] = &fstest.MapFile{Data: nil}
	}

	type result struct {
		name string
		data []byte
		err  error
	}
	jobs := make(chan file)
	results := make(chan result, len(files))
	var wg sync.WaitGroup
	count := workers
	if len(files) < count {
		count = len(files)
	}
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range jobs {
				u := base + (&url.URL{Path: path.Clean(f.File)}).EscapedPath()
				data, err := get(u)
				results <- result{name: f.Name, data: data, err: err}
			}
		}()
	}
	go func() {
		for _, f := range files {
			jobs <- f
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	cache := make(map[string][]byte, len(files))
	for result := range results {
		if result.err != nil {
			// A failed speculative download is retried if the game later opens the file.
			continue
		}
		cache[result.name] = result.data
	}
	return &httpFS{base: base, entries: entries, served: served, cache: cache}, nil
}

type httpFS struct {
	base    string
	entries fstest.MapFS
	served  map[string]string
	mu      sync.Mutex
	cache   map[string][]byte
}

func (f *httpFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrInvalid
	}
	if served, ok := f.served[name]; ok {
		f.mu.Lock()
		data, cached := f.cache[name]
		f.mu.Unlock()
		if !cached {
			var err error
			data, err = get(f.base + (&url.URL{Path: path.Clean(served)}).EscapedPath())
			if err != nil {
				return nil, err
			}
			f.mu.Lock()
			f.cache[name] = data
			f.mu.Unlock()
		}
		return fstest.MapFS{name: &fstest.MapFile{Data: data}}.Open(name)
	}
	return f.entries.Open(name)
}

func get(u string) (b []byte, err error) {
	resp, err := http.Get(u) //nolint:gosec,noctx // The page's own files, next to the program.
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := resp.Body.Close(); err == nil {
			err = cerr
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", u, resp.Status)
	}
	return io.ReadAll(resp.Body)
}
