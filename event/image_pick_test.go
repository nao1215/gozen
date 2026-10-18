package event_test

import (
	"testing"

	"github.com/nao1215/gozen/event"
	"github.com/nao1215/gozen/internal/testgame"
)

// pick は files の 何枚目を 式で 選ぶ（値を 枚数で 割った 余り + 1 枚目）。負の 値も 余りを 正に して 選ぶ。
func TestImagePick(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
events:
  main:
    do:
      - image: {files: ["cg/d_*.png"], pick: "var.n", look: dream}
      - say: shown
`, "assets/cg/d_1.png": "x", "assets/cg/d_2.png": "x", "assets/cg/d_3.png": "x"})
	for _, c := range []struct {
		n    float64
		want string
	}{{0, "cg/d_1.png"}, {1, "cg/d_2.png"}, {2, "cg/d_3.png"}, {4, "cg/d_2.png"}, {-1, "cg/d_3.png"}} {
		st := testgame.State(db, 1)
		st.Vars["n"] = c.n
		h := &host{}
		r := event.New(db, st, h)
		r.Log = func(err error) { t.Error(err) }
		r.Start(db.Events["main"].Do)
		if say, ok := r.Next().(event.Say); !ok || say.Text != "shown" || h.image != c.want {
			t.Errorf("var.n = %v: shown %s, want %s (%+v)", c.n, h.image, c.want, say)
		}
	}
}

func TestImagePickAndLookAreChecked(t *testing.T) {
	for _, bad := range []string{
		`{file: cg/d_1.png, pick: "1"}`,             // files が 要る
		`{files: [cg/d_1.png], page: 1, pick: "1"}`, // page と 一緒に 書かない
		`{file: cg/d_1.png, look: glow}`,            // look は dream だけ
		`{file: cg/d_1.png, haze: "#ffc0d0"}`,       // haze は look: dream と
	} {
		_, err := testgame.TryLoad(map[string]string{"data/z.yaml": "events:\n  main:\n    do:\n      - image: " + bad + "\n",
			"assets/cg/d_1.png": "x"})
		if err == nil {
			t.Errorf("%s: want an error", bad)
		}
	}
}
