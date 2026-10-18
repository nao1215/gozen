package event_test

import (
	"testing"

	"github.com/nao1215/gozen/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
	"github.com/nao1215/gozen/rpg"
)

type host struct{ moved, image, mood string }

func (h *host) Move(l string)       { h.moved = l }
func (h *host) Music(data.MusicCue) {}
func (h *host) Mood(m string)       { h.mood = m }
func (h *host) Image(c data.CG)     { h.image = c.File }
func (h *host) SFX(string)          {}
func (h *host) Effect(string)       {}

const script = `
events:
  main:
    do:
      - say: {who: scout, face: happy, text: "I have {gold}G"}
      - choice:
          - label: Yes
            do: [{set: {flag.yes: true, var.n: "var.n + 2"}}]
          - label: Hidden
            when: "flag.never"
            do: []
          - label: No
            do: [{set: {flag.no: true}}]
        prompt: "Well?"
      - if: "flag.yes"
        then: [{give: {item: key}}, {gold: "-30"}]
        else: [{gold: "1000"}]
      - call: sub
      - battle: one
        on_win: [{set: {flag.won: true}}]
        on_lose: [{set: {flag.lost: true}}]
      - join: guide
      - move: field
      - image: cg.png
      - mood: calm
      - end: true
      - say: "never shown"
`

func TestRunner(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": script, "assets/cg.png": "x"})
	st := testgame.State(db, 1)
	h := &host{}
	r := event.New(db, st, h)
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["main"].Do)

	say, ok := r.Next().(event.Say)
	if !ok || say.Name != "Scout" || say.Text != "I have 100G" || say.Face != "" {
		t.Fatalf("say = %+v", say)
	}
	ch, ok := r.Next().(event.Choice)
	if !ok || len(ch.Options) != 2 || ch.Options[1] != "No" || ch.Prompt != "Well?" {
		t.Fatalf("choice = %+v (the hidden option must not show)", ch)
	}
	r.Answer(0)
	bt, ok := r.Next().(event.Battle)
	if !ok || bt.Troop != "one" || !bt.CanLose {
		t.Fatalf("battle = %+v", bt)
	}
	r.Answer(event.BattleWon)
	join, ok := r.Next().(event.Say)
	if !ok || join.Text == "" {
		t.Fatalf("no note for the join: %+v", join)
	}
	if req := r.Next(); req != nil {
		t.Fatalf("after end: %+v", req)
	}
	if !st.Flag("yes") || st.Vars["n"] != 2.0 || st.Items["key"] != 1 || st.Gold != 75 || !st.Flag("won") {
		t.Errorf("state: flags %v vars %v items %v gold %d", st.Flags, st.Vars, st.Items, st.Gold)
	}
	if len(st.Party) != 2 || h.moved != "field" || h.image != "cg.png" || h.mood != "calm" {
		t.Errorf("party %v host %+v", st.Party, h)
	}
	if r.Running() {
		t.Error("still running")
	}
}

func TestLosingWithoutOnLoseIsGameOver(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  e: {do: [{battle: one}, {say: after}]}
`})
	st := testgame.State(db, 1)
	r := event.New(db, st, &host{})
	r.Start(db.Events["e"].Do)
	if _, ok := r.Next().(event.Battle); !ok {
		t.Fatal("no battle")
	}
	r.Answer(event.BattleLost)
	if _, ok := r.Next().(event.GameOver); !ok {
		t.Fatal("no game over")
	}
	if r.Running() {
		t.Error("the event went on after the game over")
	}
}

// files shows the pictures one after another, a tap between them; a pattern takes the
// files it matches in name order, and the last picture stays for the line after it.
func TestImageFiles(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  main:
    do:
      - image: {files: [cg/a.png, cg/night_*.png], fade: 1}
      - say: "after"
`, "assets/cg/a.png": "x", "assets/cg/night_2.png": "x", "assets/cg/night_1.png": "x"})
	st := testgame.State(db, 1)
	h := &host{}
	r := event.New(db, st, h)
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["main"].Do)
	for _, want := range []string{"cg/a.png", "cg/night_1.png"} {
		if _, ok := r.Next().(event.Tap); !ok || h.image != want {
			t.Fatalf("want a tap on %s, shown %s", want, h.image)
		}
	}
	if say, ok := r.Next().(event.Say); !ok || say.Text != "after" || h.image != "cg/night_2.png" {
		t.Fatalf("after the pictures: %+v, shown %s", say, h.image)
	}
}

