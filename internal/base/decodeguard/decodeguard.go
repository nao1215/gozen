// Package decodeguard turns a panic inside a third-party decoder into an error.
//
// goccy/go-yaml v1.19.2 dereferences nil when a tagged scalar is decoded into a
// slice (for example "names: !x 0"). Game data, books and scripts are untrusted
// input (SECURITY.md), so every YAML decode in Gozen runs through Run and reports
// such input as an error instead of crashing the host. The fuzz tests of chat and
// react keep inputs of that shape in their corpus.
package decodeguard

import "fmt"

// Run calls decode and returns its error, or an error describing the panic when
// decode panics.
func Run(decode func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("yaml: the decoder rejected the input: %v", r)
		}
	}()
	return decode()
}
