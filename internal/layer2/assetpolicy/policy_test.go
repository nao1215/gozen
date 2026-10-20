package assetpolicy

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func sum(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }
func write(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, s := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

// A work can change field names and approval values, so the old format is not fixed in the engine.
func TestApprovalPolicyMapping(t *testing.T) {
	src, top := t.TempDir(), t.TempDir()
	write(t, src, map[string]string{"asset-policy.yaml": `report: inventory.json
inspection:
  file: name
  source: hash
  required: replace
  original: original.accepted
  status: decision
  accepted: [ok, unchanged]
records:
  file: name
  source: hash
  output: result
  reject: pending
`})
	p, err := Load(os.DirFS(src), "asset-policy.yaml")
	if err != nil {
		t.Fatal(err)
	}
	write(t, top, map[string]string{"inventory.json": `[{"name":"a.png","hash":"abc","result":"a.png"}]`})
	r, outputs, err := p.ReadRecords(os.DirFS(top))
	if err != nil || len(outputs) != 1 {
		t.Fatalf("出力記録 %v %v", outputs, err)
	}
	for _, tc := range []struct {
		name, body         string
		approved, required bool
	}{
		{"差し替え", `{"name":"a.png","replace":true,"decision":"ok"}`, true, true},
		{"元のまま承認", `{"name":"a.png","replace":true,"original":{"accepted":true},"decision":"ok"}`, true, false},
		{"差し替え不要", `{"name":"a.png","decision":"unchanged"}`, true, false},
		{"要確認", `{"name":"a.png","decision":"pending"}`, false, false},
		{"未知の状態", `{"name":"a.png","decision":"unknown"}`, false, false},
		{"判定値の型違い", `{"name":"a.png","replace":"true","decision":"ok"}`, false, false},
		{"検査と出力の元画像が違う", `{"name":"a.png","hash":"def","decision":"ok"}`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "inspection.jsonl")
			if err := os.WriteFile(file, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			as, err := p.ReadInspections(file, r)
			if err != nil {
				t.Fatal(err)
			}
			if len(as) != 1 || as[0].Approved != tc.approved || as[0].Required != tc.required || as[0].Source == "" {
				t.Fatalf("変換結果 %+v", as)
			}
		})
	}
	for _, body := range []string{"", `null`, "{}", "{", `{"name":"a.png"}` + "\n" + `{"name":"a.png"}`, `{"name":"../a.png"}`} {
		file := filepath.Join(t.TempDir(), "bad.jsonl")
		write(t, filepath.Dir(file), map[string]string{filepath.Base(file): body})
		if _, err := p.ReadInspections(file, r); err == nil {
			t.Errorf("不正な記録が通った %q", body)
		}
	}
}

func TestApprovalNativeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p, err := Load(os.DirFS(dir), "asset-policy.yaml")
	if err != nil {
		t.Fatal(err)
	}
	want := Approval{File: "a.png", Source: sum("元画像"), Approved: true, Required: true}
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, map[string]string{"inspect.jsonl": string(b)})
	got, err := p.ReadInspections(filepath.Join(dir, "inspect.jsonl"), nil)
	if err != nil || len(got) != 1 || got[0] != want {
		t.Fatalf("中立形式の往復 %+v %v", got, err)
	}
}
