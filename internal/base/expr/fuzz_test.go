package expr

import "testing"

// FuzzParse checks that invalid expressions are rejected and that accepted ones can be re-parsed and have their references enumerated.
func FuzzParse(f *testing.F) {
	for _, src := range []string{"", "1 +", "(", "'open", "1 + 2 * 3", "flag.ready && gold > 0", "self.hp / self.mhp", "actor['scout'].hp", "true ? max(1, 2) : 0", "not false", "a[0].b", "未知"} {
		f.Add(src)
	}
	f.Fuzz(func(t *testing.T, src string) {
		if len(src) > 64*1024 {
			return
		}
		e, err := Parse(src)
		if err != nil {
			return
		}
		encoded, err := e.MarshalText()
		if err != nil {
			t.Fatal(err)
		}
		again, err := Parse(string(encoded))
		if err != nil {
			t.Fatalf("受理した式を再解析できない %v", err)
		}
		if e.String() != again.String() {
			t.Fatal("再解析で式が変わった")
		}
		_ = again.Refs()
		_ = again.Names()
	})
}
