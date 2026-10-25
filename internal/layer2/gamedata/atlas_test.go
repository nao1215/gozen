package gamedata_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

func TestAtlasCameraValidation(t *testing.T) {
	for _, value := range []string{"{}", "{zoom: 2, max_zoom: 8}", "{zoom: 1, max_zoom: 1}"} {
		_, files := scenesWith("    map:", "    map:\n      camera: "+value)
		if _, err := testgame.TryLoad(files); err != nil {
			t.Fatalf("valid camera %s: %v", value, err)
		}
	}
	for _, value := range []string{"{zoom: 0.5}", "{zoom: .nan}", "{max_zoom: .inf}", "{max_zoom: 0.5}", "{zoom: 9, max_zoom: 8}", "{max_zoom: 65}"} {
		_, files := scenesWith("    map:", "    map:\n      camera: "+value)
		if _, err := testgame.TryLoad(files); err == nil || !strings.Contains(err.Error(), "camera") {
			t.Fatalf("invalid camera %s: %v", value, err)
		}
	}
}
