package react

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/expr"
	"github.com/nao1215/gozen/internal/layer1/decodeguard"
)

// Built-in tags that the analyzers attach besides the lexicon tags.
const (
	// TagRepeat is attached when the sender says the same thing again.
	TagRepeat = "repeat"
	// TagRecall is attached when the text mentions something the person remembers.
	TagRecall = "recall"
	// TagEmpty is attached to a "say" stimulus with no text.
	TagEmpty = "empty"
	// AcrosticPrefix prefixes a lexicon tag found in the first characters of the lines
	// (acrostic_cat for the tag cat).
	AcrosticPrefix = "acrostic_"
)

// KindSay is the stimulus kind of spoken or typed words.
const KindSay = "say"

// Book is a lexicon shared by the people and the people themselves, as Load and Parse
// read them. A Book is read-only after loading and may be shared by many Minds.
type Book struct {
	// Lexicon maps a tag to the words that suggest it.
	Lexicon map[string][]Word `yaml:"lexicon"`
	// Tags declares the extra tags that games attach (event tags, custom analyzers),
	// so rules may refer to them.
	Tags []string `yaml:"tags"`
	// People are the characters.
	People []Person `yaml:"people"`
}

// Word is a lexicon entry. In YAML it is a plain string (weight 1) or a one-entry
// map {word: weight}. The weight in 0..1 is the confidence the word gives its tag.
type Word struct {
	Text   string
	Weight float64
}

// UnmarshalYAML reads "word" or {word: weight}.
func (w *Word) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err == nil {
		*w = Word{Text: s, Weight: 1}
		return nil
	}
	var m map[string]float64
	if err := unmarshal(&m); err != nil || len(m) != 1 {
		return errors.New("a lexicon word is a string or {word: weight}")
	}
	for k, v := range m {
		*w = Word{Text: k, Weight: v}
	}
	return nil
}

// Person is one character.
type Person struct {
	ID string `yaml:"id"`
	// Mood is the starting mood, clamped to MoodMin..MoodMax (0..100 when both are 0).
	Mood    float64 `yaml:"mood"`
	MoodMin float64 `yaml:"mood_min"`
	MoodMax float64 `yaml:"mood_max"`
	// Faces, when given, lists the faces rules may use.
	Faces []string `yaml:"faces"`
	// Likes adds weight × confidence to the mood for each tag found.
	Likes map[string]float64 `yaml:"likes"`
	// Rules are tried in order; the first whose condition holds reacts.
	Rules []Rule `yaml:"rules"`
	// Asks are questions the person can put, which narrow the next answer.
	Asks []Ask `yaml:"asks"`
	// Dodge holds the in-character lines used when no rule holds.
	Dodge Dodge `yaml:"dodge"`
}

// Rule is a reaction with a condition.
type Rule struct {
	ID string `yaml:"id"`
	// Kind limits the rule to stimuli of this kind; empty matches any kind.
	Kind string `yaml:"kind"`
	// When is the condition (an expr expression); empty always holds.
	When *expr.Expr `yaml:"when"`
	// Say is a set of lines; one is chosen, avoiding the previous line when possible.
	// Lines may use {from}, {mood}, {slot.TAG} and {mem.KEY}.
	Say  []string `yaml:"say"`
	Face string   `yaml:"face"`
	// Delta is added to the mood on top of the likes.
	Delta float64 `yaml:"delta"`
	// Remember and Forget change the memory.
	Remember []Remember `yaml:"remember"`
	Forget   []string   `yaml:"forget"`
	// Ask puts the question with this ID after the line.
	Ask string `yaml:"ask"`
	// Cooldown is how many stimuli must pass before the rule may react again.
	Cooldown int `yaml:"cooldown"`
}

// Remember writes one memory entry: Set (a template) or Add (a number added to the
// current value). Turns keeps it through that many following stimuli; Time keeps it for
// that many seconds of the game clock (Config.Clock). 0 keeps it until overwritten.
type Remember struct {
	Key   string  `yaml:"key"`
	Set   string  `yaml:"set"`
	Add   float64 `yaml:"add"`
	Turns int     `yaml:"turns"`
	Time  float64 `yaml:"time"`
}

