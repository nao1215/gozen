// Command files writes the list of the game's files (paths under DIR, slash-separated) as
// JSON to DIR/files.json, for the browser build, which fetches them before it starts.
//
//	go run ./web/files DIR
package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

func main() {
	if len(os.Args) != 2 {
		fail(2, "usage: files DIR")
	}
	dir := os.Args[1]
	var names []string
	err := fs.WalkDir(os.DirFS(dir), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || p == "files.json" {
			return err
		}
		names = append(names, p)
		return nil
	})
	if err == nil {
		sort.Strings(names)
		var b []byte
		if b, err = json.Marshal(names); err == nil {
			err = os.WriteFile(filepath.Join(dir, "files.json"), b, 0o600) //nolint:gosec // The directory the builder names.
		}
	}
	if err != nil {
		fail(1, err)
	}
}

func fail(code int, msg any) {
	if _, err := fmt.Fprintln(os.Stderr, msg); err != nil {
		os.Exit(2)
	}
	os.Exit(code)
}
