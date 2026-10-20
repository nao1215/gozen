package event_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestItemCountVersionCompatibility(t *testing.T) {
	for _, version := range []int{0, 1} {
		for _, verb := range []string{"give", "take"} {
			for _, value := range []string{"", "0", "2", "-2", "null", "'2'", "0.5"} {
				t.Run(fmt.Sprintf("版%d/%s/%s", version, verb, value), func(t *testing.T) {
					count := ""
					if value != "" {
						count = ", count: " + value
					}
					db, err := testgame.TryLoad(map[string]string{
						"game.yaml":       fmt.Sprintf("schema_version: %d\nid: neutral\n", version) + testgame.Game,
						"data/count.yaml": fmt.Sprintf("events: {count_test: {do: [{%s: {item: stick%s}}]}}", verb, count),
					})
					bad := version == 1 && (value == "0.5" || value == "-2" || value == "null" || value == "'2'")
					if bad {
						if err == nil || !strings.Contains(err.Error(), "count") {
							t.Fatalf("個数の診断なし %v", err)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					st := testgame.State(db, 1)
					st.Items["stick"] = 3
					cmd := db.Events["count_test"].Do[0]
					n := 1
					switch value {
					case "0":
						if version == 1 {
							n = 0
						}
					case "2", "'2'":
						n = 2
					case "-2":
						n = -2
					}
					var item *gamedata.ItemCount
					if verb == "give" {
						item = cmd.Give
					} else {
						item = cmd.Take
					}
					if item.N() != n {
						t.Fatalf("N() %d、期待 %d", item.N(), n)
					}
					r := event.New(db, st, &host{})
					r.Start(db.Events["count_test"].Do)
					if got := r.Next(); got != nil {
						t.Fatalf("予期しない待ち %v", got)
					}
					want := 3
					if verb == "give" {
						want += n
					} else if n > 0 {
						want -= n
					}
					if st.Items["stick"] != want || st.Actors["scout"].Equip["weapon"] != "stick" {
						t.Fatalf("品の増減が違う %v、装備 %v", st.Items, st.Actors["scout"].Equip)
					}
				})
			}
		}
	}
	for _, count := range []*int{nil, new(0), new(2), new(-2)} {
		want := 1
		if count != nil {
			want = *count
		}
		item := gamedata.ItemCount{Item: "key", Count: count}
		if got := (gamedata.ItemCount{Item: "key", Count: count}).N(); got != want {
			t.Fatalf("Go の個数 %d、期待 %d", got, want)
		}
		if count == nil || *count >= 0 {
			b, err := yaml.Marshal(item)
			if err != nil {
				t.Fatal(err)
			}
			var restored gamedata.ItemCount
			if err := yaml.Unmarshal(b, &restored); err != nil {
				t.Fatal(err)
			}
			if restored.N() != want || (count == nil && strings.Contains(string(b), "count")) {
				t.Fatalf("個数の YAML 往復で省略・０が変わった %s", b)
			}
		}
	}
	t.Run("旧版のゼロを新版へ移す", func(t *testing.T) {
		for _, verb := range []string{"give", "take"} {
			for _, tc := range []struct{ version, count int }{{0, 0}, {1, 1}} {
				db := testgame.Load(t, map[string]string{
					"game.yaml":       fmt.Sprintf("schema_version: %d\nid: neutral\n", tc.version) + testgame.Game,
					"data/count.yaml": fmt.Sprintf("events: {migration: {do: [{%s: {item: stick, count: %d}}]}}", verb, tc.count),
				})
				st := testgame.State(db, 1)
				// Taking away an item not in the bag removes the equipment. With a new-version 0, the equipment is left alone.
				r := event.New(db, st, &host{})
				r.Start(db.Events["migration"].Do)
				r.Next()
				if verb == "take" && st.Actors["scout"].Equip["weapon"] != "" {
					t.Fatal("移行で装備から取らなくなった")
				}
				if verb == "give" && st.Items["stick"] != 1 {
					t.Fatal("移行で渡す数が変わった")
				}
			}
		}
	})
	t.Run("新版のゼロは装備と能力を触らない", func(t *testing.T) {
		db := testgame.Load(t, map[string]string{
			"game.yaml":       "schema_version: 1\nid: neutral\n" + testgame.Game,
			"data/count.yaml": "events: {zero: {do: [{take: {item: stick, count: 0}}, {give: {item: key, count: 0}}]}}",
		})
		st := testgame.State(db, 1)
		a := st.Actors["scout"]
		a.HP = 999
		r := event.New(db, st, &host{})
		r.Start(db.Events["zero"].Do)
		r.Next()
		if a.HP != 999 || a.Equip["weapon"] != "stick" || st.ItemCount("key", true) != 0 {
			t.Fatal("増減なしの命令が状態を変えた")
		}
	})
}

func TestNumericPresenceWait(t *testing.T) {
	for _, value := range []string{"0", "0.5", "-1", "null", ".nan", ".inf"} {
		db, err := testgame.TryLoad(map[string]string{
			"game.yaml":      "schema_version: 1\nid: neutral\n" + testgame.Game,
			"data/wait.yaml": "events: {waiting: {do: [{wait: " + value + "}, {gold: '7'}]}}",
		})
		if value != "0" && value != "0.5" {
			if err == nil || !strings.Contains(err.Error(), "wait") {
				t.Fatal("待ち時間の診断なし", err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		st := testgame.State(db, 1)
		r := event.New(db, st, &host{})
		r.Start(db.Events["waiting"].Do)
		w, ok := r.Next().(event.Wait)
		want := 0.
		if value == "0.5" {
			want = 0.5
		}
		if !ok || w.Seconds != want {
			t.Fatalf("待ち時間 %v", w)
		}
		r.Next()
		if st.Gold != 107 {
			t.Fatal("待った後へ進まない")
		}
	}
}
