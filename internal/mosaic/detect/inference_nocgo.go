//go:build !cgo

package detect

import (
	"fmt"
	"image"
)

type Detector struct{}

func New(model, lib string, threads int) (*Detector, error) {
	return NewWithLabels(model, lib, threads, Labels)
}
func NewWithLabels(string, string, int, []string) (*Detector, error) {
	return nil, fmt.Errorf("ONNX inference requires a mosaic build with CGO_ENABLED=1 and a C compiler")
}
func (*Detector) Close() error { return nil }
func (*Detector) Detect(image.Image, []Pass, float32) ([]Box, error) {
	return nil, fmt.Errorf("ONNX inference is unavailable in this build")
}
