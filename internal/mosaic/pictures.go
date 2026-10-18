package mosaic

import (
	"os"
)

func Pictures(gameDir string) (pics, missing []string, err error) {
	return references(os.DirFS(gameDir), "assets", []string{".png", ".jpg", ".jpeg", ".webp"})
}
