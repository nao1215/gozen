//go:build js

package play

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"syscall/js"
	"time"
)

// defaultStore is the browser page's localStorage (keys are gozen/<ID>/<name>).
func defaultStore(id string) (Store, error) { return localStore{prefix: "gozen/" + id + "/"}, nil }

// localStore is the localStorage save. In browsers where it is unavailable (private windows, quota exhausted)
// the exception becomes a panic, so call turns it into an error. The value is the saved JSON string.
type localStore struct{ prefix string }

var errNoStorage = errors.New("this browser keeps no localStorage for the page")

func call(method string, args ...any) (v js.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("localStorage.%s: %v", method, r)
		}
	}()
	ls := js.Global().Get("Reflect").Call("get", js.Global(), "localStorage")
	if ls.IsUndefined() || ls.IsNull() {
		return js.Undefined(), errNoStorage
	}
	return ls.Call(method, args...), nil
}

func (s localStore) Read(name string) ([]byte, error) {
	v, err := call("getItem", s.prefix+name)
	if err != nil {
		return nil, err
	}
	if v.Type() != js.TypeString {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrNotExist}
	}
	return []byte(v.String()), nil
}

func (s localStore) Write(name string, b []byte) error {
	if _, err := call("setItem", s.prefix+name, string(b)); err != nil {
		return err
	}
	_, err := call("setItem", s.prefix+name+"#time", time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s localStore) List() ([]string, error) {
	n, err := call("valueOf")
	if err != nil {
		return nil, err
	}
	length := n.Get("length").Int()
	var out []string
	for i := range length {
		k, err := call("key", i)
		if err != nil {
			return nil, err
		}
		if name, ok := strings.CutPrefix(k.String(), s.prefix); ok && !strings.HasSuffix(name, "#time") {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (s localStore) Remove(name string) error {
	if _, err := call("removeItem", s.prefix+name); err != nil {
		return err
	}
	_, err := call("removeItem", s.prefix+name+"#time")
	return err
}

// ModTime is the time of writing (Write leaves it under another key).
func (s localStore) ModTime(name string) (time.Time, error) {
	v, err := call("getItem", s.prefix+name+"#time")
	if err != nil {
		return time.Time{}, err
	}
	if v.Type() != js.TypeString {
		return time.Time{}, fs.ErrNotExist
	}
	return time.Parse(time.RFC3339, v.String())
}
