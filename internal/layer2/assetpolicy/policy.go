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

	"github.com/nao1215/gozen/assetfs"
)

type Approval = assetfs.Approval

// RecordFields は制作記録を承認へ変換する項目名。入れ子はドットで区切る。
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

// Policy は作品側の承認記録の読み方。分類名や加工方法をエンジンで固定しない。
type Policy struct {
	Overlay    string       `yaml:"overlay"`
	Report     string       `yaml:"report"`
	Extensions []string     `yaml:"extensions"`
	Inspection RecordFields `yaml:"inspection"`
	Records    RecordFields `yaml:"records"`
}

// LoadPolicy は作品の設定を読む。省略時は中立の承認形式を使う。
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
	// 書かれた設定は全体で受け取り、標準形式の項目が紛れ込まないようにする。
	p = Policy{}
	if err := yaml.UnmarshalWithOptions(b, &p, yaml.Strict()); err != nil {
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
	// 型の違う判定値を省略と同じ扱いにして承認しない。
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

// ReadRecords は出力一覧と承認記録を同時に読み、前回の残存出力を区別する。
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

// ReadInspections は旧形式も設定に従って変換する。元ハッシュが無い旧記録は出力記録で補う。
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
		// 新しい検査記録だけで、古い出力を新しい元画像の差し替えと認めない。
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
