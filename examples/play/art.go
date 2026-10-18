package main

import (
	"image"
	"image/color"
	"image/draw"
)

// artwork draws every sample picture on a small pixel grid. No external image
// files, model output, or game assets are needed, including at build time.
func artwork() map[string]image.Image {
	return map[string]image.Image{
		"landscape.png": landscape(false),
		"ridge.png":     landscape(true),
		"pebble.png":    pebble(),
		"courier.png":   courier(),
	}
}

func rgb(r, g, b uint8) color.NRGBA { return color.NRGBA{R: r, G: g, B: b, A: 255} }

func box(img *image.NRGBA, x, y, w, h int, c color.NRGBA) {
	draw.Draw(img, image.Rect(x, y, x+w, y+h), image.NewUniform(c), image.Point{}, draw.Src)
}

func disc(img *image.NRGBA, cx, cy, radius int, c color.NRGBA) {
	for y := cy - radius; y <= cy+radius; y++ {
		for x := cx - radius; x <= cx+radius; x++ {
			if (x-cx)*(x-cx)+(y-cy)*(y-cy) <= radius*radius {
				img.SetNRGBA(x, y, c)
			}
		}
	}
}

func landscape(ridge bool) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 320, 180))
	for y := range 180 {
		// A dusk gradient, with a clear horizon behind the lantern tower.
		box(img, 0, y, 320, 1, rgb(uint8(28+y/5), uint8(53+y/4), uint8(73+y/5)))
	}
	disc(img, 249, 41, 19, rgb(239, 210, 151))
	disc(img, 242, 36, 19, rgb(35, 62, 80))
	for _, p := range []image.Point{{182, 22}, {216, 15}, {286, 31}, {302, 12}, {160, 38}} {
		box(img, p.X, p.Y, 1, 2, rgb(211, 216, 186))
	}
	for x := range 320 {
		back := 89 + (x/17)%3*4
		box(img, x, back, 1, 180-back, rgb(49, 83, 93))
		front := 125 + (x-210)*(x-210)/3500
		box(img, x, front, 1, 180-front, rgb(43, 73, 70))
	}
	// The winding path and square stones are original geometric pixel art.
	for y := 125; y < 180; y++ {
		x := 220 - (y-125)*2
		box(img, x, y, 6+(y-125)/3, 1, rgb(140, 143, 112))
	}
	for i := range 16 {
		x, y := 24+i*19, 150+(i*7)%24
		box(img, x, y, 4, 2, rgb(75, 106, 87))
		box(img, x+2, y-3, 1, 3, rgb(113, 141, 105))
	}
	// A compact stone tower with a warm lantern and a sloped roof.
	box(img, 232, 84, 34, 47, rgb(94, 123, 124))
	box(img, 237, 87, 22, 44, rgb(118, 144, 139))
	for i := range 14 {
		box(img, 228+i, 72-i/2, 42-2*i, 1, rgb(36, 57, 69))
	}
	box(img, 231, 73, 36, 10, rgb(36, 57, 69))
	box(img, 240, 86, 15, 15, rgb(240, 188, 102))
	box(img, 247, 86, 2, 15, rgb(71, 89, 88))
	box(img, 242, 113, 10, 18, rgb(39, 61, 67))
	for i := range 5 {
		box(img, 237, 106+i*5, 4, 1, rgb(72, 101, 105))
	}
	if !ridge {
		box(img, 194, 127, 3, 16, rgb(47, 56, 58))
		box(img, 190, 122, 11, 7, rgb(242, 196, 116))
		box(img, 193, 123, 5, 4, rgb(255, 230, 157))
	} else {
		box(img, 290, 96, 3, 42, rgb(46, 62, 67))
		box(img, 284, 93, 15, 12, rgb(240, 188, 102))
		box(img, 288, 94, 7, 9, rgb(255, 231, 163))
	}
	return img
}

func pebble() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 96, 96))
	disc(img, 48, 53, 31, rgb(48, 78, 88))
	disc(img, 46, 48, 29, rgb(107, 158, 164))
	disc(img, 42, 41, 22, rgb(140, 185, 183))
	box(img, 30, 43, 5, 8, rgb(27, 54, 65))
	box(img, 56, 43, 5, 8, rgb(27, 54, 65))
	box(img, 39, 61, 13, 3, rgb(62, 103, 112))
	box(img, 28, 70, 10, 7, rgb(89, 137, 146))
	box(img, 59, 70, 10, 7, rgb(89, 137, 146))
	return img
}

func courier() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 96, 144))
	box(img, 27, 77, 42, 42, rgb(56, 104, 110))
	box(img, 33, 116, 11, 18, rgb(36, 59, 75))
	box(img, 53, 116, 11, 18, rgb(36, 59, 75))
	box(img, 22, 131, 22, 7, rgb(42, 53, 60))
	box(img, 53, 131, 22, 7, rgb(42, 53, 60))
	box(img, 32, 38, 32, 38, rgb(221, 180, 135))
	box(img, 26, 30, 44, 18, rgb(207, 162, 90))
	box(img, 18, 44, 60, 7, rgb(232, 193, 123))
	box(img, 38, 55, 3, 4, rgb(39, 55, 63))
	box(img, 55, 55, 3, 4, rgb(39, 55, 63))
	box(img, 32, 74, 34, 9, rgb(234, 188, 103))
	box(img, 65, 87, 15, 24, rgb(168, 127, 84))
	box(img, 57, 79, 4, 30, rgb(168, 127, 84))
	return img
}
