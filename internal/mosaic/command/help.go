package command

import (
	"slices"

	"github.com/spf13/pflag"

	"github.com/nao1215/gozen/internal/base/clihelp"
)

// The help pages of gozen mosaic. The examples use the synthetic fixture of
// docs/examples/production (a game directory "game" with assets/pattern.png).

const requirements = "Requirements: Go 1.26.10 or later to build gozen. ONNX inference needs a C compiler " +
	"and CGO_ENABLED=1 when gozen is built, an ONNX Runtime shared library (--ort or ONNXRUNTIME_LIB), " +
	"and user-supplied models (--model; optionally --face, --seg, --tagger). Help, manual regions, " +
	"and checks of an existing report need no ONNX Runtime. A gozen built with CGO_ENABLED=0 says " +
	"that inference is missing and exits 1. Models are never bundled. See " + clihelp.README

const fetchRequirements = "Requirements: network access to huggingface.co. No cgo or ONNX Runtime is needed to download. See " + clihelp.README

const exitStatus = "Exit status: 0 for success and help, 1 when processing, a prerequisite, or I/O fails, " +
	"2 for an unknown subcommand or invalid options."

var (
	inputGroup     = clihelp.Group{Title: "Input", Flags: []string{"fix", "manual", "tags"}}
	detectionGroup = clihelp.Group{Title: "Detection", Flags: []string{"model", "yes", "ort", "face", "seg", "tagger", "classes", "labels", "threshold", "sure", "passes", "threads"}}
	regionGroup    = clihelp.Group{Title: "Regions", Flags: []string{"shape", "block", "margin", "pad", "min", "dilate"}}
	generalGroup   = clihelp.Group{Title: "General", Flags: []string{"help"}}
)

var topPage = clihelp.Page{
	Name:    "gozen mosaic",
	Summary: "pixelate image regions, with optional ONNX detection",
	Usage:   []string{"gozen mosaic <command> [OPTIONS] <game-dir> [IMAGE ...]"},
	About: "The main use is to pixelate adult-oriented anime-style images distributed in Japan, following the " +
		"distribution standard. Swap the model and its classes and it also serves other uses, such as " +
		"anonymizing faces. Reads images from <game-dir>/assets/ (or every image the YAML files refer to), pixelates the " +
		"regions found by a user-supplied detection model or written by hand, and writes the results " +
		"and report.json outside assets/. The original images are never changed. Automatic detection " +
		"does not replace a person checking the output.",
	Commands: [][2]string{
		{"auto", "Detect and pixelate regions, and write the images and the report"},
		{"inspect", "Report per image whether it needs and has pixelation, as JSON"},
		{"list", "List images needing review"},
		{"show", "Show image instructions"},
		{"view", "Render a coordinate preview"},
		{"add", "Add regions"},
		{"drop", "Drop regions"},
		{"grow", "Resize regions"},
		{"skip", "Skip automatic detection"},
		{"mark", "Mark an image as reviewed"},
		{"clear", "Clear manual instructions"},
	},
	Examples: []clihelp.Example{
		{"pixelate the regions of a hand-written fix file, without a model", "gozen mosaic auto --fix fixes.yaml -o processed game pattern.png"},
		{"detect with a model", "gozen mosaic auto --model model.onnx --ort libonnxruntime.so -o processed game"},
		{"check the result without writing", "gozen mosaic inspect --fix fixes.yaml -o processed game pattern.png"},
		{"use only hand-drawn regions for an image, without a model", "gozen mosaic skip --manual edits.yaml game pattern.png"},
	},
	Groups: []clihelp.Group{generalGroup},
	Notes:  []string{exitStatus, requirements},
	Hint:   `Run "gozen mosaic <command> --help" for a command's options and examples.`,
}

