package trial

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/testgame"
)

// writeTree writes the files of a game under root.
func writeTree(t *testing.T, root string, tree map[string]string) {
	t.Helper()
	for name, text := range tree {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// The files gozen export takes besides the data and the pictures (the
// translations, the license notices) are in the trial tree too, and the files it
// leaves behind are not.
func TestWriteCarriesWhatTheGameShipsWith(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	carried := map[string]string{
		"lang/ja.yaml":      "text:\n  Village: Mura\n",
		"LICENSE":           "the license of the game's pictures\n",
		"LICENSES/font.txt": "the license of the font\n",
		"assets/NOTICE":     "credits\n",
		"ASSETS.md":         "where the pictures come from\n",
	}
	writeTree(t, src, carried)
	writeTree(t, src, map[string]string{
		"game.yaml":           "schema_version: 11\nid: g\ntitle: G\nlanguage: en\nlanguages: [en, ja]\nstart: {location: village}\n",
		"data/world.yaml":     "locations:\n  village:\n    name: Village\n",
		"asset-policy.yaml":   "{}\n",
		"saves/save1.json":    "{}\n",
		"lang/notes.txt":      "not a translation\n",
		"assets/source.psd":   "an authoring file\n",
		"assets/.hidden.png":  "x",
		"assets/unnamed.png":  "x",
		".git/config":         "x",
		"data/.swap.yaml.swp": "x",
	})
	cfg, err := parse([]byte("keep: ['*.psd', '.hid*']\n"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Build(os.DirFS(src), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := res.Check(); err != nil {
		t.Fatal(err)
	}
	if err := res.Write(dst); err != nil {
		t.Fatal(err)
	}
	for name, want := range carried {
		b, err := os.ReadFile(filepath.Join(dst, filepath.FromSlash(name))) //nolint:gosec // Test-owned path under t.TempDir.
		if err != nil || string(b) != want {
			t.Errorf("the trial tree lacks %s, which gozen export takes from the game: %q, %v", name, b, err)
		}
	}
	if words := res.FS()["lang/ja.yaml"]; words == nil {
		t.Error("the check of the trial reads it without its translations")
	}
	for _, name := range []string{"asset-policy.yaml", "saves/save1.json", "lang/notes.txt", "assets/source.psd", "assets/.hidden.png", "assets/unnamed.png", ".git/config", "data/.swap.yaml.swp"} {
		if _, err := os.Lstat(filepath.Join(dst, filepath.FromSlash(name))); err == nil {
			t.Errorf("the trial tree holds %s, which is not selected or does not ship", name)
		}
	}
}

// A link in the game is refused, as gozen export refuses it: copying through it
// would put a file from outside the game into the trial.
func TestBuildRefusesSymlinks(t *testing.T) {
	src, outside := t.TempDir(), t.TempDir()
	writeTree(t, outside, map[string]string{"secret.txt": "not part of the game\n"})
	writeTree(t, src, map[string]string{
		"game.yaml":       "schema_version: 11\nid: g\ntitle: G\nstart: {location: village}\n",
		"data/world.yaml": "locations:\n  village:\n    name: Village\n",
		"assets/x.png":    "x",
	})
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(src, "assets", "leak.txt")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := Build(os.DirFS(src), &Config{Keep: []string{"leak.txt"}}); err == nil || !strings.Contains(err.Error(), "symlinks cannot be exported: assets/leak.txt") {
		t.Fatalf("a linked asset: %v", err)
	}
	// A link that appears after the build is not followed by the copy either.
	if err := os.Remove(filepath.Join(src, "assets", "leak.txt")); err != nil {
		t.Fatal(err)
	}
	res, err := Build(os.DirFS(src), &Config{Keep: []string{"x.png"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(src, "assets", "x.png")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(src, "assets", "x.png")); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	if err := res.Write(dst); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("an asset replaced by a link: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(dst, "assets", "x.png")); err == nil { //nolint:gosec // Test-owned path under t.TempDir.
		t.Errorf("the trial tree holds the target of a link out of the game: %q", b)
	}
}

// A stream game names its episodes under streams/, outside the data the trial
// prunes: the trial keeps them, so it loads.
func TestTrialOfAStreamGameKeepsItsEpisodes(t *testing.T) {
	src := fstest.MapFS{
		"game.yaml":      {Data: []byte(testgame.Game + "id: stream-trial\n" + testgame.StreamsGame)},
		"data/main.yaml": {Data: []byte(testgame.Data)},
		"assets/x.png":   {Data: []byte("x")},
	}
	for name, text := range testgame.StreamFiles() {
		src[name] = &fstest.MapFile{Data: []byte(text)}
	}
	res, err := Build(src, &Config{Keep: []string{"p/*"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := res.Check(); err != nil {
		t.Fatalf("the trial of a stream game does not load: %v", err)
	}
	dst := t.TempDir()
	if err := res.Write(dst); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"streams/ann/1.yaml", "streams/bo/1.yaml", "streams/ann-bo/1.yaml"} {
		if _, err := os.Stat(filepath.Join(dst, filepath.FromSlash(name))); err != nil {
			t.Errorf("the trial tree lacks the episode %s", name)
		}
	}
}
