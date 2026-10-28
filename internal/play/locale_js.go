//go:build js

package play

import (
	"context"
	"syscall/js"
)

// DeviceLocales are the browser's languages (navigator.languages, else navigator.language).
func DeviceLocales(context.Context) []string {
	nav := js.Global().Get("navigator")
	if !nav.Truthy() {
		return nil
	}
	var out []string
	if ls := nav.Get("languages"); ls.Truthy() {
		for i := range ls.Length() {
			out = append(out, ls.Index(i).String())
		}
	}
	if l := nav.Get("language"); len(out) == 0 && l.Type() == js.TypeString {
		out = append(out, l.String())
	}
	return out
}
