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
	active map[string]string // nil before the first Use: every key is its own word
)

// Use makes T answer with the built-in words of language ("" is English) and the game's
// words on top (words may be nil). It copies words, so the caller may change its map
// afterwards. An unknown language is an error and changes nothing.
//
// The words are shared by the whole process: call Use once at the host boundary (the
// runner does it from game.yaml), not from games running concurrently. Before the first
// Use, T returns each key itself (the Japanese source).
func Use(language string, words map[string]string) error {
	m, err := catalog.Words(language)
	if err != nil {
		return fmt.Errorf("text: %w", err)
	}
	maps.Copy(m, words)
	mu.Lock()
	active = m
	mu.Unlock()
	return nil
}

// T is the word for the engine's key s in the language and words chosen by Use (s itself
// when there is none).
func T(s string) string {
	mu.RLock()
	defer mu.RUnlock()
	if w, ok := active[s]; ok {
		return w
	}
	return s
}
