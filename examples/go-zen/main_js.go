//go:build js

// Command go-zen runs Go Zen in the browser: the game files are fetched next to
// the page and the saves go into localStorage.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"testing/fstest"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen"
	"github.com/nao1215/gozen/play"
)

// In a browser the game's files are not in the program: the page serves them next to it
// (game/files.json lists them), and they are fetched once before the game starts. The
// game runs as a gozen.Session on a play.Screen; the saves go to the page's localStorage.
func main() {
	fsys, err := fetchGame("game/")
	if err != nil {
		log.Fatal(err)
	}
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		log.Fatal(err)
	}
	s, err := gozen.New(g, gozen.Config{})
	if err != nil {
		log.Fatal(err)
	}
	sc, err := play.New(s, play.Config{})
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := sc.Close(); err != nil {
			log.Print(err)
		}
	}()
	ebiten.SetWindowTitle("Go Zen")
	if err := ebiten.RunGame(sc); err != nil {
		log.Print(err)
	}
}

// fetchGame reads the list base+"files.json" and every file in it.
func fetchGame(base string) (fstest.MapFS, error) {
	var names []string
	b, err := get(base + "files.json")
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &names); err != nil {
		return nil, fmt.Errorf("files.json: %w", err)
	}
	fsys := fstest.MapFS{}
	for _, n := range names {
		b, err := get(base + (&url.URL{Path: n}).EscapedPath())
		if err != nil {
			return nil, err
		}
		fsys[n] = &fstest.MapFile{Data: b}
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
