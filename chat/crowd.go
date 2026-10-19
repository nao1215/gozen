package chat

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/layer1/decodeguard"
)

// Script is what a Crowd says, as Load and Parse read it.
type Script struct {
	// Names are the bot senders.
	Names []string `yaml:"names"`
	// Rate is the average number of messages per second.
	Rate float64 `yaml:"rate"`
	// Lines are the sets of lines by mood ("idle", "hype"); Start names the first.
	Lines map[string][]string `yaml:"lines"`
	Start string              `yaml:"start"`
}

// Load reads and validates a crowd script from fsys.
func Load(fsys fs.FS, path string) (*Script, error) {
	b, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, err
	}
	s, err := Parse(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

// Parse reads and validates a crowd script from YAML.
func Parse(b []byte) (*Script, error) {
	var s Script
	if err := decodeguard.Run(func() error { return yaml.UnmarshalWithOptions(b, &s, yaml.Strict()) }); err != nil {
		return nil, err
	}
	return &s, s.Validate()
}

// Validate checks that there are names, a positive rate and a start mood with lines.
func (s *Script) Validate() error {
	if len(s.Names) == 0 || slices.Contains(s.Names, "") {
		return errors.New("names: none or empty")
	}
	if !(s.Rate > 0 && s.Rate <= 100) { // also rejects NaN
		return errors.New("rate: must be in (0, 100]")
	}
	if len(s.Lines[s.Start]) == 0 {
		return fmt.Errorf("start: mood %q has no lines", s.Start)
	}
	return nil
}

// CrowdConfig sets up a Crowd. Nil Rand uses a fixed seed.
type CrowdConfig struct {
	Rand *rand.Rand
}

// maxPerPoll caps the messages one Poll returns after a long pause.
const maxPerPoll = 20

// Crowd is a Source of bot messages: about Rate per second from the current mood's
// lines, plus bursts. It is not safe for concurrent use.
type Crowd struct {
	s     *Script
	rng   *rand.Rand
	mood  string
	next  time.Duration
	burst []string
}

// NewCrowd makes a crowd from a validated script.
func NewCrowd(s *Script, cfg CrowdConfig) *Crowd {
	rng := cfg.Rand
	if rng == nil {
		rng = rand.New(rand.NewPCG(1, 2)) //nolint:gosec // G404: reproducible bot chatter
	}
	c := &Crowd{s: s, rng: rng, mood: s.Start, next: -1}
	return c
}

// Mood switches the lines the crowd uses.
func (c *Crowd) Mood(mood string) error {
	if len(c.s.Lines[mood]) == 0 {
		return fmt.Errorf("chat: mood %q has no lines", mood)
	}
	c.mood = mood
	return nil
}

// Burst queues n lines of the mood for the next Poll (a reaction to an event).
func (c *Crowd) Burst(mood string, n int) error {
	lines := c.s.Lines[mood]
	if len(lines) == 0 {
		return fmt.Errorf("chat: mood %q has no lines", mood)
	}
	for range n {
		c.burst = append(c.burst, lines[c.rng.IntN(len(lines))])
	}
	return nil
}

// Poll implements Source.
func (c *Crowd) Poll(now time.Duration) []Message {
	var out []Message
	say := func(text string) {
		out = append(out, Message{From: c.s.Names[c.rng.IntN(len(c.s.Names))], Text: text, At: now})
	}
	for _, t := range c.burst {
		if len(out) < maxPerPoll {
			say(t)
		}
	}
	c.burst = c.burst[:0]
	if c.next < 0 {
		c.next = now + c.gap()
	}
	for c.next <= now {
		if len(out) >= maxPerPoll {
			c.next = now + c.gap()
			break
		}
		lines := c.s.Lines[c.mood]
		say(lines[c.rng.IntN(len(lines))])
		c.next += c.gap()
	}
	return out
}

// gap is an exponential wait with mean 1/Rate seconds.
func (c *Crowd) gap() time.Duration {
	return time.Duration(-math.Log(1-c.rng.Float64()) / c.s.Rate * float64(time.Second))
}
