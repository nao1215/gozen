package play

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/screen/gfx"
)

// materialCache owns one reduced backdrop, reused by every panel. Static authored
// backgrounds are keyed by path and output size; moving stages refresh the same
// allocation before drawing UI, so text never leaks into the next frame's blur.
type materialCache struct {
	wallpaper     *ebiten.Image
	wallpaperKey  string
	small         *ebiten.Image
	shader        *ebiten.Shader
	key           string
	width, height int
	blur          float64
	// src is the image small was last reduced from; a panel drawn on it
	// reduces again only the part under itself (refreshMaterial).
	src *ebiten.Image
	// drawn are the footprints (device pixels) of the panels drawn on src
	// since it was last reduced: a panel over none of them samples what is
	// already reduced, without reading src again.
	drawn []image.Rectangle
}

// materialDrawn tells the glass that something other than a panel was drawn
// on dst at x, y, w, h (UI units): a panel over it reduces that part again
// before it samples.
func (g *Game) materialDrawn(dst *ebiten.Image, x, y, w, h float64) {
	c := &g.glass
	if c.src != dst {
		return
	}
	k := gfx.Scale()
	c.drawn = append(c.drawn, image.Rect(int(x*k), int(y*k), int((x+w)*k)+1, int((y+h)*k)+1))
}

// reachOf is how far (device pixels) a panel's glass reads beyond its box: the
// refraction's lens and the bilinear read of the reduced backdrop.
func reachOf(m *data.MaterialStyle, k float64) float64 {
	if m == nil {
		return 0
	}
	return (1.8+m.Refraction)*k + 2*math.Max(1, m.Blur*k)
}

// beginMaterialFrame forgets the last frame's reduction, so a frame's first
// panel reduces its own layer and never samples last frame's panels.
func (g *Game) beginMaterialFrame() {
	g.glass.src, g.glass.drawn = nil, g.glass.drawn[:0]
}

func (g *Game) prepareMaterial(dst *ebiten.Image, key string) {
	m := g.db.Game.UI.Material
	if m == nil {
		return
	}
	c := &g.glass
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	if key != "" && key == c.key && w == c.width && h == c.height && m.Blur == c.blur {
		return
	}
	divisor := math.Max(1, m.Blur*gfx.Scale())
	bw, bh := max(1, int(float64(w)/divisor)), max(1, int(float64(h)/divisor))
	if c.small == nil || c.small.Bounds().Dx() != bw || c.small.Bounds().Dy() != bh {
		if c.small != nil {
			c.small.Deallocate()
		}
		c.small = ebiten.NewImage(bw, bh)
	}
	c.small.Clear()
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(float64(bw)/float64(w), float64(bh)/float64(h))
	c.small.DrawImage(dst, op)
	c.key, c.width, c.height, c.blur, c.src = key, w, h, m.Blur, dst
	c.drawn = c.drawn[:0]
}