// page は files の 段階を 1 枚だけ 出し、待たずに 次の 文へ 進む（文章の 間に 段階ごとに 書く）。
// 足りない 段階は 最後の 絵の まま。
func TestImagePage(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  main:
    do:
      - image: {files: [cg/x.png, cg/x.*.png], page: 1}
      - say: one
      - image: {files: [cg/x.png, cg/x.*.png], page: 2}
      - say: two
      - image: {files: [cg/x.png, cg/x.*.png], page: 3}
      - say: three
`, "assets/cg/x.png": "x", "assets/cg/x.002.png": "x"})
	st := testgame.State(db, 1)
	h := &host{}
	r := event.New(db, st, h)
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["main"].Do)
	for _, c := range []struct{ text, image string }{{"one", "cg/x.png"}, {"two", "cg/x.002.png"}, {"three", "cg/x.002.png"}} {
		if say, ok := r.Next().(event.Say); !ok || say.Text != c.text || h.image != c.image {
			t.Fatalf("want %q over %s, got %+v over %s", c.text, c.image, say, h.image)
		}
	}
}

func TestImageFilesMatchingNothingIsAnError(t *testing.T) {
	_, err := testgame.TryLoad(map[string]string{"data/z.yaml": `
events:
  main:
    do:
      - image: {files: [cg/none_*.png]}
`})
	if err == nil {
		t.Fatal("a pattern matching no picture must be reported")
	}
}

// hurt は 戦闘の外で HP を けずる（1 は残る）。affect は 状態に する・治す
func TestHurtAndAffect(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  main:
    do:
      - hurt: {who: all, amount: "self.mhp * 0.5"}
      - affect: {who: all, add: poison}
  more:
    do:
      - hurt: {who: all, amount: "99999"}
      - affect: {who: all, remove: bad}
`})
	st := testgame.State(db, 1)
	r := event.New(db, st, &host{})
	r.Log = func(err error) { t.Error(err) }
	a := st.Members()[0]
	full := a.HP
	r.Start(db.Events["main"].Do)
	for r.Next() != nil {
	}
	if a.HP != full-full/2 || len(a.States) != 1 {
		t.Fatalf("after main: hp %d of %d, states %v", a.HP, full, a.States)
	}
	r.Start(db.Events["more"].Do)
	for r.Next() != nil {
	}
	if a.HP != 1 || len(a.States) != 0 {
		t.Fatalf("after more: hp %d (want 1), states %v", a.HP, a.States)
	}
}

// TestAffectForever は affect で付けた期限なし（turns: 0）の状態が、戦闘中の付与と同じ
// 「期限なし」で持たれ、状態一覧に出ることを確かめる。
func TestAffectForever(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  e:
    do:
      - affect: {who: all, add: poison}
      - affect: {who: all, add: sleep, turns: 3}
`})
	st := testgame.State(db, 1)
	r := event.New(db, st, &host{})
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["e"].Do)
	for r.Next() != nil {
	}
	a := st.Members()[0]
	if a.States["poison"] != rpg.StateForever {
		t.Errorf("poison = %d, want %d (期限なし)", a.States["poison"], rpg.StateForever)
	}
	if a.States["sleep"] != 3 {
		t.Errorf("sleep = %d, want 3", a.States["sleep"])
	}
}

// TestLaterSurvivesEnd は Won で予約した after_win が、元のイベントの end で消えずに後で走ることを確かめる。
func TestLaterSurvivesEnd(t *testing.T) {
	db := testgame.Load(t, map[string]string{
		"game.yaml": testgame.Game + "  after_win: aw\n",
		"data/z.yaml": `
events:
  aw:
    do: [{say: "after"}]
  e:
    do:
      - battle: one
      - end: true
      - say: "never"
`})
	st := testgame.State(db, 1)
	r := event.New(db, st, &host{})
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["e"].Do)
	if _, ok := r.Next().(event.Battle); !ok {
		t.Fatal("no battle")
	}
	r.Answer(event.BattleWon)
	r.Won()
	say, ok := r.Next().(event.Say)
	if !ok || say.Text != "after" {
		t.Fatalf("after_win が走らない: %+v", say)
	}
	if req := r.Next(); req != nil || r.Running() {
		t.Fatalf("残り: %+v", req)
	}
}

// TestSayInstead は say の instead（戦闘で話し手が倒れているときの言い替え）を、元の台詞と同じ作り方で持たせることを確かめる。
func TestSayInstead(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  e:
    do:
      - say: {who: scout, text: "I have {gold}G", instead: [{who: guide, text: "Scout has {gold}G", side: right}, {who: guide, text: ""}]}
`})
	r := event.New(db, testgame.State(db, 1), &host{})
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["e"].Do)
	say, ok := r.Next().(event.Say)
	if !ok || say.Actor != "scout" || len(say.Instead) != 1 {
		t.Fatalf("say = %+v (the empty instead must be dropped)", say)
	}
	if alt := say.Instead[0]; alt.Actor != "guide" || alt.Name != "Guide" || alt.Text != "Scout has 100G" || !alt.Right {
		t.Errorf("instead = %+v", alt)
	}
}

// TestNotice は notice が会話の窓（Say）でなく Notice を返し、答えを待たずに次へ進むことを確かめる。
func TestNotice(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  e:
    do:
      - set: {var.n: 3}
      - notice: "{var.n}G落とした"
      - say: "next"
`})
	st := testgame.State(db, 1)
	r := event.New(db, st, &host{})
	r.Log = func(err error) { t.Error(err) }
	r.Start(db.Events["e"].Do)
	n, ok := r.Next().(event.Notice)
	if !ok || n.Text != "3G落とした" {
		t.Fatalf("notice が返らない: %+v", n)
	}
	if say, ok := r.Next().(event.Say); !ok || say.Text != "next" {
		t.Fatalf("notice の後が進まない: %+v", say)
	}
}

func TestOptionalImageFilesMayMatchNothing(t *testing.T) {
	if _, err := testgame.TryLoad(map[string]string{"data/z.yaml": `
events:
  main:
    do:
      - image: {files: [cg/none_*.png], optional: true}
`}); err != nil {
		t.Fatal(err)
	}
}

func (h *host) Handle(e event.Effect) {
	switch e := e.(type) {
	case event.MoveEffect:
		h.Move(e.Location)
	case event.MusicEffect:
		h.Music(e.Cue)
	case event.MoodEffect:
		h.Mood(e.Name)
	case event.ImageEffect:
		h.Image(e.Picture)
	case event.SoundEffect:
		h.SFX(e.Name)
	case event.VisualEffect:
		h.Effect(e.Name)
	}
}
