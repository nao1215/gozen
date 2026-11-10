package puppet

import (
	"strings"
	"testing"
	"testing/fstest"
)

// Pack makes file names from the names of the faces, and `gozen puppet --out`
// joins them to its output directory: a face whose name would leave that
// directory does not load, and every name Pack gives is one file name.
func TestPackNamesStayInTheOutputDirectory(t *testing.T) {
	for _, name := range []string{"../../outside", "a/b", `a\b`, "..", "c:d"} {
		fsys := fixture()
		fsys["p/rig.yaml"] = &fstest.MapFile{Data: []byte("base: base.png\nface: calm\nfaces:\n  calm: {}\n  \"" + strings.ReplaceAll(name, `\`, `\\`) + "\":\n    image: happy.png\n")}
		if _, err := Load(fsys, "p/rig.yaml"); err == nil || !strings.Contains(err.Error(), "faces: name") {
			t.Errorf("face %q loaded: %v", name, err)
		}
	}
	r := loadYAML(t, `
base: base.png
face: calm
faces:
  calm: {}
  "..happy face.":
    image: happy.png
    eyes: {closed: eyes.png}
`)
	_, files := Pack(r)
	if len(files) < 3 {
		t.Fatalf("packed %d pictures", len(files))
	}
	for name := range files {
		if strings.ContainsAny(name, `/\:`) || name == "." || name == ".." {
			t.Errorf("Pack names a picture %q", name)
		}
	}
}