// refreshMaterial brings the reduced backdrop up to date under a panel about to
// be drawn on dst at x, y, w, h (UI units), reaching margin device pixels
// beyond it. The panel's shader samples nothing outside that, so reducing only
// that part draws the same panel as reducing the whole layer again, which a
// screen of twenty cards did twenty times a frame.
func (g *Game) refreshMaterial(dst *ebiten.Image, x, y, w, h, margin float64) {
	m := g.db.Game.UI.Material
	c := &g.glass
	dw, dh := dst.Bounds().Dx(), dst.Bounds().Dy()
	k := gfx.Scale()
	// What this panel leaves on dst that a later panel would see in its glass
	// is its box (its soft shadow beyond it is a few percent at most), and what
	// it shows of dst is its box and the refraction's reach.
	box := image.Rect(int(x*k), int(y*k), int((x+w)*k)+1, int((y+h)*k)+1)
	reach := box.Inset(-int(math.Ceil(reachOf(m, k))))
	defer func() { c.drawn = append(c.drawn, box) }()
	if m == nil || c.small == nil || c.src != dst || c.width != dw || c.height != dh || c.blur != m.Blur {
		g.prepareMaterial(dst, "")
		return
	}
	bw, bh := c.small.Bounds().Dx(), c.small.Bounds().Dy()
	sx, sy := float64(bw)/float64(dw), float64(bh)/float64(dh)
	// The part of small the panel samples, with two reduced pixels to spare for
	// the bilinear reads, and one more on the source side so the reduction's own
	// reads stay inside the part it reads from.
	part := image.Rect(int(math.Floor((x*k-margin)*sx))-2, int(math.Floor((y*k-margin)*sy))-2,
		int(math.Ceil(((x+w)*k+margin)*sx))+2, int(math.Ceil(((y+h)*k+margin)*sy))+2).Intersect(c.small.Bounds())
	if part.Empty() {
		return
	}
	if !slices.ContainsFunc(c.drawn, reach.Overlaps) {
		// Nothing drawn since the reduction lies under this panel: what it
		// samples is already up to date. (Panels side by side, as on a board
		// of cards, read the layer once.)
		return
	}
	from := image.Rect(int(math.Floor(float64(part.Min.X-1)/sx)), int(math.Floor(float64(part.Min.Y-1)/sy)),
		int(math.Ceil(float64(part.Max.X+1)/sx)), int(math.Ceil(float64(part.Max.Y+1)/sy))).Intersect(dst.Bounds())
	source, ok1 := dst.SubImage(from).(*ebiten.Image)
	target, ok2 := c.small.SubImage(part).(*ebiten.Image)
	if !ok1 || !ok2 {
		g.prepareMaterial(dst, "")
		return
	}
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear, Blend: ebiten.BlendCopy}
	op.GeoM.Translate(float64(from.Min.X), float64(from.Min.Y))
	op.GeoM.Scale(sx, sy)
	target.DrawImage(source, op)
	// small is no longer the keyed backdrop alone.
	c.key = ""
}

func (g *Game) materialBackground(dst *ebiten.Image, path string) {
	if g.tr != nil && g.tr.layout != nil {
		g.tr.layout.backgrounds = append(g.tr.layout.backgrounds, path)
	}
	sw, sh := g.screen()
	gfx.DrawImageCover(dst, g.pics.get(path), 0, 0, sw, sh, 1)
	g.prepareMaterial(dst, "")
}

func (g *Game) materialPanel(dst *ebiten.Image, x, y, w, h float64, tint color.NRGBA) bool {
	return g.materialSurface(dst, x, y, w, h, tint, "control")
}

func (g *Game) materialSurface(dst *ebiten.Image, x, y, w, h float64, tint color.NRGBA, role string) bool {
	return g.materialSurfaceDeep(dst, x, y, w, h, tint, role, 0)
}

// materialSurfaceDeep is materialSurface with the role's opacity replaced by deep
// when it is above zero (a message window over a call, call.window_opacity).
func (g *Game) materialSurfaceDeep(dst *ebiten.Image, x, y, w, h float64, tint color.NRGBA, role string, deep float64) bool {
	m := g.db.Game.UI.Material
	c := &g.glass
	if m == nil || c.small == nil || w <= 0 || h <= 0 {
		return false
	}
	if c.shader == nil {
		var err error
		c.shader, err = ebiten.NewShader([]byte(materialShader))
		if err != nil {
			panic(err)
		}
	}
	k := gfx.Scale()
	// Sample the current layer, including previously drawn windows and content,
	// under the panel's footprint: the shadow pad and the refraction's reach.
	g.refreshMaterial(dst, x, y, w, h, 24*k+(1.8+m.Refraction)*k+4)
	kind := m.Kind
	if role == "chart" && m.Surfaces[role] == "" {
		role = "base"
	}
	finish := role
	if (role == "message" || role == "choice") && m.Surfaces[role] == "" {
		finish = "control" // the message window and the choices were controls before they had roles
	}
	if override := m.Surfaces[finish]; override != "" {
		kind = override
	}
	opacity := materialOpacity(kind, m.Opacity)
	if o, ok := m.Opacities[role]; ok {
		opacity = materialOpacity(kind, o)
	}
	if deep > 0 {
		opacity = materialOpacity(kind, deep)
	}
	shadow, border := m.Shadow, m.Border
	if role == "plate" {
		// A plate is read, not pressed: no lit edge and no lift off the page.
		shadow, border = 0, 0
	}
	refraction := 0.0
	if kind == "glass" {
		refraction = (1.8 + m.Refraction) * k
	}
	op := &ebiten.DrawTrianglesShaderOptions{}
	op.Images[0] = c.small
	op.Uniforms = map[string]any{
		"Refraction": float32(refraction),
		"Box":        []float32{float32(x * k), float32(y * k), float32(w * k), float32(h * k)},
		"Screen":     []float32{float32(dst.Bounds().Dx()), float32(dst.Bounds().Dy())},
		"Tint":       []float32{float32(tint.R) / 255, float32(tint.G) / 255, float32(tint.B) / 255, float32(opacity)},
		"Style":      []float32{float32(min(18, h/2) * k), float32(k), float32(border), float32(shadow)},
	}
	// Cover the soft shadow, but shade only this panel's bounded footprint.
	pad := 24 * k
	left, top := float32(x*k-pad), float32(y*k-pad)
	right, bottom := float32((x+w)*k+pad), float32((y+h)*k+pad)
	vertices := []ebiten.Vertex{{DstX: left, DstY: top}, {DstX: right, DstY: top}, {DstX: left, DstY: bottom}, {DstX: right, DstY: bottom}}
	dst.DrawTrianglesShader(vertices, []uint16{0, 1, 2, 1, 3, 2}, c.shader, op)
	return true
}

