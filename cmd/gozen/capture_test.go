package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCaptureListIsValidated(t *testing.T) {
	for _, tc := range []struct{ name, list, msg string }{
		{"empty", "shots: []\n", "empty list"},
		{"unknown key", "shots: [{id: a, start: camp, surprise: 1}]\n", "surprise"},
		{"no id", "shots: [{start: camp}]\n", "the id is missing or repeated"},
		{"bad id", "shots: [{id: ../a, start: camp}]\n", "the id is missing or repeated"},
		{"repeated id", "shots: [{id: a, start: camp}, {id: a, start: camp}]\n", "the id is missing or repeated"},
		{"negative wait", "shots: [{id: a, start: camp, wait: -1}]\n", "negative shot count"},
		{"negative line", "shots: [{id: a, start: camp, line: -1}]\n", "negative shot count"},
		{"negative answer", "shots: [{id: a, start: camp, answers: [-1]}]\n", "negative answer"},
		{"picture path", "shots: [{id: a, start: camp, picture: ../x.png}]\n", "invalid picture path"},
		{"scene and event", "shots: [{id: a, scene: s, event: e}]\n", "only one of scene and event"},
		{"nothing to start", "shots: [{id: a}]\n", "give scene, event or start"},
		{"watch prefix", "watch: ['../x']\nshots: [{id: a, start: camp}]\n", "invalid watch prefix"},
		{"not yaml", "shots: [", "shots.yaml"},
	} {
		dir := fixture(t)
		list := filepath.Join(t.TempDir(), "shots.yaml")
		out := filepath.Join(t.TempDir(), "result")
		write(t, list, tc.list)
		_, stderr := invoke(t, 1, "capture", dir, "--config", list, "--out", out)
		if !strings.Contains(stderr, tc.msg) {
			t.Errorf("%s: %q", tc.name, stderr)
		}
		if _, err := os.Lstat(out); err == nil {
			t.Errorf("%s: a rejected list created the output", tc.name)
		}
	}
}

func TestCaptureIndexEscapesMetadataAndPreservesResults(t *testing.T) {
	out := t.TempDir()
	list := &shotList{Title: `<Workshop & "Friends">`}
	results := []shotResult{
		{N: 1, ID: "scene", Status: "ok", Artifact: "001-scene.png", Frames: 3, Seed: 17, Size: "1280x720", Pictures: []string{"portraits/keeper.png"}, item: shotItem{ID: "scene", Title: "<Arrival>", Who: "Keeper & Weaver", Phase: "morning", Pose: "wide", Scene: "arrival", Picture: "portraits/keeper.png", Line: 2, Note: "<script>note</script>"}},
		{N: 2, ID: "event", Status: "approval_wait", Reason: "<pending>", item: shotItem{ID: "event", Event: "greeting", Phase: "morning", Pose: "wide"}},
		{N: 3, ID: "battle", Status: "failed", Reason: "<unavailable>", item: shotItem{ID: "battle", Start: "camp", Floor: 2, Battle: "guards"}},
	}
	if err := writeShotIndex(out, list, results, true); err != nil {
		t.Fatal(err)
	}
	var decoded []shotResult
	if err := json.Unmarshal(read(t, filepath.Join(out, "shots.json")), &decoded); err != nil {
		t.Fatal(err)
	}
	for i := range results {
		want := results[i]
		want.item = shotItem{}
		if !reflect.DeepEqual(decoded[i], want) {
			t.Fatalf("JSON result %d: %+v, want %+v", i, decoded[i], want)
		}
	}
	html := string(read(t, filepath.Join(out, "index.html")))
	for _, want := range []string{`&lt;Workshop &amp; &#34;Friends&#34;&gt;`, "&lt;Arrival&gt;", "Keeper &amp; Weaver", "&lt;script&gt;note&lt;/script&gt;", `href="001-scene.png"`, "&lt;pending&gt;", "&lt;unavailable&gt;", "1280x720", "B2", "guards", "greeting"} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML lacks %q", want)
		}
	}
	if strings.Contains(html, "<script>") || strings.Count(html, "<img ") != 1 || strings.Count(html, "<tr>") != 3 {
		t.Fatal("unsafe metadata or incorrect result grouping", html)
	}
	if len(shotRepeats([]shotItem{{ID: "one"}, {ID: "two"}})) != 0 || len(shotRepeats([]shotItem{results[0].item, results[1].item})) != 2 {
		t.Fatal("repeat warnings include empty fields or miss repeated metadata")
	}
	if err := writeShotIndex(out, &shotList{}, nil, false); err != nil {
		t.Fatal(err)
	}
	if html = string(read(t, filepath.Join(out, "index.html"))); strings.Contains(html, "<table>") || strings.Contains(html, "<p class=f>") {
		t.Fatal("empty approved report retained raw status or old results")
	}
}

