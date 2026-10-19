package main

import (
	"fmt"
	"strings"
)

func appendf(b *strings.Builder, format string, args ...any) {
	_, _ = fmt.Fprintf(b, format, args...) //nolint:errcheck // strings.Builder never returns a write error.
}
func appendText(b *strings.Builder, s string) {
	_, _ = b.WriteString(s) //nolint:errcheck // strings.Builder never returns a write error.
}