var autoPage = clihelp.Page{
	Name:    "gozen mosaic auto",
	Summary: "detect and pixelate regions, and write images",
	Usage:   []string{"gozen mosaic auto [OPTIONS] <game-dir> [IMAGE ...]"},
	About: "IMAGE is a path relative to <game-dir>/assets/; without one, every image the YAML files refer " +
		"to is processed. Regions come from the detection model (--model), the hand-written fix file " +
		"(--fix), and the manual file (--manual). A fix file alone needs no model. The images, report.json, " +
		"and the review sheets go to --out. With --dry-run nothing is written.",
	Examples: []clihelp.Example{
		{"pixelate the regions of a fix file, without a model", "gozen mosaic auto --fix fixes.yaml -o processed game pattern.png"},
		{"detect every referenced image with a model", "gozen mosaic auto --model model.onnx --ort libonnxruntime.so -o processed game"},
		{"follow the outline with a segmentation model", "gozen mosaic auto --model model.onnx --shape mask --seg seg.onnx game"},
		{"show what would be pixelated, writing nothing", "gozen mosaic auto -n --model model.onnx game"},
		{"pixelate faces with a face detection model", "gozen mosaic auto --model face.onnx --classes face game"},
	},
	Groups: []clihelp.Group{
		inputGroup, detectionGroup, regionGroup,
		{Title: "Output", Flags: []string{"out", "quality", "dry-run"}},
		generalGroup,
	},
	Notes: []string{
		classesNote,
		"Pixelation averages the original pixels of each cell. A block of 0 uses max(4, ceil(long side / 100)) " +
			"pixels; 1 leaves the pixels unchanged. Every cell that a region touches is filled. Inputs are " +
			"PNG, JPEG, and WebP; outputs are PNG and JPEG, and a WebP input is written as <name>.png.",
		exitStatus, requirements,
	},
}

var inspectPage = clihelp.Page{
	Name:    "gozen mosaic inspect",
	Summary: "report if images need and have pixelation",
	Usage:   []string{"gozen mosaic inspect [OPTIONS] <game-dir> [IMAGE ...]"},
	About: "Writes one JSON object per image, one per line (indented with --pretty), and writes no file. " +
		"IMAGE is a path relative to assets/ or a file path; a file inside --out is looked up by its " +
		"original image. When report.json from auto matches the current image and fixes, the answer comes " +
		"from the report without running a model. A changed image or fix is detected again and reported " +
		"as todo.",
	Examples: []clihelp.Example{
		{"inspect one image with the regions of a fix file", "gozen mosaic inspect --fix fixes.yaml -o processed game pattern.png"},
		{"inspect every referenced image as indented JSON", "gozen mosaic inspect --pretty -o processed game"},
		{"detect every image again instead of trusting the report", "gozen mosaic inspect --redetect --model model.onnx game"},
	},
	Groups: []clihelp.Group{
		inputGroup, detectionGroup, regionGroup,
		{Title: "Output", Flags: []string{"out", "quality", "pretty", "redetect"}},
		generalGroup,
	},
	Notes: []string{
		classesNote,
		"Fields: needs_mosaic (whether a region needs pixelation, from detection, fixes, or tags), " +
			"categories (the classes chosen by --labels that were found), objects (class, score, rectangle " +
			"[x,y,w,h], and whether it is pixelated), mosaicked (whether the image and the output in --out " +
			"cover every object with the standard block), status (none, todo, done, or review), and review " +
			"(the reasons to look). An image marked checked is none without regions and done with an output. " +
			"todo and review do not fail the command.",
		exitStatus, requirements,
	},
}

// manualAction is one of the manual commands: what its help says, which options it reads
// beyond the common ones, and whether it writes the --manual file.
type manualAction struct {
	summary, about string
	flags          []string
	writes         bool
	examples       []clihelp.Example
}

const (
	manualImage = "IMAGE is a path relative to <game-dir>/assets/ (a leading assets/ is accepted) or a path to the image file."
	manualWrite = "Writes the instruction to the --manual YAML file (auto reads it too) and rebuilds only that " +
		"image, with a check image in <out>/manual/. Rebuilding detects again, so it needs the model unless the " +
		"image is skipped. The original image in assets/ is not changed. " + manualImage
	coordinatesNote = "Coordinates are pixels of the original image with 0,0 at the top left; with --norm they are " +
		"fractions from 0 to 1 of the width and the height. --rect takes x,y,w,h, --box x0,y0,x1,y1, " +
		"--poly x1,y1,x2,y2,... (3 points or more), and --ellipse cx,cy,rx,ry. Every block that a " +
		"region touches is filled, so include the outline."
	drawingNote = "In the drawing, the scale is in original pixels, the pale fill is what was pixelated, A1... are " +
		"automatic regions, M1... rectangles added by hand, and P1... polygons added by hand."
)

