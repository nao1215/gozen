package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// Every example goes through the real command adapters. The same seed and
// script must produce identical gameplay traces after syntax/default migration.
// The start record's source-byte fingerprint necessarily changes with YAML.
func TestMigrateSamplesCheckAndSimulation(t *testing.T) {
	script := filepath.Join(t.TempDir(), "play.txt")
	write(t, script, "until idle\nsnapshot\nquit\n")
	err := filepath.WalkDir("../../examples", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "game.yaml" {
			return nil
		}
		t.Run(filepath.ToSlash(filepath.Dir(name)), func(t *testing.T) {
			source := legacySample(t, filepath.Dir(name))
			dest := filepath.Join(t.TempDir(), "migrated")
			db, err := data.Load(os.DirFS(source))
			if err != nil {
				t.Fatal(err)
			}
			original := string(read(t, filepath.Join(source, "game.yaml")))
			invoke(t, 0, "migrate", source, "--out", dest)
			if string(read(t, filepath.Join(source, "game.yaml"))) != original {
				t.Fatal("source changed")
			}
			before, _ := invoke(t, 0, "check", source)
			after, _ := invoke(t, 0, "check", dest)
			if before != after {
				t.Fatalf("check result changed:\n%s\n%s", before, after)
			}
			for _, seed := range []string{"1", "17"} {
				before, _ = invoke(t, 0, "sim", "script", source, "--start", db.Game.Start.Location, "--event", db.Game.Start.Event, "--seed", seed, "--script", script, "--trace", "-")
				after, _ = invoke(t, 0, "sim", "script", dest, "--start", db.Game.Start.Location, "--event", db.Game.Start.Event, "--seed", seed, "--script", script, "--trace", "-")
				if !reflect.DeepEqual(migrationTrace(t, before), migrationTrace(t, after)) {
					t.Fatalf("seed %s: trace changed\nbefore: %s\nafter: %s", seed, before, after)
				}
				if !strings.Contains(before, `"t":"snapshot"`) {
					t.Fatal("simulation did not reach snapshot")
				}
			}
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMigrateRefusesUnsafeDestinations(t *testing.T) {
	source := filepath.Join(t.TempDir(), "story")
	invoke(t, 0, "new", source, "--template", "novel")
	original := string(read(t, filepath.Join(source, "game.yaml")))
	for _, destination := range []string{source, filepath.Join(source, "nested"), filepath.Dir(source)} {
		invoke(t, 1, "migrate", source, "--out", destination)
		if string(read(t, filepath.Join(source, "game.yaml"))) != original {
			t.Fatal("source changed")
		}
	}
	dest := filepath.Join(t.TempDir(), "result")
	write(t, filepath.Join(source, "data", "broken.yaml"), "events: [\n")
	invoke(t, 1, "migrate", source, "--out", dest)
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("published invalid output: %v", err)
	}
	invoke(t, 2, "migrate", source)
	invoke(t, 2, "migrate", "--out", dest)
}

func migrationTrace(t *testing.T, trace string) []map[string]any {
	t.Helper()
	var result []map[string]any
	snapshot := false
	for line := range strings.Lines(trace) {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if record["t"] == "start" {
			delete(record, "fingerprint")
		}
		// Background decoder readiness is asynchronous even in headless mode.
		// Compare gameplay, not the tick at which a music file finishes loading.
		if record["t"] == "music" {
			continue
		}
		if record["t"] == "end" {
			delete(record, "lines")
		}
		if record["t"] == "snapshot" && record["state"] != nil {
			snapshot = true
		}
		result = append(result, record)
	}
	if !snapshot {
		t.Fatal("no gameplay snapshot")
	}
	return result
}

// Reconstruct the equivalent schema-10 spelling so this regression continues to
// exercise upgrades after the checked-in samples themselves move to schema 11.
// This uses ordered values, independently of the production AST transformer.
func legacySample(t *testing.T, source string) string {
	t.Helper()
	db, err := data.Load(os.DirFS(source))
	if err != nil {
		t.Fatal(err)
	}
	if db.Game.SchemaVersion < 11 {
		return source
	}
	dest := filepath.Join(t.TempDir(), "legacy")
	if err := os.CopyFS(dest, os.DirFS(source)); err != nil {
		t.Fatal(err)
	}
	output, err := os.OpenRoot(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := output.Close(); err != nil {
			t.Error(err)
		}
	}()
	var expand func(any) any
	expand = func(value any) any {
		switch v := value.(type) {
		case yaml.MapSlice:
			for i, entry := range v {
				switch entry.Key {
				case "events":
					if events, ok := entry.Value.(yaml.MapSlice); ok {
						for j, event := range events {
							if list, ok := event.Value.([]any); ok {
								events[j].Value = yaml.MapSlice{{Key: "do", Value: list}}
							}
						}
					}
				case "give", "take":
					if id, ok := entry.Value.(string); ok {
						v[i].Value = yaml.MapSlice{{Key: "item", Value: id}}
					}
				}
				v[i].Value = expand(v[i].Value)
			}
			return v
		case []any:
			for i := range v {
				v[i] = expand(v[i])
			}
			return v
		}
		return value
	}
	err = filepath.WalkDir(dest, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(dest, name)
		if err != nil {
			return err
		}
		if entry.IsDir() || relative != "game.yaml" && (!strings.HasPrefix(filepath.ToSlash(relative), "data/") || filepath.Ext(name) != ".yaml" && filepath.Ext(name) != ".yml") {
			return nil
		}
		var root yaml.MapSlice
		if err := yaml.UnmarshalWithOptions(read(t, name), &root, yaml.UseOrderedMap()); err != nil {
			return err
		}
		converted, ok := expand(root).(yaml.MapSlice)
		if !ok {
			t.Fatal("legacy root is not a mapping")
		}
		root = converted
		if relative == "game.yaml" {
			for i, entry := range root {
				if entry.Key == "schema_version" {
					root[i].Value = 10
				}
				if entry.Key == "battle" {
					legacyTactics(entry.Value)
				}
			}
		}
		b, err := yaml.Marshal(root)
		if err != nil {
			return err
		}
		return output.WriteFile(relative, b, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dest
}

func legacyTactics(value any) {
	// Only battle definitions contain TacticRow lists in the current samples.
	switch v := value.(type) {
	case yaml.MapSlice:
		for _, entry := range v {
			legacyTactics(entry.Value)
		}
	case []any:
		for i, entry := range v {
			if fields, ok := entry.(yaml.MapSlice); ok {
				condition, action, enabled := false, false, false
				for _, f := range fields {
					switch f.Key {
					case "condition":
						condition = true
					case "action":
						action = true
					case "enabled":
						enabled = true
					}
				}
				if condition && action && !enabled {
					v[i] = append(fields, yaml.MapItem{Key: "enabled", Value: true})
				}
			}
			legacyTactics(v[i])
		}
	}
}
