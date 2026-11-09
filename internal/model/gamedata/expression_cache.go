package gamedata

import (
	"strings"
	"sync"

	"github.com/nao1215/gozen/internal/base/expr"
)

// Bound both entry count and source size. Long expressions remain valid but
// bypass the cache; parsed trees are proportional to their source length.
const (
	expressionCacheLimit       = 1024
	expressionCacheSourceLimit = 4096
)

// expressionCache owns parsed expressions, never their caller's backing text.
// FIFO eviction keeps hot reads read-only and does not change evaluation.
type expressionCache struct {
	mu      sync.RWMutex
	entries map[string]*expr.Expr
	keys    [expressionCacheLimit]string
	next    int
}

var textCache expressionCache

func parseExpr(src string) (Expr, error) {
	return textCache.parse(src)
}

func (c *expressionCache) parse(src string) (Expr, error) {
	if len(src) > expressionCacheSourceLimit {
		e, err := expr.Parse(src)
		return Expr{e}, err
	}
	c.mu.RLock()
	e := c.entries[src]
	c.mu.RUnlock()
	if e != nil {
		return Expr{e}, nil
	}
	// Interpolations are substrings of potentially very large dialogue lines.
	// The AST also keeps substrings, so clone before parsing, not just the key.
	src = strings.Clone(src)
	e, err := expr.Parse(src)
	if err != nil {
		return Expr{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if previous := c.entries[src]; previous != nil {
		return Expr{previous}, nil
	}
	if c.entries == nil {
		c.entries = make(map[string]*expr.Expr)
	}
	delete(c.entries, c.keys[c.next])
	c.entries[src] = e
	c.keys[c.next] = src
	c.next = (c.next + 1) % expressionCacheLimit
	return Expr{e}, nil
}
