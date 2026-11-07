package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nao1215/gozen/internal/diag"
)

type reproBundle struct {
	GameDir      string                     `json:"game_dir"`
	Seed         uint64                     `json:"seed"`
	StartOptions map[string]json.RawMessage `json:"start_options"`
	InitialSaves map[string][]byte          `json:"initial_saves"`
	InitialMemo  json.RawMessage            `json:"initial_memo,omitempty"`
	Tick         int                        `json:"tick"`
	Until        int                        `json:"until"`
	Trace        string                     `json:"trace"`
	Screenshot   string                     `json:"screenshot"`
	Argv         []string                   `json:"argv"`
	Command      string                     `json:"command"`
}

type reproRecord struct {
	raw  []byte
	kind string
	tick int
	shot string
}

func readRepro(path, target string) (reproBundle, []byte, error) {
	b := reproBundle{}
	src, err := os.ReadFile(path) //nolint:gosec // Explicit trace input.
	if err != nil {
		return b, nil, err
	}
	records, err := readReproRecords(src)
	if err != nil {
		return b, nil, err
	}
	var start struct {
		GameDir string `json:"game_dir"`
		Seed    uint64 `json:"seed"`
		Setup   struct {
			Saves       map[string]string `json:"saves"`
			Data        map[string][]byte `json:"save_data"`
			Memo        json.RawMessage   `json:"memo"`
			Unsupported []string          `json:"unsupported"`
		} `json:"setup"`
	}
	if err := json.Unmarshal(records[0].raw, &start); err != nil {
		return b, nil, err
	}
	if start.GameDir == "" {
		return b, nil, errors.New("trace lacks game_dir; record again with gozen run --trace")
	}
	if start.Seed == 0 {
		return b, nil, errors.New("trace lacks a reproducible seed")
	}
	if len(start.Setup.Unsupported) > 0 {
		return b, nil, fmt.Errorf("unsupported start conditions: %v", start.Setup.Unsupported)
	}
	if len(start.Setup.Saves) > 0 && start.Setup.Data == nil {
		return b, nil, errors.New("trace contains save digests but no initial save bytes; record again with --trace")
	}
	tick, err := reproTick(target, records, records[len(records)-1].tick)
	if err != nil {
		return b, nil, err
	}
	b.GameDir, b.Seed, b.Tick, b.Until = start.GameDir, start.Seed, tick, tick+1
	b.InitialSaves, b.InitialMemo = start.Setup.Data, start.Setup.Memo
	if err := json.Unmarshal(records[0].raw, &b.StartOptions); err != nil {
		return b, nil, err
	}
	for _, key := range []string{"t", "tick", "game", "game_dir", "seed", "setup", "fingerprint", "screen", "version"} {
		delete(b.StartOptions, key)
	}
	cut, err := cutReproRecords(records, tick)
	return b, cut, err
}

func readReproRecords(src []byte) ([]reproRecord, error) {
	var records []reproRecord
	sc := bufio.NewScanner(bytes.NewReader(src))
	sc.Buffer(make([]byte, 65536), 1<<24)
	last := 0
	for sc.Scan() {
		var h struct {
			Kind string `json:"t"`
			Tick *int   `json:"tick"`
			Shot string `json:"shot"`
			File string `json:"file"`
		}
		if err := json.Unmarshal(sc.Bytes(), &h); err != nil {
			return nil, err
		}
		if h.Tick == nil || *h.Tick < last || h.Kind == "" {
			return nil, errors.New("invalid trace order or tick")
		}
		if len(records) == 0 && (h.Kind != "start" || *h.Tick != 0) {
			return nil, errors.New("trace needs a start record")
		}
		if len(records) > 0 && h.Kind == "start" {
			return nil, errors.New("duplicate start record")
		}
		if h.Kind == "shot" {
			h.Shot = h.File
		}
		records = append(records, reproRecord{bytes.Clone(sc.Bytes()), h.Kind, *h.Tick, h.Shot})
		last = *h.Tick
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, errors.New("empty trace")
	}
	return records, nil
}

// cutReproRecords validates replayable actions through tick and adds the seek boundary.
func cutReproRecords(records []reproRecord, tick int) ([]byte, error) {
	var cut bytes.Buffer
	type viewportSize struct {
		Width  int     `json:"width"`
		Height int     `json:"height"`
		Scale  float64 `json:"scale"`
		Tall   bool    `json:"tall"`
	}
	var viewport *viewportSize
	for _, r := range records {
		if r.tick > tick {
			break
		}
		if r.kind == "live_reload" {
			return nil, errors.New("cannot reproduce across live reload; record again after the edit")
		}
		if r.kind == "battle_setup" {
			return nil, fmt.Errorf("cannot reproduce unsupported script operation battle-hp at tick %d", r.tick)
		}
		if r.kind == "unsupported_script" {
			return nil, fmt.Errorf("cannot reproduce unsupported script operation at tick %d: %s", r.tick, r.raw)
		}
		if r.kind == "act" {
			var act struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(r.raw, &act); err != nil {
				return nil, err
			}
			if act.Name == "window" {
				return nil, errors.New("cannot reproduce exactly: the recording changed the window size")
			}
		}
		if r.kind == "viewport" {
			var current viewportSize
			if err := json.Unmarshal(r.raw, &current); err != nil {
				return nil, err
			}
			if viewport != nil && current != *viewport {
				return nil, errors.New("cannot reproduce exactly: the recording changed the window size")
			}
			viewport = &current
		}
		if r.kind == "guide" {
			return nil, errors.New("cannot reproduce exactly: guides cannot be reproduced exactly")
		}
		if r.kind == "end" {
			continue
		}
		cut.Write(r.raw)
		cut.WriteByte('\n')
	}
	// A sparse trace still needs an explicit boundary for --until validation.
	fmt.Fprintf(&cut, "{\"t\":\"repro\",\"tick\":%d}\n", tick)
	return cut.Bytes(), nil
}

