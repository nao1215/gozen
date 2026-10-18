//go:build !cgo

package segment

import (
	"fmt"
	"image"
)

type YoloSeg struct{}

func NewYoloSeg(string, string, int, int, []string) (*YoloSeg, error) {
	return nil, fmt.Errorf("segmentation requires CGO_ENABLED=1 and a C compiler")
}
func (*YoloSeg) Close() error { return nil }
func (*YoloSeg) Find(image.Image, float32, bool) ([]Object, error) {
	return nil, fmt.Errorf("segmentation is unavailable in this build")
}