// manualActions describes the manual commands, by name.
func manualActions() map[string]manualAction {
	return map[string]manualAction{
		"list": {
			summary: "list the images to review",
			about: "Prints one JSON line for each image that report.json gives reasons to review: the file, the " +
				"reasons, and whether it was pixelated. Reads the report that auto wrote to --out and writes nothing.",
			examples: []clihelp.Example{{"list the images to review", "gozen mosaic list -o processed game"}},
		},
		"show": {
			summary: "print the regions and instructions of an image",
			about: "Prints, as indented JSON, the manual instructions of the image, its fixes (--fix and --manual " +
				"combined), and its entry in report.json. Writes nothing. " + manualImage,
			examples: []clihelp.Example{{"print what is known about an image", "gozen mosaic show --manual edits.yaml -o processed game pattern.png"}},
		},
		"view": {
			summary: "draw an image with a coordinate scale",
			about: "Draws the image with a scale and the outlines of its regions, to read coordinates from, and " +
				"writes the drawing to <out>/manual/ or to --view-out. --src masked draws the pixelated output when " +
				"there is one. " + manualImage + " An image outside assets/ can be viewed, though not changed.",
			flags: []string{"src", "region", "norm", "size", "step", "view-out"},
			examples: []clihelp.Example{
				{"draw the pixelated image with a scale", "gozen mosaic view --manual edits.yaml --src masked game pattern.png"},
				{"zoom into a part of the original", "gozen mosaic view --region 0,0,16,16 --size 512 game pattern.png"},
			},
		},
		"add": {
			summary: "add regions by hand",
			about:   manualWrite,
			flags:   []string{"rect", "box", "poly", "ellipse", "norm", "note", "size", "step"},
			writes:  true,
			examples: []clihelp.Example{
				{"add a rectangle and an ellipse", "gozen mosaic add --manual edits.yaml --rect 8,8,6,6 --ellipse 4,4,2,3 game pattern.png"},
				{"add a triangle given as fractions of the image", "gozen mosaic add --manual edits.yaml --norm --poly 0.1,0.1,0.5,0.1,0.3,0.5 game pattern.png"},
			},
		},
		"drop": {
			summary: "drop automatic regions",
			about: "A detection that overlaps a dropped rectangle is not pixelated. --region-id names automatic " +
				"regions of the last run as view shows them. " + manualWrite,
			flags:  []string{"rect", "box", "region-id", "norm", "note", "size", "step"},
			writes: true,
			examples: []clihelp.Example{
				{"drop the automatic region A2", "gozen mosaic drop --manual edits.yaml --model model.onnx --region-id A2 game pattern.png"},
				{"drop whatever is detected under a rectangle", "gozen mosaic drop --manual edits.yaml --model model.onnx --rect 8,8,6,6 game pattern.png"},
			},
		},
		"grow": {
			summary: "grow or shrink regions",
			about: "Moves the sides of the automatic regions named by --region-id, or of those under a --rect, by " +
				"--by or by --left, --top, --right, and --bottom; negative values shrink. --margin alone sets the " +
				"margin of this image. " + manualWrite,
			flags:  []string{"rect", "region-id", "by", "left", "top", "right", "bottom", "norm", "note", "size", "step"},
			writes: true,
			examples: []clihelp.Example{
				{"grow the region under a rectangle by 2 pixels on every side", "gozen mosaic grow --manual edits.yaml --rect 8,8,6,6 --by 2 game pattern.png"},
				{"grow every automatic region to the right", "gozen mosaic grow --manual edits.yaml --model model.onnx --region-id all --right 4 game pattern.png"},
			},
		},
		"skip": {
			summary: "use only regions added by hand for an image",
			about:   "Turns automatic detection off for the image, so that only regions added by hand are pixelated and no model is needed. " + manualWrite,
			flags:   []string{"note", "size", "step"},
			writes:  true,
			examples: []clihelp.Example{
				{"use only regions drawn by hand for an image, so no model is needed", "gozen mosaic skip --manual edits.yaml game pattern.png"},
			},
		},
		"mark": {
			summary: "mark an image as reviewed",
			about: "Records with --checked that a person looked at the image, which clears its reasons to review. " +
				"A later add, drop, grow, or skip takes the mark away again. " + manualWrite,
			flags:  []string{"checked", "note"},
			writes: true,
			examples: []clihelp.Example{
				{"mark an image as reviewed, with a note", "gozen mosaic mark --manual edits.yaml --checked --note \"checked by hand\" game pattern.png"},
			},
		},
		"clear": {
			summary: "remove the manual instructions of an image",
			about:   "Removes every instruction of the image from the --manual file; the --fix file is not touched. " + manualWrite,
			flags:   []string{"note", "size", "step"},
			writes:  true,
			examples: []clihelp.Example{
				{"start over for an image", "gozen mosaic clear --manual edits.yaml --model model.onnx game pattern.png"},
			},
		},
	}
}

