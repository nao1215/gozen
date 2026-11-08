// Package perf generates neutral workloads for maintainer speed budgets and
// whole-CLI benchmark comparisons. Generated assets are never shipped.
package perf

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

// Cases is the fixed workload list shared by the fixture command and tests.
func Cases() []string {
	return []string{"huge-map", "pictures-4k", "standing-pictures", "phone-video", "long-conversation", "large-sim"}
}

// Generate writes one original fixture, its assets and input.txt into dir.
// dir must be a caller-owned scratch directory; errors leave partial output.
// fontPath selects the existing licensed test font; its bytes are copied unchanged.
// It is used by benchmark setup and the budget integration tests.
func Generate(dir, scenario, fontPath string) error {
	font, err := os.ReadFile(fontPath) //nolint:gosec // Caller selects the existing licensed test font.
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o750); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o750); err != nil {
		return err
	}
	write := func(name, body string) error { return os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600) }
	if err := os.WriteFile(filepath.Join(dir, "assets", "regular.ttf"), font, 0o600); err != nil { //nolint:gosec // Caller owns the scratch directory; the relative path is fixed.
		return err
	}
	game := "schema_version: 4\nid: speed-budget\ntitle: Speed budget\nscreen: {width: 960, height: 540}\nfonts: {regular: regular.ttf}\nstart: {location: room, party: [reader]}\nui: {message: novel}\nbattle: {variance: 0, critical: 0}\n"
	body := `classes: {walker: {stats: {mhp: 40, mmp: 5, atk: 10, def: 3, mat: 2, mdf: 2, agi: 8, luk: 1}, exp_for: level * 30}}
actors: {reader: {name: Reader, class: walker}}
skills:
  attack: {name: Tap, effects: [{damage: 10, variance: 0}]}
  guard: {name: Wait, scope: self}
enemies:
  mote: {name: Mote, image: mote.png, stats: {mhp: 10, mmp: 0, atk: 1, def: 0, mat: 0, mdf: 0, agi: 1, luk: 1}, ai: [{use: attack}]}
troops: {lesson: {members: [mote]}}
`
	script := "gap 1\nwait 30\nmark " + scenario + "\nwait 60\nquit\n"
	location := "locations:\n  room: {name: Room}\n"
	event := "events:\n  workload:\n    do:\n"
	picture := func(name string, w, h, phase int) error {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := range h {
			for x := range w {
				img.SetRGBA(x, y, color.RGBA{uint8((x/32 + phase) & 255), uint8((y/32 + phase) & 255), 128, 255})
			}
		}
		f, err := os.Create(filepath.Join(dir, "assets", name)) //nolint:gosec // Fixed generated asset names under caller-owned scratch directory.
		if err != nil {
			return err
		}
		err = png.Encode(f, img)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		return closeErr
	}
	if err := picture("mote.png", 16, 16, 0); err != nil {
		return err
	}
	switch scenario {
	case "huge-map":
		if err := picture("map.png", 1024, 1024, 0); err != nil {
			return err
		}
		var b strings.Builder
		b.WriteString("locations:\n  room:\n    name: Map\n    map:\n      picture: map.png\n      unknown: named\n      points:\n")
		for i := range 2000 {
			fmt.Fprintf(&b, "        p%d: {at: [%.5f, %.5f], label: Point %d, start: %t}\n", i, float64(i%50)/50, float64(i/50)/40, i, i == 0)
		}
		b.WriteString("      roads:\n")
		for i := 1; i < 2000; i++ {
			fmt.Fprintf(&b, "        - {between: [p%d, p%d]}\n", i-1, i)
		}
		location = b.String()
		event += "      - wait: 120\n"
	case "pictures-4k":
		if err := picture("picture.png", 3840, 2160, 0); err != nil {
			return err
		}
		event += "      - image: picture.png\n      - say: A large picture.\n"
	case "standing-pictures":
		if err := picture("figure.png", 256, 512, 0); err != nil {
			return err
		}
		for i := range 32 {
			event += fmt.Sprintf("      - stand: {who: Reader%d, face: figure.png, at: %.4f}\n", i, float64(i)/32)
		}
		event += "      - say: The group is ready.\n"
	case "phone-video":
		for i := range 24 {
			if err := picture(fmt.Sprintf("frame%02d.png", i), 1280, 720, i); err != nil {
				return err
			}
		}
		event += "      - phone: {app: video, hold: landscape, video: {frames: ['frame*.png'], fps: 12, seconds: 10}}\n"
	case "long-conversation":
		for i := range 2000 {
			event += fmt.Sprintf("      - say: 'Line %d: The markers show the way through the quiet garden.'\n", i)
		}
		script = "gap 1\nwait 30\nmark long-conversation\nread 60\nquit\n"
	case "large-sim":
		event += "      - say: Ready.\n"
	default:
		return fmt.Errorf("unknown speed workload %q", scenario)
	}
	for name, body := range map[string]string{"game.yaml": game, "data/workload.yaml": body + location + event, "input.txt": script} {
		if err := write(name, body); err != nil {
			return err
		}
	}
	return nil
}
