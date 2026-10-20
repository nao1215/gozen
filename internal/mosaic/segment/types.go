package segment

import (
	"image"

	"github.com/nao1215/gozen/internal/mosaic/settings"
)

var SegLabels = settings.Defaults.SegmentationLabels

type Object struct {
	Label string
	Score float32
	Rect  image.Rectangle
	Mask  *image.Alpha
}
