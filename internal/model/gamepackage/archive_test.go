package gamepackage

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type zipEntry struct {
	name string
	data []byte
	mode fs.FileMode
}

func archiveFixture(t testing.TB, more []zipEntry) (Release, []byte) {
	t.Helper()
	var icon bytes.Buffer
	if e := png.Encode(&icon, image.NewNRGBA(image.Rect(0, 0, 32, 32))); e != nil {
		t.Fatal(e)
	}
	entries := []zipEntry{{"game.yaml", []byte("id: child\nschema_version: 11\n"), 0o600}, {"assets/icon.png", icon.Bytes(), 0o600}, {"LICENSE", []byte("Original test fixture. CC0-1.0.\n"), 0o600}}
	entries = append(entries, more...)
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	var expanded int64
	for _, f := range entries {
		h := &zip.FileHeader{Name: f.name, Method: zip.Deflate}
		h.SetMode(f.mode)
		w, e := z.CreateHeader(h)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write(f.data); e != nil {
			t.Fatal(e)
		}
		expanded += int64(len(f.data))
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	data := b.Bytes()
	return Release{ID: "sample/child", Version: "1.0.0", Title: "Child", Author: "Test", Description: "Original fixture", Icon: "assets/icon.png", IconData: base64.StdEncoding.EncodeToString(icon.Bytes()), License: "CC0-1.0", Orientation: "portrait", URL: "https://example.com/child-1.0.0.zip", SHA256: checksum(data), Size: int64(len(data)), UnpackedSize: expanded, Engine: ">=0.23.0 <0.24.0"}, data
}

func TestArchiveVerifiedIsolation(t *testing.T) {
	r, b := archiveFixture(t, []zipEntry{{"data/story.yaml", []byte("events: {}\n"), 0o600}})
	game, e := VerifyContext(t.Context(), r, b)
	if e != nil {
		t.Fatal(e)
	}
	clear(b)
	content, e := fs.ReadFile(game.FS, "data/story.yaml")
	if e != nil || string(content) != "events: {}\n" {
		t.Fatalf("owned bytes: %q %v", content, e)
	}
	if _, e = game.FS.Open("../game.yaml"); e == nil {
		t.Fatal("escaped package")
	}
}

func TestArchiveRejectsUnsafeEntries(t *testing.T) {
	for _, tc := range []zipEntry{
		{"../escape", []byte("x"), 0o600},
		{"assets\\escape.txt", []byte("x"), 0o600},
		{"assets/CON.txt", []byte("x"), 0o600},
		{"assets/trailing .txt ", []byte("x"), 0o600},
		{"assets/.hidden", []byte("x"), 0o600},
		{"assets/program.exe", []byte("x"), 0o600},
		{"assets/code.js", []byte("x"), 0o600},
		{"assets/link.png", []byte("../secret"), fs.ModeSymlink | 0o777},
		{"assets/executable.txt", []byte("x"), 0o700},
		{"game.yaml", []byte("x"), 0o600},
		{"GAME.yaml", []byte("x"), 0o600},
		{"assets/ICON.png", []byte("x"), 0o600},
		{"assets/icon.png/child.txt", []byte("x"), 0o600},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, b := archiveFixture(t, []zipEntry{tc})
			if _, e := VerifyContext(t.Context(), r, b); e == nil {
				t.Fatal("accepted unsafe archive")
			}
		})
	}
}

func TestArchiveDigestSizeAndIcon(t *testing.T) {
	r, b := archiveFixture(t, nil)
	changed := bytes.Clone(b)
	changed[len(changed)/2] ^= 1
	if _, e := VerifyContext(t.Context(), r, changed); e == nil {
		t.Fatal("changed package accepted")
	}
	wrong := r
	wrong.UnpackedSize++
	if _, e := VerifyContext(t.Context(), wrong, b); e == nil {
		t.Fatal("wrong expansion size accepted")
	}
	for _, name := range []string{"assets/missing.png", "game.yaml"} {
		wrong = r
		wrong.Icon = name
		if _, e := VerifyContext(t.Context(), wrong, b); e == nil {
			t.Fatalf("invalid icon %s", name)
		}
	}
	r, b = archiveFixture(t, []zipEntry{{"assets/fake.png", []byte("not a PNG"), 0o600}})
	r.Icon = "assets/fake.png"
	if _, e := VerifyContext(t.Context(), r, b); e == nil {
		t.Fatal("fake icon accepted")
	}
	var icon bytes.Buffer
	if e := png.Encode(&icon, image.NewNRGBA(image.Rect(0, 0, 32, 64))); e != nil {
		t.Fatal(e)
	}
	r, b = archiveFixture(t, []zipEntry{{"assets/rectangle.png", icon.Bytes(), 0o600}})
	r.Icon = "assets/rectangle.png"
	if _, e := VerifyContext(t.Context(), r, b); e == nil {
		t.Fatal("nonsquare icon accepted")
	}
}