// Ask is a question. While it is pending, the expected tags found in the answer get
// full confidence and the answer is available to rules as `answer`.
type Ask struct {
	ID     string   `yaml:"id"`
	Say    []string `yaml:"say"`
	Face   string   `yaml:"face"`
	Expect []string `yaml:"expect"`
	// Turns is how many stimuli the question stays pending (1 when 0).
	Turns int `yaml:"turns"`
}

// Dodge lines are used when no rule holds: Repeat when the sender repeats themselves,
// Unsure when some tag was found, Unknown when nothing was.
type Dodge struct {
	Repeat  []string `yaml:"repeat"`
	Unsure  []string `yaml:"unsure"`
	Unknown []string `yaml:"unknown"`
	Face    string   `yaml:"face"`
}

// Load reads and validates a book from fsys.
func Load(fsys fs.FS, path string) (*Book, error) {
	b, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, err
	}
	bk, err := Parse(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return bk, nil
}

// Parse reads and validates a book from YAML.
func Parse(b []byte) (*Book, error) {
	var bk Book
	if err := decodeguard.Run(func() error { return yaml.UnmarshalWithOptions(b, &bk, yaml.Strict()) }); err != nil {
		return nil, err
	}
	if err := bk.Validate(); err != nil {
		return nil, err
	}
	return &bk, nil
}

// Validate checks every reference: tags in conditions, likes and asks, faces, ask IDs,
// memory keys and template fields. It does not modify the book.
func (b *Book) Validate() error {
	for tag, words := range b.Lexicon {
		if tag == "" || strings.HasPrefix(tag, AcrosticPrefix) || builtin(tag) {
			return fmt.Errorf("lexicon: tag %q is empty or reserved", tag)
		}
		for _, w := range words {
			if Normalize(w.Text) == "" || w.Weight <= 0 || w.Weight > 1 {
				return fmt.Errorf("lexicon.%s: word %q needs text and a weight in (0, 1]", tag, w.Text)
			}
		}
	}
	if len(b.People) == 0 {
		return errors.New("people: none")
	}
	seen := map[string]bool{}
	for i := range b.People {
		p := &b.People[i]
		if p.ID == "" || seen[p.ID] {
			return fmt.Errorf("people[%d]: id %q is empty or repeated", i, p.ID)
		}
		seen[p.ID] = true
		if err := b.checkPerson(p); err != nil {
			return fmt.Errorf("people.%s: %w", p.ID, err)
		}
	}
	return nil
}

func builtin(tag string) bool {
	return tag == TagRepeat || tag == TagRecall || tag == TagEmpty
}

// knownTag reports whether rules may refer to the tag.
func (b *Book) knownTag(tag string) bool {
	if builtin(tag) || slices.Contains(b.Tags, tag) {
		return true
	}
	_, ok := b.Lexicon[strings.TrimPrefix(tag, AcrosticPrefix)]
	return ok
}

func (b *Book) checkPerson(p *Person) error {
	if lo, hi := p.bounds(); lo >= hi {
		return errors.New("mood_min must be below mood_max")
	}
	face := func(f string) error {
		if f != "" && len(p.Faces) > 0 && !slices.Contains(p.Faces, f) {
			return fmt.Errorf("face %q is not in faces", f)
		}
		return nil
	}
	for tag := range p.Likes {
		if !b.knownTag(tag) {
			return fmt.Errorf("likes: unknown tag %q", tag)
		}
	}
	asks := map[string]bool{}
	for _, a := range p.Asks {
		if a.ID == "" || asks[a.ID] || len(a.Say) == 0 {
			return fmt.Errorf("asks: %q needs a unique id and say", a.ID)
		}
		asks[a.ID] = true
		for _, t := range a.Expect {
			if !b.knownTag(t) {
				return fmt.Errorf("asks.%s: unknown tag %q", a.ID, t)
			}
		}
		if err := face(a.Face); err != nil {
			return fmt.Errorf("asks.%s: %w", a.ID, err)
		}
		if err := checkLines(a.Say); err != nil {
			return fmt.Errorf("asks.%s: %w", a.ID, err)
		}
	}
	ids := map[string]bool{}
	for _, r := range p.Rules {
		if r.ID == "" || ids[r.ID] {
			return fmt.Errorf("rules: id %q is empty or repeated", r.ID)
		}
		ids[r.ID] = true
		if err := b.checkRule(r, asks); err != nil {
			return fmt.Errorf("rules.%s: %w", r.ID, err)
		}
		if err := face(r.Face); err != nil {
			return fmt.Errorf("rules.%s: %w", r.ID, err)
		}
	}
	if err := face(p.Dodge.Face); err != nil {
		return fmt.Errorf("dodge: %w", err)
	}
	for _, l := range [][]string{p.Dodge.Repeat, p.Dodge.Unsure, p.Dodge.Unknown} {
		if err := checkLines(l); err != nil {
			return fmt.Errorf("dodge: %w", err)
		}
	}
	return nil
}

