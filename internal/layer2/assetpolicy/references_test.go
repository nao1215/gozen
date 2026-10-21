package assetpolicy

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestReferencesFindsPicturesAndTheMissing(t *testing.T) {
	fsys := fstest.MapFS{
		"assets/a.png":           {Data: []byte("a")},
		"assets/dir/b.JPG":       {Data: []byte("b")},
		"assets/dir/c.webp":      {Data: []byte("c")},
		"assets/cg/one.png":      {Data: []byte("1")},
		"assets/cg/two.png":      {Data: []byte("2")},
		"assets/unused.png":      {Data: []byte("u")},
		"assets/note.txt":        {Data: []byte("n")},
		"asset-policy.yaml":      {Data: []byte("files: [policy-only.png]\n")},
		"game.yaml":              {Data: []byte("title: Game\ncover: assets/a.png\nnote: assets/note.txt\nsentence: has a space.png\n")},
		"data/world.yaml":        {Data: []byte("pics:\n  - dir/b.JPG\n  - nested: {x: [dir/c.webp, missing.png]}\n  - cg/*.png\n  - ./a.png\n  - '  a.png  '\nnum: 3\nflag: true\nnone: ~\n")},
		"data/other.yml":         {Data: []byte("p: gone/also.png\nmulti: \"line\\nbreak.png\"\n")},
		"data/not-yaml.txt":      {Data: []byte("ignored.png")},
		"assets/inner/skip.yaml": {Data: []byte("p: inside-assets.png\n")},
	}
	pics, missing, err := References(fsys, "assets", []string{".png", ".jpg", ".webp"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.png", "cg/one.png", "cg/two.png", "dir/b.JPG", "dir/c.webp"}
	if !slices.Equal(pics, want) {
		t.Errorf("pictures %v, want %v", pics, want)
	}
	wantMissing := []string{"gone/also.png", "missing.png"}
	if !slices.Equal(missing, wantMissing) {
		t.Errorf("missing %v, want %v", missing, wantMissing)
	}
}

func TestReferencesReportsBrokenInput(t *testing.T) {
	good := fstest.MapFS{"assets/a.png": {Data: []byte("a")}}
	if _, _, err := References(good, "nope", []string{".png"}); err != nil {
		// fs.Sub of a missing directory is fine; walking reports nothing
		t.Logf("missing prefix: %v", err)
	}
	broken := fstest.MapFS{"assets/a.png": {Data: []byte("a")}, "data/x.yaml": {Data: []byte("a: [\n")}}
	if _, _, err := References(broken, "assets", []string{".png"}); err == nil {
		t.Error("broken YAML must be reported")
	}
	bad := fstest.MapFS{"assets/a.png": {Data: []byte("a")}, "data/x.yaml": {Data: []byte("p: 'dir/[.png'\n")}}
	if _, _, err := References(bad, "assets", []string{".png"}); err == nil {
		t.Error("a malformed pattern must be reported")
	}
	if _, _, err := References(good, "../assets", []string{".png"}); err == nil {
		t.Error("an invalid prefix must be reported")
	}
}

func TestLoadDefaultsAndErrors(t *testing.T) {
	empty := fstest.MapFS{}
	p, err := Load(empty, "asset-policy.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if p.Report != "approvals.json" || !slices.Contains(p.Extensions, ".png") || p.Records.Output != "output" || p.Inspection.Approved != "approved" {
		t.Errorf("neutral defaults %+v", p)
	}
	for name, body := range map[string]string{
		"unknown key":               "report: a.json\nfoo: 1\n",
		"broken yaml":               "report: [\n",
		"report outside":            "report: ../a.json\ninspection: {file: f, approved: a}\nrecords: {file: f, source: s}\n",
		"no record file":            "report: a.json\ninspection: {file: f, approved: a}\nrecords: {source: s}\n",
		"no inspection judgement":   "report: a.json\ninspection: {file: f}\nrecords: {file: f, source: s}\n",
		"status without accepted":   "report: a.json\ninspection: {file: f, status: st}\nrecords: {file: f, source: s}\n",
		"no inspection file at all": "report: a.json\nrecords: {file: f, source: s}\n",
	} {
		_, err := Load(fstest.MapFS{"p.yaml": {Data: []byte(body)}}, "p.yaml")
		if err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	ok := "report: a.json\ninspection: {file: f, status: st, accepted: [ok]}\nrecords: {file: f, source: s}\n"
	if _, err := Load(fstest.MapFS{"p.yaml": {Data: []byte(ok)}}, "p.yaml"); err != nil {
		t.Errorf("a status with accepted values is a judgement: %v", err)
	}
	// reading the policy from a directory fails with a real error, not defaults
	if _, err := Load(fstest.MapFS{"p.yaml/x": {Data: nil}}, "p.yaml"); err == nil {
		t.Error("a directory where the policy should be")
	}
}

func TestReadRecordsRejectsBadReports(t *testing.T) {
	p, err := Load(fstest.MapFS{}, "none")
	if err != nil {
		t.Fatal(err)
	}
	read := func(body string) ([]Approval, []string, error) {
		return p.ReadRecords(fstest.MapFS{"approvals.json": {Data: []byte(body)}})
	}
	if _, _, err := p.ReadRecords(fstest.MapFS{}); err == nil {
		t.Error("a missing report")
	}
	for name, body := range map[string]string{
		"not json":          "{",
		"not a list":        `{"file":"a.png"}`,
		"duplicate file":    `[{"file":"a.png","source":"s"},{"file":"a.png","source":"s"}]`,
		"invalid file path": `[{"file":"../a.png","source":"s"}]`,
		"empty file":        `[{"source":"s"}]`,
		"output differs":    `[{"file":"a.png","source":"s","output":"b.png"}]`,
	} {
		if _, _, err := read(body); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	rows, outs, err := read(`[{"file":"a.png","source":"s","output":"a.png"},{"file":"b.png","source":"t","required":true,"approved":true}]`)
	if err != nil || len(rows) != 2 || len(outs) != 1 || outs[0] != "a.png" {
		t.Fatalf("%+v %v %v", rows, outs, err)
	}
	if !rows[0].Required {
		t.Error("a record with an output is required")
	}
	if !rows[1].Required || !rows[1].Approved {
		t.Errorf("explicit flags are kept: %+v", rows[1])
	}
}

func TestInspectionJudgements(t *testing.T) {
	p := Policy{Inspection: RecordFields{File: "file", Source: "source", Required: "required", Original: "original", Approved: "approved", Reject: "problems"}}
	dir := t.TempDir()
	cases := []struct {
		name, row string
		approved  bool
	}{
		{"all fine", `{"file":"a.png","approved":true}`, true},
		{"not approved", `{"file":"a.png","approved":false}`, false},
		{"no approval field", `{"file":"a.png"}`, false},
		{"empty problem list", `{"file":"a.png","approved":true,"problems":[]}`, true},
		{"a problem listed", `{"file":"a.png","approved":true,"problems":["blur"]}`, false},
		{"problem as text", `{"file":"a.png","approved":true,"problems":"blur"}`, false},
		{"empty problem text", `{"file":"a.png","approved":true,"problems":""}`, true},
		{"problem flag set", `{"file":"a.png","approved":true,"problems":true}`, false},
		{"problem flag clear", `{"file":"a.png","approved":true,"problems":false}`, true},
		{"problem of a number", `{"file":"a.png","approved":true,"problems":3}`, false},
		{"approved as text", `{"file":"a.png","approved":"yes"}`, false},
	}
	for _, c := range cases {
		path := filepath.Join(dir, "i.jsonl")
		if err := os.WriteFile(path, []byte(c.row+"\n\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := p.ReadInspections(path, nil)
		if err != nil || len(got) != 1 {
			t.Fatalf("%s: %v %v", c.name, got, err)
		}
		if got[0].Approved != c.approved {
			t.Errorf("%s: approved = %v, want %v", c.name, got[0].Approved, c.approved)
		}
	}
	if _, err := p.ReadInspections(filepath.Join(dir, "nope.jsonl"), nil); err == nil {
		t.Error("a missing inspection file")
	}
	// a line longer than the default scanner buffer is still read
	long := `{"file":"a.png","approved":true,"pad":"` + strings.Repeat("x", 200000) + `"}`
	path := filepath.Join(dir, "long.jsonl")
	if err := os.WriteFile(path, []byte(long), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := p.ReadInspections(path, nil); err != nil || len(got) != 1 || !got[0].Approved {
		t.Errorf("a long line: %v %v", got, err)
	}
}
