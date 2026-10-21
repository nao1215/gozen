package gamedata

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"

	"github.com/nao1215/gozen/internal/diag"
)

// FacesFile is where gozen art faces writes the face boxes of the standing pictures (ADR 0022):
// one file for the game, made by the tool and read by the screen without any inference, so
// a web build uses it too. It is not written by hand; run gozen art faces again after changing
// a picture.
const FacesFile = "assets/faces.json"

// FaceBoxes are the faces found in the pictures, by the picture's path under assets/.
type FaceBoxes struct {
	Version  int                `json:"version"`
	Model    string             `json:"model"` // the detection model used (its file name)
	Pictures map[string]FaceBox `json:"pictures"`
}

// FaceBox is the face of one picture: its box (x0, y0, x1, y1 in the picture's pixels;
// absent when no face was found), the picture's size, and its SHA-256 (to tell a changed
// picture).
type FaceBox struct {
	Box    *[4]int `json:"box,omitempty"`
	Size   [2]int  `json:"size"`
	SHA256 string  `json:"sha256"`
	Score  float32 `json:"score,omitempty"`
}

// ReadFaceBoxes reads the game's face boxes (false when the game has none).
func ReadFaceBoxes(fsys fs.FS) (FaceBoxes, bool, error) {
	var f FaceBoxes
	b, err := fs.ReadFile(fsys, FacesFile)
	if errors.Is(err, fs.ErrNotExist) {
		return f, false, nil
	}
	if err != nil {
		return f, false, err
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return f, false, fmt.Errorf("%s: %w", FacesFile, err)
	}
	return f, true, nil
}

// FileSHA256 is the hex SHA-256 of a file under the game's directory.
func FileSHA256(fsys fs.FS, name string) (string, error) {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// ScenePictures are the standing pictures face icons are cut from (under assets/): the
// pictures of the people placed in a stage, the default face picture of each actor (the
// leader's doll, the icons) when the game has a stage or a character book, and the
// pictures of the book's profiles (ADR 0031).
func (db *DB) ScenePictures() []string {
	var out []string
	add := func(p string) {
		if p != "" && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	staged := false
	for _, id := range db.Ordered("locations") {
		l := db.Locations[id]
		if l == nil || l.Stage == nil {
			continue
		}
		staged = true
		for _, sp := range l.Stage.People {
			if sp.Doll != nil && sp.Doll.Icon != "" {
				continue // its own face picture
			}
			for _, p := range l.People {
				if p.ID == sp.ID {
					add(p.Image)
				}
			}
		}
	}
	if staged || len(db.Profiles) > 0 {
		for _, id := range db.Ordered("actors") {
			if a := db.Actors[id]; a != nil && a.Faces != "" && (a.Doll == nil || a.Doll.Icon == "") {
				add(db.FacePicture(a, a.Face, ""))
			}
		}
	}
	for _, id := range db.Ordered("profiles") {
		add(db.ProfileIcon(id))
	}
	return out
}

// iconWarnings warns of a doll's icon that is not square (it is cut round).
func (c *checker) iconWarnings() {
	check := func(w string, d *Doll) {
		if d == nil || d.Icon == "" {
			return
		}
		if s, ok := c.pictureSize(d.Icon); ok && s.X != s.Y {
			c.warnf(diag.DrawnDifferently, w+".icon", "the icon is %dx%d, not square: it is cut round from its middle", s.X, s.Y)
		}
	}
	for _, id := range c.db.Ordered("locations") {
		if l := c.db.Locations[id]; l != nil && l.Stage != nil {
			for i := range l.Stage.People {
				check(fmt.Sprintf("locations.%s.stage.people[%d].doll", id, i), l.Stage.People[i].Doll)
			}
		}
	}
	for _, id := range c.db.Ordered("actors") {
		if a := c.db.Actors[id]; a != nil {
			check("actors."+id+".doll", a.Doll)
		}
	}
}

// faceWarnings warns of the scenes' pictures without a face box: no faces file, a picture
// not in it or changed since, or one where no face was found.
func (c *checker) faceWarnings() {
	pics := c.db.ScenePictures()
	if len(pics) == 0 || c.db.FS == nil {
		return
	}
	faces, ok, err := ReadFaceBoxes(c.db.FS)
	switch {
	case err != nil:
		c.warnf(diag.FaceBox, FacesFile, "%v", err)
		return
	case !ok:
		c.warnf(diag.FaceBox, FacesFile, "no such file: the faces of the balloons and the icons are guessed (run gozen art faces to find them)")
		return
	}
	for _, p := range pics {
		f, found := faces.Pictures[p]
		sum, err := FileSHA256(c.db.FS, "assets/"+p)
		switch {
		case err != nil:
		case !found:
			c.warnf(diag.FaceBox, "assets/"+p, "no face box in %s (run gozen art faces)", FacesFile)
		case f.SHA256 != sum:
			c.warnf(diag.FaceBox, "assets/"+p, "the picture changed since gozen art faces found its face (run it again)")
		case f.Box == nil:
			c.warnf(diag.FaceBox, "assets/"+p, "no face was found: its balloon face and icon are guessed (or give the doll face_at)")
		}
	}
}
