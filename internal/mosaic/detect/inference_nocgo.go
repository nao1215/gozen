//go:build !cgo

package detect

import (
	"errors"
	"image"
)

type Detector struct{}

func New(model, lib string, threads int) (*Detector, error) {
	return NewWithLabels(model, lib, threads, Labels)
}

func NewWithLabels(string, string, int, []string) (*Detector, error) {
	return nil, errors.New("this gozen build has no ONNX inference (built with CGO_ENABLED=0); reinstall with a C compiler: CGO_ENABLED=1 go install github.com/nao1215/gozen/cmd/gozen@latest")
}
func metadataClasses(string, string) []string { return nil }
func (*Detector) Close() error                { return nil }
func (*Detector) Detect(image.Image, []Pass, float32) ([]Box, error) {
	return nil, errors.New("this gozen build has no ONNX inference (built with CGO_ENABLED=0)")
}