func reproTick(target string, records []reproRecord, last int) (int, error) {
	if n, err := strconv.Atoi(target); err == nil {
		if n < 0 || n > last || n == math.MaxInt {
			return 0, errors.New("reproduction tick is outside the trace")
		}
		return n, nil
	}
	if strings.HasSuffix(target, "s") {
		if d, err := time.ParseDuration(target); err == nil {
			// Round to the closest 60 Hz tick, avoiding floating point seed/tick loss.
			n := int(math.Round(d.Seconds() * 60))
			if d < 0 || n > last || n < 0 || n == math.MaxInt {
				return 0, errors.New("reproduction time is outside the trace")
			}
			return n, nil
		}
	}
	wanted, err := filepath.Abs(target)
	if err != nil {
		return 0, err
	}
	tick := -1
	for _, r := range records {
		if r.shot == "" {
			continue
		}
		p, err := filepath.Abs(r.shot)
		if err != nil {
			return 0, err
		}
		if p == wanted || (filepath.Base(target) == target && filepath.Base(r.shot) == target) {
			if tick >= 0 && tick != r.tick {
				return 0, errors.New("ambiguous screenshot path; use a tick")
			}
			tick = r.tick
		}
	}
	if tick < 0 {
		return 0, fmt.Errorf("no screenshot %q in trace (use a tick or a time such as 12.5s)", target)
	}
	return tick, nil
}

func shellArg(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
func reproCommand(args []string) string {
	quoted := make([]string, len(args))
	for i, s := range args {
		quoted[i] = shellArg(s)
	}
	return strings.Join(quoted, " ")
}

func (c *command) traceRepro(path, target, out string) error {
	b, cut, err := readRepro(path, target)
	if err != nil {
		return diag.TraceBroken.Wrap(err)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return err
	}
	shots := path + fmt.Sprintf(".repro-%d", b.Tick)
	if out != "" {
		out, err = filepath.Abs(out)
		if err != nil {
			return err
		}
		// Never overwrite an earlier bundle or its screenshots.
		if err := os.Mkdir(out, 0o700); err != nil {
			return err
		}
		path = filepath.Join(out, "trace.jsonl")
		shots = filepath.Join(out, "shots")
		if err := os.WriteFile(path, cut, 0o600); err != nil {
			return err
		}

		if len(b.InitialMemo) > 0 && string(b.InitialMemo) != "null" {
			var initial struct {
				Save []byte `json:"save"`
			}
			if err := json.Unmarshal(b.InitialMemo, &initial); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(out, "initial.save"), initial.Save, 0o600); err != nil {
				return err
			}
		}
		for name, data := range b.InitialSaves {
			if filepath.Base(name) != name || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
				return errors.New("invalid save filename in trace")
			}
			if err := os.MkdirAll(filepath.Join(out, "saves"), 0o700); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(out, "saves", name), data, 0o600); err != nil {
				return err
			}
		}
	}
	b.Trace = path
	b.Screenshot = filepath.Join(shots, fmt.Sprintf("until_%06d.png", b.Until))
	b.Argv = []string{"gozen", "run", b.GameDir, "--dev", "--mute", "--replay", path, "--until", strconv.Itoa(b.Until), "--shots", shots}
	b.Command = reproCommand(b.Argv)
	if out != "" {
		// Resolve bundled artifacts relative to this script, so the bundle can move.
		args := []string{"gozen", "run", b.GameDir, "--dev", "--mute", "--replay", "trace.jsonl", "--until", strconv.Itoa(b.Until), "--shots", "shots"}
		script := "#!/bin/sh\nset -eu\ncd -- \"$(dirname -- \"$0\")\"\nexec " + reproCommand(args) + "\n"
		if err := os.WriteFile(filepath.Join(out, "repro.sh"), []byte(script), 0o600); err != nil {
			return err
		}
		b.Command = reproCommand([]string{"sh", filepath.Join(out, "repro.sh")})
		data, err := json.MarshalIndent(b, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, "repro.json"), append(data, '\n'), 0o600); err != nil {
			return err
		}
	}
	if c.report.Machine() {
		return c.report.Result(c.out, "ok", b)
	}
	c.out.Println(b.Command)
	return nil
}
