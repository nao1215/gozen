package gamedata_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

// TestFlagWarnings: a flag set but never read and one read but never set are warned of
// apart from the scene warnings; a flag declared in state.names, a gallery scene's flag
// and a read in any expression count (ADR 0029).
func TestFlagWarnings(t *testing.T) {
	db := testgame.Load(t, map[string]string{
		"game.yaml": testgame.Game + "state: {names: {flag.kept: {type: bool, initial: false}}}\n",
		"data/z.yaml": `events: {e: {do: [
{set: {flag.unread: true, flag.kept: true, flag.used: true}},
{say: "{flag.used ? 'yes' : 'no'}"},
{if: "flag.unset", then: [{gold: 1}]}
]}}`,
	})
	w := errors.Join(db.FlagWarnings()...)
	if w == nil {
		t.Fatal("no warnings")
	}
	for _, want := range []string{"flag.unread is set but nothing reads it", "flag.unset is read but nothing sets it", "flag.saw_two is set", "flag.ghost is read"} {
		if !strings.Contains(w.Error(), want) {
			t.Errorf("no %q in\n%v", want, w)
		}
	}
	for _, not := range []string{"flag.kept", "flag.used "} {
		if strings.Contains(w.Error(), not) {
			t.Errorf("warned of %s:\n%v", not, w)
		}
	}
	if errors.Join(db.SceneWarnings()...) != nil {
		t.Errorf("the flags are in the scene warnings: %v", db.SceneWarnings())
	}
}
