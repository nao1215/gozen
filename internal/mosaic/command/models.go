package command

import (
	"image"

	"github.com/nao1215/gozen/internal/mosaic/detect"
	"github.com/nao1215/gozen/internal/mosaic/segment"
	"github.com/nao1215/gozen/internal/mosaic/tagger"
)

// The three kinds of ONNX model the command uses, as the command sees them. The real
// ones (detect.Detector, segment.YoloSeg, tagger.Tagger) need the ONNX Runtime; these
// interfaces let the planning, review and report code run without it.

// boxDetector finds labeled boxes in a picture.
type boxDetector interface {
	Detect(img image.Image, passes []detect.Pass, minScore float32) ([]detect.Box, error)
	Close() error
}

// segmenter outlines objects with masks.
type segmenter interface {
	Find(img image.Image, minScore float32, flip bool) ([]segment.Object, error)
	Close() error
}

// tagModel gives the tags of a picture with their scores.
type tagModel interface {
	Tags(img image.Image) (map[string]float32, error)
	Close() error
}

// How the command opens the models. They are variables so that tests can put in models
// that need no ONNX Runtime.
var (
	newDetector = func(model, lib string, threads int, labels []string) (boxDetector, error) {
		d, err := detect.NewWithLabels(model, lib, threads, labels)
		if err != nil {
			return nil, err
		}
		return d, nil
	}
	newSegmenter = func(model, lib string, threads, size int, labels []string) (segmenter, error) {
		s, err := segment.NewYoloSeg(model, lib, threads, size, labels)
		if err != nil {
			return nil, err
		}
		return s, nil
	}
	newTagger = func(dir, lib string, threads int) (tagModel, error) {
		t, err := tagger.New(dir, lib, threads)
		if err != nil {
			return nil, err
		}
		return t, nil
	}
)
