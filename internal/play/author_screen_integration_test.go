package play

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/session"
)

// The visible answer index must identify the same authored action in both
// frontends, including when a preceding entry is filtered out.
func TestAuthorScreenSessionEntryContract(t *testing.T) {
	for _, amount := range []int{0, 2} {
		t.Run(strconv.Itoa(amount), func(t *testing.T) {
			fsys := wldWorldFiles(t, `screens:
  stock:
    params: {amount: number}
    title: 'Stock {arg.amount}'
    entries:
    - {label: Hidden, when: 'arg.amount < 0', do: [{set: {var.selected: 99}}]}
    - {label: 'Collect {arg.amount}', when: 'arg.amount > 0', do: [{set: {var.selected: 1}}]}
    - {label: Inspect, text: 'Amount {arg.amount}', do: [{set: {var.selected: 2}}]}
`, map[string]string{"data/stock.yaml": fmt.Sprintf(`events:
  inspect_stock:
    do:
    - open: {screen: stock, args: {amount: '%d'}}
    - set: {var.closed: 1}
`, amount)})
			fsys["game.yaml"].Data = []byte(strings.Replace(string(fsys["game.yaml"].Data), "schema_version: 4", "schema_version: 10", 1))
			g := wldOpenWith(t, Options{FS: fsys, Location: "camp", Seed: 17})
			wldFrames(t, g, 30)
			sg, err := session.Load(fsys, ".")
			if err != nil {
				t.Fatal(err)
			}
			ss, err := session.New(sg, session.Config{Seed: 17})
			if err != nil {
				t.Fatal(err)
			}
			if err := ss.Start("- call: inspect_stock"); err != nil {
				t.Fatal(err)
			}
			wldStartEvent(t, g, "inspect_stock")
			wldFrames(t, g, 4)
			q := ss.Next()
			view := sysAs[*authorScreen](t, g.stack[len(g.stack)-1])
			want := []string{"Inspect"}
			indices := []int{2}
			if amount > 0 {
				want = []string{"Collect 2", "Inspect"}
				indices = []int{1, 2}
			}
			labels := make([]string, len(view.entries))
			for i, e := range view.entries {
				labels[i] = e.Label
			}
			if q.Kind != "open" || q.Text != fmt.Sprintf("Stock %d", amount) || q.Text != view.title || !reflect.DeepEqual(q.Options, want) || !reflect.DeepEqual(labels, want) || !reflect.DeepEqual(view.indices, indices) {
				t.Fatalf("session=%+v player=%s/%v/%v", q, view.title, labels, view.indices)
			}
			// Caller-owned request labels must not alter subsequent presentation.
			q.Options[0] = "Changed"
			if got := ss.Next(); !reflect.DeepEqual(got.Options, want) {
				t.Fatal(got)
			}
			ss.Answer(len(want) - 1)
			view.list.sel = len(want) - 1
			wldPlay(t, g, "press z\nwait 8\n", 40)
			q = ss.Next()
			var selected float64
			if err := ss.Query("var.selected", &selected); err != nil {
				t.Fatal(err)
			}
			if selected != 2 || g.st.Vars["selected"] != float64(2) || q.Kind != "open" {
				t.Fatalf("wrong authored action: %v/%v/%+v", selected, g.st.Vars, q)
			}
			ss.Answer(-1)
			ss.Next()
			wldPlay(t, g, "press x\nwait 8\n", 40)
			var closed float64
			if err := ss.Query("var.closed", &closed); err != nil {
				t.Fatal(err)
			}
			if closed != 1 || g.st.Vars["closed"] != float64(1) {
				t.Fatal("opener did not continue once")
			}
		})
	}
}
