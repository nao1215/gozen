//go:build !cgo

package tagger

import (
	"fmt"
	"image"
)

type Tagger struct{}

func New(string, string, int) (*Tagger, error) {
	return nil, fmt.Errorf("this gozen build has no ONNX tagging (built with CGO_ENABLED=0); reinstall with a C compiler: CGO_ENABLED=1 go install github.com/nao1215/gozen/cmd/gozen@latest")
}
func (*Tagger) Close() error { return nil }
func (*Tagger) Tags(image.Image) (map[string]float32, error) {
	return nil, fmt.Errorf("this gozen build has no ONNX tagging (built with CGO_ENABLED=0)")
}
