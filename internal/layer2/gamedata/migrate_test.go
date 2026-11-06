package gamedata

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

type migrationFS struct {
	fs.FS
	changed map[string][]byte
}

func (m migrationFS) Open(name string) (fs.File, error) {
	if b, ok := m.changed[name]; ok {
		return (fstest.MapFS{name: &fstest.MapFile{Data: b}}).Open(name)
	}
	return m.FS.Open(name)
}

func TestMigrateSamples(t *testing.T) {
	err := filepath.WalkDir("../../../examples", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "game.yaml" {
			return nil
		}
		t.Run(filepath.ToSlash(filepath.Dir(name)), func(t *testing.T) {
			source := os.DirFS(filepath.Dir(name))
			before, err := Load(source)
			if err != nil {
				t.Fatal(err)
			}
			changed, err := Migrate(source)
			if err != nil {
				t.Fatal(err)
			}
			migrated := migrationFS{source, changed}
			after, err := Load(migrated)
			if err != nil {
				t.Fatalf("migrated load: %v", err)
			}
			if before.Game.ID != after.Game.ID || before.Game.DataVersion != after.Game.DataVersion {
				t.Fatal("save identity changed")
			}
			before.eachNamed(func(name string, cmds []Command) bool {
				if cmds == nil {
					return false
				}
				a, err := before.CommandsFingerprint(name)
				if err != nil {
					t.Fatal(err)
				}
				b, err := after.CommandsFingerprint(name)
				if err != nil || a != b {
					t.Fatalf("replay fingerprint %s changed: %v", name, err)
				}
				return false
			})
			before.Game.SchemaVersion = SchemaVersion11
			a, b := migrationBehavior(t, before), migrationBehavior(t, after)
			if !bytes.Equal(a, b) {
				t.Fatalf("loaded behavior changed (before %d bytes, after %d)", len(a), len(b))
			}
			second, err := Migrate(migrated)
			if err != nil || len(second) != 0 {
				t.Fatalf("migration not idempotent: %v, changed %v", err, reflect.ValueOf(second).MapKeys())
			}
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func migrationBehavior(t *testing.T, db *DB) []byte {
	t.Helper()
	db.FS = nil
	b, err := json.Marshal(db)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestMigrateSupportedVersions(t *testing.T) {
	for version := 1; version <= CurrentSchemaVersion; version++ {
		t.Run(strconv.Itoa(version), func(t *testing.T) {
			source := fstest.MapFS{
				"game.yaml":       {Data: []byte("schema_version: " + strconv.Itoa(version) + "\nid: versioned-story\nstart: {location: room}\n")},
				"data/world.yaml": {Data: []byte("locations: {room: {name: Room}}\nitems: {ticket: {name: Ticket}}\nevents: {opening: {do: [{give: {item: ticket}}, {say: Hello}]}}\n")},
			}
			changes, err := Migrate(source)
			if err != nil {
				t.Fatal(err)
			}
			after, err := Load(migrationFS{source, changes})
			if err != nil {
				t.Fatal(err)
			}
			if after.Game.SchemaVersion != CurrentSchemaVersion || after.Events["opening"].Do[0].Give.N() != 1 {
				t.Fatal("version or item semantics changed")
			}
		})
	}
}

func TestMigrateUnsupportedHeaders(t *testing.T) {
	for _, header := range []string{"schema_version: 0", "schema_version: &version 10"} {
		source := fstest.MapFS{
			"game.yaml":       {Data: []byte(header + "\nid: legacy-story\nstart: {location: room}\n")},
			"data/world.yaml": {Data: []byte("locations: {room: {name: Room}}\n")},
		}
		original := bytes.Clone(source["game.yaml"].Data)
		changes, err := Migrate(source)
		if err == nil || !strings.Contains(err.Error(), "schema_version") || changes != nil {
			t.Fatalf("%s: expected actionable refusal, got %v, %v", header, changes, err)
		}
		if !bytes.Equal(source["game.yaml"].Data, original) {
			t.Fatal("source changed after refusal")
		}
	}
}

func TestMigrateCommentsAnchorsAndExplicitCounts(t *testing.T) {
	source := fstest.MapFS{
		"game.yaml":       {Data: []byte("schema_version: 10\nid: unchanged\ndata_version: 7\nstart: {location: room}\n")},
		"data/world.yaml": {Data: []byte("locations: {room: {name: Room}}\nitems: {key: {name: Key}}\nevents:\n  opening: # Keep the note.\n    do:\n      - give: {item: key}\n      - give: {item: key, count: 1}\n      - take: {item: key, count: 0}\n      - say: |\n          Keep this line.\n          And this one.\n  anchored: &action\n    do: [{say: Hello}]\n  alias: *action\n")},
	}
	changes, err := Migrate(source)
	if err != nil {
		t.Fatal(err)
	}
	after, err := Load(migrationFS{source, changes})
	if err != nil {
		t.Fatal(err)
	}
	before, err := Load(source)
	if err != nil {
		t.Fatal(err)
	}
	for name := range before.Events {
		a, err := before.CommandsFingerprint(name)
		if err != nil {
			t.Fatal(err)
		}
		b, err := after.CommandsFingerprint(name)
		if err != nil || a != b {
			t.Fatalf("%s replay changed: %v", name, err)
		}
	}
	if !bytes.Contains(changes["data/world.yaml"], []byte("# Keep the note.")) {
		t.Fatal("lost comment")
	}
}

func TestMigrateTacticAnchors(t *testing.T) {
	for _, source := range []string{
		"rows:\n  - &base {enabled: true, condition: any, action: attack}\n  - {<<: *base}\n",
		"rows:\n  - &base {condition: any, action: attack}\n  - {<<: *base}\n",
		"rows: [{condition: any, enabled: true, action: attack}, {condition: any, action: attack, enabled: true}]\n",
		"rows: [{condition: any, action: attack,}, {enabled: false, condition: any, action: attack}]\n",
		"rows:\n  - enabled: true\n    condition: any\n    action: attack\n  - condition: any\n    action: attack\n",
		"rows:\n  - condition: any\n    enabled: true # Keep the author's note.\n    action: attack\n",
		"{rows: [{condition: any, action: attack}]}\r# Keep the note.",
		"{rows: [{enabled: true, condition: any, action: attack}]}\r# Keep the note.",
	} {
		type rows struct {
			Rows []TacticRow `yaml:"rows"`
		}
		var before rows
		if err := yaml.Unmarshal([]byte(source), &before); err != nil {
			t.Fatal(err)
		}
		rewritten, err := migrateSource([]byte(source), reflect.TypeFor[rows](), 10)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := parser.ParseBytes(rewritten, 0)
		if err != nil {
			t.Fatal(err)
		}
		resolveMerges(parsed.Docs[0].Body, map[string]ast.Node{})
		var after rows
		if err := yaml.NodeToValue(parsed.Docs[0].Body, &after); err != nil {
			t.Fatal(err)
		}
		var raw yaml.MapSlice
		if err := yaml.NodeToValue(parsed.Docs[0].Body, &raw, yaml.UseOrderedMap()); err != nil {
			t.Fatal(err)
		}
		db := &DB{Game: Game{SchemaVersion: 11}}
		if err := db.resolveNumbers(raw, &after, "rows"); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("rows changed: %+v -> %+v\n%s", before, after, rewritten)
		}
	}
}

func TestMigrateSourceText(t *testing.T) {
	for _, newline := range []string{"\n", "\r\n", "\r"} {
		for _, ending := range []string{"", newline} {
			original := strings.ReplaceAll("locations: {room: {name: Room}}\nitems: {ticket: {name: Ticket}}\nevents:\n  café: {do: [{say: 'Welcome to the café.'}, {give: {item: \"ticket\"}}]}\n  opening:\n    do:\n      # Keep this comment.\n      - say: Hello", "\n", newline) + ending
			source := fstest.MapFS{
				"game.yaml":       {Data: []byte("{schema_version: 10, id: text, start: {location: room}}" + newline + "# Preserve this header comment." + ending)},
				"data/world.yaml": {Data: []byte(original)},
			}
			changes, err := Migrate(source)
			if err != nil {
				t.Fatal(err)
			}
			text := string(changes["data/world.yaml"])
			if !bytes.Contains(changes["game.yaml"], []byte("# Preserve this header comment.")) {
				t.Fatal("lost header comment")
			}
			for _, preserved := range []string{"café:", "'Welcome to the café.'", "\"ticket\"", "# Keep this comment."} {
				if !strings.Contains(text, preserved) {
					t.Fatalf("lost %q in %q", preserved, text)
				}
			}
			if newline == "\r\n" && strings.Contains(strings.ReplaceAll(text, newline, ""), "\n") {
				t.Fatal("changed line endings")
			}
		}
	}
}

func FuzzMigratePreservesDefinitions(f *testing.F) {
	fixture := func(source string) (fstest.MapFS, string) {
		fsys := fstest.MapFS{
			"game.yaml":        {Data: []byte("schema_version: 10\nid: fuzz-story\nstart: {location: room}\n")},
			"assets/scene.png": {Data: []byte("neutral placeholder")},
			"data/base.yaml":   {Data: []byte("locations: {room: {name: Room}}\nitems: {ticket: {name: Ticket}}\n")},
			"data/story.yaml":  {Data: []byte(source)},
		}
		name := "data/story.yaml"
		if strings.HasPrefix(source, "battle:") {
			name = "game.yaml"
			fsys[name].Data = append(fsys[name].Data, source...)
			fsys["data/story.yaml"].Data = []byte("skills:\n  attack: {name: Strike, scope: one_enemy, effects: [{damage: 1}]}\n  guard: {name: Guard, scope: self, effects: []}\nai: {scout: [{use: guard}]}\ntactic_conditions: {any: {name: Always, kind: always}}\ntactic_targets: {self: {name: Self, scope: self}}\ntactic_actions: {guard: {name: Guard, use: guard, target: self}}\n")
		}
		return fsys, name
	}
	for _, source := range []string{
		"", "# Empty authoring file.\n",
		"{}\r#0",
		"events:\t{0:{do:[]}}",
		"events:",
		"events:\n 0:\n  do:\n  #000000\x98",
		"events:\n 0\n: do:\n  - say: A",
		"events:\n 0:\r  do:\n  - say: A",
		"events:\n 0:\n  do:\r  - say: A",
		"events: {0:{do:[{say:!0000 0}]}}",
		"events:\n 0:\n  do:\n  - say: # Note.\n    08\n",
		"events:\n  0: # Note.\n   do:\n   - say: Hello\n",
		"events:\n e:\n  do:\n  - say: Hi\n",
		"events:\n    e:\n        do:\n            - say: Hi\n",
		"events: {opening: {do: [{say: Hello}, {give: {item: ticket}}]}}\n",
		"events:\n  opening: &story\n    do:\n      - say: |\n          Two lines.\n          Still two.\n  again: *story\n",
		"events: {opening: {do: [{give: {item: ticket, count: 0}}, {take: {item: ticket, count: 1}}]}}\n",
		"events: {opening: {when: true, do: [{choice: [{label: Go, do: [{say: Hello}]}]}]}}\n",
		"events: {opening: {do: []}}\n",
		"events: {opening: {do: [{phone: {app: chat, messages: [{text: Hello}]}}]}}\n",
		"events: {opening: {do: [{phone: {app: feed, posts: [{picture: scene.png, text: Morning}]}}]}}\n",
		"events: {opening: {do: [{phone: {app: video, hold: landscape, video: {picture: scene.png, seconds: 2, bounce: [0, 1, 2]}}}]}}\n",
		"battle: {auto_tactics: {scout: [{condition: any, action: guard}]}}\n",
		"battle: {auto_tactics: {scout: [{enabled: true, condition: any, action: guard}]}}\n",
		"battle:\n  auto_tactics:\n    scout:\n      - &row {enabled: false, condition: any, action: guard}\n      - {<<: *row}\n",
	} {
		fsys, _ := fixture(source)
		if _, err := Load(fsys); err != nil {
			f.Fatalf("invalid migration seed %q: %v", source, err)
		}
		f.Add(source)
	}
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 16384 {
			return
		}
		fsys, name := fixture(source)
		original := bytes.Clone(fsys[name].Data)
		before, err := Load(fsys)
		if err != nil {
			return
		}
		changed, err := Migrate(fsys)
		if errors.As(err, new(errUnrestorable)) {
			return // refused safely: nothing written
		}
		if err != nil {
			t.Fatal(err)
		}
		migrated := migrationFS{fsys, changed}
		after, err := Load(migrated)
		if err != nil {
			t.Fatalf("%v\nrewritten:\n%s", err, changed["data/story.yaml"])
		}
		before.Game.SchemaVersion = 11
		if !bytes.Equal(migrationBehavior(t, before), migrationBehavior(t, after)) {
			t.Fatal("loaded definitions changed")
		}
		second, err := Migrate(migrated)
		if err != nil || len(second) != 0 {
			t.Fatalf("not idempotent: %v", err)
		}
		if !bytes.Equal(fsys[name].Data, original) {
			t.Fatal("source mutated")
		}
	})
}

func TestMigratePhoneEvents(t *testing.T) {
	for _, event := range []string{
		`{do: [{phone: {app: chat, messages: [{text: Welcome}, {video: {picture: scene.png, seconds: 2, bounce: [0, 1, 2]}}]}}]}`,
		`{do: [{phone: {app: feed, posts: [{picture: scene.png, text: 'A quiet morning'}]}}]}`,
		`{do: [{phone: {app: video, hold: landscape, video: {picture: scene.png, seconds: 3}}}]}`,
	} {
		t.Run(event, func(t *testing.T) {
			source := fstest.MapFS{
				"game.yaml":        {Data: []byte("schema_version: 10\nid: phone-migration\nstart: {location: room}\nui: {message: novel}\n")},
				"data/world.yaml":  {Data: []byte("locations: {room: {name: Room}}\nevents:\n  phone_demo: " + event + "\n")},
				"assets/scene.png": {Data: []byte("neutral placeholder")},
			}
			before, err := Load(source)
			if err != nil {
				t.Fatal(err)
			}
			original := bytes.Clone(source["data/world.yaml"].Data)
			changes, err := Migrate(source)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(original, source["data/world.yaml"].Data) || bytes.Contains(changes["data/world.yaml"], []byte("do:")) {
				t.Fatal("migration changed its source or failed to compact the phone event")
			}
			migrated := migrationFS{source, changes}
			after, err := Load(migrated)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before.Events["phone_demo"].Do, after.Events["phone_demo"].Do) {
				t.Fatal("migration changed the phone command")
			}
			a, err := before.CommandsFingerprint("phone_demo")
			if err != nil {
				t.Fatal(err)
			}
			b, err := after.CommandsFingerprint("phone_demo")
			if err != nil || a != b {
				t.Fatalf("phone replay fingerprint changed: %v", err)
			}
			if again, err := Migrate(migrated); err != nil || len(again) != 0 {
				t.Fatalf("phone migration is not idempotent: %v, %v", again, err)
			}
		})
	}
}

func TestMigrateRefusesUnrestorableMergeToken(t *testing.T) {
	// The decoder accepts this prefix as a merge key, but its token drops the 0.
	source := []byte("rows:\n  - &base {condition: any, action: attack}\n  - 0<<: *base\n")
	original := bytes.Clone(source)
	type rows struct {
		Rows []TacticRow `yaml:"rows"`
	}
	changed, err := migrateSource(source, reflect.TypeFor[rows](), 10)
	if !errors.As(err, new(errUnrestorable)) || changed != nil {
		t.Fatalf("ambiguous token must refuse edits: changed=%q error=%v", changed, err)
	}
	if !bytes.Equal(source, original) {
		t.Fatal("refusal mutated the source")
	}
}
