package play

import (
	"os"
	"testing"
)

// sysAs asserts the dynamic type of v, failing the test when it is another.
func sysAs[T any](t *testing.T, v any) T {
	t.Helper()
	x, ok := v.(T)
	if !ok {
		var zero T
		t.Fatalf("got %T, want %T", v, zero)
	}
	return x
}

// sysScript parses a script that is meant to be valid.
func sysScript(t *testing.T, src string) *script {
	t.Helper()
	s, err := parseScript(src)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// sysClose closes a game and fails the test when closing reports an error.
func sysClose(t *testing.T, g *Game) {
	t.Helper()
	if err := g.Close(); err != nil {
		t.Error(err)
	}
}

// sysRead reads a file the test expects to exist.
func sysRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // reads a file this test wrote
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// sysReadDir lists a directory the test expects to exist.
func sysReadDir(t *testing.T, dir string) []os.DirEntry {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

// sysStat stats a file the test expects to exist.
func sysStat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

// sysReadInput runs readInput and fails the test on its error.
func sysReadInput(t *testing.T, m *memoBox, update func() (bool, error), chars func([]rune) []rune) bool {
	t.Helper()
	handled, err := m.readInput(update, chars)
	if err != nil {
		t.Fatal(err)
	}
	return handled
}