func TestArchiveRejectsAmbiguousDirectories(t *testing.T) {
	r, b := archiveFixture(t, []zipEntry{{"assets/Sub/first.txt", []byte("x"), 0o600}, {"assets/sub/second.txt", []byte("y"), 0o600}})
	if _, e := VerifyContext(t.Context(), r, b); e == nil {
		t.Fatal("case-colliding implicit directories accepted")
	}
}

func TestArchiveExpansionAndCRC(t *testing.T) {
	r, b := archiveFixture(t, nil)
	central := bytes.Index(b, []byte{'P', 'K', 1, 2})
	if central < 0 {
		t.Fatal("missing central directory")
	}
	for _, tc := range []struct {
		name   string
		offset int
		value  uint32
	}{
		{"expansion", 24, uint32(MaxUnpackedBytes + 1)},
		{"CRC", 16, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := bytes.Clone(b)
			binary.LittleEndian.PutUint32(changed[central+tc.offset:], tc.value)
			altered := r
			altered.SHA256 = checksum(changed)
			if _, e := VerifyContext(t.Context(), altered, changed); e == nil {
				t.Fatal("accepted malicious ZIP metadata")
			}
		})
	}
	entries := make([]zipEntry, maxEntries)
	for i := range entries {
		entries[i] = zipEntry{name: fmt.Sprintf("assets/f%d.txt", i), mode: 0o600}
	}
	r, b = archiveFixture(t, entries)
	if _, e := VerifyContext(t.Context(), r, b); e == nil {
		t.Fatal("accepted excessive entries")
	}
}

func TestCatalogIconMatchesArchive(t *testing.T) {
	r, b := archiveFixture(t, nil)
	var out bytes.Buffer
	icon := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	icon.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	if e := png.Encode(&out, icon); e != nil {
		t.Fatal(e)
	}
	r.IconData = base64.StdEncoding.EncodeToString(out.Bytes())
	if _, e := VerifyContext(t.Context(), r, b); e == nil {
		t.Fatal("accepted different listing and archive icons")
	}
}

// A regular file-backed archive may exceed memory limits without allocating its
// bytes. This also exercises the lifetime of the reader retained by Installed.
func TestDiskArchiveBeyondMemoryLimit(t *testing.T) {
	r, name := diskFixture(t, MaxArchiveBytes+1)
	game, e := OpenArchive(t.Context(), os.DirFS(filepath.Dir(name)), filepath.Base(name), r)
	if e != nil {
		t.Fatal(e)
	}
	f, e := game.FS.Open("assets/large.txt")
	if e != nil {
		t.Fatal(e)
	}
	head := make([]byte, 16)
	if _, e = io.ReadFull(f, head); e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
	if e = game.Close(); e != nil {
		t.Fatal(e)
	}
	if e = game.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e = VerifyContext(t.Context(), r, nil); e == nil || !strings.Contains(e.Error(), "memory/browser") {
		t.Fatalf("memory limit: %v", e)
	}
}

type zeroReader struct{}

func (zeroReader) Read(b []byte) (int, error) { clear(b); return len(b), nil }
func diskFixture(t testing.TB, size int64) (Release, string) {
	t.Helper()
	r, small := archiveFixture(t, nil)
	source, e := zip.NewReader(bytes.NewReader(small), int64(len(small)))
	if e != nil {
		t.Fatal(e)
	}
	name := filepath.Join(t.TempDir(), "game.zip")
	f, e := os.Create(name) //nolint:gosec // Fixed filename inside the test temporary directory.
	if e != nil {
		t.Fatal(e)
	}
	z := zip.NewWriter(f)
	for _, entry := range source.File {
		if e = z.Copy(entry); e != nil {
			t.Fatal(e)
		}
	}
	w, e := z.CreateHeader(&zip.FileHeader{Name: "assets/large.txt", Method: zip.Store})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = io.CopyN(w, zeroReader{}, size); e != nil {
		t.Fatal(e)
	}
	if e = z.Close(); e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
	f, e = os.Open(name) //nolint:gosec // Fixed filename inside the test temporary directory.
	if e != nil {
		t.Fatal(e)
	}
	hash := sha256.New()
	n, e := io.Copy(hash, f)
	if e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
	r.SHA256 = hex.EncodeToString(hash.Sum(nil))
	r.Size = n
	r.UnpackedSize += size
	return r, name
}

func BenchmarkDiskArchiveVerification(b *testing.B) {
	for _, size := range []int64{1 << 20, 65 << 20} {
		b.Run(fmt.Sprint(size>>20, "MiB"), func(b *testing.B) {
			r, name := diskFixture(b, size)
			b.ReportAllocs()
			b.SetBytes(size)
			b.ResetTimer()
			for b.Loop() {
				game, e := OpenArchive(b.Context(), os.DirFS(filepath.Dir(name)), filepath.Base(name), r)
				if e != nil {
					b.Fatal(e)
				}
				if e = game.Close(); e != nil {
					b.Fatal(e)
				}
			}
		})
	}
}