const materialShader = `//kage:unit pixels
package main
var Refraction float
var Box vec4
var Screen vec2
var Tint vec4
var Style vec4
func distance(p vec2) float {
 q:=abs(p-(Box.xy+Box.zw/2))-(Box.zw/2-vec2(Style.x))
 return length(max(q,vec2(0)))+min(max(q.x,q.y),0)-Style.x
}
func Fragment(pos vec4, uv vec2, col vec4) vec4 {
 d:=distance(pos.xy)
 coverage:=1-smoothstep(-0.6,0.6,d)
 shadowD:=distance(pos.xy-vec2(0,5*Style.y))
 shadow:=exp(-max(shadowD,0)/ (9*Style.y))*Style.w*0.075
 // Bilinear sampling in reduced space reconstructs a soft backdrop without a
 // full-resolution offscreen image per panel.
 lens:=normalize(pos.xy-(Box.xy+Box.zw/2)+vec2(0.001))*Refraction*exp(-abs(d)/(7*Style.y))
 p:=clamp((pos.xy+lens)/Screen*imageSrc0Size()-vec2(0.5),vec2(0),imageSrc0Size()-vec2(2))
 base:=floor(p)
 f:=fract(p)
 origin:=imageSrc0Origin()
 a:=imageSrc0At(origin+base+vec2(0.5))
 b:=imageSrc0At(origin+base+vec2(1.5,0.5))
 c:=imageSrc0At(origin+base+vec2(0.5,1.5))
 e:=imageSrc0At(origin+base+vec2(1.5))
 bg:=mix(mix(a,b,f.x),mix(c,e,f.x),f.y)
 rgb:=mix(bg.rgb,Tint.rgb,Tint.a)
 sheen:=0.025*clamp(1-(pos.y-Box.y)/Box.w,0,1)
 rgb=mix(rgb,vec3(1),sheen)
 edge:=(1-smoothstep(0.4,1.3*Style.y,abs(d)))*Style.z
 light:=mix(0.22,0.75,clamp(1-(pos.y-Box.y)/Box.w,0,1))
 rgb=mix(rgb,vec3(1),edge*light)
 alpha:=coverage+(1-coverage)*shadow
 return vec4(rgb*coverage,alpha)
}
`

// toastOpacityFloor is how opaque a notice's card is at least under ui.material: a
// guide's line and an inbox notice float over a screen full of text, and glass would
// let that text show through behind their own. At this opacity the darkest backdrop
// leaves the light card light enough for the light palette's text and accent to keep
// a WCAG contrast of 4.5:1.
const toastOpacityFloor = .94

// toastOpacity is the opacity of a notice's card: the card role's opacity of
// ui.material, raised to toastOpacityFloor.
func toastOpacity(m *data.MaterialStyle) float64 {
	o := m.Opacity
	if v, ok := m.Opacities["card"]; ok {
		o = v
	}
	return max(toastOpacityFloor, o)
}

// toastSurface draws the card of a notice (a guide's line, an inbox notice and the
// message it opens) nearly opaque, so the text under it does not show through.
// It reports false without ui.material, as materialSurface does.
func (g *Game) toastSurface(dst *ebiten.Image, x, y, w, h float64, tint color.NRGBA) bool {
	m := g.db.Game.UI.Material
	if m == nil {
		return false
	}
	return g.materialSurfaceDeep(dst, x, y, w, h, tint, "card", toastOpacity(m))
}

