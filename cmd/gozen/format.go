package main

import (
	"fmt"
	"strings"
)

func appendf(b *strings.Builder, format string, args ...any) {
	fmt.Fprintf(b, format, args...)
}

func appendText(b *strings.Builder, s string) {
	b.WriteString(s)
}
