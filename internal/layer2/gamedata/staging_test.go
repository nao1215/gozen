package gamedata_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

// TestStagingWords: stand and the map forms of effect, music, wait and sfx are checked, and
// need schema version 4; their old forms stay valid in every version (ADR 0028).
func TestStagingWords(t *testing.T) {
	v4 := "schema_version: 4\nid: b\n"
	v3 := "schema_version: 3\nid: b\n"
	music := "music: {songs: {theme: {file: x.png}}}\n"
	for _, c := range []struct{ head, do, want string }{
		{v4, `[{stand: {who: Courier, face: x.png, at: left}}, {stand: {who: Courier, at: 0.3}}, {stand: {who: Courier, leave: slide}}, {stand: none}]`, ""},
		{v4, `[{effect: {name: shake, who: scout, seconds: 0.4}}, {effect: nod}, {effect: {name: fade, color: "#102030", seconds: 1, hold: 0.5}}]`, ""},
		{v4, `[{music: {song: theme, fade: 2}}, {music: {stop: true, fade: 3}}, {wait: {seconds: 0.8, skip: false}}, {sfx: {name: decide, wait: true}}]`, ""},
		{v3, `[{effect: shake}, {wait: 0.5}, {sfx: decide}, {music: theme}]`, ""},
		{v3, `[{stand: none}]`, "stand needs schema_version: 4"},
		{v3, `[{effect: nod}]`, "the reaction nod needs schema_version: 4"},
		{v3, `[{effect: {name: flash, seconds: 1}}]`, "effect as a map needs schema_version: 4"},
		{v3, `[{wait: {seconds: 1, skip: false}}]`, "wait as a map needs schema_version: 4"},
		{v3, `[{sfx: {name: decide, wait: true}}]`, "sfx as a map needs schema_version: 4"},
		{v3, `[{music: {song: theme, fade: 1}}]`, "music fade and stop needs schema_version: 4"},
		{v4, `[{stand: {face: x.png}}]`, "stand needs who"},
		{v4, `[{stand: {who: Courier, face: x.png, at: up}}]`, "want left, center, right or a fraction"},
		{v4, `[{stand: {who: Courier, face: x.png, enter: spin}}]`, "want fade or slide"},
		{v4, `[{stand: {who: Courier, leave: fade, face: x.png}}]`, "a stand that leaves takes no enter or face"},
		{v4, `[{effect: wobble}]`, "or a reaction"},
		{v4, `[{effect: {name: flash, who: scout}}]`, "flash is of the whole screen"},
		{v4, `[{effect: {name: shake, color: "#000000"}}]`, "color and hold go with fade"},
		{v4, `[{music: {stop: true, song: theme}}]`, "music stop takes no song"},
		{v4, `[{music: {song: theme, fade: -1}}]`, "fade is seconds"},
		{v4, `[{wait: {seconds: -1}}]`, "seconds are a number, 0 or more"},
		{v4, `[{sfx: {wait: true}}]`, "sfx needs the name"},
	} {
		_, err := testgame.TryLoad(map[string]string{
			"game.yaml":   c.head + testgame.Game,
			"data/z.yaml": music + "events: {e: {do: " + c.do + "}}",
		})
		if c.want == "" && err != nil || c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%s: %v", c.do, err)
		}
	}
	_, err := testgame.TryLoad(map[string]string{"game.yaml": v4 + testgame.Game, "data/z.yaml": "locations: {hall: {music: {song: theme, fade: 1}}}\n" + music})
	if err == nil || !strings.Contains(err.Error(), "fade and stop go only in the music command") {
		t.Errorf("a place's music with a fade: %v", err)
	}
}
