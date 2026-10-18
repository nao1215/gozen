package event_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/event"
	"github.com/nao1215/gozen/internal/testgame"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// sfxHost は鳴らした効果音を覚える。
type sfxHost struct {
	host
	played []string
}

func (h *sfxHost) SFX(name string) { h.played = append(h.played, name) }

// TestExpLevelUpPlaysSound はイベントの経験値でレベルが上がった行を出すとき、
// 効果音 levelup を鳴らすことを確かめる（上がらなければ鳴らさない）。
func TestExpLevelUpPlaysSound(t *testing.T) {
	db, st := spGame(t)
	h := &sfxHost{}
	lines := runAll(t, db, st, h, []data.Command{{Exp: data.Expr{Expr: testgame.Expr("1")}}})
	if len(h.played) != 0 || len(lines) != 0 {
		t.Fatalf("上がらない経験値で 鳴った %q 行 %q", h.played, lines)
	}
	lines = runAll(t, db, st, h, []data.Command{{Exp: data.Expr{Expr: testgame.Expr("100")}}})
	if len(lines) == 0 || !strings.Contains(lines[len(lines)-1], "Lv3") {
		t.Fatalf("レベルの行 %q", lines)
	}
	// 上がった行ごとに 1 回
	if len(h.played) != len(lines) || h.played[0] != "levelup" {
		t.Errorf("鳴った音 %q（行 %q）", h.played, lines)
	}
}

func (h *sfxHost) Handle(e event.Effect) {
	if sound, ok := e.(event.SoundEffect); ok {
		h.SFX(sound.Name)
		return
	}
	h.host.Handle(e)
}
