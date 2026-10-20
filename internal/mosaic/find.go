package mosaic

import "image"

type Found struct {
	Mosaic bool
	Block  int
	Flat   float64
}

// Find recognizes exact repeated square cells. Lossy encodings may not match;
// inspection exposes this as a hint, never a verification of image suitability.
func Find(img image.Image, r image.Rectangle) Found {
	r = r.Intersect(img.Bounds())
	for block := 4; block <= min(r.Dx(), r.Dy())/3; block++ {
		for oy := range block {
			for ox := range block {
				var previous [4]uint32
				distinct := false
				count := 0
				ok := true
				for y := r.Min.Y + oy; y+block <= r.Max.Y && ok; y += block {
					for x := r.Min.X + ox; x+block <= r.Max.X && ok; x += block {
						rr, gg, bb, aa := img.At(x, y).RGBA()
						v := [4]uint32{rr, gg, bb, aa}
						if count > 0 && v != previous {
							distinct = true
						}
						previous = v
						count++
						for yy := y; yy < y+block && ok; yy++ {
							for xx := x; xx < x+block; xx++ {
								a, b, c, d := img.At(xx, yy).RGBA()
								if [4]uint32{a, b, c, d} != v {
									ok = false
									break
								}
							}
						}
					}
				}
				if ok && distinct && count >= 9 {
					return Found{true, block, 1}
				}
			}
		}
	}
	return Found{}
}
