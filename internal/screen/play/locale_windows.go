//go:build windows

package play

import (
	"context"
	"os"
	"syscall"
	"unsafe"
)

// DeviceLocales are the languages of the locale variables (LC_ALL, LC_MESSAGES, LANG),
// then the user's locale (GetUserDefaultLocaleName).
func DeviceLocales(context.Context) []string {
	out := envLocales(os.Getenv)
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultLocaleName")
	if proc.Find() != nil {
		return out
	}
	buf := make([]uint16, 85) // LOCALE_NAME_MAX_LENGTH
	// the buffer is passed as Windows takes it; the third value is GetLastError, which a
	// length above 0 leaves unread
	if n, _, _ := proc.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); n > 1 { //nolint:gosec,errcheck // see above
		out = append(out, syscall.UTF16ToString(buf[:n-1]))
	}
	return out
}
