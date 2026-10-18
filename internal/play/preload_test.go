package play

import (
	"bytes"
	"image"
	"image/png"
	"testing"
	"testing/fstest"
)

// 画面の初期化が失敗しても、先読みの worker は描画 API を呼ばない。
func TestPreloadDoesNotCreateGraphicsImages(t *testing.T) {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	p := newPictures(fstest.MapFS{"assets/tile.png": {Data: b.Bytes()}})
	t.Cleanup(p.close)
	p.preload("tile.png")
	p.workers.Wait()
	if len(p.imgs) != 0 {
		t.Fatal("background preload created a graphics image")
	}
	img := p.get("tile.png")
	if img == missing || img.Bounds() != image.Rect(0, 0, 2, 2) {
		t.Fatal("preloaded image cannot be used on the game thread")
	}
}