// materialOpacity separates quiet content surfaces from floating controls.
func materialOpacity(kind string, opacity float64) float64 {
	switch kind {
	case "mica":
		return math.Max(.9, opacity)
	case "acrylic":
		return math.Max(.72, opacity)
	case "opaque":
		return 1
	default:
		return opacity
	}
}

// drawMaterialWallpaper caches a neutral, authored color field, never a scene photo.
func (g *Game) drawMaterialWallpaper(dst *ebiten.Image) bool {
	m := g.db.Game.UI.Material
	if m == nil || len(m.Wallpaper) != 3 {
		return false
	}
	c := &g.glass
	key := fmt.Sprintf("%v/%g", m.Wallpaper, m.WallpaperStrength)
	if c.wallpaper == nil || c.wallpaperKey != key {
		if c.wallpaper != nil {
			c.wallpaper.Deallocate()
		}
		c.wallpaperKey = key
		c.key = ""
		img := image.NewNRGBA(image.Rect(0, 0, 256, 144))
		colors := [3]color.NRGBA{}
		for i := range colors {
			colors[i] = m.Wallpaper[i].Or(color.NRGBA{230, 235, 240, 255})
		}
		for y := range 144 {
			for x := range 256 {
				u, v := float64(x)/255, float64(y)/143
				b := math.Exp(-((u-.75)*(u-.75)*2 + (v-.22-.12*math.Sin(u*8))*(v-.22-.12*math.Sin(u*8))) * (3 + 6*m.WallpaperStrength))
				d := math.Exp(-((u-.24)*(u-.24)*2 + (v-.78-.16*math.Sin(u*7))*(v-.78-.16*math.Sin(u*7))) * (4 + 5*m.WallpaperStrength))
				b *= .65 + .35*m.WallpaperStrength
				d *= .35 + .65*m.WallpaperStrength
				if b+d > 1 {
					b, d = b/(b+d), d/(b+d)
				}
				mix := func(a, z, q uint8) uint8 {
					return uint8(float64(a)*(1-b-d) + float64(z)*b + float64(q)*d)
				}
				img.SetNRGBA(x, y, color.NRGBA{mix(colors[0].R, colors[1].R, colors[2].R), mix(colors[0].G, colors[1].G, colors[2].G), mix(colors[0].B, colors[1].B, colors[2].B), 255})
			}
		}
		c.wallpaper = ebiten.NewImageFromImage(img)
	}
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(float64(dst.Bounds().Dx())/256, float64(dst.Bounds().Dy())/144)
	dst.DrawImage(c.wallpaper, op)
	g.prepareMaterial(dst, "wallpaper")
	return true
}

func (g *Game) surfaceWindow(dst *ebiten.Image, x, y, w, h float64, role string) {
	if !g.materialSurface(dst, x, y, w, h, g.th.window, role) {
		g.window(dst, x, y, w, h)
	}
}

// controlMaterial gives actionable panels quiet hover and depressed feedback.
// It changes only drawing coordinates; hit testing retains the original bounds.
func (g *Game) controlMaterial(r image.Rectangle) (image.Rectangle, color.NRGBA) {
	tint := g.th.window
	if g.in.mouse.In(r) {
		tint.R = uint8(min(255, int(tint.R)+8))
		tint.G = uint8(min(255, int(tint.G)+8))
		tint.B = uint8(min(255, int(tint.B)+8))
		if g.in.click || g.in.pheld {
			r = r.Add(image.Pt(0, 1))
			tint.R = uint8(float64(tint.R) * .96)
			tint.G = uint8(float64(tint.G) * .96)
			tint.B = uint8(float64(tint.B) * .96)
		}
	}
	return r, tint
}

