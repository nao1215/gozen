package rpg_test

import (
	"encoding/json"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/expr"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

type keyed interface{ Keys() []string }

// TestViewsListTheirKeys: the objects an expression reads can list their names (so that Query can turn them into Go values).
func TestViewsListTheirKeys(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	st.Flags["met"], st.Flags["armed"] = true, true
	st.Vars["b"], st.Vars["a"] = 1.0, "x"
	st.AddItem("key", 1)
	env := st.Env()
	look := func(name string) keyed {
		t.Helper()
		v, ok := env.Lookup(name)
		if !ok {
			t.Fatalf("no %s", name)
		}
		k, ok := v.(keyed)
		if !ok {
			t.Fatalf("%s is %T, which lists no keys", name, v)
		}
		return k
	}
	if got := look("flag").Keys(); !slices.Equal(got, []string{"armed", "met"}) {
		t.Errorf("flag keys %v", got)
	}
	if got := look("var").Keys(); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("var keys %v", got)
	}
	if got := look("item").Keys(); !slices.Equal(got, []string{"key", "potion"}) {
		t.Errorf("item keys %v", got)
	}
	if got := look("actor").Keys(); !slices.Equal(got, []string{"scout"}) {
		t.Errorf("actor keys %v", got)
	}
	party := look("party")
	if !slices.Contains(party.Keys(), "size") || !slices.Contains(party.Keys(), "members") {
		t.Errorf("party keys %v", party.Keys())
	}
	actor, _ := look("actor").(expr.Object).Field("scout")
	ak := actor.(keyed).Keys()
	for _, want := range []string{"id", "name", "hp", "in_party", "atk", "equip", "wears"} {
		if !slices.Contains(ak, want) {
			t.Errorf("an actor lists %q: %v", want, ak)
		}
	}
	equip, _ := actor.(expr.Object).Field("equip")
	if got := equip.(keyed).Keys(); !slices.Equal(got, []string{"weapon"}) {
		t.Errorf("equip keys %v", got)
	}
	wears, _ := actor.(expr.Object).Field("wears")
	if got := wears.(keyed).Keys(); !slices.Equal(got, []string{"stick"}) {
		t.Errorf("wears keys %v", got)
	}
	states, _ := actor.(expr.Object).Field("states")
	if got := states.(keyed).Keys(); len(got) != 0 {
		t.Errorf("states keys %v", got)
	}
	if got := (rpg.WearsView{}).Keys(); got != nil {
		t.Errorf("nobody wears nothing: %v", got)
	}
}

// TestSaveOfNothing: a state with no game data cannot be saved; no data cannot load a save.
func TestSaveOfNothing(t *testing.T) {
	if _, err := (*rpg.State)(nil).Save(); err == nil {
		t.Error("saving a nil state")
	}
	if _, err := (&rpg.State{}).Save(); err == nil {
		t.Error("saving a state with no data")
	}
	db := testgame.Load(t, nil)
	b, err := testgame.State(db, 1).Save()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rpg.Parse(nil, b, nil); err == nil {
		t.Error("loading with no data")
	}
}

// TestLegacySavesNeedPermissionAndFillMissingTables: a save with no envelope is read only when the caller vouches for it;
// maps the old format never had come back empty, and the report says what was done.
func TestLegacySavesNeedPermissionAndFillMissingTables(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	b, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		State map[string]json.RawMessage `json:"state"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"flags", "done", "vars", "items", "mapped"} {
		delete(env.State, k)
	}
	legacy, _ := json.Marshal(env.State)
	if _, err := rpg.Parse(db, legacy, nil); err == nil || !strings.Contains(err.Error(), "識別") && !strings.Contains(err.Error(), "identif") {
		t.Fatalf("a save with no version needs permission: %v", err)
	}
	got, err := rpg.ParseWithConfig(db, legacy, rand.New(rand.NewPCG(1, 2)), rpg.Config{AllowLegacy: true})
	if err != nil {
		t.Fatal(err)
	}
	if !got.LoadReport.Legacy || !got.LoadReport.Changed() {
		t.Errorf("report %+v", got.LoadReport)
	}
	if got.Flags == nil || got.Done == nil || got.Vars == nil || got.Items == nil || got.Mapped == nil {
		t.Error("the tables a legacy save lacks must be usable maps")
	}
	got.Flags["x"] = true // must not panic
	if len(got.Party) != 1 || got.Party[0] != "scout" {
		t.Errorf("party %v", got.Party)
	}
}

func TestSaveFileKeepsWhatItReplaces(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	path := filepath.Join(t.TempDir(), "slot", "save.json")
	if backup, err := st.SaveFile(path); err != nil || backup != "" {
		t.Fatalf("a first save has nothing to back up: %q %v", backup, err)
	}
	st.Gold = 55
	if backup, err := st.SaveFile(path); err != nil || backup != "" {
		t.Fatalf("a normal save over a normal save: %q %v", backup, err)
	}
	// an unreadable file is kept as a backup before being replaced
	if err := os.WriteFile(path, []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	backup, err := st.SaveFile(path)
	if err != nil || backup == "" {
		t.Fatalf("garbage is backed up: %q %v", backup, err)
	}
	if b, _ := os.ReadFile(backup); string(b) != "garbage" { //nolint:gosec // Path is isolated in t.TempDir.
		t.Errorf("backup holds %q", b)
	}
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), `"format_version"`) { //nolint:gosec // Path is isolated in t.TempDir.
		t.Errorf("the slot was not replaced: %q", b)
	}
	// a directory where the slot should be
	dir := filepath.Join(t.TempDir(), "isdir")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveFile(dir); err == nil {
		t.Error("a directory cannot be a slot")
	}
	if _, err := (*rpg.State)(nil).SaveFile(path); err == nil {
		t.Error("a nil state cannot be saved to a file")
	}
}
