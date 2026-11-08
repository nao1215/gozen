// Package repro builds trace reproduction artifacts for gozen and dango.
package repro

import (
	"bufio"
	"bytes"
	"crypto/sha256"
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
	"github.com/nao1215/gozen/internal/layer1/traceformat"
)

// Bundle describes generated replay artifacts for both command adapters.
// Generation never executes its Argv or Command.
type Bundle struct {
	// Screen is the last drawn screen at or before Tick, with its original tick.
	Screen       json.RawMessage            `json:"screen,omitempty"`
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

func readRepro(path, target string) (Bundle, []byte, error) {
	b := Bundle{}
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
		return b, nil, errors.New("game directory is unknown; record again with gozen run <game-dir> --dev --trace")
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
	for _, record := range records[1:] {
		if record.kind != "initial_save" {
			continue
		}
		var save struct {
			File string `json:"file"`
			Data []byte `json:"data"`
		}
		if err := json.Unmarshal(record.raw, &save); err != nil {
			return b, nil, err
		}
		if record.tick != 0 || start.Setup.Data == nil || len(save.Data) > 8<<20 {
			return b, nil, errors.New("invalid initial save record")
		}
		if _, exists := start.Setup.Data[save.File]; exists {
			return b, nil, errors.New("duplicate initial save record")
		}
		start.Setup.Data[save.File] = save.Data
	}
	for name, digest := range start.Setup.Saves {
		data, ok := start.Setup.Data[name]
		if !ok || fmt.Sprintf("%x", sha256.Sum256(data)) != digest {
			return b, nil, fmt.Errorf("initial save %q is missing or does not match its digest", name)
		}
	}
	tick, err := reproTick(target, records, records[len(records)-1].tick)
	if err != nil {
		return b, nil, err
	}
	if tick == math.MaxInt {
		return b, nil, errors.New("reproduction tick leaves no room for the replay boundary")
	}
	b.GameDir, b.Seed, b.Tick, b.Until = start.GameDir, start.Seed, tick, tick+1
	b.InitialSaves, b.InitialMemo = start.Setup.Data, start.Setup.Memo
	if err := json.Unmarshal(records[0].raw, &b.StartOptions); err != nil {
		return b, nil, err
	}
	for _, key := range []string{"t", "tick", "game", "game_dir", "seed", "setup", "fingerprint", "screen", "version"} {
		delete(b.StartOptions, key)
	}
	for _, r := range records {
		if r.tick > tick {
			break
		}
		if r.kind == "screen" {
			b.Screen = json.RawMessage(r.raw)
		}
	}
	cut, err := cutReproRecords(records, tick)
	return b, cut, err
}

func readReproRecords(src []byte) ([]reproRecord, error) {
	var records []reproRecord
	sc := bufio.NewScanner(bytes.NewReader(src))
	sc.Buffer(make([]byte, 65536), traceformat.LineLimit)
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
	var exact traceformat.ExactReplay
	for _, r := range records {
		if r.tick > tick {
			// A later draw can describe a gap crossing the requested boundary.
			if r.kind == "draw_skip" {
				var span struct {
					First int `json:"first"`
					Last  int `json:"last"`
				}
				if err := json.Unmarshal(r.raw, &span); err != nil {
					return nil, err
				}
				if span.First <= tick {
					fmt.Fprintf(&cut, "{\"t\":\"draw_skip\",\"tick\":%d,\"first\":%d,\"last\":%d}\n", tick, span.First, min(tick, span.Last))
				}
			}
			continue
		}
		if err := exact.Check(r.kind, r.tick, r.raw); err != nil {
			return nil, err
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
		if n < 0 || n > last {
			return 0, errors.New("reproduction tick is outside the trace")
		}
		return n, nil
	}
	if strings.HasSuffix(target, "s") {
		if d, err := time.ParseDuration(target); err == nil {
			// Round to the closest 60 Hz tick, avoiding floating point seed/tick loss.
			n := int(math.Round(d.Seconds() * 60))
			if d < 0 || n > last || n < 0 {
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

// Config selects a trace file, a tick/time/PNG target, and an optional new output
// directory. An empty Out only describes replay of the original trace.
type Config struct {
	Path   string
	Target string
	Out    string
}

// Generate reads the trace and optionally writes a bundle for both CLI adapters.
// It never executes replay. Existing output directories are refused. On failure,
// callers must discard the result; partial output files may remain.
func Generate(cfg Config) (Bundle, error) {
	path, target, out := cfg.Path, cfg.Target, cfg.Out
	b, cut, err := readRepro(path, target)
	if err != nil {
		return Bundle{}, diag.TraceBroken.Wrap(err)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return Bundle{}, err
	}
	shots := path + fmt.Sprintf(".repro-%d", b.Tick)
	if out != "" {
		out, err = filepath.Abs(out)
		if err != nil {
			return Bundle{}, err
		}
		// Never overwrite an earlier bundle or its screenshots.
		if err := os.Mkdir(out, 0o700); err != nil {
			return Bundle{}, err
		}
		path = filepath.Join(out, "trace.jsonl")
		shots = filepath.Join(out, "shots")
		if err := os.WriteFile(path, cut, 0o600); err != nil {
			return Bundle{}, err
		}

		if len(b.InitialMemo) > 0 && string(b.InitialMemo) != "null" {
			var initial struct {
				Save []byte `json:"save"`
			}
			if err := json.Unmarshal(b.InitialMemo, &initial); err != nil {
				return Bundle{}, err
			}
			if err := os.WriteFile(filepath.Join(out, "initial.save"), initial.Save, 0o600); err != nil {
				return Bundle{}, err
			}
		}
		for name, data := range b.InitialSaves {
			if filepath.Base(name) != name || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
				return Bundle{}, errors.New("invalid save filename in trace")
			}
			if err := os.MkdirAll(filepath.Join(out, "saves"), 0o700); err != nil {
				return Bundle{}, err
			}
			if err := os.WriteFile(filepath.Join(out, "saves", name), data, 0o600); err != nil {
				return Bundle{}, err
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
			return Bundle{}, err
		}
		b.Command = reproCommand([]string{"sh", filepath.Join(out, "repro.sh")})
		data, err := json.MarshalIndent(b, "", "  ")
		if err != nil {
			return Bundle{}, err
		}
		if err := os.WriteFile(filepath.Join(out, "repro.json"), append(data, '\n'), 0o600); err != nil {
			return Bundle{}, err
		}
	}
	return b, nil
}
