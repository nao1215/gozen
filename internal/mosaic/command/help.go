package command

import (
	"github.com/nao1215/gozen/internal/clihelp"
)

// The help pages of gozen art mosaic. The examples use the synthetic fixture of
// docs/examples/production (a game directory "game" with assets/pattern.png).

const requirements = "Requirements: Go 1.26.9 or later to build gozen. ONNX inference needs a C compiler " +
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
	Name:    "gozen art mosaic",
	Summary: "pixelate image regions, with optional ONNX detection",
	Usage:   []string{"gozen art mosaic <command> [OPTIONS] <game-dir> [IMAGE ...]"},
	About: "The main use is to pixelate adult-oriented anime-style images distributed in Japan, following the " +
		"distribution standard. Swap the model and its classes and it also serves other uses, such as " +
		"anonymizing faces. Reads images from <game-dir>/assets/ (or every image the YAML files refer to), pixelates the " +
		"regions found by a user-supplied detection model or written by hand, and writes the results " +
		"and report.json outside assets/. The original images are never changed. Automatic detection " +
		"does not replace a person checking the output.",
	Commands: [][2]string{
		{"auto", "Detect and pixelate regions, and write the images and the report"},
		{"inspect", "Report per image whether it needs and has pixelation, as JSON"},
		{"manual", "Add, drop, grow, or check regions by coordinates"},
	},
	Examples: []clihelp.Example{
		{"pixelate the regions of a hand-written fix file, without a model", "gozen art mosaic auto --fix fixes.yaml -o processed game pattern.png"},
		{"detect with a model", "gozen art mosaic auto --model model.onnx --ort libonnxruntime.so -o processed game"},
		{"check the result without writing", "gozen art mosaic inspect --fix fixes.yaml -o processed game pattern.png"},
		{"use only hand-drawn regions for an image, without a model", "gozen art mosaic manual skip --manual edits.yaml game pattern.png"},
	},
	Groups: []clihelp.Group{generalGroup},
	Notes:  []string{exitStatus, requirements},
	Hint:   `Run "gozen art mosaic <command> --help" for a command's options and examples.`,
}

var autoPage = clihelp.Page{
	Name:    "gozen art mosaic auto",
	Summary: "detect and pixelate regions, and write images",
	Usage:   []string{"gozen art mosaic auto [OPTIONS] <game-dir> [IMAGE ...]"},
	About: "IMAGE is a path relative to <game-dir>/assets/; without one, every image the YAML files refer " +
		"to is processed. Regions come from the detection model (--model), the hand-written fix file " +
		"(--fix), and the manual file (--manual). A fix file alone needs no model. The images, report.json, " +
		"and the review sheets go to --out. With --dry-run nothing is written.",
	Examples: []clihelp.Example{
		{"pixelate the regions of a fix file, without a model", "gozen art mosaic auto --fix fixes.yaml -o processed game pattern.png"},
		{"detect every referenced image with a model", "gozen art mosaic auto --model model.onnx --ort libonnxruntime.so -o processed game"},
		{"follow the outline with a segmentation model", "gozen art mosaic auto --model model.onnx --shape mask --seg seg.onnx game"},
		{"show what would be pixelated, writing nothing", "gozen art mosaic auto -n --model model.onnx game"},
		{"pixelate faces with a face detection model", "gozen art mosaic auto --model face.onnx --classes face game"},
	},
	Groups: []clihelp.Group{inputGroup, detectionGroup, regionGroup,
		{Title: "Output", Flags: []string{"out", "quality", "dry-run"}}, generalGroup},
	Notes: []string{
		classesNote,
		"Pixelation averages the original pixels of each cell. A block of 0 uses max(4, ceil(long side / 100)) " +
			"pixels; 1 leaves the pixels unchanged. Every cell that a region touches is filled. Inputs are " +
			"PNG, JPEG, and WebP; outputs are PNG and JPEG, and a WebP input is written as <name>.png.",
		exitStatus, requirements,
	},
}