func TestCaptureIndexReportsWriteFailures(t *testing.T) {
	for _, blocked := range []string{"shots.json", "index.html"} {
		t.Run(blocked, func(t *testing.T) {
			out := t.TempDir()
			if err := os.Mkdir(filepath.Join(out, blocked), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := writeShotIndex(out, &shotList{}, nil, false); err == nil {
				t.Fatal("ignored output write failure")
			}
		})
	}
}

func TestCaptureFailedJobsPublishOrderedReports(t *testing.T) {
	for _, format := range []string{"text", "json", "jsonl"} {
		t.Run(format, func(t *testing.T) {
			dir := fixture(t)
			config := filepath.Join(t.TempDir(), "shots.yaml")
			out := filepath.Join(t.TempDir(), "result")
			write(t, config, `title: Capture failures
seed: 23
shots:
  - {id: first, start: missing-first, setup: [{set: {gold: 3}}]}
  - {id: second, start: missing-second, seed: 41, wait: 1, gif: {fps: 2, duration: 1s, width: 4}}
`)
			args := []string{"capture", dir, "--config", config, "--out", out, "--raw"}
			if format != "text" {
				args = append(args, "--format", format)
			}
			stdout, _ := invoke(t, 1, args...)
			var rows []shotResult
			if err := json.Unmarshal(read(t, filepath.Join(out, "shots.json")), &rows); err != nil {
				t.Fatal(err)
			}
			if len(rows) != 2 || rows[0].ID != "first" || rows[1].ID != "second" || rows[0].Seed != 23 || rows[1].Seed != 41 {
				t.Fatalf("report order or seeds: %+v", rows)
			}
			for _, row := range rows {
				if row.Status != "failed" || row.Reason == "" || row.Artifact != "" || row.Frames != 0 {
					t.Fatalf("failed job reported an artifact: %+v", row)
				}
			}
			files, err := os.ReadDir(out)
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != 2 {
				t.Fatalf("failed capture published extra files: %v", files)
			}
			if !strings.Contains(string(read(t, filepath.Join(out, "index.html"))), "missing-first") {
				t.Fatal("HTML lost the job error")
			}
			switch format {
			case "text":
				if !strings.Contains(stdout, "first failed") || !strings.Contains(stdout, "second failed") {
					t.Fatalf("text report: %s", stdout)
				}
			case "json":
				var summary struct {
					Items             []shotResult
					Succeeded, Failed int
				}
				if err := json.Unmarshal(resultData(t, stdout, "capture"), &summary); err != nil {
					t.Fatal(err)
				}
				if summary.Succeeded != 0 || summary.Failed != 2 || len(summary.Items) != 2 {
					t.Fatalf("summary: %+v", summary)
				}
			case "jsonl":
				for line := range strings.SplitSeq(strings.TrimSpace(stdout), "\n") {
					if !json.Valid([]byte(line)) {
						t.Fatalf("invalid JSONL: %q", line)
					}
				}
				if !strings.Contains(stdout, `"first"`) || !strings.Contains(stdout, `"second"`) {
					t.Fatalf("JSONL omitted jobs: %s", stdout)
				}
			}
		})
	}
}

func TestCaptureOptionFailuresDoNotPublish(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       []string
		code       int
		text, list string
	}{
		{name: "timeout", args: []string{"--timeout", "0s"}, code: 2, text: "positive"},
		{name: "conflict", args: []string{"--raw", "--overlay", "unused"}, code: 2, text: "cannot be combined"},
		{name: "unknown-selection", args: []string{"--only", "absent"}, code: 2, text: "unknown capture"},
		{name: "bad-duration", code: 1, text: "duration", list: "shots: [{id: one, start: absent, gif: {duration: later}}]"},
		{name: "invalid-setup", code: 1, text: "setup", list: "shots: [{id: one, start: absent, setup: [{unknown: true}]}]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := fixture(t)
			config := filepath.Join(t.TempDir(), "shots.yaml")
			out := filepath.Join(t.TempDir(), "result")
			list := tc.list
			if list == "" {
				list = "shots: [{id: one, start: absent}]"
			}
			write(t, config, list)
			args := append([]string{"capture", dir, "--config", config, "--out", out}, tc.args...)
			_, stderr := invoke(t, tc.code, args...)
			if !strings.Contains(stderr, tc.text) {
				t.Fatalf("missing diagnostic %q: %s", tc.text, stderr)
			}
			if _, err := os.Stat(out); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("rejected capture published output: %v", err)
			}
		})
	}
}

func TestCapturePolicyPreflightAndSelection(t *testing.T) {
	for _, mode := range []string{"base", "overlay", "bad-policy", "missing-records"} {
		t.Run(mode, func(t *testing.T) {
			dir := fixture(t)
			top, _ := approvals(t, dir, true)
			write(t, filepath.Join(dir, "asset-policy.yaml"), "report: approvals.json\ninspection: {file: file, source: source, approved: approved}\nrecords: {file: file, source: source, required: required, approved: approved, output: output}\n")
			config := filepath.Join(t.TempDir(), "shots.yaml")
			out := filepath.Join(t.TempDir(), "result")
			write(t, config, "shots: [{id: first, start: missing-first}, {id: second, start: missing-second, seed: 29}]\n")
			args := []string{"capture", dir, "--config", config, "--out", out, "--only", "second", "--format", "json"}
			switch mode {
			case "base":
				write(t, filepath.Join(dir, "approvals.json"), string(read(t, filepath.Join(top, "approvals.json"))))
			case "overlay":
				args = append(args, "--overlay", top)
			case "bad-policy":
				write(t, filepath.Join(dir, "asset-policy.yaml"), "unknown: true\n")
			}
			stdout, _ := invoke(t, 1, args...)
			if mode == "bad-policy" || mode == "missing-records" {
				if _, err := os.Stat(out); !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("failed policy preflight published output: %v", err)
				}
				return
			}
			var summary struct {
				Items  []shotResult
				Failed int
			}
			if err := json.Unmarshal(resultData(t, stdout, "capture"), &summary); err != nil {
				t.Fatal(err)
			}
			if summary.Failed != 1 || len(summary.Items) != 1 || summary.Items[0].ID != "second" || summary.Items[0].Seed != 29 {
				t.Fatalf("selection lost job identity: %+v", summary)
			}
			if html := string(read(t, filepath.Join(out, "index.html"))); strings.Contains(html, "<p class=f>") {
				t.Fatal("policy capture report incorrectly marked raw")
			}
		})
	}
}
