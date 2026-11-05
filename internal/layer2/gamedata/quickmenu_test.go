package gamedata_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

// TestQuickMenuAndNovelWindow: ui.quick_menu is below or inside, ui.novel_window is in
// fractions of the screen, both at schema_version 10.
func TestQuickMenuAndNovelWindow(t *testing.T) {
	for _, c := range []struct{ head, want string }{
		{"schema_version: 10\nid: b\nui: {quick_menu: below, novel_window: {width: 0.8, bottom: 0.02}}\n", ""},
		{"schema_version: 10\nid: b\nui: {quick_menu: inside, novel_window: {x: 0.1, width: 0.6}}\n", ""},
		{"schema_version: 10\nid: b\nui: {quick_menu: right}\n", `ui.quick_menu: want below or inside, got "right"`},
		{"schema_version: 9\nid: b\nui: {quick_menu: below}\n", "ui.quick_menu needs schema_version: 10"},
		{"schema_version: 9\nid: b\nui: {novel_window: {width: 0.8}}\n", "ui.novel_window needs schema_version: 10"},
		{"schema_version: 10\nid: b\nui: {novel_window: {x: 0.5, width: 0.6}}\n", "x+width must stay within 1"},
		{"schema_version: 10\nid: b\nui: {novel_window: {x: 0.2}}\n", "x needs a width"},
		{"schema_version: 10\nid: b\nui: {novel_window: {width: 0.2}}\n", "ui.novel_window.width: want at least 0.3"},
		{"schema_version: 10\nid: b\nui: {novel_window: {bottom: 0.5}}\n", "ui.novel_window.bottom: want a fraction"},
		{"schema_version: 10\nid: b\nui: {choice_position: center, material: {kind: glass, opacities: {message: 0.6, choice: 0.7}, surfaces: {message: acrylic}}}\n", ""},
		{"schema_version: 10\nid: b\nui: {choice_position: left}\n", `ui.choice_position: want center, got "left"`},
		{"schema_version: 9\nid: b\nui: {choice_position: center}\n", "ui.choice_position needs schema_version: 10"},
		{"schema_version: 10\nid: b\nui: {material: {kind: glass, opacities: {window: 0.5}}}\n", "ui.material.opacities: want base, card"},
		{"schema_version: 10\nid: b\nui: {material: {kind: glass, opacities: {message: 1.5}}}\n", "ui.material.opacities: want base, card"},
	} {
		_, err := testgame.TryLoad(map[string]string{"game.yaml": c.head + testgame.Game})
		if c.want == "" && err != nil || c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%q: %v", c.head, err)
		}
	}
}
