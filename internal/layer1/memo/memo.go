package memo

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"os"

	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"

	"time"
)

const DefaultDir = "notes"

const (
	Human    = "human"
	LLM      = "llm"
	Autoplay = "autoplay"
	Bot      = "bot"
)

const Bug = "bug"

type Memo struct {
	Source string `json:"source"`
	Bot    string `json:"bot,omitempty"`
	Kind   string `json:"kind"`
	Label  string `json:"label"`
	Text   string `json:"text"`

	Time    string  `json:"time"`
	Seconds float64 `json:"seconds"`
	Play    float64 `json:"play_seconds,omitempty"`
	Tick    int     `json:"tick"`
	Seed    uint64  `json:"seed,omitempty"`
	Game    string  `json:"game,omitempty"`

	Location string         `json:"location,omitempty"`
	Place    string         `json:"place,omitempty"`
	Scenes   []string       `json:"scenes,omitempty"`
	Floor    float64        `json:"floor,omitempty"`
	Pos      []int          `json:"pos,omitempty"`
	Facing   string         `json:"facing,omitempty"`
	Map      []string       `json:"map,omitempty"`
	Seen     []string       `json:"seen,omitempty"`
	Event    string         `json:"event,omitempty"`
	Who      string         `json:"who,omitempty"`
	Line     string         `json:"line,omitempty"`
	Choices  []string       `json:"choices,omitempty"`
	Song     string         `json:"song,omitempty"`
	Picture  string         `json:"picture,omitempty"`
	Vars     map[string]any `json:"vars,omitempty"`
	Party    []string       `json:"party,omitempty"`

	Data map[string]any `json:"data,omitempty"`

	Image   string   `json:"image,omitempty"`
	Save    string   `json:"save,omitempty"`
	Rand    []byte   `json:"rand,omitempty"`
	Resume  *Resume  `json:"resume,omitempty"`
	Trace   string   `json:"trace,omitempty"`
	Version string   `json:"version,omitempty"`
	Retake  []string `json:"retake,omitempty"`
}

type Resume struct {
	From     string `json:"from"`
	Event    string `json:"event,omitempty"`
	Line     int    `json:"line,omitempty"`
	Answers  []int  `json:"answers,omitempty"`
	InBattle bool   `json:"in_battle,omitempty"`
	Battle   string `json:"battle,omitempty"`
	Note     string `json:"note,omitempty"`
}

func Write(root string, m *Memo, shot image.Image, save []byte) (string, error) {
	base, err := Next(root, m)
	if err != nil {
		return "", err
	}
	return WriteAt(base, m, shot, save)
}

func Next(root string, m *Memo) (string, error) {
	if root == "" || m == nil {
		return "", fs.ErrInvalid
	}
	t := time.Now()
	if m.Time == "" {
		m.Time = t.Format(time.RFC3339)
	} else {
		p, err := time.Parse(time.RFC3339, m.Time)
		if err != nil {
			return "", err
		}
		t = p
	}
	dir := filepath.Join(root, t.Format("2006-01-02"))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	for n := 1; ; n++ {
		base := filepath.Join(dir, fmt.Sprintf("%03d", n))
		f, err := os.OpenFile(base+".json", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // root is the caller-selected note directory.
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		return base, f.Close()
	}
}

// WriteAt commits the metadata last. base must be a reservation returned by Next.
func WriteAt(base string, m *Memo, shot image.Image, save []byte) (_ string, err error) {
	if m == nil {
		return "", fs.ErrInvalid
	}
	var created []string
	defer func() {
		if err != nil {
			for _, p := range append(created, base+".json") {
				if e := os.Remove(p); e != nil && !errors.Is(e, fs.ErrNotExist) {
					err = errors.Join(err, e)
				}
			}
		}
	}()

	if shot != nil {
		f, err := os.OpenFile(base+".png", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // base is a caller-selected Next reservation.
		if err != nil {
			return "", err
		}
		created = append(created, base+".png")
		if err := errors.Join(png.Encode(f, shot), f.Close()); err != nil {
			return "", err
		}
		m.Image = filepath.Base(base) + ".png"
	}
	if save != nil {
		created = append(created, base+".save")
		if err := os.WriteFile(base+".save", save, 0o600); err != nil {
			return "", err
		}
		m.Save = filepath.Base(base) + ".save"
	}
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		return "", err
	}
	return base + ".json", os.WriteFile(base+".json", []byte(b.String()), 0o600)
}

func Read(path string) (Memo, []byte, error) {
	var m Memo
	jsonPath, savePath := path, ""
	if strings.HasSuffix(path, ".save") {
		jsonPath, savePath = strings.TrimSuffix(path, ".save")+".json", path
	}
	if b, err := os.ReadFile(jsonPath); err == nil { //nolint:gosec // The caller explicitly selects the note to read.
		if err := json.Unmarshal(b, &m); err != nil {
			return m, nil, fmt.Errorf("%s: %w", jsonPath, err)
		}
		if savePath == "" {
			if m.Save == "" {
				return m, nil, fmt.Errorf("%s: the note has no save", jsonPath)
			}
			if filepath.Base(m.Save) != m.Save || !fs.ValidPath(m.Save) || strings.ContainsAny(m.Save, "/\\") {
				return m, nil, fs.ErrInvalid
			}
			savePath = filepath.Join(filepath.Dir(jsonPath), m.Save)
		}
	} else if savePath == "" || !errors.Is(err, fs.ErrNotExist) {
		return m, nil, err
	}
	b, err := os.ReadFile(savePath) //nolint:gosec // The caller selected the save or its validated sibling basename.
	return m, b, err
}

type Entry struct {
	Memo
	ID string
}

func All(root string) ([]Entry, error) {
	files, err := filepath.Glob(filepath.Join(root, "*", "*.json"))
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, f := range files {
		b, err := os.ReadFile(f) //nolint:gosec // f is from Glob within the caller-selected note directory.
		if err != nil {
			return nil, err
		}
		if len(b) == 0 {
			continue
		}
		e := Entry{ID: filepath.Base(filepath.Dir(f)) + "/" + strings.TrimSuffix(filepath.Base(f), ".json")}
		if err := json.Unmarshal(b, &e.Memo); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if e.Source == "" {
			e.Source = Human
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Version returns embedded build metadata only when explicitly enabled.
// It never invokes an external command or inspects a working tree.
func Version(enabled bool) string {
	if !enabled {
		return ""
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	var rev string
	var dirty bool
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if rev != "" && dirty {
		rev += "+dirty"
	}
	return rev
}
