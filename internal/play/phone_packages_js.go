//go:build js

package play

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"syscall/js"

	"github.com/nao1215/gozen/internal/layer2/gamepackage"
)

func defaultPhoneArchiveStore(g *Game) (gamepackage.Store, error) {
	return phoneArchiveStore{g.store}, nil
}

// Browser downloads use fetch rather than pulling net/http into every WASM
// player. Direct HTTPS endpoints must permit this page's origin through CORS.
type phoneFetcher struct{}

func (phoneFetcher) Fetch(ctx context.Context, address string, limit int64) (out []byte, err error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || limit <= 0 || limit > gamepackage.MaxArchiveBytes {
		return nil, errors.New("invalid browser package download")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	defer func() {
		if p := recover(); p != nil {
			out = nil
			err = fmt.Errorf("browser download: %v", p)
		}
	}()
	controller := js.Global().Get("AbortController").New()
	finished := make(chan struct{})
	defer close(finished)
	defer controller.Call("abort")
	go func() {
		select {
		case <-ctx.Done():
			controller.Call("abort")
		case <-finished:
		}
	}()
	response, err := phonePromise(js.Global().Call("fetch", address, map[string]any{"signal": controller.Get("signal"), "credentials": "omit", "redirect": "error"}))
	if err != nil {
		return nil, err
	}
	if !response.Get("ok").Bool() {
		return nil, fmt.Errorf("download returned HTTP %d", response.Get("status").Int())
	}
	body := response.Get("body")
	if body.IsNull() || body.IsUndefined() {
		return nil, errors.New("browser streaming downloads unavailable")
	}
	reader := body.Call("getReader")
	for {
		chunk, err := phonePromise(reader.Call("read"))
		if err != nil {
			return nil, err
		}
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if chunk.Get("done").Bool() {
			return out, nil
		}
		value := chunk.Get("value")
		n := value.Get("byteLength").Int()
		if int64(n) > limit-int64(len(out)) {
			return nil, errors.New("download exceeds limit")
		}
		start := len(out)
		out = append(out, make([]byte, n)...)
		js.CopyBytesToGo(out[start:], value)
	}
}

func phonePromise(p js.Value) (js.Value, error) {
	type result struct {
		value js.Value
		err   error
	}
	done := make(chan result, 1)
	resolve := js.FuncOf(func(_ js.Value, args []js.Value) any { done <- result{value: args[0]}; return nil })
	reject := js.FuncOf(func(_ js.Value, args []js.Value) any {
		done <- result{err: errors.New(args[0].Call("toString").String())}
		return nil
	})
	defer resolve.Release()
	defer reject.Release()
	p.Call("then", resolve, reject)
	r := <-done
	return r.value, r.err
}

const phoneBrowser = true

func openPhoneArchive(ctx context.Context, fsys fs.FS, path string, r gamepackage.Release) (*gamepackage.Installed, error) {
	if r.Size > gamepackage.MaxArchiveBytes || r.UnpackedSize > gamepackage.MaxUnpackedBytes {
		return nil, errors.New("package exceeds browser limits")
	}
	f, err := fsys.Open(path)
	if err != nil {
		return nil, err
	}
	b, err := io.ReadAll(io.LimitReader(f, r.Size+1))
	err = errors.Join(err, f.Close())
	if err != nil {
		return nil, err
	}
	return gamepackage.VerifyContext(ctx, r, b)
}
