package play

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"
)

// FuzzReadKeys: whatever read.json holds, reading it never panics; an answer is either keys or an
// error, never both; and a record this program writes is read back key for key.
func FuzzReadKeys(f *testing.F) {
	for _, seed := range []string{
		"", "[]", "[1,2,3]", "null", "{}", `{"format_version":1,"game_id":"g","keys":[1,2]}`,
		`{"format_version":2,"game_id":"g","keys":[]}`, `{"format_version":1,"game_id":"h","keys":[1]}`,
		`{"format_version":1,"game_id":"g","keys":null}`, `{"format_version":1,"game_id":"g","keys":[1],"x":1}`,
		"[1,", "\x00", `[-1]`, `[18446744073709551616]`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		keys, legacy, err := readKeys(b, "g")
		if err != nil && keys != nil {
			t.Fatalf("keys and an error from %q", b)
		}
		if err != nil {
			return
		}
		trimmed := bytes.TrimSpace(b)
		if legacy != (len(trimmed) > 0 && trimmed[0] == '[') {
			t.Fatalf("legacy flag %v for %q", legacy, b)
		}
		rec, merr := json.Marshal(readRecord{FormatVersion: readFormatVersion, GameID: "g", Keys: append([]uint64{}, keys...)})
		if merr != nil {
			t.Fatal(merr)
		}
		back, legacy2, err2 := readKeys(rec, "g")
		if err2 != nil || legacy2 || !slices.Equal(back, append([]uint64{}, keys...)) {
			t.Fatalf("round trip of %v: %v %v %v", keys, back, legacy2, err2)
		}
	})
}
