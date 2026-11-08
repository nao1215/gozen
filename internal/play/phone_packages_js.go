//go:build js

package play

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"syscall/js"

	"github.com/nao1215/gozen/internal/layer2/gamepackage"
)

func defaultPhoneArchiveStore(g *Game) (gamepackage.Store, error) {
	if g.storeGiven || g.opts.Store != nil || g.opts.StoreFor != nil {
		return phoneArchiveStore{g.store}, nil
	}
	return phoneIndexedArchiveStore{namespace: fmt.Sprintf("%x", sha256.Sum256([]byte(g.db.Game.ID))), legacy: phoneArchiveStore{g.store}}, nil
}

// Large immutable archives belong in a binary cache, separate from small game saves.
// A successful transaction is required before the installer publishes its selection.
type phoneIndexedArchiveStore struct {
	namespace string
	legacy    phoneArchiveStore
}

func phoneCacheDigest(digest string) bool {
	b, err := hex.DecodeString(digest)
	return err == nil && len(b) == sha256.Size && hex.EncodeToString(b) == digest
}

func (s phoneIndexedArchiveStore) Load(ctx context.Context, digest string) (out []byte, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !phoneCacheDigest(digest) {
		return nil, errors.New("invalid cache digest")
	}
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("browser package cache: %v", p)
		}
	}()
	value, cacheErr := s.transaction(ctx, digest, nil, false)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cacheErr != nil || value.IsUndefined() {
		out, err = s.legacy.Load(ctx, digest)
		if err != nil {
			if cacheErr != nil && errors.Is(err, fs.ErrNotExist) {
				return nil, cacheErr
			}
			return nil, errors.Join(cacheErr, err)
		}
	} else {
		if !value.InstanceOf(js.Global().Get("Uint8Array")) {
			return nil, errors.New("invalid cached archive type")
		}
		n := value.Get("byteLength").Int()
		if int64(n) > gamepackage.MaxArchiveBytes {
			return nil, errors.New("cached package exceeds limit")
		}
		out = make([]byte, n)
		js.CopyBytesToGo(out, value)
	}
	if fmt.Sprintf("%x", sha256.Sum256(out)) != digest {
		return nil, errors.New("cache digest mismatch")
	}
	return out, ctx.Err()
}

func (s phoneIndexedArchiveStore) Save(ctx context.Context, digest string, b []byte) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !phoneCacheDigest(digest) || fmt.Sprintf("%x", sha256.Sum256(b)) != digest {
		return errors.New("cache digest mismatch")
	}
	if int64(len(b)) > gamepackage.MaxArchiveBytes {
		return errors.New("package exceeds cache limit")
	}
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("browser package cache: %v", p)
		}
	}()
	_, err = s.transaction(ctx, digest, b, true)
	return err
}

type phoneDatabaseResult struct {
	value js.Value
	err   error
}

func phoneOpenArchiveDatabase(ctx context.Context) (js.Value, error) {
	if err := ctx.Err(); err != nil {
		return js.Undefined(), err
	}
	factory := js.Global().Get("indexedDB")
	if factory.IsUndefined() || factory.IsNull() {
		return js.Undefined(), errors.New("IndexedDB package cache unavailable")
	}
	request := factory.Call("open", "gozen-phone-packages", 1)
	done := make(chan phoneDatabaseResult, 1)
	upgrade := js.FuncOf(func(_ js.Value, _ []js.Value) any {
		defer func() {
			if recover() != nil {
				phoneAbortArchiveTransaction(request.Get("transaction"))
			}
		}()
		request.Get("result").Call("createObjectStore", "archives")
		return nil
	})
	success := js.FuncOf(func(_ js.Value, _ []js.Value) any {
		done <- phoneDatabaseResult{value: request.Get("result")}
		return nil
	})
	failure := js.FuncOf(func(_ js.Value, _ []js.Value) any {
		done <- phoneDatabaseResult{err: phoneDatabaseError(request)}
		return nil
	})
	request.Set("onupgradeneeded", upgrade)
	request.Set("onsuccess", success)
	request.Set("onerror", failure)
	cleanup := func() {
		request.Set("onupgradeneeded", js.Null())
		request.Set("onsuccess", js.Null())
		request.Set("onerror", js.Null())
		upgrade.Release()
		success.Release()
		failure.Release()
	}
	select {
	case result := <-done:
		cleanup()
		return result.value, result.err
	case <-ctx.Done():
		// An open request cannot be aborted. Keep its callbacks alive until it
		// settles, then close a late connection without publishing anything.
		go func() {
			result := <-done
			cleanup()
			if result.err == nil {
				result.value.Call("close")
			}
		}()
		return js.Undefined(), ctx.Err()
	}
}

func phoneDatabaseError(source js.Value) error {
	value := source.Get("error")
	if value.IsNull() || value.IsUndefined() {
		return errors.New("browser package cache transaction aborted")
	}
	return fmt.Errorf("browser package cache: %s", value.Call("toString").String())
}

func phoneAbortArchiveTransaction(transaction js.Value) {
	// A cancellation can race with a completed transaction; its commit result
	// still must not be reported as installation success to the canceled caller.
	defer func() {
		if p := recover(); p != nil {
			return
		}
	}()
	transaction.Call("abort")
}

func (s phoneIndexedArchiveStore) transaction(ctx context.Context, digest string, b []byte, write bool) (result js.Value, err error) {
	defer func() {
		if p := recover(); p != nil {
			result, err = js.Undefined(), fmt.Errorf("browser package cache: %v", p)
		}
	}()
	db, err := phoneOpenArchiveDatabase(ctx)
	if err != nil {
		return js.Undefined(), err
	}
	defer db.Call("close")
	if err := ctx.Err(); err != nil {
		return js.Undefined(), err
	}
	mode := "readonly"
	if write {
		mode = "readwrite"
	}
	tx := db.Call("transaction", "archives", mode)
	done := make(chan error, 1)
	complete := js.FuncOf(func(_ js.Value, _ []js.Value) any { done <- nil; return nil })
	abort := js.FuncOf(func(_ js.Value, _ []js.Value) any { done <- phoneDatabaseError(tx); return nil })
	tx.Set("oncomplete", complete)
	tx.Set("onabort", abort)
	defer func() {
		tx.Set("oncomplete", js.Null())
		tx.Set("onabort", js.Null())
		complete.Release()
		abort.Release()
	}()
	store := tx.Call("objectStore", "archives")
	key := s.namespace + ":" + digest
	var request js.Value
	if write {
		value := js.Global().Get("Uint8Array").New(len(b))
		js.CopyBytesToJS(value, b)
		request = store.Call("put", value, key)
	} else {
		request = store.Call("get", key)
	}
	select {
	case err = <-done:
	case <-ctx.Done():
		phoneAbortArchiveTransaction(tx)
		err = errors.Join(ctx.Err(), <-done)
	}
	if err != nil {
		return js.Undefined(), err
	}
	if err := ctx.Err(); err != nil {
		return js.Undefined(), err
	}
	return request.Get("result"), nil
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
