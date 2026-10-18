//go:build !cgo

package tagger

import (
	"fmt"
	"image"
)

type Tagger struct{}

func New(string, string, int) (*Tagger, error) {
	return nil, fmt.Errorf("tagging requires CGO_ENABLED=1 and a C compiler")
}
func (*Tagger) Close() error { return nil }
func (*Tagger) Tags(image.Image) (map[string]float32, error) {
	return nil, fmt.Errorf("tagging is unavailable in this build")
}
