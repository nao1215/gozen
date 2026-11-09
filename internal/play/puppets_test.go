package play

import (
	"io/fs"
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/battle"
	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

// stagePuppetData stands two actors who share the example puppet, lets the first talk and
// change its face with a line, and the second with a stand.
const stagePuppetData = `actors:
  streamer: {name: Streamer, faces: streamer, face: smile, puppet: streamer/puppet.yaml}
  twin: {name: Twin, faces: streamer, face: smile, puppet: streamer/puppet.yaml}
events:
  scene:
    do:
      - stand: {who: streamer, face: smile, at: left}
      - stand: {who: twin, face: smile, at: right}
      - say: {who: streamer, face: smile, text: "A line long enough to keep the text typing for a good while, and then some more."}
      - stand: {who: twin, face: surprised}
      - say: {who: streamer, face: surprised, text: "Oh."}
      - say: {who: twin, text: "Hm."}
`

func stagePuppetGame(t *testing.T, ui string) *Game {
	t.Helper()
	fsys := gameFiles(t)
	fsys["game.yaml"] = &fstest.MapFile{Data: append([]byte("schema_version: 8\nui: "+ui+"\n"), fsys["game.yaml"].Data...)}
	fsys["data/stage_puppet.yaml"] = &fstest.MapFile{Data: []byte(stagePuppetData)}
	src := os.DirFS("../../examples/puppet/assets/streamer")
	for _, n := range []string{"puppet.yaml", "streamer.png", "surprised.png", "eyes_closed.png", "mouth_closed.png", "mouth_half.png"} {
		b, err := fs.ReadFile(src, n)
		if err != nil {
			t.Fatal(err)
		}
		fsys["assets/streamer/"+n] = &fstest.MapFile{Data: b}
	}
	fsys["assets/streamer/smile.png"] = fsys["assets/streamer/streamer.png"]
	g := wldOpenWith(t, Options{FS: fsys, Location: "camp"})
	wldFrames(t, g, 5)
	wldStartEvent(t, g, "scene")
	return g
}

// TestStagePuppetsMoveOnTheNovelStage: with ui.stage_puppets, every stood actor with a
// puppet moves on the novel stage, only the speaker's mouth talks, the faces of a line and
// of a stand fade in, and a face the puppet lacks shows the picture.
func TestStagePuppetsMoveOnTheNovelStage(t *testing.T) {
	covSlow(t)
	g := stagePuppetGame(t, "{message: novel, stage_puppets: true}")
	wldUntil(t, g, 100, func() bool { return g.talk.active })
	speaker, other := g.puppets.live["streamer"], g.puppets.live["twin"]
	if speaker == nil || other == nil {
		t.Fatalf("stood puppets %v", g.puppets.live)
	}
	if !speaker.Reduce || !other.Reduce {
		t.Fatal("stage puppets compose at the drawn size")
	}
	if got := g.stagedFaces(); len(got) != 2 || got[0][0] != "actor:streamer" || got[1][0] != "actor:twin" {
		t.Fatalf("staged %v", got)
	}
	start := other.Motion.Pose()
	talked, quiet := false, true
	screen := wldScreen(g)
	frames := 0
	for ; g.talk.shown < g.talk.total && frames < 600; frames++ {
		if !wldStep(t, g, screen) {
			t.Fatal("quit")
		}
		if frames%10 == 0 {
			g.Draw(screen)
		}
		talked = talked || speaker.Motion.Pose().Mouth >= 0
		quiet = quiet && other.Motion.Pose().Mouth < 0
	}
	if frames < 20 {
		t.Fatalf("the line typed in %d frames", frames)
	}
	if !talked || !quiet {
		t.Fatalf("speaker talked %v, the other kept quiet %v", talked, quiet)
	}
	if other.Motion.Pose() == start {
		t.Fatal("the figure who does not speak stood still")
	}

	wldPlay(t, g, "press ok\n", 5)
	wldUntil(t, g, 60, func() bool { return g.talk.say.Text == "Oh." })
	if p := speaker.Motion.Pose(); p.Face != 1 || p.PrevFace != 0 || p.Fade >= 1 {
		t.Fatalf("the line's face should fade in: %+v", p)
	}
	if p := other.Motion.Pose(); p.Face != 1 || p.PrevFace != 0 || p.Fade >= 1 {
		t.Fatalf("the stand's face should fade in: %+v", p)
	}
	wldFrames(t, g, 5) // the example's fade is a quarter of a second (15 ticks)
	if p := speaker.Motion.Pose(); p.Fade <= 0.2 || p.Fade >= 1 {
		t.Fatalf("a third of the way through the fade: %+v", p.Fade)
	}
	wldFrames(t, g, 25)
	if p := speaker.Motion.Pose(); p.Face != 1 || p.Fade != 1 {
		t.Fatalf("after the fade: %+v", p)
	}

	wldPlay(t, g, "press ok\n", 5)
	wldUntil(t, g, 60, func() bool { return g.talk.say.Text == "Hm." })
	if p := other.Motion.Pose(); p.Face != 0 || p.PrevFace != 1 {
		t.Fatalf("the line's own face (the actor's usual one) replaces the stand's: %+v", p)
	}
	// a face the puppet lacks (gozen check refuses it in the data) shows the picture
	if p := g.stagePuppet("actor:twin", "x.png"); p != nil {
		t.Fatal("a face the puppet lacks was drawn by the puppet")
	}
	if g.stageBig("actor:twin", "x.png") {
		t.Fatal("a small missing picture stood on the stage")
	}
	if p := g.stagePuppet("name:Courier", "streamer/smile.png"); p != nil {
		t.Fatal("a figure that is no actor got a puppet")
	}
	wldFramesDrawn(t, g, 10)
}

// TestStagePuppetsAreOptIn: without ui.stage_puppets the novel stage shows the face
// pictures, the figures who do not speak get no puppet, and in the box window the
// speaker's puppet is drawn as before.
func TestStagePuppetsAreOptIn(t *testing.T) {
	covSlow(t)
	g := stagePuppetGame(t, "{message: novel}")
	wldUntil(t, g, 100, func() bool { return g.talk.active })
	wldFramesDrawn(t, g, 20)
	if g.stagePuppets() {
		t.Fatal("stage puppets without the setting")
	}
	if p := g.stagePuppet("actor:streamer", "streamer/smile.png"); p != nil {
		t.Fatal("the stage drew a puppet without the setting")
	}
	if g.puppets.live["twin"] != nil || len(g.puppets.staged) != 0 {
		t.Fatalf("puppets made for the stage: %v %v", g.puppets.live, g.puppets.staged)
	}
	if p := g.puppets.live["streamer"]; p == nil || p.Reduce {
		t.Fatal("the speaker's puppet is kept for the talk as before, at the base size")
	}
}

// TestStagePuppetsLeaveTheBoxWindow: the box window has no stage, so ui.stage_puppets
// changes nothing there.
func TestStagePuppetsLeaveTheBoxWindow(t *testing.T) {
	covSlow(t)
	box := stagePuppetGame(t, "{message: box, stage_puppets: true}")
	wldUntil(t, box, 100, func() bool { return box.talk.active })
	wldFramesDrawn(t, box, 20)
	if box.stagePuppets() || box.puppets.live["twin"] != nil {
		t.Fatal("the box window has no stage")
	}
	if p := box.puppets.live["streamer"]; p == nil || p.Reduce {
		t.Fatal("the box window's speaker puppet")
	}
}

// enemyPuppetFiles is shared by the motion contract and real PNG reproduction.
func enemyPuppetFiles(t *testing.T) fstest.MapFS {
	t.Helper()
	files := gameFiles(t)
	for name, body := range testgame.MonsterFiles() {
		files[name] = &fstest.MapFile{Data: []byte(body)}
	}
	files["game.yaml"].Data = []byte(strings.Replace(string(files["game.yaml"].Data), "schema_version: 6", "schema_version: 7", 1) + "\nfonts: {regular: test.ttf}\n")
	files["data/main.yaml"].Data = []byte(strings.Replace(string(files["data/main.yaml"].Data), "    name: Mote\n    image: x.png", "    puppet: streamer/puppet.yaml\n    name: Mote\n    image: streamer/streamer.png", 1))
	for _, name := range []string{"puppet.yaml", "streamer.png", "surprised.png", "eyes_closed.png", "mouth_closed.png", "mouth_half.png"} {
		body, err := fs.ReadFile(os.DirFS("../../examples/puppet/assets/streamer"), name)
		if err != nil {
			t.Fatal(err)
		}
		files["assets/streamer/"+name] = &fstest.MapFile{Data: body}
	}
	return files
}

func TestEnemyPuppetMotionDoesNotDependOnDrawing(t *testing.T) {
	open := func() *Game {
		g := wldOpenWith(t, Options{FS: enemyPuppetFiles(t), Location: "field", Battle: "one", Party: []string{"scout", "guide"}})
		return g
	}
	drawn := open()
	for range 90 {
		if err := drawn.Update(); err != nil {
			t.Fatal(err)
		}
		drawn.Draw(btlImage())
	}
	wantMotion := drawn.puppets.enemies[drawn.currentBattle().b.Foes[0]].Motion
	want := wantMotion.Pose()
	if err := drawn.Close(); err != nil {
		t.Fatal(err)
	}
	skipped := open()
	for range 90 {
		if err := skipped.Update(); err != nil {
			t.Fatal(err)
		}
	}
	gotMotion := skipped.puppets.enemies[skipped.currentBattle().b.Foes[0]].Motion
	if !reflect.DeepEqual(gotMotion, wantMotion) {
		t.Fatal("motion state differs without draws (including time, phases, springs and RNG)")
	}
	got := gotMotion.Pose()
	if got != want {
		t.Fatalf("motion without draws %+v; with draws %+v", got, want)
	}
	for range 3 {
		skipped.Draw(btlImage())
	}
	if after := skipped.puppets.enemies[skipped.currentBattle().b.Foes[0]].Motion.Pose(); after != got {
		t.Fatalf("repeated drawing advanced motion: %+v -> %+v", got, after)
	}
}

func TestEnemyPuppetsBelongToBattleUnits(t *testing.T) {
	g := wldOpenWith(t, Options{FS: enemyPuppetFiles(t), Location: "field", Battle: "one", Party: []string{"scout", "guide"}})
	s := g.currentBattle()
	first := s.b.Foes[0]
	second := *first
	s.b.Foes = append(s.b.Foes, &second)
	first.Def.Images = []data.HPImage{{Below: 0.5, File: "streamer/surprised.png"}}
	s.shownHP[first], s.shownHP[&second] = first.MHP(), 1
	s.actor = first
	g.closeEnemyPuppets()
	s.stepFoePictures(g)
	a, b := g.puppets.enemies[first], g.puppets.enemies[&second]
	if a == nil || b == nil || a == b || a.Rig() != b.Rig() {
		t.Fatal("identical enemies must share only their rig")
	}
	if a.Motion.Pose().Face == b.Motion.Pose().Face {
		t.Fatal("the healthy and hurt enemies share a face")
	}
	// Compare the entire motion (including health and speaking) to independent
	// puppets driven by each unit's own inputs; pointers/RNG seeds never enter it.
	for _, u := range []*battle.Unit{first, &second} {
		want := seededPuppet(a.Rig(), "enemy."+u.EID)
		t.Cleanup(want.Close)
		face, ok := want.Rig().Face(map[bool]string{true: "smile", false: "surprised"}[u == first])
		if !ok {
			t.Fatal("fixture face missing")
		}
		want.Motion.SetFace(face)
		want.Motion.SetHealth(float64(s.shownHP[u]) / float64(u.MHP()))
		want.Motion.Step()
		want.Motion.Speak(u == first)
		if !reflect.DeepEqual(g.puppets.enemies[u].Motion, want.Motion) {
			t.Fatal("unit face, health, or speaking motion was overwritten")
		}
	}
	// Units removed by a merge and fully faded defeated units release their state.
	s.b.Foes = s.b.Foes[:1]
	s.stepFoePictures(g)
	if len(g.puppets.enemies) != 1 || g.puppets.enemies[&second] != nil {
		t.Fatal("removed unit retained")
	}
	s.gone[first] = 40
	s.stepFoePictures(g)
	if len(g.puppets.enemies) != 0 {
		t.Fatal("faded unit retained")
	}
	delete(s.gone, first)
	s.stepFoePictures(g)
	if g.puppets.enemies[first] == a {
		t.Fatal("departed unit reused old motion")
	}
	g.pop()
	if len(g.puppets.enemies) != 0 {
		t.Fatal("closed battle retained puppets")
	}
}
