//go:build !windows

package gamepackage

import "os"

func openCacheFile(name string) (*os.File, error) {
	return os.Open(name) // #nosec G304 -- Digest-derived filename under caller-owned cache root.
}
