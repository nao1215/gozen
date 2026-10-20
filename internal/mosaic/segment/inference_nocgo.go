//go:build !cgo

package segment

import (
	"errors"
	"image"
)

type YoloSeg struct{}

func NewYoloSeg(string, string, int, int, []string) (*YoloSeg, error) {
	return nil, errors.New("this gozen build has no ONNX segmentation (built with CGO_ENABLED=0); reinstall with a C compiler: CGO_ENABLED=1 go install github.com/nao1215/gozen/cmd/gozen@latest")
}
func (*YoloSeg) Close() error { return nil }
func (*YoloSeg) Find(image.Image, float32, bool) ([]Object, error) {
	return nil, errors.New("this gozen build has no ONNX segmentation (built with CGO_ENABLED=0)")
}