// desktopFrame shares the rounded border, shadow and title chrome across tools.
// A transparent body preserves the wallpaper for independently frosted panels.
func (g *Game) desktopFrame(dst *ebiten.Image, bottom, titleHeight float64, body color.NRGBA) {
	sw, _ := g.screen()
	// Shadow outside the body only: do not darken the glass backdrop.
	for i := 8; i >= 1; i-- {
		d := float32(i)
		gfx.StrokeRoundRect(dst, 18-d/2, 18+d/2, float32(sw-36)+d, float32(bottom-18), 12+d/2, 1, color.NRGBA{25, 33, 49, 10})
	}
	if body.A != 0 {
		gfx.FillRoundRect(dst, 18, 16, float32(sw-36), float32(bottom-16), 12, body)
		g.materialDrawn(dst, 18, 16, sw-36, bottom-16)
	}
	ch := g.chrome()
	gfx.StrokeRoundRect(dst, 18, 16, float32(sw-36), float32(bottom-16), 12, 1, ch.frame)
	gfx.FillRoundRect(dst, 18, 16, float32(sw-36), float32(titleHeight), 12, ch.bar)
	g.materialDrawn(dst, 18, 16, sw-36, titleHeight)
	if ch.buttons != "" {
		g.text(dst, ch.buttons, sw-125, 28, 16, ch.text)
	}
}

// chromeColors are the resolved colors and marks of ui.chrome.
type chromeColors struct {
	bar, text, field, tab, frame, title, address color.NRGBA
	buttons, nav                                 string
}

// chrome resolves ui.chrome over the built-in light desktop decoration.
func (g *Game) chrome() chromeColors {
	c := chromeColors{
		bar: color.NRGBA{224, 230, 239, 255}, text: color.NRGBA{50, 60, 70, 255},
		field: color.NRGBA{249, 251, 255, 255}, tab: color.NRGBA{248, 250, 255, 255},
		frame: color.NRGBA{243, 248, 255, 180}, title: color.NRGBA{40, 49, 60, 255}, address: color.NRGBA{65, 75, 88, 255},
		buttons: "−   □   ×", nav: "‹   ›",
	}
	u := g.db.Game.UI.Chrome
	if u == nil {
		return c
	}
	c.bar, c.field, c.frame = u.Bar.Or(c.bar), u.Field.Or(c.field), u.Frame.Or(c.frame)
	c.tab = u.Field.Or(c.tab)
	if u.Text.Valid {
		c.text, c.title, c.address = u.Text.NRGBA, u.Text.NRGBA, u.Text.NRGBA
	}
	if u.Buttons != nil {
		c.buttons = *u.Buttons
	}
	if u.Nav != nil {
		c.nav = *u.Nav
	}
	return c
}

// lightColors is the paper palette of view.light surfaces.
type lightColors struct{ paper, window, text, dim, accent color.NRGBA }

// lightPalette resolves ui.light over the built-in quiet paper colors.
func (g *Game) lightPalette() lightColors {
	c := lightColors{
		paper: color.NRGBA{236, 234, 228, 255}, window: color.NRGBA{247, 249, 248, 255},
		text: color.NRGBA{40, 49, 58, 255}, dim: color.NRGBA{91, 104, 116, 255}, accent: color.NRGBA{37, 105, 109, 255},
	}
	if u := g.db.Game.UI.Light; u != nil {
		c = lightColors{u.Paper.Or(c.paper), u.Window.Or(c.window), u.Text.Or(c.text), u.Dim.Or(c.dim), u.Accent.Or(c.accent)}
	}
	return c
}

// toneColors fills workspace entries with an authored tone.
type toneColors struct{ note, primary, primaryCard, primaryText color.NRGBA }

// tonePalette resolves ui.tones over the built-in note and primary colors.
func (g *Game) tonePalette() toneColors {
	c := toneColors{
		note: color.NRGBA{255, 241, 194, 255}, primary: color.NRGBA{213, 235, 237, 255},
		primaryCard: color.NRGBA{26, 113, 122, 255}, primaryText: color.NRGBA{255, 255, 255, 255},
	}
	if u := g.db.Game.UI.Tones; u != nil {
		c = toneColors{u.Note.Or(c.note), u.Primary.Or(c.primary), u.PrimaryCard.Or(c.primaryCard), u.PrimaryText.Or(c.primaryText)}
	}
	return c
}

// close releases every GPU object owned by this cache, also on live reload.
func (c *materialCache) close() {
	if c.wallpaper != nil {
		c.wallpaper.Deallocate()
	}
	if c.small != nil {
		c.small.Deallocate()
	}
	if c.shader != nil {
		c.shader.Deallocate()
	}
	*c = materialCache{}
}
