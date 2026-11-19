package gamedata

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

func TestLoadNarrativeWithoutCombat(t *testing.T) {
	for _, tc := range []struct{ party, command, want string }{
		{"", "say: {who: guide, text: Welcome}", ""},
		{"party: [guide],", "say: Welcome", "needs a class"},
		{"", "join: guide", "needs a class"},
	} {
		fsys := fstest.MapFS{
			"game.yaml":       {Data: []byte("schema_version: 8\nid: story\nstart: {" + tc.party + "location: station, event: opening}\n")},
			"data/story.yaml": {Data: []byte("actors:\n  guide: {name: Rowan}\nlocations:\n  station: {name: Station}\nevents:\n  opening:\n    do:\n      - " + tc.command + "\n")},
		}
		db, err := Load(fsys)
		if tc.want != "" {
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(db.Classes) != 0 || len(db.Skills) != 0 || len(db.Game.Start.Party) != 0 {
			t.Fatal("narrative acquired combat definitions")
		}
	}
}

func TestFacePictureWithoutPortrait(t *testing.T) {
	db := &DB{}
	for _, actor := range []*Actor{nil, {Name: "Rowan"}} {
		for _, outfit := range []string{"", "travel"} {
			if got := db.FacePicture(actor, "normal", outfit); got != "" {
				t.Fatalf("portrait without a directory: %q", got)
			}
		}
	}
}

func TestCompactEventAndItems(t *testing.T) {
	for _, body := range []string{
		"  opening:\n    - give: token\n    - take: token\n    - give: {item: token, count: 0}\n    - wait: {seconds: 0}\n",
		"  opening: [{give: token}, {take: token}, {give: {item: token, count: 0}}, {wait: {seconds: 0}}]\n",
	} {
		fsys := fstest.MapFS{
			"game.yaml":       {Data: []byte("schema_version: 11\nid: story\nstart: {location: station, event: opening}\n")},
			"data/story.yaml": {Data: []byte("locations: {station: {name: Station}}\nitems: {token: {name: Token}}\nevents:\n" + body)},
		}
		db, err := Load(fsys)
		if err != nil {
			t.Fatal(err)
		}
		commands := db.Events["opening"].Do
		if len(commands) != 4 || commands[0].Give.N() != 1 || commands[1].Take.N() != 1 || commands[2].Give.N() != 0 {
			t.Fatalf("item count changed: %+v", commands)
		}
		fsys["game.yaml"].Data = []byte(strings.Replace(string(fsys["game.yaml"].Data), "11", "10", 1))
		if _, err := Load(fsys); err == nil || !strings.Contains(err.Error(), "schema_version: 11") {
			t.Fatalf("old schema: %v", err)
		}
	}
}

func TestCompactEventDiagnosticPosition(t *testing.T) {
	fsys := fstest.MapFS{
		"game.yaml":       {Data: []byte("schema_version: 11\nid: story\nstart: {location: station}\n")},
		"data/story.yaml": {Data: []byte("locations: {station: {name: Station}}\nevents:\n  broken:\n    - give: missing\n")},
	}
	_, err := Load(fsys)
	if err == nil || !strings.Contains(err.Error(), "data/story.yaml:4:7:") || !strings.Contains(err.Error(), `unknown item "missing"`) {
		t.Fatalf("diagnostic: %v", err)
	}
}

func TestTacticYAMLDefaults(t *testing.T) {
	for _, tc := range []struct {
		body    string
		enabled bool
	}{
		{"{condition: any, action: attack}", false},
		{"{enabled: true, condition: any, action: attack}", true},
		{"{enabled: false, condition: any, action: attack}", false},
		{"{action: attack}", false},
	} {
		for _, version := range []int{10, 11} {
			var row TacticRow
			if err := yaml.Unmarshal([]byte(tc.body), &row); err != nil {
				t.Fatal(err)
			}
			// Exercise the version-aware loader walk with the authored raw presence.
			var raw yaml.MapSlice
			if err := yaml.UnmarshalWithOptions([]byte(tc.body), &raw, yaml.UseOrderedMap()); err != nil {
				t.Fatal(err)
			}
			db := &DB{Game: Game{SchemaVersion: version}}
			if err := db.resolveNumbers(raw, &row, "row"); err != nil {
				t.Fatal(err)
			}
			want := tc.enabled || version == 11 && !strings.Contains(tc.body, "enabled")
			if row.Enabled != want {
				t.Fatalf("version %d %s: got %v", version, tc.body, row.Enabled)
			}
			if row.Condition != "any" {
				t.Fatalf("version %d %s: condition %q, want any", version, tc.body, row.Condition)
			}
		}
	}
}

// TestLoadFillsConventions: an enemy without image takes assets/enemies/<id>.png when the
// file is there, a monster without monster_class takes monster_<family> when the game has
// it, and a stat left out of an enemy or a class is 0, while mhp stays required.
func TestLoadFillsConventions(t *testing.T) {
	game := "schema_version: 11\nid: t\nstart: {location: a, party: [hero]}\n"
	data := `skills:
  attack: {name: Attack, effects: [{damage: 1}]}
  guard: {name: Guard, scope: self}
classes:
  hero: {name: Hero, stats: {mhp: 10, atk: 2}, exp_for: level * 10}
  monster_slime: {name: Slime, stats: {mhp: 5}, exp_for: level * 10}
actors: {hero: {name: Hero, class: hero}}
locations: {a: {name: A}}
monster_families: {slime: Slime}
enemies:
  slime: {name: Slime, family: slime, rank: 1, stats: {mhp: 5, atk: 1}}
  bat: {name: Bat, image: other/bat.png, monster_class: hero, family: slime, rank: 1, stats: {mhp: 3}}
`
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	pic := &fstest.MapFile{Data: b.Bytes()}
	fsys := fstest.MapFS{
		"game.yaml":                {Data: []byte(game)},
		"data/w.yaml":              {Data: []byte(data)},
		"assets/enemies/slime.png": pic,
		"assets/other/bat.png":     pic,
	}
	db, err := Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	slime, bat := db.Enemies["slime"], db.Enemies["bat"]
	if slime.Image.File != "enemies/slime.png" || slime.MonsterClass != "monster_slime" {
		t.Fatalf("slime: image %q, class %q", slime.Image.File, slime.MonsterClass)
	}
	if bat.Image.File != "other/bat.png" || bat.MonsterClass != "hero" {
		t.Fatalf("bat kept its own: image %q, class %q", bat.Image.File, bat.MonsterClass)
	}
	if v, ok := slime.Stats["luk"]; !ok || v != 0 || slime.Stats["atk"] != 1 {
		t.Fatalf("slime stats %v", slime.Stats)
	}
	if e, ok := db.Classes["hero"].Stats["mdf"]; !ok || !e.Set() {
		t.Fatalf("hero stats %v", db.Classes["hero"].Stats)
	}
	fsys["data/w.yaml"] = &fstest.MapFile{Data: []byte(strings.Replace(data, "stats: {mhp: 10, atk: 2}", "stats: {atk: 2}", 1))}
	if _, err := Load(fsys); err == nil || !strings.Contains(err.Error(), "stats.mhp is missing") {
		t.Fatalf("a class without mhp: %v", err)
	}
	delete(fsys, "assets/enemies/slime.png")
	fsys["data/w.yaml"] = &fstest.MapFile{Data: []byte(data)}
	if _, err := Load(fsys); err == nil || !strings.Contains(err.Error(), "image is missing") {
		t.Fatalf("an enemy without its picture: %v", err)
	}
}

// TestAssetNamesStayUnderAssets: a name that climbs out of assets/ names no asset,
// even when the file it reaches exists.
func TestAssetNamesStayUnderAssets(t *testing.T) {
	db := &DB{FS: fstest.MapFS{"game.yaml": {Data: []byte("title: T")}, "assets/x.png": {Data: []byte("x")}}}
	c := &checker{db: db}
	c.file("game.yaml fonts.regular", "../game.yaml")
	if len(c.errs) == 0 {
		t.Error(`fonts.regular: ../game.yaml passes as an asset (assets/../game.yaml)`)
	}
}

// overlayFS reads the files of over in place of base's, and everything else from base.
type overlayFS struct {
	base fs.FS
	over fstest.MapFS
}

func (o overlayFS) Open(name string) (fs.File, error) {
	if _, ok := o.over[name]; ok {
		return o.over.Open(name)
	}
	return o.base.Open(name)
}

func (o overlayFS) ReadDir(name string) ([]fs.DirEntry, error) { return fs.ReadDir(o.base, name) }

// reverseMappings writes every mapping of a YAML document with its keys in reverse order,
// but a mapping holding an anchor or an alias keeps its order: an alias must follow its
// anchor, and a merge key (<<) written after a key would replace it.
func reverseMappings(t *testing.T, b []byte) []byte {
	t.Helper()
	f, err := parser.ParseBytes(b, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	for _, doc := range f.Docs {
		ast.Walk(reverser{}, doc.Body)
	}
	return []byte(f.String())
}

type reverser struct{}

func (r reverser) Visit(n ast.Node) ast.Visitor {
	if m, ok := n.(*ast.MappingNode); ok && len(ast.Filter(ast.AnchorType, m)) == 0 && len(ast.Filter(ast.AliasType, m)) == 0 {
		slices.Reverse(m.Values)
	}
	return r
}

// loaded is what a load of a game gives, without where each line was written: the order
// of each section's ids follows the file, so it is kept as a set.
func loaded(t *testing.T, fsys fs.FS) (*DB, []string, error) {
	t.Helper()
	db, err := Load(fsys)
	if err != nil {
		return nil, nil, err
	}
	strip := regexp.MustCompile(`:\d+(:\d+)?`)
	var warns []string
	for _, w := range slices.Concat(db.SceneWarnings(), db.FlagWarnings(), db.DefinitionWarnings()) {
		warns = append(warns, strip.ReplaceAllString(w.Error(), ""))
	}
	slices.Sort(warns)
	for k, ids := range db.Order {
		db.Order[k] = slices.Sorted(slices.Values(ids))
	}
	db.FS, db.sources, db.sourceFiles = nil, nil, nil
	db.sceneWarns, db.flags, db.definitionWarns = nil, flagFindings{}, nil
	return db, warns, nil
}

// differ names the first place where a and b are not deeply equal, or "".
func differ(path string, a, b reflect.Value) string {
	if a.Kind() != b.Kind() || a.Type() != b.Type() {
		return path + ": type"
	}
	switch a.Kind() { //nolint:exhaustive // Scalars and the rest compare after the switch.
	case reflect.Pointer, reflect.Interface:
		if a.IsNil() || b.IsNil() {
			if a.IsNil() != b.IsNil() {
				return path + ": nil"
			}
			return ""
		}
		return differ(path, a.Elem(), b.Elem())
	case reflect.Struct:
		for i := range a.NumField() {
			if d := differ(path+"."+a.Type().Field(i).Name, a.Field(i), b.Field(i)); d != "" {
				return d
			}
		}
		return ""
	case reflect.Slice, reflect.Array:
		if a.Len() != b.Len() {
			return fmt.Sprintf("%s: len %d, %d", path, a.Len(), b.Len())
		}
		// what follows the order of the file (menus, a map's points) may come in another
		// order, but each item of one is an item of the other
		used := make([]bool, b.Len())
	items:
		for i := range a.Len() {
			for j := range b.Len() {
				if !used[j] && differ("", a.Index(i), b.Index(j)) == "" {
					used[j] = true
					continue items
				}
			}
			return differ(fmt.Sprintf("%s[%d]", path, i), a.Index(i), b.Index(i))
		}
		return ""
	case reflect.Map:
		if a.Len() != b.Len() {
			return fmt.Sprintf("%s: len %d, %d", path, a.Len(), b.Len())
		}
		if a.Type().Key().Kind() == reflect.Pointer { // keyed by where a value lives: compare the values
			values := func(m reflect.Value) []string {
				var out []string
				for _, k := range m.MapKeys() {
					out = append(out, fmt.Sprint(m.MapIndex(k)))
				}
				return slices.Sorted(slices.Values(out))
			}
			if !slices.Equal(values(a), values(b)) {
				return path + ": values"
			}
			return ""
		}
		for _, k := range a.MapKeys() {
			if !b.MapIndex(k).IsValid() {
				return fmt.Sprintf("%s[%v]: missing", path, k)
			}
			if d := differ(fmt.Sprintf("%s[%v]", path, k), a.MapIndex(k), b.MapIndex(k)); d != "" {
				return d
			}
		}
		return ""
	case reflect.Func:
		return ""
	}
	if !a.CanInterface() {
		if fmt.Sprint(a) != fmt.Sprint(b) {
			return fmt.Sprintf("%s: %v, %v", path, a, b)
		}
		return ""
	}
	if !reflect.DeepEqual(a.Interface(), b.Interface()) {
		return fmt.Sprintf("%s: %v, %v", path, a.Interface(), b.Interface())
	}
	return ""
}

// TestLoadIgnoresKeyOrder (metamorphic): an example game with the keys of every mapping in
// every YAML file written in reverse loads to the same game with the same warnings.
func TestLoadIgnoresKeyOrder(t *testing.T) {
	t.Parallel()
	games, err := filepath.Glob("../../../examples/*/game")
	if err != nil || len(games) == 0 {
		t.Fatal(games, err)
	}
	for _, dir := range games {
		t.Run(filepath.Base(filepath.Dir(dir)), func(t *testing.T) {
			t.Parallel()
			base := os.DirFS(dir)
			want, wantWarns, err := loaded(t, base)
			if err != nil {
				t.Fatal(err)
			}
			over := fstest.MapFS{}
			err = fs.WalkDir(base, ".", func(name string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() || filepath.Ext(name) != ".yaml" {
					return err
				}
				b, err := fs.ReadFile(base, name)
				over[name] = &fstest.MapFile{Data: reverseMappings(t, b)}
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			got, gotWarns, err := loaded(t, overlayFS{base, over})
			if err != nil {
				t.Fatalf("reversed keys do not load: %v", err)
			}
			if d := differ("db", reflect.ValueOf(want).Elem(), reflect.ValueOf(got).Elem()); d != "" {
				t.Errorf("reversed keys load another game at %s", d)
			}
			if !slices.Equal(gotWarns, wantWarns) {
				t.Errorf("reversed keys warn %q, want %q", gotWarns, wantWarns)
			}
		})
	}
}