func (b *Book) checkRule(r Rule, asks map[string]bool) error {
	if len(r.Say) == 0 && r.Delta == 0 && len(r.Remember) == 0 && len(r.Forget) == 0 && r.Ask == "" {
		return errors.New("does nothing (no say, delta, remember, forget or ask)")
	}
	if r.Ask != "" && !asks[r.Ask] {
		return fmt.Errorf("unknown ask %q", r.Ask)
	}
	if r.Cooldown < 0 {
		return errors.New("cooldown is negative")
	}
	for _, ref := range r.When.Refs() {
		if err := b.checkRef(ref); err != nil {
			return err
		}
	}
	for _, m := range r.Remember {
		if m.Key == "" || (m.Set != "" && m.Add != 0) || m.Turns < 0 || m.Time < 0 {
			return fmt.Errorf("remember %q: needs a key, set or add (not both), and no negative limits", m.Key)
		}
		if err := checkLines([]string{m.Set}); err != nil {
			return fmt.Errorf("remember %q: %w", m.Key, err)
		}
	}
	return checkLines(r.Say)
}

// envNames are the names a condition may use, with whether they take a field.
var envNames = map[string]bool{
	"tag": true, "slot": true, "mem": true, "value": true,
	"mood": false, "turn": false, "kind": false, "from": false, "length": false,
	"asked": false, "answer": false, "repeats": false, "confidence": false,
}

func (b *Book) checkRef(ref expr.Ref) error {
	field, ok := envNames[ref.Name]
	if !ok {
		return fmt.Errorf("unknown name %q", ref.Name)
	}
	if !field {
		if len(ref.Path) > 0 {
			return fmt.Errorf("%s has no fields", ref.Name)
		}
		return nil
	}
	if len(ref.Path) != 1 || ref.Path[0].Name == "" {
		return fmt.Errorf("%s needs one field (%s.NAME)", ref.Name, ref.Name)
	}
	if (ref.Name == "tag" || ref.Name == "slot") && !b.knownTag(ref.Path[0].Name) {
		return fmt.Errorf("unknown tag %q", ref.Path[0].Name)
	}
	return nil
}

// checkLines checks the {name} fields of templates.
func checkLines(lines []string) error {
	for _, l := range lines {
		for rest := l; ; {
			i := strings.IndexByte(rest, '{')
			if i < 0 {
				break
			}
			j := strings.IndexByte(rest[i:], '}')
			if j < 0 {
				return fmt.Errorf("line %q: unclosed {", l)
			}
			f := rest[i+1 : i+j]
			if f != "from" && f != "mood" && !strings.HasPrefix(f, "slot.") && !strings.HasPrefix(f, "mem.") {
				return fmt.Errorf("line %q: unknown field {%s}", l, f)
			}
			rest = rest[i+j+1:]
		}
	}
	return nil
}

// bounds is the mood range (0..100 when both limits are 0).
func (p *Person) bounds() (float64, float64) {
	if p.MoodMin == 0 && p.MoodMax == 0 {
		return 0, 100
	}
	return p.MoodMin, p.MoodMax
}

func (b *Book) person(id string) (*Person, bool) {
	for i := range b.People {
		if b.People[i].ID == id {
			return &b.People[i], true
		}
	}
	return nil, false
}
