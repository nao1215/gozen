//go:build !js

package play

import (
	"github.com/nao1215/gozen"
	"github.com/nao1215/gozen/internal/savefile"
)

// defaultStore is gozen/<ID> in the OS user's configuration directory.
func defaultStore(id string) (Store, error) {
	dir, err := savefile.Prepare(id, savefile.Config{})
	if err != nil {
		return nil, err
	}
	return gozen.DirStore(dir), nil
}
