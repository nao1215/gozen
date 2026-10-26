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

func TestAtlasSafeAreaValidation(t *testing.T) {
	for _, value := range []string{"[]", "[0.1, 0.2, 0.15, 0]", "[0.45, 0, 0.35, 0.45]"} {
		_, files := scenesWith("    map:", "    map:\n      safe_area: "+value)
		if _, err := testgame.TryLoad(files); err != nil {
			t.Fatalf("valid safe area %s: %v", value, err)
		}
	}
	for _, value := range []string{"[0.1, 0.1]", "[0.5, 0, 0, 0]", "[-0.1, 0, 0, 0]", "[0.45, 0, 0.45, 0]", "[.nan, 0, 0, 0]"} {
		_, files := scenesWith("    map:", "    map:\n      safe_area: "+value)
		if _, err := testgame.TryLoad(files); err == nil || !strings.Contains(err.Error(), "safe_area") {
			t.Fatalf("invalid safe area %s: %v", value, err)
		}
	}
}

func TestAtlasTalkValidation(t *testing.T) {
	for _, value := range []string{"dim", "hide", "fade"} {
		_, files := scenesWith("    map:", "    map:\n      talk: "+value)
		_, err := testgame.TryLoad(files)
		if value == "fade" {
			if err == nil || !strings.Contains(err.Error(), "want dim or hide") {
				t.Fatalf("talk %s: %v", value, err)
			}
		} else if err != nil {
			t.Fatalf("talk %s: %v", value, err)
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

func TestAtlasHideValidation(t *testing.T) {
	for _, hide := range []string{"", "closed", "always"} {
		world := strings.Replace(testgame.AtlasMenuData, "label: Tower", "label: Tower\n          hide: '"+hide+"'", 1)
		_, err := testgame.TryLoad(map[string]string{"game.yaml": "schema_version: 3\nid: hide-points\n" + testgame.Game, "data/menu.yaml": world, "assets/maps/menu.png": string(testgame.Picture(64, 36))})
		if hide == "always" {
			if err == nil || !strings.Contains(err.Error(), "points.tower.hide") {
				t.Fatalf("invalid hide: %v", err)
			}
		} else if err != nil {
			t.Fatal(err)
		}
	}
}
