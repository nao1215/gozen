//go:build js

// Command go-zen runs Go Zen in the browser: the game files are fetched next to
// the page and the saves go into localStorage.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"path"
	"sync"
	"syscall/js"
	"testing/fstest"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen"
	"github.com/nao1215/gozen/play"
)

// In a browser the game's files are not in the program: the page serves them next to it
// (game/files.json lists them, written by web/assets.py with the pictures as WebP), and
// files are fetched when opened so unused pictures do not delay the first screen.
// The game runs as a gozen.Session on a play.Screen; the saves go to the page's
// localStorage.
func main() {
	if err := run(); err != nil {
		page("goZenFail", err.Error())
		log.Fatal(err)
	}
}

func run() error {
	fsys, err := fetchGame("game/")
	if err != nil {
		return err
	}
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		return err
	}
	s, err := gozen.New(g, gozen.Config{})
	if err != nil {
		return err
	}
	sc, err := play.New(s, play.Config{})
	if err != nil {
		return err
	}
	defer func() {
		if err := sc.Close(); err != nil {
			log.Print(err)
		}
	}()
	ebiten.SetWindowTitle("Go Zen")
	page("goZenStarting")
	return ebiten.RunGame(&shown{Screen: sc})
}

// shown is the game's screen that takes the page's loading note away once the first
// frame is drawn: the first frames read the pictures and can take some seconds, and
// until then the canvas is black.
type shown struct {
	*play.Screen
	done bool
}

func (s *shown) Draw(dst *ebiten.Image) {
	s.Screen.Draw(dst)
	if !s.done {
		s.done = true
		page("goZenReady")
	}
}

// page calls a function of index.html when the page has it.
func page(name string, args ...any) {
	if f := js.Global().Get(name); f.Type() == js.TypeFunction {
		f.Invoke(args...)
	}
}

// file is one entry of files.json: the name the game reads, the file the page serves
// under it (a picture may be served as WebP) and that file's size.
type file struct {
	Name string `json:"name"`
	File string `json:"file"`
	Size int64  `json:"size"`
}

// fetchGame reads the small manifest and returns a filesystem that fetches each file
// only when Gozen opens it. Directory entries come from the manifest.
func fetchGame(base string) (fs.FS, error) {
	var files []file
	b, err := get(base + "files.json")
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &files); err != nil {
		return nil, fmt.Errorf("files.json: %w", err)
	}
	entries := fstest.MapFS{}
	served := make(map[string]string, len(files))
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
	return &httpFS{base: base, entries: entries, served: served, cache: map[string][]byte{}}, nil
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
			if prior, ok := f.cache[name]; ok {
				data = prior
			} else {
				f.cache[name] = data
			}
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
