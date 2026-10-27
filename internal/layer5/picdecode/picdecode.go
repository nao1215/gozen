// Package picdecode decodes the game's pictures (PNG, JPEG and WebP). On the
// desktop it is image.Decode. In a browser build the browser decodes them when it
// can (decode_js.go), so a picture coming in does not hold the page still.
package picdecode

import (
	"bytes"
	"image"
	_ "image/png" // game pictures include PNG

	_ "image/jpeg" // the decoders image.Decode uses

	"github.com/nao1215/gozen/internal/layer1/picturelimit"

	_ "github.com/deepteams/webp" // a browser build ships its pictures as WebP
)

// goDecode decodes b with Go's decoders.
func goDecode(b []byte) (image.Image, error) {
	if err := picturelimit.Header(b); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	return img, err
}

// Size checks b's header as Decode does and returns the picture's width and height
// without decoding its pixels. A run that never draws (play.Simulate) needs only that.
func Size(b []byte) (image.Point, error) {
	if err := picturelimit.Header(b); err != nil {
		return image.Point{}, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	return image.Pt(cfg.Width, cfg.Height), err
}

// browserKind reports whether b is a WebP or a JPEG, the pictures a browser build hands
// to the browser to decode.
func browserKind(b []byte) bool {
	webp := len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP"
	jpeg := len(b) >= 3 && b[0] == 0xff && b[1] == 0xd8 && b[2] == 0xff
	return webp || jpeg
}
