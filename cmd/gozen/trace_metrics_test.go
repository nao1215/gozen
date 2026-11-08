package main

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func metricsFixture(t *testing.T, lines ...string) playMetrics {
	t.Helper()
	m, err := measureTrace(strings.NewReader(strings.Join(lines, "\n")), 10, 2)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestTraceMetricsText(t *testing.T) {
	m := metricsFixture(t,
		`{"t":"start","tick":0,"event":"opening"}`,
		`{"t":"say","tick":10,"text":"é猫\nEcho","choices":["Go","Stay"]}`,
		`{"t":"screen","tick":11,"texts":[{"text":"é"}]}`,
		`{"t":"screen","tick":12,"texts":[{"text":"é猫"}]}`,
		`{"t":"answer","tick":60,"index":0}`,
		`{"t":"say","tick":70,"event":"hall","text":"Echo"}`,
		`{"t":"decision","tick":90,"kind":"menu"}`,
		`{"t":"answer","tick":90,"index":0}`,
		`{"t":"say","tick":100,"text":"Echo"}`,
		`{"t":"answer","tick":120,"phone":true,"index":1}`,
		`{"t":"say","tick":130,"text":"Tail"}`,
		`{"t":"end","tick":300}`)
	if len(m.Decisions) != 3 || m.Decisions[0].Characters != 6 || m.Decisions[0].Lines != 2 ||
		m.Decisions[1].Decision != "menu" || m.Decisions[2].Decision != "phone_result" {
		t.Fatalf("decisions: %+v", m.Decisions)
	}
	if m.Longest.Ticks != 180 || m.Longest.Seconds != 3 || m.Longest.Characters != 4 ||
		m.Longest.From.Event != "hall" || m.Longest.To.Tick != 300 {
		t.Fatalf("longest: %+v", m.Longest)
	}
	if len(m.Repeated) != 1 || m.Repeated[0].Count != 3 || len(m.Repeated[0].FirstTicks) != 3 || m.Repeated[0].FirstTicks[0] != 10 {
		t.Fatalf("repeats: %+v", m.Repeated)
	}
}

func TestTraceMetricsSignals(t *testing.T) {
	m := metricsFixture(t,
		`{"t":"music","tick":0,"song":"first"}`,
		`{"t":"stand","tick":0,"cg":""}`,
		`{"t":"screen","tick":0,"backgrounds":["room"],"phone":""}`,
		`{"t":"scene","tick":60,"stack":["field","battle"]}`,
		`{"t":"battle","tick":120,"cutin":"Scout"}`,
		`{"t":"music","tick":120,"song":"second"}`,
		`{"t":"music","tick":130,"song":"second","asked":"third"}`,
		`{"t":"stand","tick":180,"cg":"card"}`,
		`{"t":"screen","tick":240,"backgrounds":["hall"],"phone":"chat","phone_call":true}`,
		`{"t":"screen","tick":241,"backgrounds":["hall"],"phone":"chat","phone_call":true,"speaker":"Scout"}`)
	if len(m.Signals) != 4 || len(m.Signals[1].Kinds) != 2 || len(m.Signals[3].Kinds) != 2 ||
		m.Signals[0].Ticks != nil || *m.Signals[3].Ticks != 60 || *m.Signals[3].Seconds != 1 {
		t.Fatalf("signals: %+v", m.Signals)
	}
}

func TestTraceMetricsStalls(t *testing.T) {
	m := metricsFixture(t,
		`{"t":"screen","tick":0,"texts":[{"text":"Wait"}]}`,
		`{"t":"state","tick":0,"gold":1}`,
		`{"t":"input","tick":30,"press":["ok"]}`,
		`{"t":"state","tick":120,"gold":1,"terminal":true}`,
		`{"t":"section","tick":180,"name":"wait","from_tick":0,"frames":40}`,
		`{"t":"screen","tick":240,"texts":[{"text":"Done"}]}`,
		`{"t":"state","tick":240,"gold":2}`,
		`{"t":"end","tick":245}`)
	found := map[string]metricStall{}
	for _, s := range m.Stalls {
		found[s.Kind] = s
	}
	if len(found) != 4 || found["same_screen"].Ticks != 240 ||
		found["no_state_change"].Ticks != 240 || found["input_without_observed_progress"].Inputs != 1 ||
		num(found["input_without_observed_progress"].Screen["tick"]) != 0 {
		t.Fatalf("stalls: %+v", m.Stalls)
	}
	if len(m.Sections) != 1 || num(m.Sections[0]["frames"]) != 40 {
		t.Fatal(m.Sections)
	}
}

func TestTraceMetricsFactsNeverFail(t *testing.T) {
	file := traceFile(t, `{"t":"say","tick":0,"text":"Echo"}`, `{"t":"say","tick":1,"text":"Echo"}`, `{"t":"end","tick":9000}`)
	out, stderr := invoke(t, 0, "trace", file, "--metrics", "--format", "json")
	var m playMetrics
	if err := json.Unmarshal(resultData(t, out, "trace"), &m); err != nil {
		t.Fatal(err)
	}
	if stderr != "" || m.Longest.Ticks != 9000 || len(m.Stalls) != 1 {
		t.Fatal(out, stderr)
	}
	jsonl, _ := invoke(t, 0, "trace", file, "--metrics", "--format", "jsonl")
	if len(strings.Split(strings.TrimSpace(jsonl), "\n")) != 1 {
		t.Fatal(jsonl)
	}
	if err := json.Unmarshal(resultData(t, jsonl, "trace"), &m); err != nil {
		t.Fatal(err)
	}
	out, _ = invoke(t, 0, "trace", file, "--metrics")
	if !strings.Contains(out, "2x \"Echo\"") || !strings.Contains(out, "facts, never verdicts") {
		t.Fatal(out)
	}
	for _, args := range [][]string{
		{"--metrics", "--check"},
		{"--metrics", "--only", "say"},
		{"--metrics", "--from", "1"},
		{"--metrics", "--repro", "1"},
		{"--top", "1"},
		{"--metrics", "--top", "0"},
		{"--metrics", "--stall-seconds", "NaN"},
		{"--metrics", "--stall-seconds", "0"},
	} {
		invoke(t, 2, append([]string{"trace", "absent.jsonl"}, args...)...)
	}
}

func TestTraceMetricsStreamErrorsAndTop(t *testing.T) {
	for _, input := range []string{`{"t":"say","tick":2}
{"t":"end","tick":1}`, `{"t":`, `{"tick":0}`} {
		if _, err := measureTrace(strings.NewReader(input), 1, 5); err == nil {
			t.Fatal(input)
		}
	}
	if _, err := measureTrace(failingMetricReader{}, 1, 5); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
	m, err := measureTrace(strings.NewReader(`{"t":"say","text":"B\nA\nB\nA","tick":0}`), 1, 5)
	if err != nil || len(m.Repeated) != 1 || m.Repeated[0].Text != "A" {
		t.Fatal(m, err)
	}
	m = metricsFixture(t)
	if m.Longest.Ticks != 0 || len(m.Decisions) != 0 {
		t.Fatal(m)
	}
}

type failingMetricReader struct{}

func (failingMetricReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestTraceMetricsAcknowledgementAndAutoplay(t *testing.T) {
	m := metricsFixture(t,
		`{"t":"screen","tick":0,"phone":"page"}`,
		`{"t":"autoplay_wait","tick":0,"waiting":"choice"}`,
		`{"t":"input_result","tick":10,"accepted":false,"phone":true}`,
		`{"t":"input_result","tick":20,"accepted":false,"phone":true}`,
		`{"t":"input_result","tick":180,"accepted":true,"phone":true}`,
		`{"t":"answer","tick":190,"phone":true,"index":0}`,
		`{"t":"autoplay_wait","tick":200,"waiting":""}`,
		`{"t":"end","tick":210}`)
	if len(m.Decisions) != 1 || m.Decisions[0].Decision != "phone_action" || m.Decisions[0].To.Tick != 180 {
		t.Fatalf("phone counted twice: %+v", m.Decisions)
	}
	var waiting, rejected bool
	for _, s := range m.Stalls {
		if s.Kind == "autoplay_wait:choice" {
			waiting = s.Ticks == 200
		}
		if s.Kind == "input_not_acknowledged" {
			rejected = s.Ticks == 170 && s.Screen != nil
		}
	}
	if !waiting || !rejected {
		t.Fatalf("missing observations: %+v", m.Stalls)
	}
}
