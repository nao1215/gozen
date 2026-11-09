package gamedata

import (
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestExpressionCacheEviction(t *testing.T) {
	var cache expressionCache
	first, err := cache.parse("0")
	if err != nil {
		t.Fatal(err)
	}
	again, err := cache.parse("0")
	if err != nil || first.Expr != again.Expr {
		t.Fatal("cache did not reuse a parsed expression")
	}
	for i := 1; i <= expressionCacheLimit; i++ {
		e, err := cache.parse(strconv.Itoa(i))
		if err != nil {
			t.Fatal(err)
		}
		if got, err := e.Number(nil); err != nil || got != float64(i) {
			t.Fatalf("evaluation = %v, %v", got, err)
		}
	}
	if len(cache.entries) != expressionCacheLimit || cache.entries["0"] != nil {
		t.Fatal("oldest expression was not evicted at the limit")
	}
	// Eviction releases cache ownership without invalidating active consumers.
	if got, err := first.Number(nil); err != nil || got != 0 {
		t.Fatalf("evicted expression = %v, %v", got, err)
	}
	for _, src := range []string{"1 +", strings.Repeat(" ", expressionCacheSourceLimit) + "1"} {
		_, err := cache.parse(src)
		if src == "1 +" && err == nil {
			t.Fatal("invalid expression accepted")
		}
		if cache.entries[src] != nil || len(cache.entries) != expressionCacheLimit {
			t.Fatal("invalid or oversized expression was retained")
		}
	}
}

func TestExpressionCacheConcurrent(t *testing.T) {
	var cache expressionCache
	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Go(func() {
			for i := range 512 {
				n := worker*512 + i
				e, err := cache.parse(strconv.Itoa(n))
				if err != nil {
					t.Error(err)
					return
				}
				if got, err := e.Number(nil); err != nil || got != float64(n) {
					t.Errorf("evaluation = %v, %v", got, err)
				}
			}
		})
	}
	wg.Wait()
	if len(cache.entries) != expressionCacheLimit {
		t.Fatalf("cache size = %d", len(cache.entries))
	}
}
