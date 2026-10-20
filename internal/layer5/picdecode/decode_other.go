//go:build !js

package picdecode

import "image"

// Decode decodes the picture b with Go's decoders.
func Decode(b []byte) (image.Image, error) { return goDecode(b) }

// BrowserAfter does nothing on the desktop, where Go decodes every picture.
func BrowserAfter(<-chan struct{}) {}
