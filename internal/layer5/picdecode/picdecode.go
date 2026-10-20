// Package picdecode decodes the game's pictures (PNG, JPEG and WebP). On the
// desktop it is image.Decode. In a browser build the browser decodes them when it
// can (decode_js.go), so a picture coming in does not hold the page still.
package picdecode

import (
	"bytes"
	"image"
	_ "image/jpeg" // the decoders image.Decode uses
	_ "image/png"

	_ "golang.org/x/image/webp" // a browser build ships its pictures as WebP
)

// goDecode decodes b with Go's decoders.
func goDecode(b []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(b))
	return img, err
}

// browserKind reports whether b is a WebP or a JPEG, the pictures a browser build hands
// to the browser to decode.
func browserKind(b []byte) bool {
	webp := len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP"
	jpeg := len(b) >= 3 && b[0] == 0xff && b[1] == 0xd8 && b[2] == 0xff
	return webp || jpeg
}
