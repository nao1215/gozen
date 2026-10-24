package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Whatever a trace line holds, reading it ends with an exit status and never a panic, and a
// line that was refused leaves a message.
func FuzzPlayTraceLine(f *testing.F) {
	for _, seed := range []string{
		`{"t":"say","tick":60,"text":"Hello","who":"A","choices":["x"],"read":false}`,
		`{"t":"state","tick":1,"party":[{"name":"A","lv":1,"hp":1,"mhp":1,"mp":0,"mmp":0,"back":true}],"gold":3,"location":"c","floor":2,"pos":[1,2],"facing":"n"}`,
		`{"t":"stand","tick":5,"since":5,"set":1,"speaker":"cast:a","standing":true,"figures":[{"id":"cast:a","x":0.5,"alpha":1,"speaking":true}]}`,
		`{"t":"music","tick":0,"wanted":"a","song":"","asked_at":0,"wait":99999}`,
		`{"t":"view","tick":2,"pictures":["a","b"],"missing":["b"],"shot":"s"}`,
		`{"t":"skip","tick":3,"on":true,"scene":true,"event":"e","line":1,"read":1,"lines":2}`,
		`{"t":"rollback","tick":4,"from":1e308,"to":-1e308}`,
		`{"t":"battle_capture","tick":4,"capture":{"troop":"t"}}`,
		`{"t":"x","tick":9223372036854775807}`,
		`{"t":"end","tick":1e300}`,
		`[`, `null`, `{"t":null}`, `{"t":1}`, ``, `{"tick":1}`,
	} {
		f.Add(seed)
	}
	dir := f.TempDir()
	f.Fuzz(func(t *testing.T, line string) {
		if strings.Contains(line, "\n") {
			line = strings.ReplaceAll(line, "\n", " ")
		}
		p := filepath.Join(dir, "trace.jsonl")
		if err := os.WriteFile(p, []byte(`{"t":"start","tick":0,"game":"g","seed":1,"language":"en"}`+"\n"+line+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{"trace", p}, {"trace", p, "--check"}, {"trace", p, "--input", "--from", "0.5"}} {
			var out, errOut bytes.Buffer
			got := run(args, &out, &errOut)
			if got != 0 && got != 1 {
				t.Fatalf("%q: exit %d", args, got)
			}
			if got == 1 && errOut.Len() == 0 {
				t.Fatalf("%q: failed silently", args)
			}
		}
	})
}

// A shot list that is any text is either read or refused with a message.
func FuzzShotList(f *testing.F) {
	for _, seed := range []string{
		"shots: [{id: a, start: camp}]\n",
		"title: T\nwatch: ['x/']\nseed: 3\nshots:\n  - {id: a, scene: s, picture: x.png, line: 2, answers: [1]}\n",
		"shots: [{id: a, scene: s, event: e}]\n",
		"shots: []\n", "", "[", "shots: 3\n", "shots: [{id: ../a, start: c}]\n", "shots: [{id: a, start: c, wait: -1}]\n",
	} {
		f.Add(seed)
	}
	dir := f.TempDir()
	f.Fuzz(func(t *testing.T, list string) {
		p := filepath.Join(dir, "shots.yaml")
		if err := os.WriteFile(p, []byte(list), 0o600); err != nil {
			t.Fatal(err)
		}
		l, err := readShotList(p)
		if err == nil {
			if l == nil || len(l.Shots) == 0 || l.Seed == 0 {
				t.Fatalf("an accepted list has shots and a seed: %+v", l)
			}
			ids := map[string]bool{}
			for _, it := range l.Shots {
				if it.ID == "" || ids[it.ID] || strings.ContainsAny(it.ID, "/\\") {
					t.Fatalf("an accepted list has unique, plain ids: %+v", l.Shots)
				}
				ids[it.ID] = true
			}
		}
	})
}
