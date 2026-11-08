package text

import (
	"fmt"
	"maps"
	"sync"

	"github.com/nao1215/gozen/internal/text/catalog"
)

// The built-in languages, the values of game.yaml's language. The empty language is
// English.
const (
	English  = catalog.English
	Japanese = catalog.Japanese
)

var (
	mu     sync.RWMutex
	active *Context // nil before the first Use: every key is its own word
)

// Use makes T answer with the built-in words of language ("" is English) and the game's
// words on top (words may be nil). It copies words, so the caller may change its map
// afterwards. An unknown language is an error and changes nothing.
//
// Use selects the host default; nested presentation uses Context.Activate.
func Use(language string, words map[string]string) error {
	c, err := NewContext(language, words)
	if err != nil {
		return err
	}
	mu.Lock()
	active = c
	mu.Unlock()
	return nil
}

// Context owns immutable translated engine words for one game's presentation.
type Context struct{ words map[string]string }

// NewContext copies the selected built-in and authored words. Unknown languages
// return an error without changing the currently active game.
func NewContext(language string, words map[string]string) (*Context, error) {
	m, err := catalog.Words(language)
	if err != nil {
		return nil, fmt.Errorf("text: %w", err)
	}
	maps.Copy(m, words)
	return &Context{words: m}, nil
}

// Activate selects this game's words until restore is called. Presentation uses
// nested activations on the game thread; workers must not depend on active words.
func (c *Context) Activate() func() {
	mu.Lock()
	previous := active
	active = c
	mu.Unlock()
	return func() { mu.Lock(); active = previous; mu.Unlock() }
}

// T is the word for the engine's key s in the language and words chosen by Use (s itself
// when there is none).
func T(s string) string {
	mu.RLock()
	defer mu.RUnlock()
	if active == nil {
		return s
	}
	if w, ok := active.words[s]; ok {
		return w
	}
	return s
}

// Use replaces this game's words after an in-game language change or live reload.
// Existing activation scopes continue to select this context, not another game.
func (c *Context) Use(language string, words map[string]string) error {
	next, err := NewContext(language, words)
	if err != nil {
		return err
	}
	mu.Lock()
	c.words = next.words
	mu.Unlock()
	return nil
}
