package mosaic

import "image"

type Mask struct{ *image.Alpha }

func (m Mask) Bounds() image.Rectangle {
	r := m.Rect
	minX, minY, maxX, maxY := r.Max.X, r.Max.Y, r.Min.X, r.Min.Y
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := m.Pix[m.PixOffset(r.Min.X, y) : m.PixOffset(r.Min.X, y)+r.Dx()]
		for i, v := range row {
			if v != 0 {
				x := r.Min.X + i
				minX, maxX = min(minX, x), max(maxX, x+1)
				minY, maxY = min(minY, y), max(maxY, y+1)
			}
		}
	}
	if minX >= maxX {
		return image.Rectangle{}
	}
	return image.Rect(minX, minY, maxX, maxY)
}

func (m Mask) Hits(cell image.Rectangle) bool {
	c := cell.Intersect(m.Rect)
	for y := c.Min.Y; y < c.Max.Y; y++ {
		o := m.PixOffset(c.Min.X, y)
		for _, v := range m.Pix[o : o+c.Dx()] {
			if v != 0 {
				return true
			}
		}
	}
	return false
}

func (m Mask) Area(r image.Rectangle) int {
	if r.Empty() {
		r = m.Rect
	}
	r = r.Intersect(m.Rect)
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		o := m.PixOffset(r.Min.X, y)
		for _, v := range m.Pix[o : o+r.Dx()] {
			if v != 0 {
				n++
			}
		}
	}
	return n
}

func (m Mask) Dilate(d int, bounds image.Rectangle) Mask {
	d = max(d, 0)
	r := m.Rect.Inset(-d).Intersect(bounds)
	on := func(x, y int) bool { return (image.Point{x, y}).In(m.Rect) && m.Pix[m.PixOffset(x, y)] != 0 }
	tmp := image.NewAlpha(r)
	count := make([]int, r.Dx()+2*d+1)
	for y := r.Min.Y; y < r.Max.Y; y++ {

		for i := range r.Dx() + 2*d {
			v := 0
			if on(r.Min.X-d+i, y) {
				v = 1
			}
			count[i+1] = count[i] + v
		}
		for x := r.Min.X; x < r.Max.X; x++ {
			i := x - r.Min.X
			if count[i+2*d+1]-count[i] > 0 {
				tmp.Pix[tmp.PixOffset(x, y)] = 255
			}
		}
	}
	out := image.NewAlpha(r)
	col := make([]int, r.Dy()+1)
	for x := r.Min.X; x < r.Max.X; x++ {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			v := 0
			if tmp.Pix[tmp.PixOffset(x, y)] != 0 {
				v = 1
			}
			col[y-r.Min.Y+1] = col[y-r.Min.Y] + v
		}
		for y := r.Min.Y; y < r.Max.Y; y++ {
			lo, hi := max(0, y-r.Min.Y-d), min(r.Dy(), y-r.Min.Y+d+1)
			if col[hi]-col[lo] > 0 {
				out.Pix[out.PixOffset(x, y)] = 255
			}
		}
	}
	return Mask{out}
}

func (m Mask) Clip(r image.Rectangle) Mask {
	r = r.Intersect(m.Rect)
	out := image.NewAlpha(r)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		copy(out.Pix[out.PixOffset(r.Min.X, y):out.PixOffset(r.Min.X, y)+r.Dx()], m.Pix[m.PixOffset(r.Min.X, y):m.PixOffset(r.Min.X, y)+r.Dx()])
	}
	return Mask{out}
}
