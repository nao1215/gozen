package gamedata_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

// TestCallViewShareChecks: call_view.share names someone who can be in a call and a
// picture under assets/ (an empty picture ends the sharing), and a call_view carries
// camera or share, not both or neither.
func TestCallViewShareChecks(t *testing.T) {
	for _, tc := range []struct{ name, command, want string }{
		{"share", `call_view: {share: {who: scout, picture: x.png}}`, ""},
		{"end", `call_view: {share: {who: scout, picture: ""}}`, ""},
		{"no picture", `call_view: {share: {who: scout, picture: none.png}}`, `call_view.share.picture: no picture assets/none.png`},
		{"no one", `call_view: {share: {who: nobody, picture: x.png}}`, `call_view.share.who: "nobody" is not a participant or an actor`},
		{"both", `call_view: {camera: {who: scout, on: true}, share: {who: scout, picture: x.png}}`, `call_view needs camera or share`},
		{"neither", `call_view: {}`, `call_view needs camera or share`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := testgame.TryLoad(map[string]string{"data/z.yaml": "events: {e: {do: [{" + tc.command + "}]}}"})
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("want no error, got %v", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}
