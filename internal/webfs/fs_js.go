//go:build js

// Package webfs provides the browser examples with a filesystem backed by game files
// served next to the page.
package webfs

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path"
	"slices"
	"strings"
	"sync"
	"syscall/js"
	"testing/fstest"
)

const workers = 20

type file struct {
	Name string `json:"name"`
	File string `json:"file"`
	Size int64  `json:"size"`
	// Gzip marks a file served gzipped (gozen export), which get undoes.
	Gzip bool `json:"gzip"`
	// Later marks a file Load does not fetch (a recorded stream voice): it is fetched when
	// the game opens it, and the progress leaves it out.
	Later bool `json:"later"`
}

// Load reads files.json and downloads its files with bounded concurrency into an
// in-memory filesystem, the pictures and the sound last. It returns once the other files
// (the game's YAML, its font) have come, so the game reads them while its media still
// come: opening a file that is still coming waits for it, and a file whose download
// failed is fetched again when the game opens it. While the files download, progress
// (when not nil) is called with the bytes of the files come so far and the bytes the
// manifest lists, as a file comes (at most every half percent) and once at the end. A file
// marked later (a recorded stream voice) is fetched only when the game opens it and is not
// in the progress.
func Load(base string, progress func(done, total int64)) (fs.FS, error) {
	b, err := fetch(base+"files.json", nil)
	if err != nil {
		return nil, err
	}
	var files []file
	if err := json.Unmarshal(b, &files); err != nil {
		return nil, fmt.Errorf("files.json: %w", err)
	}
	h := &httpFS{
		base: base, entries: fstest.MapFS{}, served: make(map[string]file, len(files)),
		cache: make(map[string][]byte, len(files)), coming: make(map[string]chan struct{}, len(files)),
	}
	counter := &count{report: progress}
	for _, f := range files {
		if !fs.ValidPath(f.Name) || !fs.ValidPath(f.File) || f.Size < 0 {
			return nil, fmt.Errorf("files.json: invalid entry %q", f.Name)
		}
		if _, exists := h.served[f.Name]; exists {
			return nil, fmt.Errorf("files.json: duplicate entry %q", f.Name)
		}
		h.served[f.Name] = f
		h.entries[f.Name] = &fstest.MapFile{Data: nil}
		if f.Later {
			continue
		}
		h.coming[f.Name] = make(chan struct{})
		counter.total += f.Size
	}
	files = slices.DeleteFunc(files, func(f file) bool { return f.Later })
	slices.SortStableFunc(files, func(a, b file) int { return cmp.Compare(media(a.Name), media(b.Name)) })
	jobs := make(chan file, len(files))
	var first, all sync.WaitGroup
	for _, f := range files {
		jobs <- f
		if media(f.Name) == 0 {
			first.Add(1)
		}
	}
	close(jobs)
	for range min(workers, len(files)) {
		all.Go(func() {
			for f := range jobs {
				data, err := get(base, f, counter)
				h.mu.Lock()
				if err == nil {
					h.cache[f.Name] = data
				}
				close(h.coming[f.Name])
				delete(h.coming, f.Name)
				h.mu.Unlock()
				if media(f.Name) == 0 {
					first.Done()
				}
			}
		})
	}
	go func() {
		all.Wait()
		counter.finish()
	}()
	first.Wait()
	return h, nil
}

// media is 1 for a picture or a sound, which Load fetches last: by the name the game reads,
// as the file served may be another (a picture as WebP, a sound gzipped).
func media(name string) int {
	switch strings.ToLower(path.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".ogg", ".mp3", ".wav":
		return 1
	}
	return 0
}

type httpFS struct {
	base    string
	entries fstest.MapFS
	served  map[string]file
	mu      sync.Mutex
	cache   map[string][]byte
	// coming holds the files Load still downloads, closed when one has come.
	coming map[string]chan struct{}
}

func (f *httpFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrInvalid
	}
	if served, ok := f.served[name]; ok {
		f.mu.Lock()
		data, cached := f.cache[name]
		coming := f.coming[name]
		f.mu.Unlock()
		if !cached && coming != nil {
			<-coming
			f.mu.Lock()
			data, cached = f.cache[name]
			f.mu.Unlock()
		}
		if !cached {
			var err error
			data, err = get(f.base, served, nil)
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

// count adds up the bytes the downloads of Load have read and reports them.
type count struct {
	mu          sync.Mutex
	done, shown int64
	total       int64
	report      func(done, total int64)
}

func (c *count) add(n int) {
	if c == nil || c.report == nil || n == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.done += int64(n)
	if c.done-c.shown >= c.total/200 {
		c.shown = c.done
		c.report(c.done, c.total)
	}
}

// finish reports the end, the failed downloads left out.
func (c *count) finish() {
	if c.report != nil {
		c.report(c.done, c.total)
	}
}

// get downloads f from base, counting the bytes that come into c when c is not nil.
func get(base string, f file, c *count) ([]byte, error) {
	b, err := fetch(base+(&url.URL{Path: path.Clean(f.File)}).EscapedPath(), c)
	// A host may already have undone the gzip (Content-Encoding); then it is not gzip.
	if err != nil || !f.Gzip || !bytes.HasPrefix(b, []byte{0x1f, 0x8b}) {
		return b, err
	}
	z, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", f.File, err)
	}
	if b, err = io.ReadAll(z); err != nil {
		return nil, fmt.Errorf("%s: %w", f.File, err)
	}
	return b, nil
}

// fetch downloads u, adding its bytes to c when c is not nil. The browser reads the
// whole of it (arrayBuffer) while the game goes on: read in Go, a file came only as fast
// as the game, busy reading the files before it, took its pieces.
func fetch(u string, c *count) ([]byte, error) {
	resp, err := await(js.Global().Call("fetch", u))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", u, err)
	}
	if !resp.Get("ok").Bool() {
		return nil, fmt.Errorf("%s: %d %s", u, resp.Get("status").Int(), resp.Get("statusText").String())
	}
	buf, err := await(resp.Call("arrayBuffer"))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", u, err)
	}
	b := make([]byte, buf.Get("byteLength").Int())
	js.CopyBytesToGo(b, js.Global().Get("Uint8Array").New(buf))
	c.add(len(b))
	return b, nil
}

// await waits for the promise p to settle (other goroutines run meanwhile).
func await(p js.Value) (js.Value, error) {
	type result struct {
		v   js.Value
		err error
	}
	ch := make(chan result, 1)
	ok := js.FuncOf(func(_ js.Value, args []js.Value) any {
		ch <- result{v: args[0]}
		return nil
	})
	fail := js.FuncOf(func(_ js.Value, args []js.Value) any {
		ch <- result{err: errors.New(args[0].Call("toString").String())}
		return nil
	})
	defer ok.Release()
	defer fail.Release()
	p.Call("then", ok, fail)
	r := <-ch
	return r.v, r.err
}
