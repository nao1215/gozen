package world_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
	lang "github.com/nao1215/gozen/text"
	"github.com/nao1215/gozen/world"
)

func labels(es []world.Entry) string {
	var s []string
	for _, e := range es {
		s = append(s, e.Label)
	}
	return strings.Join(s, "|")
}

func TestEntries(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	if got, want := labels(world.Entries(db, st)), "話す：Courier|Rest (5G)|→ Field|メニュー"; got != want {
		t.Errorf("entries = %q, want %q", got, want)
	}
	st.Flags["ghost"] = true
	st.AddItem("key", 1)
	if got := labels(world.Entries(db, st)); !strings.Contains(got, "Watcher") || !strings.Contains(got, "Vault") {
		t.Errorf("entries with the flag and the key = %q", got)
	}
}

func TestArriveOnceAndEncounters(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	st.Location = "field"
	cmds := world.Arrive(db, st, true)
	if len(cmds) != 1 || cmds[0].Say == nil {
		t.Fatalf("first arrival: %+v", cmds)
	}
	// the greeting is once: the next arrival meets a battle (rate 1)
	cmds = world.Arrive(db, st, true)
	if len(cmds) != 1 || cmds[0].Battle != "one" {
		t.Fatalf("second arrival: %+v", cmds)
	}
	if cmds := world.Arrive(db, st, false); len(cmds) != 0 {
		t.Errorf("a quiet arrival met %+v", cmds)
	}
	e := world.Entries(db, st)
	if e[0].Kind != world.Search || e[0].Commands(db, st)[0].Battle != "one" {
		t.Errorf("search = %+v", e[0])
	}
}

// 人と項目の mark が成り立つ間だけ、一覧のラベルに印が付く（書かなければ付かない）。
// new は新しい話の手がかり（Entry.New）で、印は付けない。印の文字はゲームの text で変えられる。
func TestNewTalkMark(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/plaza.yaml": `
locations:
  plaza:
    name: Plaza
    people:
      - {name: Old, talk: [{say: same}]}
      - {name: Bard, new: "!flag.heard", mark: "!flag.heard", talk: [{set: {flag.heard: true}}]}
      - {name: Poet, new: "!flag.heard", talk: [{say: hi}]}
    actions:
      - {label: Dance, mark: "var.songs > 0", do: []}
    exits: [{to: camp}]
`})
	st := testgame.State(db, 1)
	st.Location = "plaza"
	got := world.Entries(db, st)
	if got[0].Label != "話す：Old" || got[0].New || got[0].Marked {
		t.Errorf("a person without new = %+v", got[0])
	}
	if got[1].Label != "話す：Bard　！" || !got[1].New || !got[1].Marked {
		t.Errorf("a person with a mark = %+v", got[1])
	}
	if got[2].Label != "話す：Poet" || !got[2].New || got[2].Marked {
		t.Errorf("new alone puts no mark = %+v", got[2])
	}
	if got[3].Label != "Dance" || got[3].Marked {
		t.Errorf("an action whose mark does not hold = %+v", got[3])
	}
	st.Flags["heard"] = true
	st.Vars["songs"] = 1.0
	got = world.Entries(db, st)
	if got[1].Label != "話す：Bard" || got[1].New || got[3].Label != "Dance　！" || !got[3].Marked {
		t.Errorf("after hearing: %q", labels(got))
	}
	if err := lang.Use(lang.Japanese, map[string]string{"　！": " (new)"}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := lang.Use(lang.Japanese, nil); err != nil {
			t.Error(err)
		}
	}()
	if got := world.Entries(db, st); got[3].Label != "Dance (new)" {
		t.Errorf("the game's mark: %q", got[3].Label)
	}
}

// talk_on_enter の 場所は「話す：〜」を 並べず、入る たびに 1 人の 話を 流す（新しい 話の ある 人が 先、無ければ 順番に）。
func TestTalkOnEnter(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/inn.yaml": `
locations:
  inn:
    name: Inn
    talk_on_enter: true
    people:
      - {name: Old, talk: [{say: old}]}
      - {name: Bard, new: "!flag.heard", talk: [{say: bard}, {set: {flag.heard: true}}]}
      - {name: Poet, talk: [{say: poet}]}
    exits: [{to: camp}]
`})
	st := testgame.State(db, 1)
	st.Location = "inn"
	for _, e := range world.Entries(db, st) {
		if e.Kind == world.Talk {
			t.Fatalf("a talk entry is listed: %q", e.Label)
		}
	}
	say := func() string {
		cmds := world.Arrive(db, st, false)
		for _, c := range cmds {
			if c.Say != nil {
				if c.Say.Text == "bard" {
					st.Flags["heard"] = true
				}
				return c.Say.Text
			}
		}
		return ""
	}
	var got []string
	for range 4 {
		got = append(got, say())
	}
	if strings.Join(got, " ") != "bard old bard poet" && strings.Join(got, " ") != "bard old poet old" {
		t.Errorf("talks on entering: %q", got)
	}
	if got[0] != "bard" {
		t.Errorf("the new talk does not come first: %q", got)
	}
}
