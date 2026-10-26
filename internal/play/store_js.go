//go:build js

package play

import (
	"errors"
	"fmt"
	"syscall/js"
	"time"
)

// defaultStore is the browser page's localStorage (keys are gozen/<ID>/<name>).
func defaultStore(id string) (Store, error) {
	return kvStore{kv: localKV{}, prefix: "gozen/" + id + "/", now: time.Now}, nil
}

// localKV is localStorage. In browsers where it is unavailable (private windows, quota
// exhausted) the exception becomes a panic, so call turns it into an error.
type localKV struct{}

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

func (localKV) get(key string) (string, bool, error) {
	v, err := call("getItem", key)
	if err != nil {
		return "", false, err
	}
	if v.Type() != js.TypeString {
		return "", false, nil
	}
	return v.String(), true, nil
}

func (localKV) set(key, value string) error {
	_, err := call("setItem", key, value)
	return err
}

func (localKV) remove(key string) error {
	_, err := call("removeItem", key)
	return err
}

func (localKV) keys() ([]string, error) {
	n, err := call("valueOf")
	if err != nil {
		return nil, err
	}
	length := n.Get("length").Int()
	out := make([]string, 0, length)
	for i := range length {
		k, err := call("key", i)
		if err != nil {
			return nil, err
		}
		out = append(out, k.String())
	}
	return out, nil
}
