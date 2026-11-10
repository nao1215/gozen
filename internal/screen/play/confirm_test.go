package play

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/screen/gfx"
)

func TestConfirmFitsPortraitAndLandscape(t *testing.T) {
	for _, dims := range []image.Point{image.Pt(960, 540), image.Pt(360, 540), image.Pt(640, 360)} {
		t.Run(fmt.Sprintf("%dx%d", dims.X, dims.Y), func(t *testing.T) {
			g, _ := mnuMenu(t)
			g.view.w, g.view.h = dims.X, dims.Y
			const warning = "Loading this saved game will replace your current progress. Any progress since your last save will be lost. Do you want to continue?"
			accepted, refused := 0, 0
			c := newConfirm(warning, func() { accepted++ })
			c.onNo = func() { refused++ }
			lines, size, y, h, yes, no := c.layout(g)
			if y < 0 || y+h > float64(dims.Y) {
				t.Fatalf("dialog outside screen: y=%v h=%v", y, h)
			}
			for _, line := range lines {
				if gfx.TextWidth(line, size) > float64(dims.X-40) {
					t.Fatalf("warning outside screen: %q", line)
				}
			}
			if dims.X < 720 && strings.Contains(strings.Join(lines, ""), "…") {
				t.Fatal("compact warning was truncated instead of wrapped")
			}
			bounds := image.Rect(0, 0, dims.X, dims.Y)
			if !yes.In(bounds) || !no.In(bounds) || yes.Overlaps(no) {
				t.Fatalf("buttons outside screen: %v %v", yes, no)
			}
			for _, button := range []image.Rectangle{no, yes} {
				c.age = 6
				g.push(c)
				g.in = input{click: true, mouse: button.Min.Add(image.Pt(button.Dx()/2, button.Dy()/2))}
				c.update(g)
				if mnuTop(g) == scene(c) {
					t.Fatal("pointer did not close confirmation")
				}
			}
			if accepted != 1 || refused != 1 {
				t.Fatalf("wrong callbacks yes=%d no=%d", accepted, refused)
			}
		})
	}
}
