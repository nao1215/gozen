package assetfs

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
)

// Approval is an approval bound to the hash of the source file. Required means a replacement is
// mandatory.
// The meaning and classification of approval are decided on the production side; the engine is
// given only the checked result.
type Approval struct {
	File     string `json:"file"`
	Source   string `json:"source"`
	Approved bool   `json:"approved"`
	Required bool   `json:"required"`
	Status   string `json:"status,omitempty"`
}

func field(m map[string]any, key string) any {
	var v any = m
	if key == "" {
		return nil
	}
	for k := range strings.SplitSeq(key, ".") {
		x, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = x[k]
	}
	return v
}

// Blocking returns the unapproved records.
func Blocking(ins []Approval) []Approval {
	var out []Approval
	for _, a := range ins {
		if !a.Approved {
			out = append(out, a)
		}
	}
	return out
}

// Gate is the approval check shared by distribution and capture. Under Watch, a source image with
// no record is not accepted.
type Gate struct {
	Base    fs.FS
	Top     fs.FS
	Prefix  string
	Records []Approval
	Outputs []string
	Watch   []string
}

// Check matches the current input, the required replacements and the output list.
func (g Gate) Check(file string) error {
	file = strings.TrimPrefix(file, g.Prefix+"/")
	if !fs.ValidPath(file) {
		return fmt.Errorf("assetfs: invalid path %q", file)
	}
	var record *Approval
	for i := range g.Records {
		if g.Records[i].File == file {
			record = &g.Records[i]
			break
		}
	}
	var replacement []byte
	has := false
	if g.Top != nil {
		var err error
		replacement, err = fs.ReadFile(g.Top, file)
		has = err == nil
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if record == nil {
		watched := has
		for _, w := range g.Watch {
			watched = watched || strings.HasPrefix(file, w)
		}
		if watched {
			return fmt.Errorf("assetfs: no approval for %s", file)
		}
		return nil
	}
	if !record.Approved {
		return fmt.Errorf("assetfs: unapproved %s (%s)", file, record.Status)
	}
	orig, err := fs.ReadFile(g.Base, path.Join(g.Prefix, file))
	if err != nil {
		return err
	}
	h := sha256.Sum256(orig)
	if !strings.EqualFold(record.Source, hex.EncodeToString(h[:])) {
		return fmt.Errorf("assetfs: source changed or unchecked: %s", file)
	}
	if has && g.Outputs != nil && !slices.Contains(g.Outputs, file) {
		return fmt.Errorf("assetfs: obsolete output %s", file)
	}
	if record.Required && (!has || sha256.Sum256(replacement) == h) {
		return fmt.Errorf("assetfs: required replacement missing or unchanged: %s", file)
	}
	return nil
}
