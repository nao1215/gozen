// Package picturelimit bounds image input before the model checker or renderer allocates pixels.
// It belongs to layer 1 and leaves decoder registration to its callers.
package picturelimit

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"io/fs"
)

// MaxBytes bounds encoded pictures, including metadata, read from a game's file system.
const MaxBytes = 64 << 20

// Dimensions checks the shared CPU image budget: at most 8192 per side and
// 16 million pixels (64 MiB for RGBA). Backend texture support can be lower.
func Dimensions(w, h int) error {
	if w <= 0 || h <= 0 || w > 8192 || h > 8192 || w > (16<<20)/h {
		return fmt.Errorf("picture dimensions %dx%d exceed limits (8192 per side, 16777216 pixels)", w, h)
	}
	return nil
}

// Header validates size before either a Go or browser decoder allocates pixels.
func Header(b []byte) error {
	if len(b) > MaxBytes {
		return fmt.Errorf("picture exceeds %d encoded bytes", MaxBytes)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return err
	}
	return Dimensions(cfg.Width, cfg.Height)
}

// Read bounds reads even for a file system whose Stat size is unavailable or inaccurate.
func Read(fsys fs.FS, name string) ([]byte, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	if info, statErr := f.Stat(); statErr == nil && info.Size() > MaxBytes {
		if err := f.Close(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("picture exceeds %d encoded bytes", MaxBytes)
	}
	b, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	closeErr := f.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(b) > MaxBytes {
		return nil, fmt.Errorf("picture exceeds %d encoded bytes", MaxBytes)
	}
	return b, nil
}