var inspectPage = clihelp.Page{
	Name:    "gozen art mosaic inspect",
	Summary: "report if images need and have pixelation",
	Usage:   []string{"gozen art mosaic inspect [OPTIONS] <game-dir> [IMAGE ...]"},
	About: "Writes one JSON object per image, one per line (indented with --pretty), and writes no file. " +
		"IMAGE is a path relative to assets/ or a file path; a file inside --out is looked up by its " +
		"original image. When report.json from auto matches the current image and fixes, the answer comes " +
		"from the report without running a model. A changed image or fix is detected again and reported " +
		"as todo.",
	Examples: []clihelp.Example{
		{"inspect one image with the regions of a fix file", "gozen art mosaic inspect --fix fixes.yaml -o processed game pattern.png"},
		{"inspect every referenced image as indented JSON", "gozen art mosaic inspect --pretty -o processed game"},
		{"detect every image again instead of trusting the report", "gozen art mosaic inspect --redetect --model model.onnx game"},
	},
	Groups: []clihelp.Group{inputGroup, detectionGroup, regionGroup,
		{Title: "Output", Flags: []string{"out", "quality", "pretty", "redetect"}}, generalGroup},
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

var manualPage = clihelp.Page{
	Name:    "gozen art mosaic manual",
	Summary: "add, drop, grow, or check regions",
	Usage: []string{
		"gozen art mosaic manual list [OPTIONS] <game-dir>",
		"gozen art mosaic manual <command> [OPTIONS] <game-dir> <IMAGE>",
	},
	About: "Writes the instructions to the --manual YAML file (auto reads it too) and rebuilds only that " +
		"image, with a check image in <out>/manual/. The original image in assets/ is not changed. " +
		"An image outside assets/ can only be viewed.",
	Commands: [][2]string{
		{"list", "List the images to review and why, one JSON line each (from report.json)"},
		{"show", "Print the detections, regions, and instructions of an image as JSON"},
		{"view", "Draw the image with a scale (--src original|masked, --region to zoom)"},
		{"add", "Add regions (--rect, --box, --poly, --ellipse; repeatable)"},
		{"drop", "Drop automatic regions (--region-id A2, or overlapping --rect or --box)"},
		{"grow", "Grow regions (--region-id A1|all or --rect, with --by or --left, --top, --right, --bottom; negative shrinks), or set --margin for this image"},
		{"skip", "Use no automatic detection for the image, only regions added by hand"},
		{"check", "Mark the image as checked, removing it from review"},
		{"clear", "Remove every manual instruction of the image"},
	},
	Examples: []clihelp.Example{
		{"use only regions drawn by hand for an image, so no model is needed", "gozen art mosaic manual skip --manual edits.yaml game pattern.png"},
		{"add a rectangle and an ellipse", "gozen art mosaic manual add --manual edits.yaml --rect 8,8,6,6 --ellipse 4,4,2,3 game pattern.png"},
		{"grow the region under a rectangle by 2 pixels on every side", "gozen art mosaic manual grow --manual edits.yaml --rect 8,8,6,6 --by 2 game pattern.png"},
		{"draw the pixelated image with a scale", "gozen art mosaic manual view --manual edits.yaml --src masked game pattern.png"},
		{"list the images to review", "gozen art mosaic manual list game"},
	},

	Groups: []clihelp.Group{
		{Title: "Regions", Flags: []string{"rect", "box", "poly", "ellipse", "norm", "note"}},
		{Title: "Drop and grow", Flags: []string{"region-id", "by", "left", "top", "right", "bottom"}},
		{Title: "View", Flags: []string{"src", "region", "size", "step", "view-out"}},
		inputGroup, detectionGroup, regionGroup,
		{Title: "Output", Flags: []string{"out", "quality"}}, generalGroup,
	},
	Notes: []string{
		"Coordinates are pixels of the original image with 0,0 at the top left; with --norm they are " +
			"fractions from 0 to 1 of the width and the height. --rect takes x,y,w,h, --box x0,y0,x1,y1, " +
			"--poly x1,y1,x2,y2,... (3 points or more), and --ellipse cx,cy,rx,ry. Every block that a " +
			"region touches is filled, so include the outline. In the view image, the scale is in original " +
			"pixels, the pale fill is what was pixelated, A1... are automatic regions, M1... rectangles " +
			"added by hand, and P1... polygons added by hand.",
		exitStatus, requirements,
	},
}

const classesNote = "Classes: the names of the --model output, read from --classes, then labels.json (a JSON array of " +
	"strings) beside the model, then the ONNX metadata names, then the default. The default model " +
	"classes are nipple_f (a female nipple; the class name of deepghs/anime_censor_detection), penis, " +
	"and pussy. --labels chooses which classes to pixelate; the default is penis,pussy, because " +
	"nipples are not pixelated by default under the Japanese distribution standard. A class count " +
	"that does not match the model output is an error. --dry-run and inspect print the classes " +
	"loaded and where they came from."

var fetchPage = clihelp.Page{
	Name:    "gozen art fetch",
	Summary: "download the pinned detection models into the cache",
	Usage:   []string{"gozen art fetch [--list] [NAME ...]"},
	About: "Downloads fixed revisions of models from Hugging Face into the user cache directory " +
		"(gozen/mosaic/<name>/ under os.UserCacheDir), checks each file against a SHA-256 written in gozen, " +
		"and deletes the download with an error on a mismatch. Without NAME every model is fetched. " +
		"This command and the confirmed download prompt of gozen art mosaic are the only places gozen uses the network. " +
		"A model in the cache is used when --model is omitted: censor by gozen art mosaic, face by gozen art faces.",
	Examples: []clihelp.Example{
		{"list the models and whether each is fetched", "gozen art fetch --list"},
		{"fetch the default detector", "gozen art fetch censor"},
		{"pixelate faces with the fetched face model (Linux cache path)", "gozen art mosaic auto --model ~/.cache/gozen/mosaic/face/model.onnx game"},
	},
	Groups: []clihelp.Group{{Title: "Options", Flags: []string{"list"}}, generalGroup},
	Notes: []string{
		"Models. censor (deepghs/anime_censor_detection, MIT) detects nipple_f, penis, and pussy for the " +
			"main use. face (deepghs/anime_face_detection, MIT) and head (deepghs/anime_head_detection, MIT) " +
			"find faces and heads, to anonymize people. Class names come from labels.json in the cache.",
		exitStatus, fetchRequirements,
	},
}
