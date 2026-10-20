// Package settings owns generic defaults and model class order.
package settings

type Config struct {
	MinBlock, BlockDivisor                       int
	DetectionLabels, SegmentationLabels, Targets []string
	MinScore, SureScore                          float32
	TagSure, TagVague                            float32
	SegMin, SegAlone                             float32
	SegMinCover                                  float64
	FaceThreshold                                float32
}

// Defaults uses midpoint confidence thresholds, without production calibration.
// Users must evaluate model results on their own inputs.
var Defaults = Config{
	MinBlock: 4, BlockDivisor: 100,
	DetectionLabels:    []string{"nipple_f", "penis", "pussy"},
	SegmentationLabels: []string{"anus", "nipple", "penis", "vagina", "female face", "male face", "pubic hair"},
	Targets:            []string{"penis", "pussy"},
	MinScore:           0.5, SureScore: 0.5, TagSure: 0.5, TagVague: 0.5,
	SegMin: 0.5, SegAlone: 0.5, SegMinCover: 0.5, FaceThreshold: 0.5,
}
