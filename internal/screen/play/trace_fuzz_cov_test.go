package play

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// wldMiniTrace is the smallest record a replay accepts: a start, one input and an end that counts
// the three lines.
const wldMiniTrace = `{"t":"start","tick":0,"version":2,"seed":7,"fingerprint":"f","speed":1,"location":"camp","setup":{"config":{},"read":null,"unlocked":null,"saves":{}}}
{"t":"input","tick":3,"press":["ok"]}
{"t":"end","tick":9,"lines":3,"state":{}}
`

// FuzzLoadReplay: whatever bytes a record holds, reading it gives a replay or an error, never a
// panic, and a replay it gives ends at a tick that is not before its last input.
func FuzzLoadReplay(f *testing.F) {
	f.Add([]byte(wldMiniTrace))
	f.Add([]byte(strings.Replace(wldMiniTrace, `"version":2`, `"version":1`, 1)))
	f.Add([]byte(strings.Replace(wldMiniTrace, `"press":["ok"]`, `"held":["up","left"],"click":true,"mouse":[3,4]`, 1)))
	f.Add([]byte(`{"t":"start","tick":0}`))
	f.Add([]byte("\n\n{}\n"))
	f.Add([]byte(`{"t":"start","tick":0,"version":2,"seed":1,"setup":{"unsupported":["review"]}}`))
	f.Add([]byte(wldMiniTrace + `{"t":"act","tick":5,"name":"buy","args":{"item":"potion"}}` + "\n"))
	dir := f.TempDir()
	f.Fuzz(func(t *testing.T, b []byte) {
		path := filepath.Join(dir, "run.jsonl")
		if err := os.WriteFile(path, b, 0o600); err != nil {
			t.Fatal(err)
		}
		for _, until := range []int{0, 5} {
			o := Options{Until: until}
			r, err := loadReplay(path, &o)
			if err != nil {
				if r != nil {
					t.Fatalf("an error with a replay: %v", err)
				}
				continue
			}
			if r == nil {
				t.Fatal("neither a replay nor an error")
			}
			for tick := range r.inputs {
				if tick > r.limit && !r.partial {
					t.Fatalf("input at tick %d past the end %d", tick, r.limit)
				}
			}
		}
	})
}
