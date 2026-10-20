package savefile

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestPreservingWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	old := []byte("元の保存内容")
	if err := Write(path, old); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := Write(path, old); err != nil {
			t.Fatal(err)
		}
		backup, err := WritePreserving(path, []byte("新しい保存内容"))
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(backup) //nolint:gosec // Path is isolated in t.TempDir.
		if err != nil || !bytes.Equal(b, old) {
			t.Fatal("控えを失った", err)
		}
	}
	files, err := filepath.Glob(path + ".before-*.bak")
	if err != nil || len(files) != 1 {
		t.Fatalf("控えを増やした %v %v", files, err)
	}
	t.Run("控えが書けなければ元を保持", func(t *testing.T) {
		if err := Write(path, old); err != nil {
			t.Fatal(err)
		}
		backup := fmt.Sprintf("%s.before-%x.bak", path, sha256.Sum256(old))
		if err := os.WriteFile(backup, []byte("一致しない控え"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := WritePreserving(path, []byte("置き換えてはいけない")); err == nil {
			t.Fatal("控えの不一致を無視した")
		}
		b, err := os.ReadFile(path) //nolint:gosec // Path is isolated in t.TempDir.
		if err != nil || !bytes.Equal(b, old) {
			t.Fatal("失敗で元ファイルを変更した", err)
		}
	})
}

func TestUnnamedSaveDirectory(t *testing.T) {
	if _, err := Prepare("", Config{Root: t.TempDir()}); err == nil {
		t.Fatal("empty game ID accepted")
	}
}
