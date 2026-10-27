package rpg

import (
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/nao1215/gozen/internal/react"
)

// Mind is the reaction state of the speaker who (game.yaml reactions), restored from the
// saved memory, choosing its lines with rng and reading the play time as its clock.
// Event execution (respond) and the stream screen use it, and KeepMind saves it back.
func (s *State) Mind(who string, rng *rand.Rand) (*react.Mind, error) {
	book := s.DB.ReactionBooks[who]
	if book == nil {
		return nil, fmt.Errorf("speaker %q has no reaction book", who)
	}
	mind, err := react.New(book, who, react.Config{
		Rand:  rng,
		Clock: func() time.Duration { return time.Duration(s.Frames) * time.Second / 60 },
	})
	if err != nil {
		return nil, err
	}
	if saved := s.Reactions[who]; len(saved) > 0 {
		if err := mind.Restore(saved); err != nil {
			return nil, err
		}
	}
	return mind, nil
}

// KeepMind saves the memory of the speaker's mind with the game.
func (s *State) KeepMind(who string, mind *react.Mind) error {
	saved, err := mind.Save()
	if err != nil {
		return err
	}
	if s.Reactions == nil {
		s.Reactions = map[string][]byte{}
	}
	s.Reactions[who] = saved
	return nil
}
