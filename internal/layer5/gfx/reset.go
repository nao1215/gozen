package gfx

// Reset drops cached GPU resources between runs; use only on the game thread.
func Reset() {
	for _, img := range shapeCache {
		img.Deallocate()
	}
	clear(shapeCache)
	for _, t := range textCache {
		t.img.Deallocate()
	}
	clear(textCache)
	for _, img := range maskCache {
		img.Deallocate()
	}
	clear(maskCache)
	clear(faceCache)
	clear(popFaces)
	regularSource, popSource, titleSource, subSource = nil, nil, nil, nil
	scale = 1
}
