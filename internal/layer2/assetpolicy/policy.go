// Package assetpolicy decodes author-supplied approval mappings for tooling.
// It belongs to model layer 2; assetfs only consumes normalized approvals.
package assetpolicy

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/layer1/assetfs"
	"github.com/nao1215/gozen/internal/layer1/decodeguard"
)

type Approval = assetfs.Approval

// RecordFields are the field names that convert a production record into an approval. Nesting is separated by dots.
type RecordFields struct {
	File     string   `yaml:"file"`
	Source   string   `yaml:"source"`
	Required string   `yaml:"required"`
	Approved string   `yaml:"approved"`
	Original string   `yaml:"original"`
	Status   string   `yaml:"status"`
	Accepted []string `yaml:"accepted"`
	Output   string   `yaml:"output"`
	Reject   string   `yaml:"reject"`
}

// Policy is how a work reads its approval records. The engine does not fix category names or processing methods.
type Policy struct {
	Overlay    string       `yaml:"overlay"`
	Report     string       `yaml:"report"`
	Extensions []string     `yaml:"extensions"`
	Inspection RecordFields `yaml:"inspection"`
	Records    RecordFields `yaml:"records"`
}

// LoadPolicy reads the work's settings. If omitted, it uses a neutral approval format.
func Load(base fs.FS, name string) (Policy, error) {
	p := Policy{Report: "approvals.json", Extensions: []string{".png", ".jpg", ".jpeg", ".webp"},
		Inspection: RecordFields{File: "file", Source: "source", Required: "required", Approved: "approved"},
		Records:    RecordFields{File: "file", Source: "source", Required: "required", Approved: "approved", Output: "output"},
	}
	b, err := fs.ReadFile(base, name)
	if os.IsNotExist(err) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	// Accept the written settings as a whole so that fields of the standard format do not leak in.
	p = Policy{}
	if err := decodeguard.Run(func() error { return yaml.UnmarshalWithOptions(b, &p, yaml.Strict()) }); err != nil {
		return p, err
	}
	if !fs.ValidPath(p.Report) || p.Records.File == "" || p.Records.Source == "" || p.Inspection.File == "" || (p.Inspection.Approved == "" && (p.Inspection.Status == "" || len(p.Inspection.Accepted) == 0)) {
		return p, fmt.Errorf("assetfs: invalid approval policy")
	}
	return p, nil
}

func field(m map[string]any, key string) any {
	var v any = m
	if key == "" {
		return nil
	}
	for _, k := range strings.Split(key, ".") {
		x, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = x[k]
	}
	return v
}

func (f RecordFields) approval(m map[string]any) Approval {
	str := func(k string) string {
		s, ok := field(m, k).(string)
		if !ok {
			return ""
		}
		return s
	}
	b := func(k string) bool { v, ok := field(m, k).(bool); return ok && v }
	a := Approval{File: str(f.File), Source: str(f.Source), Status: str(f.Status), Required: b(f.Required) && !b(f.Original)}
	a.Approved = f.Status == "" || slices.Contains(f.Accepted, a.Status)
	if f.Approved != "" {
		a.Approved = a.Approved && b(f.Approved)
	}
	// Treat a judgement value of the wrong type like an omitted one and do not approve.
	for _, key := range []string{f.Required, f.Original, f.Approved} {
		if v := field(m, key); v != nil {
			if _, ok := v.(bool); !ok {
				a.Approved = false
			}
		}
	}
	if v := field(m, f.Reject); v != nil {
		switch x := v.(type) {
		case []any:
			a.Approved = a.Approved && len(x) == 0
		case string:
			a.Approved = a.Approved && x == ""
		case bool:
			a.Approved = a.Approved && !x
		default:
			a.Approved = false
		}
	}
	if str(f.Output) != "" {
		a.Required = true
	}
	return a
}

// ReadRecords reads the output list and approval records together and distinguishes leftover outputs from the previous run.
func (p Policy) ReadRecords(top fs.FS) ([]Approval, []string, error) {
	b, err := fs.ReadFile(top, p.Report)
	if err != nil {
		return nil, nil, err
	}
	var rows []map[string]any
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, nil, err
	}
	var approvals []Approval
	outputs := []string{}
	seen := map[string]bool{}
	for _, row := range rows {
		a := p.Records.approval(row)
		if !fs.ValidPath(a.File) || seen[a.File] {
			return nil, nil, fmt.Errorf("assetfs: invalid or duplicate record %q", a.File)
		}
		seen[a.File] = true
		approvals = append(approvals, a)
		if out, ok := field(row, p.Records.Output).(string); ok && out != "" {
			if out != a.File {
				return nil, nil, fmt.Errorf("assetfs: output differs from source path %q", a.File)
			}
			outputs = append(outputs, out)
		}
	}
	return approvals, outputs, nil
}

// ReadInspections also converts the old format according to the settings. Old records without a source hash are filled in from the output record.
func (p Policy) ReadInspections(file string, records []Approval) (result []Approval, err error) {
	f, err := os.Open(file) //nolint:gosec // Tool caller supplies the inspection file.
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	byFile := map[string]Approval{}
	for _, a := range records {
		byFile[a.File] = a
	}
	seen := map[string]bool{}
	var out []Approval
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for n := 1; sc.Scan(); n++ {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", file, n, err)
		}
		a := p.Inspection.approval(row)
		if !fs.ValidPath(a.File) || seen[a.File] {
			return nil, fmt.Errorf("assetfs: invalid or duplicate inspection %q", a.File)
		}
		seen[a.File] = true
		if a.Source == "" {
			a.Source = byFile[a.File].Source
		}
		// A new inspection record alone does not count as replacing the source image with a new one for old outputs.
		if r, ok := byFile[a.File]; ok && r.Required && !strings.EqualFold(r.Source, a.Source) {
			a.Approved = false
		}
		out = append(out, a)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("assetfs: empty inspection %s", file)
	}
	return out, nil
}
