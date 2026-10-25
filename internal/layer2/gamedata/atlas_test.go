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

func TestAtlasSelectionPanelValidation(t *testing.T) {
	for _, fields := range []string{
		"activation: instant\n      selection_panel: {}",
		"selection_panel: {rect: [0.65, 0.1, 0.33, 0.65], portrait: x.png}",
	} {
		_, files := scenesWith("    map:", "    map:\n      "+fields)
		if _, err := testgame.TryLoad(files); err != nil {
			t.Fatalf("valid panel: %v", err)
		}
	}
	for _, fields := range []string{
		"activation: immediate", "selection_panel: {rect: [0, 0, 1]}",
		"selection_panel: {rect: [0.9, 0, 0.2, 0.5]}",
		"selection_panel: {rect: [0, 0, .nan, 0.5]}",
		"selection_panel: {portrait: missing.png}",
	} {
		_, files := scenesWith("    map:", "    map:\n      "+fields)
		if _, err := testgame.TryLoad(files); err == nil {
			t.Fatalf("accepted invalid panel %s", fields)
		}
	}
}
