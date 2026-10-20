//go:build js

// Command novel runs the novel sample in the browser: the game files are fetched next to
// the page and the saves go into localStorage.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"syscall/js"
	"testing/fstest"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen"
	"github.com/nao1215/gozen/play"
)

// In a browser the game's files are not in the program: the page serves them next to it
// (game/files.json lists them, written by Go Zen's web/assets.py with the pictures as
// WebP), and they are fetched once before the game starts, while the page shows how much
// is left. The game runs as a gozen.Session on a play.Screen; the saves go to the page's
// localStorage. The page's functions keep the names of Go Zen's page (goZenProgress and
// the rest), which scripts/web_play_check.mjs also reads.
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
	ebiten.SetWindowTitle("After the Pass")
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

// fetchGame reads the list base+"files.json" and every file in it, a few at a time.
func fetchGame(base string) (fstest.MapFS, error) {
	var files []file
	b, err := get(base + "files.json")
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &files); err != nil {
		return nil, fmt.Errorf("files.json: %w", err)
	}
	var total int64
	for _, f := range files {
		total += f.Size
	}
	const workers = 6
	var (
		mu   sync.Mutex
		done int64
		errs []error
		wg   sync.WaitGroup
	)
	next := make(chan file)
	fsys := fstest.MapFS{}
	page("goZenProgress", 0, float64(total))
	for range workers {
		wg.Go(func() {
			for f := range next {
				b, err := get(base + (&url.URL{Path: f.File}).EscapedPath())
				mu.Lock()
				if err != nil {
					errs = append(errs, err)
				} else {
					fsys[f.Name] = &fstest.MapFile{Data: b}
					done += int64(len(b))
					page("goZenProgress", float64(done), float64(total))
				}
				mu.Unlock()
			}
		})
	}
	for _, f := range files {
		next <- f
	}
	close(next)
	wg.Wait()
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return fsys, nil
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
