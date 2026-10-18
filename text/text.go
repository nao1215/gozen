// Package text holds the words of the engine's screens (the menus, the battle's messages,
// the hints), so a game can change them: game.yaml's text maps the engine's own words (the
// key is the Japanese text as the engine writes it, a format with its %d and %s too) to the
// game's.
//
//	text:
//	  "にげられない！": "Can't run away!"
//	  "%sを %dこ かった！": "Bought %[2]d %[1]s."
package text

import (
	"maps"
	"sync"
)

var (
	mu    sync.RWMutex
	words map[string]string
)

// Use sets the game's words (nil: the engine's own).
func Use(m map[string]string) {
	mu.Lock()
	words = maps.Clone(m)
	mu.Unlock()
}

// T is the game's word for the engine's s (s itself when the game has none).
func T(s string) string {
	mu.RLock()
	defer mu.RUnlock()
	if w, ok := words[s]; ok {
		return w
	}
	return s
}
