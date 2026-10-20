package trial

import (
	"slices"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/testgame"
)

func TestBattleHookReachability(t *testing.T) {
	for _, hook := range []string{"before", "after", "after_win"} {
		t.Run(hook, func(t *testing.T) {
			src := fstest.MapFS{
				"game.yaml":      {Data: []byte(testgame.Game + "  " + hook + ": prepare\n")},
				"data/main.yaml": {Data: []byte(testgame.Data)},
				"assets/x.png":   {Data: []byte("synthetic sentinel")},
				"data/hooks.yaml": {Data: []byte(`events:
  prepare:
    do: [{call: child}]
  child:
    do: [{image: hook.png}, {set: {flag.ready: true}}]
  followup:
    trigger: enter
    location: camp
    when: flag.ready
    do: [{image: followup.png}]
  S:
    do: [{image: unused.png}]
locations:
  room:
    name: Room
    dungeon:
      map: |
        ###
        #S#
        ###
      tiles: {S: {start: true, once: true, after: S}}
`)},
			}
			for _, name := range []string{"hook.png", "followup.png", "unused.png"} {
				src["assets/"+name] = &fstest.MapFile{Data: []byte("synthetic sentinel")}
			}
			cfg, err := Parse([]byte("{}"))
			if err != nil {
				t.Fatal(err)
			}
			result, err := Build(src, cfg)
			if err != nil {
				t.Fatal(err)
			}
			if err := result.Check(); err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"prepare", "child", "followup"} {
				if !slices.Contains(result.Kept["events"], id) {
					t.Errorf("event %s removed", id)
				}
			}
			for _, name := range []string{"hook.png", "followup.png"} {
				if !slices.Contains(result.Assets, name) {
					t.Errorf("asset %s removed", name)
				}
			}
			if slices.Contains(result.Kept["events"], "S") || slices.Contains(result.Assets, "unused.png") {
				t.Fatal("tile replacement treated as event reference")
			}
		})
	}
}
