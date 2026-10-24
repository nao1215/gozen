package main

import (
	"os"
	"path/filepath"
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
