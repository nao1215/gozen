package rpg_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestNarrativeSaveRoundTrip(t *testing.T) {
	db, err := data.Load(fstest.MapFS{
		"game.yaml":       {Data: []byte("schema_version: 8\nid: narrative\nstart: {location: station}\n")},
		"data/story.yaml": {Data: []byte("actors: {guide: {name: Rowan}}\nlocations: {station: {name: Station}}\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	st := testgame.State(db, 1)
	st.Flags["arrived"] = true
	st.Vars["visits"] = float64(3)
	if st.Join("guide") {
		t.Fatal("a speaker joined without a class")
	}
	saved := saveBytes(t, st)
	restored, err := rpg.Parse(db, saved, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.Party) != 0 || len(restored.Actors) != 0 || !restored.Flags["arrived"] || restored.Vars["visits"] != float64(3) || restored.Location != "station" {
		t.Fatalf("lost narrative state: %+v", restored)
	}
}

// TestStreamsSaveRoundTrip keeps the progress of the live streams; a save without it
// loads with none.
func TestStreamsSaveRoundTrip(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	if restored, err := rpg.Parse(db, saveBytes(t, st), nil); err != nil || restored.Streams != nil {
		t.Fatalf("a save without streams: %v %+v", err, restored)
	}
	st.Streams = &rpg.Streams{Open: 1700000000, Viewer: "Pat", Channels: map[string]*rpg.ChannelProgress{"ann": {Next: 2, Points: 31.5, Day: 20000, Run: 3}}, Duos: map[string]int{"ann+bo": 1}}
	restored, err := rpg.Parse(db, saveBytes(t, st), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.Streams, st.Streams) {
		t.Fatalf("streams %+v, want %+v", restored.Streams, st.Streams)
	}
	// The next episode is an index the shows are taken round from: a save with one below 0
	// or past 1<<30 (where adding the episodes of a slot could overflow) does not load.
	for _, tc := range []struct {
		name    string
		channel int
		duo     int
		want    string
	}{
		{"at the limit", 1 << 30, 1 << 30, ""},
		{"a channel below 0", -1, 0, "streams/channels/ann/next"},
		{"a channel past the limit", 1<<30 + 1, 0, "streams/channels/ann/next"},
		{"a duo below 0", 0, -1, "streams/duos/ann+bo"},
		{"a duo past the limit", 0, math.MaxInt, "streams/duos/ann+bo"},
	} {
		st.Streams.Channels["ann"].Next, st.Streams.Duos["ann+bo"] = tc.channel, tc.duo
		_, err := rpg.Parse(db, saveBytes(t, st), nil)
		if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
			t.Errorf("%s: %v, want %q", tc.name, err, tc.want)
		}
	}
}

func saveBytes(t *testing.T, s *rpg.State) []byte {
	t.Helper()
	b, err := s.Save()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// object walks nested JSON objects by key. A fixture of another shape is a broken
// test, so it panics with the path, which fails the running test.
func object(m map[string]any, keys ...string) map[string]any {
	for i, k := range keys {
		next, ok := m[k].(map[string]any)
		if !ok {
			panic(fmt.Sprintf("fixture: %v is %T, not an object", keys[:i+1], m[k]))
		}
		m = next
	}
	return m
}

func rewriteSave(t *testing.T, b []byte, edit func(map[string]any)) []byte {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal(b, &obj); err != nil {
		t.Fatal(err)
	}
	edit(obj)
	out, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSaveEnvelopeCompatibility(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.ID, db.Game.DataVersion = "test-adventure", 2
	st := testgame.State(db, 1)
	b := saveBytes(t, st)
	var envelope struct {
		FormatVersion int             `json:"format_version"`
		GameID        string          `json:"game_id"`
		DataVersion   int             `json:"data_version"`
		State         json.RawMessage `json:"state"`
	}
	if err := json.Unmarshal(b, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.FormatVersion != 2 || envelope.GameID != db.Game.ID || envelope.DataVersion != 2 || len(envelope.State) == 0 {
		t.Fatalf("識別項目 %s", b)
	}
	t.Run("新形式の往復と乱数", func(t *testing.T) {
		rng := rand.New(rand.NewPCG(9, 8))    //nolint:gosec // Deterministic gameplay test, not a security token.
		oracle := rand.New(rand.NewPCG(9, 8)) //nolint:gosec // Deterministic gameplay test, not a security token.
		got, err := rpg.Parse(db, b, rng)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(b, saveBytes(t, got)) || got.LoadReport.Changed() {
			t.Fatal("往復で保存状態が変わった")
		}
		if rng.Uint64() != oracle.Uint64() {
			t.Fatal("ロードが乱数を消費した")
		}
	})
	t.Run("削除前の版から移行", func(t *testing.T) {
		older := rewriteSave(t, b, func(m map[string]any) { m["data_version"] = 1 })
		got, err := rpg.Parse(db, older, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.LoadReport.SavedDataVersion != 1 || got.LoadReport.DataVersion != 2 || !got.LoadReport.Changed() {
			t.Fatalf("版の通知 %+v", got.LoadReport)
		}
	})
	tests := []struct {
		name string
		edit func(map[string]any)
	}{
		{"別作品", func(m map[string]any) { m["game_id"] = "other" }},
		{"未来形式", func(m map[string]any) { m["format_version"] = 3; m["state"] = "future" }},
		{"未来データ", func(m map[string]any) { m["data_version"] = 3 }},
		{"負のデータ版", func(m map[string]any) { m["data_version"] = -1 }},
		{"包みの版０", func(m map[string]any) { m["format_version"] = 0 }},
		{"本文欠損", func(m map[string]any) { delete(m, "state") }},
		{"本文null", func(m map[string]any) { m["state"] = nil }},
		{"本文空", func(m map[string]any) { m["state"] = map[string]any{} }},
		{"未知項目", func(m map[string]any) { m["future"] = true }},
		{"未知状態項目", func(m map[string]any) { object(m, "state")["future"] = true }},
		{"仲間null", func(m map[string]any) { object(m, "state", "actors")["scout"] = nil }},
		{"仲間ID不一致", func(m map[string]any) {
			object(m, "state", "actors", "scout")["id"] = "guide"
		}},
		{"人数重複", func(m map[string]any) { object(m, "state")["party"] = []string{"scout", "scout"} }},
		{"仲間の記録欠損", func(m map[string]any) { object(m, "state")["party"] = []string{"guide"} }},
		{"所持金null", func(m map[string]any) { object(m, "state")["gold"] = nil }},
		{"所持品個数null", func(m map[string]any) { object(m, "state", "items")["potion"] = nil }},
		{"本文の大文字違い", func(m map[string]any) {
			state := object(m, "state")
			state["Gold"] = state["gold"]
			delete(state, "gold")
		}},
	}
	for _, key := range []string{"format_version", "data_version", "game_id"} {
		for _, value := range []any{nil, true, []any{}, map[string]any{}, 1.5} {
			tests = append(tests, struct {
				name string
				edit func(map[string]any)
			}{key + "/" + string(mustJSON(t, value)), func(m map[string]any) { m[key] = value }})
		}
		tests = append(tests, struct {
			name string
			edit func(map[string]any)
		}{key + "/欠損", func(m map[string]any) { delete(m, key) }})
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bad := rewriteSave(t, b, tc.edit)
			before := bytes.Clone(bad)
			for _, legacy := range []bool{false, true} {
				if got, err := rpg.ParseWithConfig(db, bad, nil, rpg.Config{AllowLegacy: legacy}); err == nil || got != nil {
					t.Fatalf("不正な保存を受け入れた %s", bad)
				}
			}
			if !bytes.Equal(before, bad) {
				t.Fatal("入力を変更した")
			}
		})
	}
	for _, bad := range [][]byte{nil, []byte(`null`), []byte(`[]`), []byte(`{}`), b[:len(b)-1], append(bytes.Clone(b), []byte(` {}`)...), bytes.Replace(b, []byte(`"format_version": 2`), []byte(`"format_version": 2, "format_version": 2`), 1), bytes.Replace(b, []byte(`"gold": 100`), []byte(`"gold": 100, "gold": 200`), 1)} {
		if got, err := rpg.ParseWithConfig(db, bad, nil, rpg.Config{AllowLegacy: true}); err == nil || got != nil {
			t.Errorf("破損を受け入れた %s", bad)
		}
	}
	t.Run("ID省略の旧作品", func(t *testing.T) {
		old := testgame.Load(t, nil)
		bytes := saveBytes(t, testgame.State(old, 1))
		if _, err := rpg.Parse(old, bytes, nil); err != nil {
			t.Fatal(err)
		}
		old.Game.ID = "rpg"
		if _, err := rpg.Parse(old, bytes, nil); err == nil {
			t.Fatal("空IDを勝手に別のIDへ移した")
		}
	})
	t.Run("旧fixtureの明示移行", func(t *testing.T) {
		legacyState := testgame.State(db, 17)
		legacyState.Join("guide")
		legacyState.Party = []string{"guide", "scout"}
		legacyState.Gold, legacyState.Items = 321, map[string]int{}
		legacyState.Actors["scout"].HP = 17
		legacyState.Steps, legacyState.Frames = 123, 4567
		legacyState.Done["greet"] = true
		legacy, err := json.Marshal(legacyState)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := rpg.Parse(db, legacy, nil); err == nil {
			t.Fatal("外部旧形式を暗黙に受け入れた")
		}
		got, err := rpg.ParseWithConfig(db, legacy, nil, rpg.Config{AllowLegacy: true})
		if err != nil {
			t.Fatal(err)
		}
		if !got.LoadReport.Legacy || got.Gold != 321 || len(got.Items) != 0 || got.Party[0] != "guide" || got.Actors["scout"].HP != 17 || got.Steps != 123 || got.Frames != 4567 || !got.Done["greet"] {
			t.Fatalf("移行で主要状態が変わった %+v", got)
		}
		modern := saveBytes(t, got)
		again, err := rpg.Parse(db, modern, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(modern, saveBytes(t, again)) || again.LoadReport.Changed() {
			t.Fatal("書き戻し後の状態が変わった")
		}
		path := filepath.Join(t.TempDir(), "save1.json")
		if err := os.WriteFile(path, legacy, 0o600); err != nil {
			t.Fatal(err)
		}
		backup, err := got.SaveFile(path)
		if err != nil {
			t.Fatal(err)
		}
		saved, err := os.ReadFile(backup) //nolint:gosec // Paths are isolated test fixtures or declared schema artifact paths.
		if err != nil || !bytes.Equal(saved, legacy) {
			t.Fatalf("元ファイルの控え %s %v", backup, err)
		}
		next, err := got.SaveFile(path)
		if err != nil || next != "" {
			t.Fatalf("移行済みなのに控えを増やした %q %v", next, err)
		}
	})
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestLoadReportsRemovedData(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.ID = "test-adventure"
	db.Classes["ranger"].Tree = []data.TreeNode{{ID: "cure", Skill: "heal", Max: 5}}
	st := testgame.State(db, 1)
	st.Join("guide")
	a := st.Actors["scout"]
	a.Learnt = []string{"fire", "heal"}
	a.States["poison"] = 3
	a.Tree["cure"] = 8
	a.Tree["old"] = 3
	a.SP = 2
	st.Items["key"] = 1
	st.Location = "vault"
	st.Flags["free_flag"] = true
	st.Done["old_event"] = true
	st.Vars["old_var"] = "kept"
	b := saveBytes(t, st)
	delete(db.Actors, "guide")
	delete(db.Skills, "fire")
	delete(db.States, "poison")
	delete(db.Items, "stick")
	delete(db.Items, "key")
	delete(db.Locations, "vault")
	got, err := rpg.Parse(db, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, c := range got.LoadReport.Changes {
		paths = append(paths, c.Path+"/"+c.ID)
		if c.Reason == "" {
			t.Fatal("理由がない")
		}
	}
	want := []string{"actors/guide", "actors/scout/equip/weapon/stick", "actors/scout/learnt/fire", "actors/scout/states/poison", "actors/scout/tree/cure", "actors/scout/tree/old", "items/key", "location/vault"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("変更一覧\n%v\n必要な一覧\n%v", paths, want)
	}
	if got.Actors["scout"].SP != 8 || got.Location != "camp" || !slices.Equal(got.Party, []string{"scout"}) || len(got.Actors["scout"].Equip) != 0 || !got.Flags["free_flag"] || !got.Done["old_event"] || got.Vars["old_var"] != "kept" {
		t.Fatal("補正結果が不正")
	}
	for range 10 {
		next, err := rpg.Parse(db, b, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(next.LoadReport, got.LoadReport) {
			t.Fatal("通知順が不定")
		}
	}
	path := filepath.Join(t.TempDir(), "save.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path) //nolint:gosec // Paths are isolated test fixtures or declared schema artifact paths.
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, b) {
		t.Fatal("ロードで元ファイルを変えた")
	}
	backup, err := got.SaveFile(path)
	if err != nil {
		t.Fatal(err)
	}
	original, err = os.ReadFile(backup) //nolint:gosec // Paths are isolated test fixtures or declared schema artifact paths.
	if err != nil || !bytes.Equal(original, b) {
		t.Fatal("削除前の状態を失った")
	}
	modern, err := os.ReadFile(path) //nolint:gosec // Paths are isolated test fixtures or declared schema artifact paths.
	if err != nil {
		t.Fatal(err)
	}
	again, err := rpg.Parse(db, modern, nil)
	if err != nil || again.LoadReport.Changed() {
		t.Fatalf("補正を繰り返した %v", err)
	}
	if !bytes.Equal(modern, saveBytes(t, again)) {
		t.Fatal("補正後の往復で変わった")
	}
	delete(db.Actors, "scout")
	if _, err := rpg.Parse(db, b, nil); err == nil || !strings.Contains(err.Error(), "仲間") {
		t.Fatalf("全員消えた保存を受け入れた %v", err)
	}
}

func TestLoadReportsRemovedDungeonData(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Locations["old/maze"] = &data.Location{Dungeon: &data.Dungeon{Tiles: map[string]*data.Tile{"S": {}}, Roamers: []data.Roamer{{}}}}
	st := testgame.State(db, 1)
	st.Location = "old/maze"
	st.Pos = rpg.Pos{In: "old/maze", X: 1, Y: 1, Facing: 2}
	st.Mapped = map[string][]byte{"old/maze/1": {1}, "gone/1": {1}}
	st.Put = map[string]string{"old/maze/1@1,1": "S", "old/maze/1@2,1": "Z", "gone/1@1,1": "S"}
	st.Roamers = []rpg.Roamer{{Kind: 0, Troop: "one", X: 2, Y: 1}, {Kind: 1, Troop: "one"}, {Kind: 0, Troop: "gone"}}
	got, err := rpg.Parse(db, saveBytes(t, st), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Mapped) != 1 || len(got.Put) != 1 || got.Put["old/maze/1@1,1"] != "S" || len(got.Roamers) != 1 || got.Pos != st.Pos {
		t.Fatalf("迷宮の補正が不正 %+v", got.LoadReport)
	}
	if len(got.LoadReport.Changes) != 5 {
		t.Fatalf("迷宮の削除一覧が不足 %+v", got.LoadReport)
	}
	b := saveBytes(t, got)
	delete(db.Locations, "old/maze")
	moved, err := rpg.Parse(db, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if moved.Location != "camp" || moved.Pos.In != "" || len(moved.Roamers) != 0 || len(moved.Mapped) != 0 || len(moved.Put) != 0 {
		t.Fatal("消えた迷宮の記録を残した")
	}
}

func TestSaveKeepsDungeonRecordsAfterReturningToTown(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Locations["maze"] = &data.Location{Dungeon: &data.Dungeon{Roamers: []data.Roamer{{}}}}
	st := testgame.State(db, 1)
	st.Pos = rpg.Pos{In: "maze", X: 2, Y: 3, Facing: 1}
	st.Roamers = []rpg.Roamer{{Kind: 0, Troop: "one", X: 3, Y: 4, Calm: 10, Walk: 2}}
	st.Mapped["maze/1"] = []byte{1, 0, 1}
	b := saveBytes(t, st)
	got, err := rpg.Parse(db, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.LoadReport.Changed() || !bytes.Equal(b, saveBytes(t, got)) {
		t.Fatalf("帰還後の有効な迷宮の記録を変更した %+v", got.LoadReport)
	}
}

func TestCheckedSavePreservesAndClonesRandomState(t *testing.T) {
	for _, mode := range []string{"between", "event", "none", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			db := testgame.Load(t, nil)
			st := testgame.State(db, 7)
			saved := saveBytes(t, st)
			want, err := rand.NewPCG(73, 91).MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "between":
				saved, err = rpg.WithRand(saved, want)
			case "event":
				saved, err = rpg.WithCursor(saved, rpg.Cursor{Event: "main", Line: 1}, want)
			case "legacy":
				saved, err = json.Marshal(st)
				want = nil
			default:
				want = nil
			}
			if err != nil {
				t.Fatal(err)
			}
			var previous []byte
			for range 3 {
				loaded, err := rpg.ParseWithConfig(db, saved, nil, rpg.Config{AllowLegacy: mode == "legacy"})
				if err != nil {
					t.Fatal(err)
				}
				got := loaded.SavedRand
				if mode == "event" {
					if loaded.Resume == nil || loaded.SavedRand != nil {
						t.Fatal("event cursor random state mixed with between-event state")
					}
					got = loaded.Resume.Rand
				} else if loaded.Resume != nil {
					t.Fatal("unexpected cursor")
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("cached random bytes = %x, want %x", got, want)
				}
				if len(previous) > 0 {
					previous[0] ^= 1
					if !bytes.Equal(got, want) {
						t.Fatal("parsed states share random bytes")
					}
				}
				previous = got
				if len(got) > 0 {
					got[0] ^= 1
				}
			}
		})
	}
}

func TestSaveRejectsInvalidBattleOutcome(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	st.RecordBattle("lose", st.Members())
	saved := saveBytes(t, st)
	for _, bad := range []map[string]any{
		{"result": "lost", "hp": map[string]any{"scout": 0}},
		{"result": "lose", "hp": map[string]any{"scout": -1}},
		{"result": "lose", "hp": map[string]any{"": 0}},
	} {
		raw := rewriteSave(t, saved, func(m map[string]any) { object(m, "state")["last_battle"] = bad })
		if _, err := rpg.Parse(db, raw, nil); err == nil {
			t.Fatalf("accepted invalid outcome: %v", bad)
		}
	}
}