// page builds the help of one manual command. Only the groups and notes of the options
// the command registered are shown.
func (a manualAction) page(sub string, fs *pflag.FlagSet) clihelp.Page {
	p := clihelp.Page{
		Name:     "gozen mosaic " + sub,
		Summary:  a.summary,
		Usage:    []string{"gozen mosaic " + sub + " [OPTIONS] <game-dir> <IMAGE>"},
		About:    a.about,
		Examples: a.examples,
	}
	if sub == "list" {
		p.Usage = []string{"gozen mosaic list [OPTIONS] <game-dir>"}
	}
	for _, g := range []clihelp.Group{
		{Title: "Instructions", Flags: []string{"rect", "box", "poly", "ellipse", "region-id", "by", "left", "top", "right", "bottom", "checked", "note"}},
		{Title: "Drawing", Flags: []string{"src", "region", "size", "step", "view-out"}},
		{Title: "Coordinates", Flags: []string{"norm"}},
	} {
		if slices.ContainsFunc(g.Flags, func(name string) bool { return fs.Lookup(name) != nil }) {
			p.Groups = append(p.Groups, g)
		}
	}
	p.Groups = append(p.Groups, inputGroup, detectionGroup, regionGroup, clihelp.Group{Title: "Output", Flags: []string{"out", "quality"}}, generalGroup)
	if fs.Lookup("norm") != nil {
		p.Notes = append(p.Notes, coordinatesNote)
	}
	if fs.Lookup("size") != nil {
		p.Notes = append(p.Notes, drawingNote)
	}
	p.Notes = append(p.Notes, exitStatus, requirements)
	return p
}

const classesNote = "Classes: the names of the --model output, read from --classes, then labels.json (a JSON array of " +
	"strings) beside the model, then the ONNX metadata names, then the default. The default model " +
	"classes are nipple_f (a female nipple; the class name of deepghs/anime_censor_detection), penis, " +
	"and pussy. --labels chooses which classes to pixelate; the default is penis,pussy, because " +
	"nipples are not pixelated by default under the Japanese distribution standard. A class count " +
	"that does not match the model output is an error. --dry-run and inspect print the classes " +
	"loaded and where they came from."

var fetchPage = clihelp.Page{
	Name:    "gozen models fetch",
	Summary: "download the pinned detection models into the cache",
	Usage:   []string{"gozen models fetch [OPTIONS] [NAME ...]"},
	About: "Downloads fixed revisions of models from Hugging Face into the user cache directory " +
		"(gozen/mosaic/<name>/ under os.UserCacheDir), checks each file against a SHA-256 written in gozen, " +
		"and deletes the download with an error on a mismatch. Without NAME every model is fetched. " +
		"A model already in the cache with matching checksums is not downloaded again. " +
		"This command and the confirmed download prompt of gozen mosaic are the only places gozen uses the network. " +
		"A model in the cache is used when --model is omitted: censor by gozen mosaic, face by gozen faces.",
	Examples: []clihelp.Example{
		{"list the models and whether each is fetched", "gozen models list"},
		{"fetch the default detector", "gozen models fetch censor"},
		{"pixelate faces with the fetched face model (Linux cache path)", "gozen mosaic auto --model ~/.cache/gozen/mosaic/face/model.onnx game"},
	},
	Groups: []clihelp.Group{generalGroup},
	Notes: []string{
		"Models. censor (deepghs/anime_censor_detection, MIT) detects nipple_f, penis, and pussy for the " +
			"main use. face (deepghs/anime_face_detection, MIT) and head (deepghs/anime_head_detection, MIT) " +
			"find faces and heads, to anonymize people. Class names come from labels.json in the cache.",
		exitStatus, fetchRequirements,
	},
}
